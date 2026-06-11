#!/usr/bin/env python3
"""
Import CC-CEDICT (Mandarin Chinese — English) into the SQLite dictionary.

This is the SQLite counterpart of import_cedict.py (which targets PostgreSQL and
is NOT wired into CI/Docker). It writes into the same SQLite schema the NLP
service reads (words + word_definitions), using language_code 'zh-cn'.

CC-CEDICT is released under CC BY-SA 4.0:
  https://cc-cedict.org/wiki/format:syntax

Entry format (one per line):
  Traditional Simplified [pin1 yin1] /def1/def2/.../

Usage:
  # download-on-demand (default) into data/dictionary.db:
  python scripts/import_cedict_sqlite.py
  # from a local file:
  python scripts/import_cedict_sqlite.py --path cedict_ts.u8 --db data/dictionary.db

The parser functions (parse_cedict_line, parse_cedict) are pure and unit-tested.
"""

from __future__ import annotations

import argparse
import gzip
import re
import sqlite3
import sys
import time
import urllib.request
from collections import defaultdict
from pathlib import Path

CEDICT_URL = "https://www.mdbg.net/chinese/export/cedict/cedict_1_0_ts_utf-8_mdbg.txt.gz"

# Schema kept in sync with import_jmdict.py. CREATE IF NOT EXISTS makes this
# safe to run after JMdict has already created the tables.
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

_ENTRY_RE = re.compile(r"^(\S+)\s+(\S+)\s+\[([^\]]+)\]\s+/(.+)/$")

# Tone-number → tone-diacritic conversion for the canonical "5-tone-on-vowel" rule.
_TONE_MARKS = {
    "a": "āáǎàa", "e": "ēéěèe", "i": "īíǐìi",
    "o": "ōóǒòo", "u": "ūúǔùu", "v": "ǖǘǚǜü",
}


def _pinyin_syllable_to_diacritic(syl: str) -> str:
    """
    Convert a single tone-numbered pinyin syllable (e.g. 'hao3') to its
    diacritic form ('hǎo'). 'u:' / 'v' map to ü. Tone 5 / no tone => plain.
    """
    m = re.match(r"^([a-zA-ZüÜ:]+?)([1-5])?$", syl)
    if not m:
        return syl
    base, tone = m.group(1), m.group(2)
    base = base.replace("u:", "v").replace("U:", "v")
    if not tone or tone == "5":
        return base.replace("v", "ü")
    t = int(tone) - 1
    # Tone-placement rule: a/e take it; else o in 'ou'; else last vowel.
    lower = base.lower()
    target = None
    if "a" in lower:
        target = "a"
    elif "e" in lower:
        target = "e"
    elif "ou" in lower:
        target = "o"
    else:
        for ch in reversed(lower):
            if ch in _TONE_MARKS:
                target = ch
                break
    if target is None:
        return base.replace("v", "ü")
    # Replace the (first matching) target vowel, preserving original case.
    idx = lower.index(target)
    accented = _TONE_MARKS[target][t]
    return (base[:idx] + accented + base[idx + 1:]).replace("v", "ü")


def pinyin_to_diacritics(pinyin_raw: str) -> str:
    """Convert space-separated tone-numbered pinyin to diacritic form."""
    return " ".join(_pinyin_syllable_to_diacritic(s) for s in pinyin_raw.split())


def parse_cedict_line(line: str) -> dict | None:
    """
    Parse a single CC-CEDICT line into an entry dict, or None if it is a
    comment / blank / malformed / pure-variant entry.

    Returns:
      {traditional, simplified, pinyin, pinyin_diacritic, definitions: [...]}
    """
    line = line.strip()
    if not line or line.startswith("#"):
        return None
    m = _ENTRY_RE.match(line)
    if not m:
        return None
    traditional, simplified, pinyin_raw, defs_raw = m.groups()
    definitions = [d.strip() for d in defs_raw.split("/") if d.strip()]
    if not definitions:
        return None
    # Drop entries that are only cross-references (no real gloss).
    if all(d.startswith(("variant of", "old variant", "see ")) for d in definitions):
        return None
    return {
        "traditional": traditional,
        "simplified": simplified,
        "pinyin": pinyin_raw,
        "pinyin_diacritic": pinyin_to_diacritics(pinyin_raw),
        "definitions": definitions,
    }


def parse_cedict(text: str) -> list[dict]:
    """Parse a full CC-CEDICT text dump into a list of entry dicts."""
    entries: list[dict] = []
    for line in text.splitlines():
        entry = parse_cedict_line(line)
        if entry:
            entries.append(entry)
    return entries


def download_cedict() -> str:
    print(f"Downloading CC-CEDICT from {CEDICT_URL} ...", flush=True)
    with urllib.request.urlopen(CEDICT_URL, timeout=120) as r:
        compressed = r.read()
    return gzip.decompress(compressed).decode("utf-8")


def import_to_sqlite(entries: list[dict], db_path: Path) -> int:
    """
    Insert CEDICT entries into SQLite under language_code 'zh-cn'.
    Groups multiple CEDICT lines that share a simplified headword into one
    words row (senses concatenated). Returns the number of words inserted.
    """
    print(f"Importing {len(entries)} CEDICT entries to {db_path} ...", flush=True)
    conn = sqlite3.connect(str(db_path))
    conn.executescript(SCHEMA_SQL)

    # Group by simplified headword so 你好 etc. collapse into one card.
    grouped: dict[str, dict] = {}
    order: list[str] = []
    for e in entries:
        key = e["simplified"]
        if key not in grouped:
            grouped[key] = {
                "simplified": key,
                "traditional": e["traditional"],
                "pinyin": e["pinyin_diacritic"],
                "definitions": [],
            }
            order.append(key)
        grouped[key]["definitions"].extend(e["definitions"])

    words_batch: list[tuple] = []
    defs_batch: list[tuple] = []
    for i, key in enumerate(order):
        g = grouped[key]
        row_id = f"cedict-{i}"
        words_batch.append((
            row_id,
            "zh-cn",
            g["simplified"],
            g["pinyin"],        # reading = pinyin with diacritics
            None,               # frequency_rank (no free SUBTLEX list bundled)
            None,               # jlpt_level (n/a)
            "n",                # pos_primary (CEDICT has no POS)
        ))
        for si, defn in enumerate(g["definitions"][:8]):
            defs_batch.append((row_id, "en", si, defn, "n", None, "cedict"))

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
    print(f"Inserted {len(words_batch)} zh-cn word rows, {len(defs_batch)} definitions", flush=True)
    return len(words_batch)


def main() -> None:
    parser = argparse.ArgumentParser(description="Import CC-CEDICT into SQLite")
    parser.add_argument("--path", help="Path to cedict .txt or .txt.gz (downloads if omitted)")
    parser.add_argument("--db", default="data/dictionary.db", help="Output SQLite path")
    args = parser.parse_args()

    if args.path:
        raw = Path(args.path).read_bytes()
        text = gzip.decompress(raw).decode("utf-8") if args.path.endswith(".gz") else raw.decode("utf-8")
    else:
        text = download_cedict()

    db_path = Path(args.db)
    db_path.parent.mkdir(parents=True, exist_ok=True)

    t0 = time.time()
    entries = parse_cedict(text)
    if not entries:
        print("ERROR: no CEDICT entries parsed", file=sys.stderr)
        sys.exit(1)
    import_to_sqlite(entries, db_path)
    print(f"Done in {time.time() - t0:.1f}s", flush=True)


if __name__ == "__main__":
    main()
