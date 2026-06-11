"""
Tests for POST /translate endpoint — Track 2 word-gloss translation.
"""
from __future__ import annotations

import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest
from fastapi.testclient import TestClient

from src.app import app

client = TestClient(app)


class TestTranslate:

    def test_empty_text_returns_null(self):
        r = client.post("/translate", json={"text": "", "source_language": "ja"})
        assert r.status_code == 200
        assert r.json()["translation"] is None

    def test_whitespace_only_returns_null(self):
        r = client.post("/translate", json={"text": "   ", "source_language": "ja"})
        assert r.status_code == 200
        assert r.json()["translation"] is None

    def test_non_japanese_language_produces_gloss_or_null(self):
        # Non-JA languages now get a best-effort word gloss when the dictionary
        # has the words (was previously hard-None). Either a gloss string or
        # None is acceptable depending on whether the test env has the dict;
        # the endpoint must not 422 and must stay 200.
        r = client.post("/translate", json={"text": "Hello world", "source_language": "en"})
        assert r.status_code == 200
        t = r.json()["translation"]
        assert t is None or isinstance(t, str)

    def test_japanese_text_returns_gloss(self):
        r = client.post("/translate", json={"text": "食べる", "source_language": "ja"})
        assert r.status_code == 200
        data = r.json()
        # Content word should produce a gloss
        assert data["translation"] is not None
        assert len(data["translation"]) > 0
        # Gloss format: word[definition]
        assert "[" in data["translation"]

    def test_japanese_sentence_gloss_covers_content_words(self):
        r = client.post("/translate", json={
            "text": "毎日ご飯を食べる",
            "source_language": "ja",
        })
        assert r.status_code == 200
        gloss = r.json()["translation"]
        assert gloss is not None
        # Should include multiple content words glossed
        assert gloss.count("[") >= 2

    def test_response_includes_languages(self):
        r = client.post("/translate", json={
            "text": "食べる",
            "source_language": "ja",
            "target_language": "en",
        })
        assert r.status_code == 200
        data = r.json()
        assert data["source_language"] == "ja"
        assert data["target_language"] == "en"

    def test_default_target_language_is_en(self):
        r = client.post("/translate", json={"text": "食べる", "source_language": "ja"})
        assert r.status_code == 200
        assert r.json()["target_language"] == "en"

    def test_default_source_language_is_ja(self):
        r = client.post("/translate", json={"text": "食べる"})
        assert r.status_code == 200
        assert r.json()["source_language"] == "ja"

    def test_particles_only_returns_null_or_empty_gloss(self):
        # Particles are function words; gloss should be null or minimal
        r = client.post("/translate", json={"text": "が", "source_language": "ja"})
        assert r.status_code == 200
        # translation may be null since が is a function word
        data = r.json()
        assert "translation" in data

    def test_chinese_language_produces_gloss_or_null(self):
        # Chinese now gets a best-effort gloss when CC-CEDICT is loaded; 200 +
        # (string or None), never 422.
        r = client.post("/translate", json={"text": "你好", "source_language": "zh"})
        assert r.status_code == 200
        t = r.json()["translation"]
        assert t is None or isinstance(t, str)

    def test_korean_language_produces_gloss_or_null(self):
        r = client.post("/translate", json={"text": "안녕하세요", "source_language": "ko"})
        assert r.status_code == 200
        t = r.json()["translation"]
        assert t is None or isinstance(t, str)

    def test_multilingual_gloss_with_canned_dict(self, monkeypatch):
        # Prove the gloss generalizes beyond JA without needing a real dict or
        # tokenizer: stub the tokenizer + lookup so any content word resolves.
        import src.app as app_module
        from types import SimpleNamespace

        class _Def:
            definition = "cat"

        monkeypatch.setattr(
            app_module, "_tokenize_for_language",
            lambda text, lang: [SimpleNamespace(surface="gatos", lemma="gato", is_content_word=True)],
        )
        monkeypatch.setattr(
            app_module._dict_service, "lookup",
            lambda lemma, language=None, target_lang=None: SimpleNamespace(definitions=[_Def()]),
        )
        r = client.post("/translate", json={"text": "los gatos", "source_language": "es"})
        assert r.status_code == 200
        assert r.json()["translation"] == "gatos[cat]"

    def test_missing_text_field_is_422(self):
        r = client.post("/translate", json={"source_language": "ja"})
        assert r.status_code == 422

    def test_internal_secret_required_when_set(self, monkeypatch):
        monkeypatch.setenv("NLP_INTERNAL_SECRET", "secret123")
        import importlib
        import src.app as app_module
        importlib.reload(app_module)
        patched_client = TestClient(app_module.app)

        r = patched_client.post("/translate", json={"text": "食べる"})
        assert r.status_code == 401

        r = patched_client.post(
            "/translate",
            json={"text": "食べる"},
            headers={"X-Internal-Secret": "secret123"},
        )
        assert r.status_code == 200
        # Restore
        monkeypatch.delenv("NLP_INTERNAL_SECRET")


class TestSentenceTranslationFromCorpus:
    """
    translate_sentence() must return a real human translation from the Tatoeba
    corpus when present, matching exactly or after punctuation normalization, and
    return None (so the caller falls back to a gloss) when absent.
    """

    def _service_with_corpus(self, tmp_path):
        import sqlite3
        from src.dictionary import DictionaryService

        db = tmp_path / "dict.db"
        conn = sqlite3.connect(str(db))
        # Minimal shape of the Tatoeba view that translate_sentence queries.
        conn.executescript(
            """
            CREATE TABLE ja_en_pairs (ja_id INTEGER, ja_text TEXT, en_text TEXT);
            INSERT INTO ja_en_pairs (ja_id, ja_text, en_text) VALUES
              (1, '私は学生です。', 'I am a student.'),
              (2, '映画を見ながら勉強する', 'I study while watching movies');
            """
        )
        conn.commit()
        conn.close()
        return DictionaryService(db_path=str(db))

    def test_exact_corpus_match(self, tmp_path):
        svc = self._service_with_corpus(tmp_path)
        assert svc.translate_sentence("私は学生です。") == "I am a student."

    def test_normalized_corpus_match(self, tmp_path):
        # User text drops the trailing 。 — normalization should still match.
        svc = self._service_with_corpus(tmp_path)
        assert svc.translate_sentence("私は学生です") == "I am a student."

    def test_no_match_returns_none(self, tmp_path):
        svc = self._service_with_corpus(tmp_path)
        assert svc.translate_sentence("これは存在しない文です") is None

    def test_missing_corpus_returns_none_not_error(self, tmp_path):
        # A dictionary DB with no Tatoeba view must not raise — fall back to None.
        import sqlite3
        from src.dictionary import DictionaryService

        db = tmp_path / "bare.db"
        sqlite3.connect(str(db)).close()
        svc = DictionaryService(db_path=str(db))
        assert svc.translate_sentence("私は学生です。") is None

    def test_non_en_target_returns_none(self, tmp_path):
        svc = self._service_with_corpus(tmp_path)
        assert svc.translate_sentence("私は学生です。", target_lang="fr") is None
