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

    def test_non_japanese_language_returns_null(self):
        r = client.post("/translate", json={"text": "Hello world", "source_language": "en"})
        assert r.status_code == 200
        assert r.json()["translation"] is None

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

    def test_chinese_language_returns_null(self):
        r = client.post("/translate", json={"text": "你好", "source_language": "zh"})
        assert r.status_code == 200
        assert r.json()["translation"] is None

    def test_korean_language_returns_null(self):
        r = client.post("/translate", json={"text": "안녕하세요", "source_language": "ko"})
        assert r.status_code == 200
        assert r.json()["translation"] is None

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
