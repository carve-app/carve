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
import threading
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path

from .pitch_accent import PITCH_ACCENT


def normalize_language_code(language: str) -> str:
    """
    Map a request language to the `language_code` stored in the dictionary.

    Chinese variants (zh, zh-tw) collapse to 'zh-cn' because CC-CEDICT is
    imported with simplified headwords under that code. Everything else passes
    through unchanged.
    """
    if language in ("zh", "zh-tw", "zh-cn"):
        return "zh-cn"
    return language


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
    pitch_accent: str | None = None  # NHK accent number as string, e.g. "2"


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
        # One sqlite3.Connection per thread. A single shared Connection is NOT
        # safe for concurrent use even with check_same_thread=False: FastAPI
        # runs sync endpoints across a threadpool, and interleaved access on one
        # Connection raises InterfaceError/ProgrammingError and can cross-wire
        # rows between requests. threading.local() gives each worker thread its
        # own Connection, opened lazily on first use.
        self._local = threading.local()

    def _get_conn(self) -> sqlite3.Connection | None:
        conn = getattr(self._local, "conn", None)
        if conn is not None:
            return conn
        if not Path(self._db_path).exists():
            return None
        conn = sqlite3.connect(self._db_path, check_same_thread=False)
        conn.row_factory = sqlite3.Row
        self._local.conn = conn
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

        language = normalize_language_code(language)

        # 1. Exact lemma match
        result = self._query_by_lemma(conn, lemma, language, target_lang)
        if result:
            return result

        # 2. Hiragana-normalized match (for katakana input) — Japanese only.
        if language == "ja":
            from .tokenizer import kata_to_hira
            normalized = kata_to_hira(lemma)
            if normalized != lemma:
                result = self._query_by_lemma(conn, normalized, language, target_lang, confidence=0.9)
                if result:
                    return result

        # 3. Case-insensitive fallback for Latin-script languages — dictionary
        #    headwords are typically lowercase, but lemmas may arrive capitalized
        #    (sentence-initial words, German nouns).
        if language not in ("ja", "zh-cn", "ko"):
            lowered = lemma.lower()
            if lowered != lemma:
                result = self._query_by_lemma(conn, lowered, language, target_lang, confidence=0.95)
                if result:
                    return result

        return None

    def _query_by_lemma(
        self,
        conn: sqlite3.Connection,
        lemma: str,
        language: str,
        target_lang: str,
        confidence: float = 1.0,
    ) -> LookupResult | None:
        row = conn.execute(
            """
            SELECT w.id, w.lemma, w.reading, w.frequency_rank, w.jlpt_level
            FROM words w
            WHERE w.lemma = ? AND w.language_code = ?
            LIMIT 1
            """,
            (lemma, language),
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
            pitch_accent=PITCH_ACCENT.get(row["lemma"]) if language == "ja" else None,
        )

    def translate_sentence(self, text: str, target_lang: str = "en") -> str | None:
        """
        Return a real human translation of a Japanese sentence from the Tatoeba
        corpus when one exists, else None.

        Matches exactly first, then on a punctuation/space-normalized form so
        trailing 。/！ or width differences don't prevent a hit. Returns None
        (never a guess) when the corpus is absent or has no match, so callers can
        fall back to a word gloss.
        """
        if target_lang != "en":
            return None
        stripped = text.strip()
        if not stripped:
            return None

        conn = self._get_conn()
        if conn is None:
            return None

        try:
            # 1) Exact match.
            row = conn.execute(
                "SELECT en_text FROM ja_en_pairs WHERE ja_text = ? LIMIT 1",
                (stripped,),
            ).fetchone()
            if row and row["en_text"]:
                return row["en_text"]

            # 2) Normalized match. Strip common terminal punctuation + spaces
            #    from BOTH sides so a user who typed "私は学生です" still matches a
            #    stored "私は学生です。" (and vice versa). Always attempted when the
            #    exact match misses — the difference may be on the stored side.
            norm = self._normalize_ja(stripped)
            if norm:
                row = conn.execute(
                    """
                    SELECT en_text FROM ja_en_pairs
                    WHERE replace(replace(replace(replace(replace(replace(
                          ja_text,'。',''),'、',''),'！',''),'？',''),' ',''),'　','') = ?
                    LIMIT 1
                    """,
                    (norm,),
                ).fetchone()
                if row and row["en_text"]:
                    return row["en_text"]
        except sqlite3.Error:
            # Tatoeba tables/view absent in this build (OperationalError), or a
            # threadpool connection hiccup (Programming/InterfaceError) — fall
            # back to the word gloss rather than 500 the endpoint.
            return None
        return None

    @staticmethod
    def _normalize_ja(text: str) -> str:
        out = text.strip()
        for ch in ("。", "、", "！", "？", " ", "　"):
            out = out.replace(ch, "")
        return out

    def batch_lookup(
        self,
        lemmas: list[str],
        language: str = "ja",
        target_lang: str = "en",
    ) -> dict[str, LookupResult | None]:
        """Look up multiple lemmas in a single DB round-trip."""
        if not lemmas:
            return {}
        conn = self._get_conn()
        if conn is None:
            return {l: None for l in lemmas}

        language = normalize_language_code(language)

        placeholders = ",".join("?" * len(lemmas))
        word_rows = conn.execute(
            f"""
            SELECT w.id, w.lemma, w.reading, w.frequency_rank, w.jlpt_level
            FROM words w
            WHERE w.lemma IN ({placeholders}) AND w.language_code = ?
            """,
            lemmas + [language],
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
