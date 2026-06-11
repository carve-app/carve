"""
Import a monolingual English dictionary (Princeton WordNet 3.1) into the same
SQLite dictionary the NLP service reads (DICT_DB_PATH), so clicking an English
word in the extension shows an English definition — the experience an
intermediate+ English learner wants (English → English, not bilingual).

WordNet is a free, redistributable lexical database (Princeton license). We
parse the `data.{noun,verb,adj,adv}` files: each line lists the words in a
synset plus a human-readable gloss after the ` | ` delimiter. We store one
`words` row per lemma (English), and its synset glosses as `word_definitions`
rows (target_language='en', source='wordnet'). Frequency rank comes from
`wordfreq` (Zipf scale → rank) so the popup can show a frequency band, matching
how the JA path ranks.

Usage:
    python scripts/import_wordnet.py [--db data/dictionary.db] [--wndir <dir>]

If --wndir is omitted, the WordNet archive is downloaded and extracted to a temp
dir (mirrors import_jmdict/import_tatoeba's download-on-demand behaviour).
"""
from __future__ import annotations

import argparse
import os
import sqlite3
import sys
import tarfile
import tempfile
import urllib.request
from pathlib import Path

# Princeton WordNet 3.1 database files (the "WNdb" tarball).
WORDNET_URL = "https://wordnetcode.princeton.edu/wn3.1.dict.tar.gz"

DATA_FILES = ("data.noun", "data.verb", "data.adj", "data.adv")

POS_NAME = {"n": "noun", "v": "verb", "a": "adj", "s": "adj", "r": "adv"}

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
    source          TEXT NOT NULL DEFAULT 'wordnet',
    confidence      REAL NOT NULL DEFAULT 1.0
);
CREATE INDEX IF NOT EXISTS wdef_word_id ON word_definitions(word_id, target_language);
"""


def parse_wordnet_line(line: str) -> tuple[list[str], str, str] | None:
    """
    Parse one WordNet `data.*` line into (lemmas, pos, gloss).

    Line format (space-separated, then ` | ` gloss):
      synset_offset lex_filenum ss_type w_cnt word lex_id [word lex_id ...]
      p_cnt [ptr ...] [frames ...] | gloss

    Returns None for the file's licence-header lines (which start with two
    spaces) or anything malformed. WordNet stores multi-word lemmas with
    underscores; we convert them to spaces.
    """
    if not line or line.startswith(" "):
        return None
    head, _, gloss = line.partition(" | ")
    if not gloss:
        return None
    parts = head.split()
    if len(parts) < 4:
        return None
    ss_type = parts[2]
    pos = POS_NAME.get(ss_type)
    if pos is None:
        return None
    try:
        w_cnt = int(parts[3], 16)  # word count is hex in WordNet
    except ValueError:
        return None
    lemmas: list[str] = []
    # words start at index 4, as (word, lex_id) pairs
    i = 4
    for _ in range(w_cnt):
        if i >= len(parts):
            break
        word = parts[i].replace("_", " ")
        # Strip the optional "(adjective-marker)" suffix WordNet appends, e.g. "good(a)".
        paren = word.find("(")
        if paren > 0:
            word = word[:paren]
        lemmas.append(word.lower())
        i += 2
    # gloss may contain "; example" usage clauses and quoted examples; keep the
    # definitional part before the first quoted example for a clean definition.
    definition = gloss.strip()
    quote = definition.find('"')
    if quote > 0:
        definition = definition[:quote].rstrip(" ;")
    if not lemmas or not definition:
        return None
    return lemmas, pos, definition


def _freq_rank(lemma: str) -> int | None:
    try:
        from wordfreq import zipf_frequency
    except ImportError:
        return None
    z = zipf_frequency(lemma, "en")
    if z <= 0:
        return None
    # Zipf 7 (most frequent) → rank ~1; Zipf 1 (rare) → rank ~60000.
    return max(1, int((7.0 - z) * 10000) + 1)


def _download_wordnet(dest: Path) -> Path:
    print("Downloading WordNet 3.1 ...", flush=True)
    archive = dest / "wn31.tar.gz"
    with urllib.request.urlopen(WORDNET_URL, timeout=120) as resp:
        archive.write_bytes(resp.read())
    with tarfile.open(archive, "r:gz") as tf:
        tf.extractall(dest)  # noqa: S202 - trusted Princeton archive
    # The archive extracts to dict/ (sometimes nested). Find the dir holding data.noun.
    for root, _dirs, files in os.walk(dest):
        if "data.noun" in files:
            return Path(root)
    raise FileNotFoundError("data.noun not found in extracted WordNet archive")


def import_wordnet(db_path: Path, wndir: Path) -> None:
    conn = sqlite3.connect(str(db_path))
    conn.executescript(SCHEMA_SQL)

    # lemma -> list of (pos, definition); dedup definitions, cap per lemma.
    senses: dict[str, list[tuple[str, str]]] = {}
    for fname in DATA_FILES:
        fpath = wndir / fname
        if not fpath.exists():
            print(f"  skip missing {fname}", file=sys.stderr)
            continue
        with open(fpath, encoding="utf-8", errors="replace") as fh:
            for line in fh:
                parsed = parse_wordnet_line(line.rstrip("\n"))
                if not parsed:
                    continue
                lemmas, pos, definition = parsed
                for lemma in lemmas:
                    bucket = senses.setdefault(lemma, [])
                    if len(bucket) < 8 and (pos, definition) not in bucket:
                        bucket.append((pos, definition))

    print(f"Parsed {len(senses)} English lemmas", flush=True)

    inserted_words = 0
    inserted_defs = 0
    for lemma, defs in senses.items():
        word_id = f"en:{lemma}"
        pos_primary = defs[0][0] if defs else None
        conn.execute(
            """
            INSERT OR IGNORE INTO words (id, language_code, lemma, reading, frequency_rank, pos_primary)
            VALUES (?, 'en', ?, ?, ?, ?)
            """,
            (word_id, lemma, lemma, _freq_rank(lemma), pos_primary),
        )
        inserted_words += 1
        for idx, (pos, definition) in enumerate(defs):
            conn.execute(
                """
                INSERT INTO word_definitions
                    (word_id, target_language, sense_index, definition, part_of_speech, source, confidence)
                VALUES (?, 'en', ?, ?, ?, 'wordnet', 1.0)
                """,
                (word_id, idx, definition, pos),
            )
            inserted_defs += 1

    conn.commit()
    conn.close()
    print(f"Imported {inserted_words} words, {inserted_defs} definitions (en, source=wordnet)", flush=True)


def main() -> None:
    parser = argparse.ArgumentParser(description="Import WordNet English dictionary into SQLite")
    parser.add_argument("--db", default="data/dictionary.db")
    parser.add_argument("--wndir", default=None, help="Dir containing WordNet data.* files (else download)")
    args = parser.parse_args()

    db_path = Path(args.db)
    if not db_path.exists():
        print(f"ERROR: {db_path} not found. Run import_jmdict.py first.", file=sys.stderr)
        sys.exit(1)

    if args.wndir:
        wndir = Path(args.wndir)
    else:
        tmp = Path(tempfile.mkdtemp(prefix="wordnet-"))
        wndir = _download_wordnet(tmp)

    import_wordnet(db_path, wndir)


if __name__ == "__main__":
    main()
