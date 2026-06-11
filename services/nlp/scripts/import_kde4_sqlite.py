#!/usr/bin/env python3
"""
Import a Korean — English word list into the SQLite dictionary.

This is the SQLite counterpart of import_kde4.py (which targets PostgreSQL and
is NOT wired into CI/Docker). It writes into the same SQLite schema the NLP
service reads (words + word_definitions), under language_code 'ko'.

A clean, fully-free ko→en lemma dictionary is awkward to source (OPUS KDE4 is a
sentence-aligned corpus, not a lemma list). Per the task spec we therefore ship
a curated high-frequency Korean vocabulary list embedded below so ko lookups
resolve out of the box, and additionally accept an optional `--path` TSV
(Korean<TAB>English) to layer extra entries on top.

Usage:
  python scripts/import_kde4_sqlite.py                       # curated list only
  python scripts/import_kde4_sqlite.py --path ko_en.tsv      # curated + TSV
  python scripts/import_kde4_sqlite.py --db data/dictionary.db

The parser function (parse_ko_en_tsv) is pure and unit-tested.
"""

from __future__ import annotations

import argparse
import re
import sqlite3
import sys
import time
from collections import defaultdict
from pathlib import Path

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

# Curated high-frequency Korean vocabulary (Korean, English gloss).
# Expanded from import_kde4.py's SEED_ENTRIES to give meaningful coverage of
# everyday nouns, verbs, and adjectives so ko lookups resolve without a download.
SEED_ENTRIES: list[tuple[str, str]] = [
    # nouns — people / relations
    ("사랑", "love"), ("친구", "friend"), ("가족", "family"), ("사람", "person"),
    ("아이", "child"), ("아기", "baby"), ("남자", "man"), ("여자", "woman"),
    ("엄마", "mom"), ("아빠", "dad"), ("선생님", "teacher"), ("학생", "student"),
    ("이름", "name"), ("나라", "country"), ("한국", "Korea"),
    # nouns — places / things
    ("학교", "school"), ("집", "home, house"), ("회사", "company"),
    ("가게", "shop, store"), ("병원", "hospital"), ("도시", "city"),
    ("방", "room"), ("문", "door"), ("창문", "window"), ("의자", "chair"),
    ("책상", "desk"), ("책", "book"), ("연필", "pencil"), ("종이", "paper"),
    ("전화", "telephone"), ("컴퓨터", "computer"), ("자동차", "car"),
    ("기차", "train"), ("비행기", "airplane"), ("길", "road, path"),
    # nouns — food / nature
    ("밥", "rice, meal"), ("물", "water"), ("음식", "food"), ("고기", "meat"),
    ("과일", "fruit"), ("채소", "vegetable"), ("커피", "coffee"), ("차", "tea, car"),
    ("우유", "milk"), ("빵", "bread"), ("맛", "taste, flavor"),
    ("하늘", "sky"), ("바다", "sea"), ("산", "mountain"), ("강", "river"),
    ("나무", "tree"), ("꽃", "flower"), ("눈", "eye, snow"), ("비", "rain"),
    ("바람", "wind"), ("불", "fire"), ("돌", "stone"),
    # nouns — time
    ("시간", "time"), ("오늘", "today"), ("내일", "tomorrow"), ("어제", "yesterday"),
    ("지금", "now"), ("아침", "morning"), ("점심", "lunch, noon"),
    ("저녁", "evening, dinner"), ("밤", "night"), ("낮", "daytime"),
    ("봄", "spring"), ("여름", "summer"), ("가을", "autumn"), ("겨울", "winter"),
    ("년", "year"), ("월", "month"), ("일", "day, work"), ("주", "week"),
    # nouns — abstract / culture
    ("일", "work, day"), ("말", "speech, words, horse"), ("돈", "money"),
    ("음악", "music"), ("영화", "movie, film"), ("노래", "song"), ("춤", "dance"),
    ("색", "color"), ("문제", "problem"), ("생각", "thought, idea"),
    ("마음", "heart, mind"), ("힘", "strength, power"), ("일자리", "job"),
    # verbs (dictionary form ends in 다)
    ("가다", "to go"), ("오다", "to come"), ("먹다", "to eat"), ("마시다", "to drink"),
    ("자다", "to sleep"), ("보다", "to see, to watch"), ("듣다", "to listen, to hear"),
    ("말하다", "to speak, to say"), ("읽다", "to read"), ("쓰다", "to write, to use"),
    ("주다", "to give"), ("받다", "to receive"), ("사다", "to buy"), ("팔다", "to sell"),
    ("만나다", "to meet"), ("좋아하다", "to like"), ("사랑하다", "to love"),
    ("일하다", "to work"), ("공부하다", "to study"), ("배우다", "to learn"),
    ("가르치다", "to teach"), ("알다", "to know"), ("모르다", "to not know"),
    ("하다", "to do"), ("되다", "to become"), ("있다", "to exist, to have"),
    ("없다", "to not exist"), ("살다", "to live"), ("죽다", "to die"),
    ("웃다", "to laugh, to smile"), ("울다", "to cry"), ("걷다", "to walk"),
    ("뛰다", "to run"), ("앉다", "to sit"), ("서다", "to stand"),
    # adjectives (descriptive verbs)
    ("크다", "to be big"), ("작다", "to be small"), ("많다", "to be many"),
    ("적다", "to be few"), ("좋다", "to be good"), ("나쁘다", "to be bad"),
    ("예쁘다", "to be pretty"), ("아름답다", "to be beautiful"),
    ("새롭다", "to be new"), ("오래되다", "to be old"), ("빠르다", "to be fast"),
    ("느리다", "to be slow"), ("덥다", "to be hot"), ("춥다", "to be cold"),
    ("따뜻하다", "to be warm"), ("어렵다", "to be difficult"), ("쉽다", "to be easy"),
    ("재미있다", "to be fun, interesting"), ("바쁘다", "to be busy"),
    ("행복하다", "to be happy"), ("슬프다", "to be sad"),
]

