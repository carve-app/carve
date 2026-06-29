"""
Tests for POST /translate — Google Cloud Translation v3 (TLLM) only.

The endpoint has a single engine and NO gloss/corpus/keyless fallback: it
returns a fluent translation when the engine is configured, else null. Network
is never hit here — the v3 parser is pure and the engine is monkeypatched.
"""
from __future__ import annotations

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from fastapi.testclient import TestClient

from carve_nlp.app import app

client = TestClient(app)


class TestTranslateEndpoint:
    def test_empty_text_returns_null(self):
        r = client.post("/translate", json={"text": "", "source_language": "ja"})
        assert r.status_code == 200
        assert r.json()["translation"] is None

    def test_whitespace_only_returns_null(self):
        r = client.post("/translate", json={"text": "   ", "source_language": "ja"})
        assert r.status_code == 200
        assert r.json()["translation"] is None

    def test_missing_text_field_is_422(self):
        r = client.post("/translate", json={"source_language": "ja"})
        assert r.status_code == 422

    def test_response_echoes_languages(self):
        r = client.post("/translate", json={
            "text": "x", "source_language": "ja", "target_language": "en",
        })
        assert r.status_code == 200
        body = r.json()
        assert body["source_language"] == "ja"
        assert body["target_language"] == "en"

    def test_returns_fluent_translation_when_engine_configured(self, monkeypatch):
        import carve_nlp.app as app_module

        monkeypatch.setattr(app_module, "_INTERNAL_SECRET", "")
        monkeypatch.setattr(
            app_module.translator, "translate_sentence",
            lambda text, src, tgt: "Black cats sleep.",
        )
        local = TestClient(app_module.app)
        r = local.post("/translate", json={
            "text": "los gatos negros duermen", "source_language": "es", "target_language": "en",
        })
        assert r.status_code == 200
        assert r.json()["translation"] == "Black cats sleep."

    def test_returns_null_when_engine_unavailable(self, monkeypatch):
        # No gloss fallback: engine None -> translation null (not a word gloss).
        import carve_nlp.app as app_module

        monkeypatch.setattr(app_module, "_INTERNAL_SECRET", "")
        monkeypatch.setattr(app_module.translator, "translate_sentence", lambda *a, **k: None)
        local = TestClient(app_module.app)
        r = local.post("/translate", json={
            "text": "el gato negro", "source_language": "es", "target_language": "en",
        })
        assert r.status_code == 200
        assert r.json()["translation"] is None

    def test_internal_secret_required_when_set(self, monkeypatch):
        monkeypatch.setenv("NLP_INTERNAL_SECRET", "secret123")
        import importlib
        import carve_nlp.app as app_module
        importlib.reload(app_module)
        patched = TestClient(app_module.app)
        try:
            assert patched.post("/translate", json={"text": "食べる"}).status_code == 401
            ok = patched.post(
                "/translate",
                json={"text": "食べる"},
                headers={"X-Internal-Secret": "secret123"},
            )
            assert ok.status_code == 200
        finally:
            monkeypatch.delenv("NLP_INTERNAL_SECRET")
            importlib.reload(app_module)


class TestTranslatorModule:
    """Unit tests for carve_nlp/translator.py — pure logic + gating, no network."""

    def test_parse_v3_response(self):
        from carve_nlp import translator as T
        out = {"translations": [{"translatedText": "Black cats sleep.", "model": "x"}]}
        assert T._parse_v3_response(out) == "Black cats sleep."

    def test_parse_v3_response_strips_and_handles_empty(self):
        from carve_nlp import translator as T
        assert T._parse_v3_response({"translations": [{"translatedText": "  hi "}]}) == "hi"
        assert T._parse_v3_response({"translations": [{"translatedText": ""}]}) is None
        assert T._parse_v3_response({"translations": []}) is None
        assert T._parse_v3_response({}) is None
        assert T._parse_v3_response(None) is None

    def test_bcp47_mapping(self):
        from carve_nlp import translator as T
        assert T._bcp47("zh-cn") == "zh-CN"
        assert T._bcp47("ZH-TW") == "zh-TW"
        assert T._bcp47("vi") == "vi"
        assert T._bcp47("ja") == "ja"
        assert T._bcp47("xx") is None

    def test_disabled_without_credentials_returns_none(self, monkeypatch):
        # No ADC -> engine disabled -> None, no network. Reset module cache.
        from carve_nlp import translator as T
        monkeypatch.setattr(T, "_init_done", False)
        monkeypatch.setattr(T, "_creds", None)
        monkeypatch.setattr(T, "_project", None)
        monkeypatch.delenv("GOOGLE_APPLICATION_CREDENTIALS", raising=False)
        monkeypatch.delenv("GOOGLE_CLOUD_PROJECT", raising=False)
        # google.auth.default raising (no creds) must be swallowed -> None.
        assert T.translate_sentence("los gatos", "es", "en") is None

    def test_unsupported_or_same_language_returns_none(self, monkeypatch):
        # Guard checks happen before any creds/network use.
        from carve_nlp import translator as T
        assert T.translate_sentence("hello", "en", "en") is None   # same language
        assert T.translate_sentence("hello", "es", "xx") is None   # unknown target
        assert T.translate_sentence("", "es", "en") is None        # empty
        assert T.translate_sentence("x" * 6000, "es", "en") is None  # too long


class TestSentenceTranslationFromCorpus:
    """The Tatoeba corpus lookup on DictionaryService still exists (no longer
    wired into /translate, but kept + tested for reuse)."""

    def _service_with_corpus(self, tmp_path):
        import sqlite3
        from carve_nlp.dictionary import DictionaryService

        db = tmp_path / "dict.db"
        conn = sqlite3.connect(str(db))
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
        svc = self._service_with_corpus(tmp_path)
        assert svc.translate_sentence("私は学生です") == "I am a student."

    def test_no_match_returns_none(self, tmp_path):
        svc = self._service_with_corpus(tmp_path)
        assert svc.translate_sentence("これは存在しない文です") is None

    def test_missing_corpus_returns_none_not_error(self, tmp_path):
        import sqlite3
        from carve_nlp.dictionary import DictionaryService

        db = tmp_path / "bare.db"
        sqlite3.connect(str(db)).close()
        svc = DictionaryService(db_path=str(db))
        assert svc.translate_sentence("私は学生です。") is None
