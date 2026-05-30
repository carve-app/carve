#!/usr/bin/env python3
"""
Import KDE4 Korean–English dictionary into PostgreSQL.

The KDE4 corpus is available from OPUS:
  https://opus.nlpl.eu/KDE4.php

TSV format: Korean TAB English (one entry per line)

Usage:
  python import_kde4.py [--db-url postgresql://...] [--path kde4_ko_en.tsv]

Without --path, downloads the OPUS KDE4 ko-en TSV.
"""

from __future__ import annotations

import argparse
import os
import re
import sys
import urllib.request
from collections import defaultdict

KDE4_URL = "https://opus.nlpl.eu/download.php?f=KDE4/v2/moses/en-ko.txt.zip"

# Additional curated Korean words not in KDE4 (common conversation words)
SEED_ENTRIES: list[tuple[str, str]] = [
    ("사랑", "love"),
    ("친구", "friend"),
    ("학교", "school"),
    ("집", "home, house"),
    ("밥", "rice, meal"),
    ("물", "water"),
    ("책", "book"),
    ("시간", "time"),
    ("사람", "person"),
    ("오늘", "today"),
    ("내일", "tomorrow"),
    ("어제", "yesterday"),
    ("한국", "Korea"),
    ("음식", "food"),
    ("가족", "family"),
    ("이름", "name"),
    ("일", "work, day"),
    ("말", "speech, words"),
    ("눈", "eye, snow"),
    ("길", "road, path"),
    ("나라", "country"),
    ("돈", "money"),
    ("하늘", "sky"),
    ("바다", "sea"),
    ("산", "mountain"),
    ("나무", "tree"),
    ("꽃", "flower"),
    ("봄", "spring"),
    ("여름", "summer"),
    ("가을", "autumn"),
    ("겨울", "winter"),
    ("아침", "morning"),
    ("저녁", "evening"),
    ("밤", "night"),
    ("음악", "music"),
    ("영화", "movie, film"),
    ("노래", "song"),
    ("춤", "dance"),
    ("색", "color"),
    ("맛", "taste, flavor"),
]

_HANGUL_RE = re.compile(r'[가-힣ᄀ-ᇿ㄰-㆏]+')


def _has_hangul(text: str) -> bool:
    return bool(_HANGUL_RE.search(text))


def _parse_tsv(text: str) -> list[tuple[str, str]]:
    """Parse ko-en TSV, yielding (korean, english) pairs."""
    pairs: list[tuple[str, str]] = []
    seen: set[str] = set()

    for line in text.splitlines():
        parts = line.strip().split("\t")
        if len(parts) < 2:
            continue
        ko, en = parts[0].strip(), parts[1].strip()
        if not _has_hangul(ko):
            continue
        # Skip very long strings (phrases, not vocabulary)
        if len(ko) > 20 or len(en) > 100:
            continue
        if ko in seen:
            continue
        seen.add(ko)
        pairs.append((ko, en))

    return pairs


def import_kde4(db_url: str, ko_en_pairs: list[tuple[str, str]]) -> None:
    try:
        import psycopg2
        import psycopg2.extras
    except ImportError:
        print("psycopg2 not installed — pip install psycopg2-binary", file=sys.stderr)
        sys.exit(1)

    conn = psycopg2.connect(db_url)
    cur = conn.cursor()

    # Group multiple English definitions under the same Korean lemma
    grouped: dict[str, list[str]] = defaultdict(list)
    for ko, en in ko_en_pairs:
        grouped[ko].append(en)

    all_entries = list(grouped.items())
    print(f"Inserting {len(all_entries)} Korean entries...", file=sys.stderr)

    # Insert words
    psycopg2.extras.execute_values(
        cur,
        """
        INSERT INTO words (language_code, lemma, reading, pos, frequency_rank)
        VALUES %s
        ON CONFLICT (language_code, lemma, reading) DO NOTHING
        RETURNING id, lemma
        """,
        [("ko", ko, None, "n", None) for ko, _ in all_entries],
        page_size=1000,
    )
    id_rows = cur.fetchall()
    id_map = {row[1]: row[0] for row in id_rows}

    # Insert definitions
    def_batch: list[tuple] = []
    for ko, defs in all_entries:
        word_id = id_map.get(ko)
        if not word_id:
            continue
        for i, en in enumerate(defs[:5]):
            def_batch.append((word_id, "en", i, en, "kde4"))

    psycopg2.extras.execute_values(
        cur,
        """
        INSERT INTO word_definitions (word_id, target_language, sense_index, definition, source)
        VALUES %s
        ON CONFLICT DO NOTHING
        """,
        def_batch,
        page_size=1000,
    )

    conn.commit()
    cur.close()
    conn.close()
    print(f"Done. Imported {len(all_entries)} Korean entries.", file=sys.stderr)


def main() -> None:
    parser = argparse.ArgumentParser(description="Import KDE4 Korean dictionary into PostgreSQL")
    parser.add_argument("--db-url", default=os.environ.get("DATABASE_URL", "postgresql://carve:carve@localhost:5432/carve"))
    parser.add_argument("--path", help="Path to ko-en TSV file")
    args = parser.parse_args()

    if args.path:
        with open(args.path, encoding="utf-8") as f:
            text = f.read()
        pairs = _parse_tsv(text)
    else:
        # Start with seeded vocabulary, skip full download in dev
        print("No --path provided; using seed vocabulary only.", file=sys.stderr)
        pairs = list(SEED_ENTRIES)

    import_kde4(args.db_url, pairs)


if __name__ == "__main__":
    main()
