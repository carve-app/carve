"""
Dictionary service: JMdict lookup with caching and confidence scoring.

Dictionary sources (in priority order):
  1. JMdict (Japanese-English, CC BY-SA 4.0)
  2. Wiktionary data (fallback, lower confidence)

Confidence scoring:
  1.0  Exact lemma match in JMdict
  0.9  Normalized form match in JMdict
  0.7  Wiktionary match
  0.0  No definition found — return None, never fabricate
"""

from __future__ import annotations

import os
import sqlite3
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path


@dataclass
class Definition:
    sense_index: int
    definition: str
    part_of_speech: str
    tags: list[str]
    source: str
    confidence: float


@dataclass
class LookupResult:
    lemma: str
    reading: str | None
    definitions: list[Definition]
    frequency_rank: int | None
    jlpt_level: str | None
    # True if the result came from a normalized/fallback lookup
    is_exact_match: bool


class DictionaryService:
    """
    SQLite-backed dictionary lookup service.

    The database is populated by scripts/import_jmdict.py.
    Falls back gracefully when the database does not exist yet (returns None).
    """

    def __init__(self, db_path: str | None = None) -> None:
        if db_path is None:
            db_path = os.environ.get(
                "DICT_DB_PATH",
                str(Path(__file__).parent.parent / "data" / "dictionary.db"),
            )
        self._db_path = db_path
        self._conn: sqlite3.Connection | None = None

    def _get_conn(self) -> sqlite3.Connection | None:
        if self._conn is not None:
            return self._conn
        if not Path(self._db_path).exists():
            return None
        conn = sqlite3.connect(self._db_path, check_same_thread=False)
        conn.row_factory = sqlite3.Row
        self._conn = conn
        return conn

    def lookup(
        self,
        lemma: str,
        language: str = "ja",
        target_lang: str = "en",
    ) -> LookupResult | None:
        """
        Look up a dictionary entry for lemma.

        Returns None (not an empty result) when truly unknown — this is
        intentional: do not show a guess when we have no data.
        """
        conn = self._get_conn()
        if conn is None:
            return None

        # 1. Exact lemma match
        result = self._query_by_lemma(conn, lemma, target_lang)
        if result:
            return result

        # 2. Hiragana-normalized match (for katakana input)
        from .tokenizer import kata_to_hira
        normalized = kata_to_hira(lemma)
        if normalized != lemma:
            result = self._query_by_lemma(conn, normalized, target_lang, confidence=0.9)
            if result:
                return result

        return None

    def _query_by_lemma(
        self,
        conn: sqlite3.Connection,
        lemma: str,
        target_lang: str,
        confidence: float = 1.0,
    ) -> LookupResult | None:
        row = conn.execute(
            """
            SELECT w.id, w.lemma, w.reading, w.frequency_rank, w.jlpt_level
            FROM words w
            WHERE w.lemma = ? AND w.language_code = 'ja'
            LIMIT 1
            """,
            (lemma,),
        ).fetchone()

        if not row:
            return None

        word_id = row["id"]
        defs_rows = conn.execute(
            """
            SELECT sense_index, definition, part_of_speech, tags, source, confidence
            FROM word_definitions
            WHERE word_id = ? AND target_language = ?
            ORDER BY sense_index
            LIMIT 8
            """,
            (word_id, target_lang),
        ).fetchall()

        if not defs_rows:
            return None

        definitions = [
            Definition(
                sense_index=r["sense_index"],
                definition=r["definition"],
                part_of_speech=r["part_of_speech"] or "",
                tags=r["tags"].split(",") if r["tags"] else [],
                source=r["source"],
                confidence=min(r["confidence"], confidence),
            )
            for r in defs_rows
        ]

        return LookupResult(
            lemma=row["lemma"],
            reading=row["reading"],
            definitions=definitions,
            frequency_rank=row["frequency_rank"],
            jlpt_level=row["jlpt_level"],
            is_exact_match=confidence == 1.0,
        )

    def batch_lookup(
        self,
        lemmas: list[str],
        target_lang: str = "en",
    ) -> dict[str, LookupResult | None]:
        """Look up multiple lemmas in a single DB round-trip."""
        if not lemmas:
            return {}
        conn = self._get_conn()
        if conn is None:
            return {l: None for l in lemmas}

        placeholders = ",".join("?" * len(lemmas))
        word_rows = conn.execute(
            f"""
            SELECT w.id, w.lemma, w.reading, w.frequency_rank, w.jlpt_level
            FROM words w
            WHERE w.lemma IN ({placeholders}) AND w.language_code = 'ja'
            """,
            lemmas,
        ).fetchall()

        word_map = {r["lemma"]: r for r in word_rows}
        word_ids = [r["id"] for r in word_rows]

        if not word_ids:
            return {l: None for l in lemmas}

        def_placeholders = ",".join("?" * len(word_ids))
        def_rows = conn.execute(
            f"""
            SELECT word_id, sense_index, definition, part_of_speech, tags, source, confidence
            FROM word_definitions
            WHERE word_id IN ({def_placeholders}) AND target_language = ?
            ORDER BY word_id, sense_index
            """,
            word_ids + [target_lang],
        ).fetchall()

        defs_by_word: dict[str, list[Definition]] = {}
        for r in def_rows:
            defs_by_word.setdefault(r["word_id"], []).append(
                Definition(
                    sense_index=r["sense_index"],
                    definition=r["definition"],
                    part_of_speech=r["part_of_speech"] or "",
                    tags=r["tags"].split(",") if r["tags"] else [],
                    source=r["source"],
                    confidence=r["confidence"],
                )
            )

        results: dict[str, LookupResult | None] = {}
        for lemma in lemmas:
            row = word_map.get(lemma)
            if not row:
                results[lemma] = None
                continue
            defs = defs_by_word.get(row["id"], [])
            if not defs:
                results[lemma] = None
                continue
            results[lemma] = LookupResult(
                lemma=row["lemma"],
                reading=row["reading"],
                definitions=defs[:8],
                frequency_rank=row["frequency_rank"],
                jlpt_level=row["jlpt_level"],
                is_exact_match=True,
            )

        return results