_HANGUL_RE = re.compile(r"[가-힣ᄀ-ᇿ㄰-㆏]+")


def has_hangul(text: str) -> bool:
    return bool(_HANGUL_RE.search(text))


def parse_ko_en_tsv(text: str) -> list[tuple[str, str]]:
    """
    Parse a Korean<TAB>English TSV into (korean, english) pairs.

    Skips blank lines, lines without Hangul, very long entries (likely phrases),
    and duplicate Korean headwords. Pure function — unit-tested.
    """
    pairs: list[tuple[str, str]] = []
    seen: set[str] = set()
    for line in text.splitlines():
        parts = line.strip().split("\t")
        if len(parts) < 2:
            continue
        ko, en = parts[0].strip(), parts[1].strip()
        if not ko or not en or not has_hangul(ko):
            continue
        if len(ko) > 20 or len(en) > 100:
            continue
        if ko in seen:
            continue
        seen.add(ko)
        pairs.append((ko, en))
    return pairs


def import_to_sqlite(pairs: list[tuple[str, str]], db_path: Path) -> int:
    """Insert (korean, english) pairs into SQLite under language_code 'ko'."""
    print(f"Importing {len(pairs)} Korean entries to {db_path} ...", flush=True)
    conn = sqlite3.connect(str(db_path))
    conn.executescript(SCHEMA_SQL)

    grouped: dict[str, list[str]] = defaultdict(list)
    order: list[str] = []
    for ko, en in pairs:
        if ko not in grouped:
            order.append(ko)
        # one CEDICT-style gloss may contain comma-separated senses
        for sense in (s.strip() for s in en.split(",")):
            if sense and sense not in grouped[ko]:
                grouped[ko].append(sense)

    words_batch: list[tuple] = []
    defs_batch: list[tuple] = []
    for i, ko in enumerate(order):
        row_id = f"ko-{i}"
        words_batch.append((row_id, "ko", ko, None, None, None, "n"))
        for si, sense in enumerate(grouped[ko][:5]):
            defs_batch.append((row_id, "en", si, sense, "n", None, "kde4"))

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
    print(f"Inserted {len(words_batch)} ko word rows, {len(defs_batch)} definitions", flush=True)
    return len(words_batch)


def main() -> None:
    parser = argparse.ArgumentParser(description="Import Korean dictionary into SQLite")
    parser.add_argument("--path", help="Optional ko<TAB>en TSV to layer on top of the curated list")
    parser.add_argument("--db", default="data/dictionary.db", help="Output SQLite path")
    args = parser.parse_args()

    pairs: list[tuple[str, str]] = list(SEED_ENTRIES)
    if args.path:
        text = Path(args.path).read_text(encoding="utf-8")
        extra = parse_ko_en_tsv(text)
        print(f"Loaded {len(extra)} entries from {args.path}", flush=True)
        pairs.extend(extra)

    db_path = Path(args.db)
    db_path.parent.mkdir(parents=True, exist_ok=True)

    t0 = time.time()
    import_to_sqlite(pairs, db_path)
    print(f"Done in {time.time() - t0:.1f}s", flush=True)


if __name__ == "__main__":
    main()
