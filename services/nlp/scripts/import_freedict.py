#!/usr/bin/env python3
"""
Import FreeDict bilingual dictionaries (xxx → English) into the SQLite dictionary.

Covers (per the task spec): spa-eng (es), deu-eng (de), fra-eng (fr),
ita-eng (it), por-eng (pt). Writes into the same SQLite schema the NLP service
reads (words + word_definitions), with the source language_code and
target_language='en'.

FreeDict distributes the dictd format as a .tar.xz containing:
    <pair>/<pair>.index    headword <TAB> base64offset <TAB> base64length
    <pair>/<pair>.dict.dz  dictzip (gzip-compatible) blob of definition bodies

A definition body looks like:
    Akut-Zeichen /ˈɑkuːt tsˈaɪçən/ (´) <neut, n, sg>
     [print] acute accent <n>, acute <n>´
       Synonym: {Akut}

i.e. the SOURCE headword + IPA + grammar tags on the first line, then one or
more indented lines of English translations, then Synonym/see/Note blocks.

We extract clean English glosses from the translation line(s). FreeDict is
released under GPL/AGPL/CC-BY-SA depending on the pair (see each COPYING).

The download URLs are version-pinned (FreeDict has no stable "latest" path), so
they are listed in FREEDICT_RELEASES below and can be refreshed from
https://freedict.org/freedict-database.json.

Usage:
  python scripts/import_freedict.py                      # download all 5 pairs
  python scripts/import_freedict.py --langs es,fr        # subset by source lang
  python scripts/import_freedict.py --path deu-eng.tar.xz --lang de
  python scripts/import_freedict.py --db data/dictionary.db

The parser functions (decode_b64_int, parse_index, extract_glosses,
parse_dictd) are pure and unit-tested.
"""

from __future__ import annotations

import argparse
import gzip
import io
import re
import sqlite3
import sys
import tarfile
import time
import urllib.request
from pathlib import Path

# Version-pinned FreeDict dictd releases. lang -> (pair, url).
# Refresh from https://freedict.org/freedict-database.json if a 404 appears.
FREEDICT_RELEASES: dict[str, tuple[str, str]] = {
    "es": ("spa-eng", "https://download.freedict.org/dictionaries/spa-eng/0.3.1/freedict-spa-eng-0.3.1.dictd.tar.xz"),
    "de": ("deu-eng", "https://download.freedict.org/dictionaries/deu-eng/1.9-fd1/freedict-deu-eng-1.9-fd1.dictd.tar.xz"),
    "fr": ("fra-eng", "https://download.freedict.org/dictionaries/fra-eng/0.4.1/freedict-fra-eng-0.4.1.dictd.tar.xz"),
    "it": ("ita-eng", "https://download.freedict.org/dictionaries/ita-eng/2025.11.23/freedict-ita-eng-2025.11.23.dictd.tar.xz"),
    "pt": ("por-eng", "https://download.freedict.org/dictionaries/por-eng/0.2/freedict-por-eng-0.2.dictd.tar.xz"),
}

SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS words (
    id              TEXT PRIMARY KEY,
    language_code   TEXT NOT NULL DEFAULT 'ja',
    lemma           TEXT NOT NULL,
    reading         TEXT,
    frequency_rank  INTEGER,
    jlpt_level      TEXT,
    pos_primary     TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS words_lemma_reading
    ON words(lemma, reading, language_code);
CREATE INDEX IF NOT EXISTS words_lemma ON words(lemma, language_code);
CREATE INDEX IF NOT EXISTS words_frequency ON words(language_code, frequency_rank);

CREATE TABLE IF NOT EXISTS word_definitions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    word_id         TEXT NOT NULL REFERENCES words(id),
    target_language TEXT NOT NULL DEFAULT 'en',
    sense_index     INTEGER NOT NULL DEFAULT 0,
    definition      TEXT NOT NULL,
    part_of_speech  TEXT,
    tags            TEXT,
    source          TEXT NOT NULL DEFAULT 'jmdict',
    confidence      REAL NOT NULL DEFAULT 1.0
);

