"""
Import Japanese-English sentence pairs from Tatoeba into the dictionary SQLite.

Downloads:
  sentences.csv  — all Tatoeba sentences (id, lang, text)
  links.csv      — translation pairs (id1, id2)

Selects Japanese sentences that have at least one English translation,
then links them to JMdict words via the NLP tokenizer.

Usage:
    python scripts/import_tatoeba.py [--db data/dictionary.db] [--limit 50000]
"""

from __future__ import annotations

import argparse
import csv
import io
import sqlite3
import sys
import time
import urllib.request
from pathlib import Path

# We only import a manageable subset for Phase 0
DEFAULT_LIMIT = 50_000

SCHEMA_ADDITIONS = """
CREATE TABLE IF NOT EXISTS tatoeba_sentences (
    id              INTEGER PRIMARY KEY,
    lang            TEXT NOT NULL,
    text            TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS tat_lang ON tatoeba_sentences(lang);

CREATE TABLE IF NOT EXISTS tatoeba_translations (
    sentence_id     INTEGER NOT NULL REFERENCES tatoeba_sentences(id),
    translation_id  INTEGER NOT NULL REFERENCES tatoeba_sentences(id),
    PRIMARY KEY (sentence_id, translation_id)
);
"""

SENTENCE_PAIRS_VIEW = """
CREATE VIEW IF NOT EXISTS ja_en_pairs AS
SELECT
    j.id    AS ja_id,
    j.text  AS ja_text,
    e.text  AS en_text
FROM tatoeba_sentences j
JOIN tatoeba_translations t ON t.sentence_id = j.id
JOIN tatoeba_sentences e ON e.id = t.translation_id
WHERE j.lang = 'jpn' AND e.lang = 'eng';
"""


def download_tsv(url: str, label: str) -> list[list[str]]:
    print(f"Downloading {label} ...", flush=True)
    with urllib.request.urlopen(url, timeout=60) as resp:
        raw = resp.read()
    # Tatoeba files are UTF-8 TSV, may be gzipped
    if raw[:2] == b"\x1f\x8b":
        import gzip
        raw = gzip.decompress(raw)
    text = raw.decode("utf-8", errors="replace")
    reader = csv.reader(io.StringIO(text), delimiter="\t")
    return list(reader)


def import_tatoeba(db_path: Path, limit: int) -> None:
    conn = sqlite3.connect(str(db_path))
    conn.executescript(SCHEMA_ADDITIONS)
    conn.executescript(SENTENCE_PAIRS_VIEW)

    # Download sentences
    rows = download_tsv(
        "https://downloads.tatoeba.org/exports/sentences.csv",
        "sentences.csv",
    )
    print(f"Total sentences: {len(rows)}", flush=True)

    ja_rows = [(int(r[0]), r[1], r[2]) for r in rows if len(r) >= 3 and r[1] == "jpn"]
    en_rows = [(int(r[0]), r[1], r[2]) for r in rows if len(r) >= 3 and r[1] == "eng"]

    print(f"Japanese: {len(ja_rows)}, English: {len(en_rows)}", flush=True)

    # Limit to first N Japanese sentences
    ja_rows = ja_rows[:limit]
    ja_ids = {r[0] for r in ja_rows}

    conn.executemany(
        "INSERT OR IGNORE INTO tatoeba_sentences (id, lang, text) VALUES (?, ?, ?)",
        ja_rows,
    )

    # Download links
    link_rows = download_tsv(
        "https://downloads.tatoeba.org/exports/links.csv",
        "links.csv",
    )
    print(f"Total links: {len(link_rows)}", flush=True)

    # Find English translations of our Japanese sentences
    relevant_en_ids: set[int] = set()
    relevant_links: list[tuple[int, int]] = []
    for row in link_rows:
        if len(row) < 2:
            continue
        try:
            id1, id2 = int(row[0]), int(row[1])
        except ValueError:
            continue
        if id1 in ja_ids:
            relevant_en_ids.add(id2)
            relevant_links.append((id1, id2))

    # Import only the English sentences we need
    en_needed = [(r[0], r[1], r[2]) for r in en_rows if r[0] in relevant_en_ids]
    conn.executemany(
        "INSERT OR IGNORE INTO tatoeba_sentences (id, lang, text) VALUES (?, ?, ?)",
        en_needed,
    )
    conn.executemany(
        "INSERT OR IGNORE INTO tatoeba_translations (sentence_id, translation_id) VALUES (?, ?)",
        relevant_links,
    )

    conn.commit()

    pair_count = conn.execute(
        "SELECT COUNT(*) FROM tatoeba_translations t "
        "JOIN tatoeba_sentences j ON j.id = t.sentence_id AND j.lang = 'jpn'"
    ).fetchone()[0]
    print(f"Imported {len(ja_rows)} Japanese sentences, {len(en_needed)} English translations, {pair_count} pairs", flush=True)
    conn.close()


def main() -> None:
    parser = argparse.ArgumentParser(description="Import Tatoeba sentences into SQLite")
    parser.add_argument("--db",    default="data/dictionary.db")
    parser.add_argument("--limit", type=int, default=DEFAULT_LIMIT,
                        help="Max Japanese sentences to import (default 50000)")
    args = parser.parse_args()

    db_path = Path(args.db)
    if not db_path.exists():
        print(f"ERROR: {db_path} not found. Run import_jmdict.py first.", file=sys.stderr)
        sys.exit(1)

    t0 = time.time()
    import_tatoeba(db_path, args.limit)
    print(f"Done in {time.time() - t0:.1f}s", flush=True)


if __name__ == "__main__":
    main()
