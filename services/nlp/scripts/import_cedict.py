#!/usr/bin/env python3
"""
Import CC-CEDICT into PostgreSQL words + word_definitions tables.

CC-CEDICT is released under CC BY-SA 4.0:
  https://cc-cedict.org/wiki/format:syntax

Entry format (one per line):
  Traditional Simplified [pin1 yin1] /def1/def2/.../

Usage:
  python import_cedict.py [--db-url postgresql://...] [--path cedict_ts.u8]

If --path is omitted, downloads the latest CC-CEDICT from the official mirror.
"""

from __future__ import annotations

import argparse
import gzip
import io
import os
import re
import sys
import urllib.request
from typing import Generator

CEDICT_URL = "https://www.mdbg.net/chinese/export/cedict/cedict_1_0_ts_utf-8_mdbg.txt.gz"

# Frequency list: top 10,000 simplified characters/words from SUBTLEX-CH.
# Embedded as a compact sorted list (rank → simplified word).
# Full dataset loaded from a separate frequency file if available.
FREQ_FILE = os.path.join(os.path.dirname(__file__), "data", "zh_frequency.txt")


def download_cedict() -> str:
    """Download and decompress CC-CEDICT. Returns text content."""
    print(f"Downloading CC-CEDICT from {CEDICT_URL}...", file=sys.stderr)
    with urllib.request.urlopen(CEDICT_URL, timeout=60) as r:
        compressed = r.read()
    data = gzip.decompress(compressed)
    return data.decode("utf-8")


def load_cedict(text: str) -> Generator[dict, None, None]:
    """Parse CC-CEDICT and yield entry dicts."""
    entry_re = re.compile(
        r'^(\S+)\s+(\S+)\s+\[([^\]]+)\]\s+/(.+)/$'
    )
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        m = entry_re.match(line)
        if not m:
            continue
        traditional, simplified, pinyin_raw, defs_raw = m.groups()
        definitions = [d.strip() for d in defs_raw.split("/") if d.strip()]
        # Skip entries that are purely proper nouns or errata markers
        if all(d.startswith("variant of") or d.startswith("old variant") for d in definitions):
            continue
        yield {
            "traditional": traditional,
            "simplified": simplified,
            "pinyin": pinyin_raw,        # e.g. "ni3 hao3"
            "definitions": definitions,
        }


def load_frequency() -> dict[str, int]:
    """Load SUBTLEX-CH frequency list if available. Returns {word: rank}."""
    freq: dict[str, int] = {}
    if os.path.exists(FREQ_FILE):
        with open(FREQ_FILE, encoding="utf-8") as f:
            for i, line in enumerate(f, 1):
                word = line.strip()
                if word:
                    freq[word] = i
    return freq


def import_cedict(db_url: str, cedict_text: str) -> None:
    try:
        import psycopg2  # type: ignore
        import psycopg2.extras  # type: ignore
    except ImportError:
        print("psycopg2 not installed — pip install psycopg2-binary", file=sys.stderr)
        sys.exit(1)

    freq_map = load_frequency()
    conn = psycopg2.connect(db_url)
    cur = conn.cursor()

    cur.execute("SET search_path TO public")

    batch_words: list[tuple] = []
    batch_defs: list[tuple] = []
    word_id_map: dict[str, str] = {}  # simplified → word_id (last inserted)

    print("Parsing CEDICT entries...", file=sys.stderr)
    entries = list(load_cedict(cedict_text))
    print(f"Loaded {len(entries)} entries.", file=sys.stderr)

    for entry in entries:
        simplified = entry["simplified"]
        traditional = entry["traditional"]
        pinyin = entry["pinyin"]
        freq_rank = freq_map.get(simplified)

        # We store one row per (language_code, simplified, pinyin) pair.
        # zh-cn uses simplified; zh-tw uses traditional.
        # We insert for zh-cn using simplified as the lemma.
        batch_words.append((
            "zh-cn",
            simplified,
            traditional,   # stored in reading field for zh-cn
            pinyin,
            freq_rank,
        ))

    print(f"Inserting {len(batch_words)} words...", file=sys.stderr)
    psycopg2.extras.execute_values(
        cur,
        """
        INSERT INTO words (language_code, lemma, reading, pos, frequency_rank)
        VALUES %s
        ON CONFLICT (language_code, lemma, reading) DO UPDATE
            SET frequency_rank = EXCLUDED.frequency_rank
        RETURNING id, lemma, reading
        """,
        [(lang, lemma, reading, "n", freq) for lang, lemma, reading, _pinyin, freq in batch_words],
        page_size=2000,
    )
    for row in cur.fetchall():
        word_id_map[row[1]] = row[0]  # simplified → id

    print("Inserting definitions...", file=sys.stderr)
    for entry in entries:
        word_id = word_id_map.get(entry["simplified"])
        if not word_id:
            continue
        for i, definition in enumerate(entry["definitions"][:8]):  # cap at 8 senses
            batch_defs.append((word_id, "en", i, definition, "cedict"))

    psycopg2.extras.execute_values(
        cur,
        """
        INSERT INTO word_definitions (word_id, target_language, sense_index, definition, source)
        VALUES %s
        ON CONFLICT DO NOTHING
        """,
        batch_defs,
        page_size=2000,
    )

    conn.commit()
    cur.close()
    conn.close()
    print(f"Done. Imported {len(entries)} CEDICT entries.", file=sys.stderr)


def main() -> None:
    parser = argparse.ArgumentParser(description="Import CC-CEDICT into PostgreSQL")
    parser.add_argument("--db-url", default=os.environ.get("DATABASE_URL", "postgresql://carve:carve@localhost:5432/carve"))
    parser.add_argument("--path", help="Path to cedict .txt or .txt.gz file (downloads if omitted)")
    args = parser.parse_args()

    if args.path:
        with open(args.path, "rb") as f:
            raw = f.read()
        if args.path.endswith(".gz"):
            text = gzip.decompress(raw).decode("utf-8")
        else:
            text = raw.decode("utf-8")
    else:
        text = download_cedict()

    import_cedict(args.db_url, text)


if __name__ == "__main__":
    main()