CREATE INDEX IF NOT EXISTS wdef_word_id ON word_definitions(word_id, target_language);
"""

# dictd base64 alphabet (no padding); each char is a base-64 digit.
_B64_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
_B64_INDEX = {c: i for i, c in enumerate(_B64_ALPHABET)}

# Lines that are metadata blocks rather than translations.
_BLOCK_RE = re.compile(r"^\s*(Synonyms?|see|Note|Antonym|Hint|cf\.?)\b", re.IGNORECASE)
# IPA between slashes at the start of the first body line.
_IPA_RE = re.compile(r"/[^/]*/")
# Grammar / register annotations like <n>, <fem, n, sg>, [print], (´).
_ANGLE_RE = re.compile(r"<[^>]*>")
_BRACKET_RE = re.compile(r"\[[^\]]*\]")
_PAREN_RE = re.compile(r"\([^)]*\)")
_BRACE_RE = re.compile(r"\{[^}]*\}")


def decode_b64_int(s: str) -> int:
    """Decode a dictd base64 integer (big-endian, no padding)."""
    n = 0
    for c in s:
        n = n * 64 + _B64_INDEX[c]
    return n


def parse_index(index_bytes: bytes) -> list[tuple[str, int, int]]:
    """
    Parse a dictd .index file into [(headword, offset, length), ...].
    Skips the 00-database-* metadata records.
    """
    out: list[tuple[str, int, int]] = []
    for line in index_bytes.decode("utf-8", errors="replace").splitlines():
        parts = line.split("\t")
        if len(parts) != 3:
            continue
        hw, off, length = parts
        if hw.startswith("00-database") or not hw.strip():
            continue
        try:
            out.append((hw, decode_b64_int(off), decode_b64_int(length)))
        except KeyError:
            continue
    return out


def _clean_gloss(text: str) -> str:
    """Strip grammar/register/synonym annotations and tidy whitespace."""
    text = _ANGLE_RE.sub("", text)
    text = _BRACKET_RE.sub("", text)
    text = _PAREN_RE.sub("", text)
    text = _BRACE_RE.sub("", text)
    text = text.replace("´", "").strip(" ,;·")
    return re.sub(r"\s+", " ", text).strip()


def extract_glosses(body: str) -> list[str]:
    """
    Extract English glosses from a FreeDict (Ding) dictd entry body.

    Body shape:
        <source headword> /IPA/ <grammar>
        english gloss <n>, another gloss <n>
            "german example phrase"  - english example
            ...
          Synonym: {...}

    Only the translation line(s) directly under the headword are real glosses.
    Lines beginning with a double-quote are bilingual EXAMPLE phrases and must
    be skipped; Synonym/see/Note blocks end the entry. Returns a de-duplicated
    list of clean gloss strings (may be empty — we never fabricate).
    """
    lines = body.splitlines()
    if not lines:
        return []

    glosses: list[str] = []
    seen: set[str] = set()
    # Skip the headword line (index 0).
    for line in lines[1:]:
        stripped = line.strip()
        if not stripped:
            continue
        if _BLOCK_RE.match(line):
            break
        # Example phrases (German quote + "- english") start with a quote.
        if stripped.startswith(('"', "“", "„", "'")):
            continue
        # Remove a leading IPA if it leaked onto a translation line.
        cleaned_line = _IPA_RE.sub("", line)
        for raw_sense in cleaned_line.split(","):
            sense = _clean_gloss(raw_sense)
            if sense and sense.lower() not in seen and len(sense) <= 120:
                seen.add(sense.lower())
                glosses.append(sense)
    return glosses


def parse_dictd(index_bytes: bytes, dict_bytes: bytes) -> list[dict]:
    """
    Parse a FreeDict dictd dictionary (decompressed .dict + .index) into
    entry dicts: {headword, glosses: [...]}.  Entries with no usable English
    gloss are skipped (we never fabricate definitions).
    """
    entries: list[dict] = []
    for hw, off, length in parse_index(index_bytes):
        body = dict_bytes[off:off + length].decode("utf-8", errors="replace")
        glosses = extract_glosses(body)
        if not glosses:
            continue
        entries.append({"headword": hw, "glosses": glosses})
    return entries


def _read_tar_member(tar: tarfile.TarFile, suffix: str) -> bytes | None:
    for m in tar.getmembers():
        if m.name.endswith(suffix):
            f = tar.extractfile(m)
            if f:
                return f.read()
    return None


def load_dictd_from_tar(tar_bytes: bytes) -> tuple[bytes, bytes]:
    """Extract (index_bytes, decompressed_dict_bytes) from a .tar.xz blob."""
    with tarfile.open(fileobj=io.BytesIO(tar_bytes), mode="r:xz") as tar:
        index_bytes = _read_tar_member(tar, ".index")
        dict_dz = _read_tar_member(tar, ".dict.dz")
        if index_bytes is None or dict_dz is None:
            raise ValueError("tarball missing .index or .dict.dz")
    dict_bytes = gzip.decompress(dict_dz)
    return index_bytes, dict_bytes


def import_to_sqlite(entries: list[dict], lang: str, db_path: Path) -> int:
    """Insert FreeDict entries into SQLite under language_code=lang, target 'en'."""
    print(f"Importing {len(entries)} {lang} entries to {db_path} ...", flush=True)
    conn = sqlite3.connect(str(db_path))
    conn.executescript(SCHEMA_SQL)

    # Group by lowercased headword so inflected duplicates collapse.
    grouped: dict[str, list[str]] = {}
    order: list[str] = []
    for e in entries:
        key = e["headword"].lower().strip()
        if not key:
            continue
        if key not in grouped:
            grouped[key] = []
            order.append(key)
        for g in e["glosses"]:
            if g not in grouped[key]:
                grouped[key].append(g)

    words_batch: list[tuple] = []
    defs_batch: list[tuple] = []
    for i, key in enumerate(order):
        row_id = f"freedict-{lang}-{i}"
        words_batch.append((row_id, lang, key, None, None, None, None))
        for si, defn in enumerate(grouped[key][:8]):
            defs_batch.append((row_id, "en", si, defn, None, None, "freedict"))

    conn.executemany(
        """
        INSERT OR IGNORE INTO words
            (id, language_code, lemma, reading, frequency_rank, jlpt_level, pos_primary)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        """,
        words_batch,
    )
    conn.executemany(
        """
        INSERT INTO word_definitions
            (word_id, target_language, sense_index, definition, part_of_speech, tags, source)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        """,
        defs_batch,
    )
    conn.commit()
    conn.close()
    print(f"Inserted {len(words_batch)} {lang} word rows, {len(defs_batch)} definitions", flush=True)
    return len(words_batch)


def download(url: str) -> bytes:
    print(f"Downloading {url} ...", flush=True)
    with urllib.request.urlopen(url, timeout=300) as r:
        return r.read()


def import_one(lang: str, db_path: Path, tar_bytes: bytes) -> int:
    index_bytes, dict_bytes = load_dictd_from_tar(tar_bytes)
    entries = parse_dictd(index_bytes, dict_bytes)
    print(f"[{lang}] parsed {len(entries)} entries with usable glosses", flush=True)
    return import_to_sqlite(entries, lang, db_path)


def main() -> None:
    parser = argparse.ArgumentParser(description="Import FreeDict dictionaries into SQLite")
    parser.add_argument("--db", default="data/dictionary.db", help="Output SQLite path")
    parser.add_argument("--langs", default="es,de,fr,it,pt",
                        help="Comma-separated source langs to import (download mode)")
    parser.add_argument("--path", help="Local .tar.xz to import (requires --lang)")
    parser.add_argument("--lang", help="Source lang for --path mode")
    args = parser.parse_args()

    db_path = Path(args.db)
    db_path.parent.mkdir(parents=True, exist_ok=True)

    t0 = time.time()
    total = 0

    if args.path:
        if not args.lang:
            print("ERROR: --path requires --lang", file=sys.stderr)
            sys.exit(1)
        tar_bytes = Path(args.path).read_bytes()
        total += import_one(args.lang, db_path, tar_bytes)
    else:
        for lang in [l.strip() for l in args.langs.split(",") if l.strip()]:
            if lang not in FREEDICT_RELEASES:
                print(f"WARN: no FreeDict release configured for '{lang}', skipping", file=sys.stderr)
                continue
            _pair, url = FREEDICT_RELEASES[lang]
            try:
                tar_bytes = download(url)
                total += import_one(lang, db_path, tar_bytes)
            except Exception as e:  # best-effort: one pair failing must not abort the rest
                print(f"WARN: failed to import {lang} ({url}): {e}", file=sys.stderr)

    print(f"Done. {total} total word rows in {time.time() - t0:.1f}s", flush=True)


if __name__ == "__main__":
    main()
