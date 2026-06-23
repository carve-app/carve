"""
Endpoint-level integration tests for dictionary-backed content behavior.

These use a tiny SQLite dictionary instead of the large local dictionary.db so
they prove the FastAPI routes, tokenizers, dictionary service, and scorer are
wired together deterministically.
"""
from __future__ import annotations

import pathlib
import sqlite3
import sys

import pytest
from fastapi.testclient import TestClient

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))

import src.app as app_module
from src.dictionary import DictionaryService


SCHEMA_SQL = """
CREATE TABLE words (
    id              TEXT PRIMARY KEY,
    language_code   TEXT NOT NULL,
    lemma           TEXT NOT NULL,
    reading         TEXT,
    frequency_rank  INTEGER,
    jlpt_level      TEXT,
    pos_primary     TEXT
);

CREATE TABLE word_definitions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    word_id         TEXT NOT NULL,
    target_language TEXT NOT NULL DEFAULT 'en',
    sense_index     INTEGER NOT NULL DEFAULT 0,
    definition      TEXT NOT NULL,
    part_of_speech  TEXT,
    tags            TEXT,
    source          TEXT NOT NULL,
    confidence      REAL NOT NULL DEFAULT 1.0
);
"""


def _insert_word(
    conn: sqlite3.Connection,
    *,
    word_id: str,
    language: str,
    lemma: str,
    reading: str | None,
    definitions: list[str],
    frequency_rank: int | None = None,
    pos: str = "noun",
    source: str = "testdict",
) -> None:
    conn.execute(
        """
        INSERT INTO words
            (id, language_code, lemma, reading, frequency_rank, jlpt_level, pos_primary)
        VALUES (?, ?, ?, ?, ?, NULL, ?)
        """,
        (word_id, language, lemma, reading, frequency_rank, pos),
    )
    for i, definition in enumerate(definitions):
        conn.execute(
            """
            INSERT INTO word_definitions
                (word_id, target_language, sense_index, definition,
                 part_of_speech, tags, source, confidence)
            VALUES (?, 'en', ?, ?, ?, NULL, ?, 1.0)
            """,
            (word_id, i, definition, pos, source),
        )


@pytest.fixture()
def client_with_test_dictionary(tmp_path, monkeypatch) -> TestClient:
    db_path = tmp_path / "dictionary.db"
    conn = sqlite3.connect(str(db_path))
    conn.executescript(SCHEMA_SQL)
    _insert_word(
        conn,
        word_id="en-cat",
        language="en",
        lemma="cat",
        reading=None,
        definitions=["a small domesticated feline"],
        frequency_rank=1500,
    )
    _insert_word(
        conn,
        word_id="en-run",
        language="en",
        lemma="run",
        reading=None,
        definitions=["move swiftly on foot"],
        frequency_rank=800,
        pos="verb",
    )
    _insert_word(
        conn,
        word_id="de-haus",
        language="de",
        lemma="haus",
        reading=None,
        definitions=["house", "home"],
    )
    _insert_word(
        conn,
        word_id="zh-zhongguo",
        language="zh-cn",
        lemma="中国",
        reading="zhōng guó",
        definitions=["China"],
    )
    conn.commit()
    conn.close()

    monkeypatch.setattr(app_module, "_INTERNAL_SECRET", "")
    monkeypatch.setattr(app_module, "_dict_service", DictionaryService(db_path=str(db_path)))
    return TestClient(app_module.app)


def test_tokenize_english_attaches_dictionary_definitions_to_unknown_content_words(
    client_with_test_dictionary,
):
    r = client_with_test_dictionary.post(
        "/tokenize",
        json={
            "text": "The cats were running quickly.",
            "language": "en",
            "include_definitions": True,
        },
    )
    assert r.status_code == 200

    tokens = r.json()["tokens"]
    by_lemma = {t["lemma"]: t for t in tokens}
    assert by_lemma["cat"]["definitions"][0]["definition"] == "a small domesticated feline"
    assert by_lemma["run"]["definitions"][0]["definition"] == "move swiftly on foot"
    assert by_lemma["the"]["definitions"] is None
    assert by_lemma["be"]["definitions"] is None
    assert by_lemma["quickly"]["definitions"] is None


def test_lookup_german_retries_raw_surface_when_lemmatizer_overstems(
    client_with_test_dictionary,
):
    r = client_with_test_dictionary.post("/lookup", json={"surface": "Haus", "language": "de"})
    assert r.status_code == 200

    body = r.json()
    assert body["found"] is True
    assert body["lemma"] == "haus"
    assert [d["definition"] for d in body["definitions"]] == ["house", "home"]


def test_lookup_chinese_variant_language_resolves_shared_cedict_entries(
    client_with_test_dictionary,
):
    r = client_with_test_dictionary.post("/lookup", json={"surface": "中国", "language": "zh-tw"})
    assert r.status_code == 200

    body = r.json()
    assert body["found"] is True
    assert body["lemma"] == "中国"
    assert body["reading"] == "zhōng guó"
    assert body["definitions"][0]["definition"] == "China"


def test_batch_lookup_chinese_variant_language_keeps_requested_keys(
    client_with_test_dictionary,
):
    r = client_with_test_dictionary.post(
        "/batch-lookup",
        json={"lemmas": ["中国", "不存在"], "language": "zh-tw"},
    )
    assert r.status_code == 200

    results = r.json()["results"]
    assert results["中国"]["definitions"][0]["definition"] == "China"
    assert results["不存在"] is None


def test_score_text_english_uses_shared_learning_half_credit(client_with_test_dictionary):
    r = client_with_test_dictionary.post(
        "/score-text",
        json={
            "text": "The cats were running quickly.",
            "language": "en",
            "known_lemmas": ["cat", "run"],
            "learning_lemmas": ["quickly"],
        },
    )
    assert r.status_code == 200

    body = r.json()
    assert body["total_content_words"] == 3
    assert body["unknown_count"] == 0
    assert body["comprehension_pct"] == 83.3
    assert body["recommended_mode"] == "study_read"


def test_score_text_english_empty_content_is_flow_read(client_with_test_dictionary):
    r = client_with_test_dictionary.post(
        "/score-text",
        json={"text": "The, and... but?", "language": "en"},
    )
    assert r.status_code == 200

    body = r.json()
    assert body["total_content_words"] == 0
    assert body["unknown_count"] == 0
    assert body["comprehension_pct"] == 100.0
    assert body["recommended_mode"] == "flow_read"
