"""
Integration tests for the NLP FastAPI service.
Runs the ASGI app in-process using HTTPX (no server required).
"""

from __future__ import annotations

import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest
from fastapi.testclient import TestClient

from src.app import app

client = TestClient(app)


def test_health():
    r = client.get("/health")
    assert r.status_code == 200
    assert r.json()["status"] == "ok"
    assert r.json()["service"] == "nlp"


class TestSelectSentence:

    def test_picks_candidate_with_target_lemma(self):
        r = client.post("/select-sentence", json={
            "candidates": [
                "今日は天気がいいです。",          # no target
                "私は毎日寿司を食べる。",          # has 食べる
            ],
            "target_lemma": "食べる",
            "language": "ja",
            "known_lemmas": ["私", "毎日", "寿司", "今日", "天気"],
        })
        assert r.status_code == 200
        data = r.json()
        assert data["best"] is not None
        assert "食べる" in data["best"]["text"]
        assert data["best"]["contains_target"] is True

    def test_empty_candidates_after_strip(self):
        r = client.post("/select-sentence", json={
            "candidates": ["   ", ""],
            "target_lemma": "食べる",
            "language": "ja",
        })
        # min_length=1 enforced before strip — but blank-after-strip returns nulls.
        # FastAPI accepts the request (one non-empty string in the list); we
        # respond with best=None and empty ranked.
        assert r.status_code == 200
        body = r.json()
        assert body["best"] is None
        assert body["ranked"] == []

    def test_unsupported_language_returns_422(self):
        r = client.post("/select-sentence", json={
            "candidates": ["bonjour"],
            "target_lemma": "x",
            "language": "fr",
        })
        assert r.status_code == 422

    def test_ranked_descending_by_fit(self):
        r = client.post("/select-sentence", json={
            "candidates": [
                "私は毎日寿司を食べる。",
                "今日は天気がいいです。",
            ],
            "target_lemma": "食べる",
            "language": "ja",
            "known_lemmas": ["私", "毎日", "寿司"],
        })
        assert r.status_code == 200
        ranked = r.json()["ranked"]
        fits = [c["fit_score"] for c in ranked]
        assert fits == sorted(fits, reverse=True)


class TestTokenize:

    def test_basic_sentence(self):
        r = client.post("/tokenize", json={
            "text": "日本語を勉強しています",
            "language": "ja",
        })
        assert r.status_code == 200
        data = r.json()
        surfaces = [t["surface"] for t in data["tokens"]]
        assert "日本語" in surfaces or any("日本" in s for s in surfaces)

    def test_returns_readings(self):
        r = client.post("/tokenize", json={"text": "食べる", "language": "ja"})
        tokens = r.json()["tokens"]
        # Must have non-empty reading
        assert all(t["reading"] for t in tokens)

    def test_content_word_flag(self):
        r = client.post("/tokenize", json={"text": "毎日ご飯を食べる", "language": "ja"})
        tokens = r.json()["tokens"]
        content = [t for t in tokens if t["is_content_word"]]
        function_ = [t for t in tokens if not t["is_content_word"]]
        assert len(content) >= 2
        assert len(function_) >= 1  # particle を

    def test_comprehension_score_all_known(self):
        r = client.post("/tokenize", json={
            "text": "食べる",
            "language": "ja",
            "known_lemmas": ["食べる"],
        })
        data = r.json()
        assert data["comprehension_pct"] == 100.0
        assert data["recommended_mode"] == "flow_read"

    def test_comprehension_score_all_unknown(self):
        r = client.post("/tokenize", json={
            "text": "食べる",
            "language": "ja",
            "known_lemmas": [],
        })
        data = r.json()
        # comprehension_pct is None when no vocab provided (no scoring requested)
        assert data["comprehension_pct"] is None

    def test_user_status_known(self):
        r = client.post("/tokenize", json={
            "text": "食べる",
            "language": "ja",
            "known_lemmas": ["食べる"],
            "learning_lemmas": [],
        })
        tokens = r.json()["tokens"]
        content = [t for t in tokens if t["is_content_word"]]
        assert any(t["user_status"] == "known" for t in content)

    def test_user_status_unknown(self):
        r = client.post("/tokenize", json={
            "text": "食べる",
            "language": "ja",
            "known_lemmas": [],
            "learning_lemmas": [],
        })
        tokens = r.json()["tokens"]
        content = [t for t in tokens if t["is_content_word"]]
        assert any(t["user_status"] == "unknown" for t in content)

    def test_unsupported_language(self):
        r = client.post("/tokenize", json={"text": "hello", "language": "fr"})
        assert r.status_code == 422

    def test_english_tokenize(self):
        r = client.post("/tokenize", json={"text": "The cats were running quickly.", "language": "en"})
        assert r.status_code == 200
        tokens = r.json()["tokens"]
        lemmas = {t["lemma"] for t in tokens}
        # plural collapses, past-progressive collapses
        assert "cat" in lemmas
        assert "run" in lemmas
        assert "quickly" in lemmas or "quick" in lemmas

    def test_migaku_failure_case_gozaimasen(self):
        """ございません must return correct full reading."""
        r = client.post("/tokenize", json={
            "text": "ございません",
            "language": "ja",
            "mode": "A",
        })
        assert r.status_code == 200
        tokens = r.json()["tokens"]
        full_reading_hira = "".join(t["reading_hira"] for t in tokens)
        assert full_reading_hira == "ございません", (
            f"Full reading was {full_reading_hira!r}"
        )

    def test_migaku_failure_case_haitte(self):
        """入って must read はいって."""
        r = client.post("/tokenize", json={
            "text": "入って",
            "language": "ja",
            "mode": "A",
        })
        tokens = r.json()["tokens"]
        full_reading = "".join(t["reading_hira"] for t in tokens)
        assert full_reading == "はいって", f"Got {full_reading!r}"


class TestLookup:

    def test_lookup_common_verb(self):
        r = client.post("/lookup", json={"surface": "食べる", "language": "ja"})
        assert r.status_code == 200
        data = r.json()
        assert data["lemma"] == "食べる"
        assert data["reading"] in ("たべる", "タベル", None)  # may vary by dict
        # Dictionary may not be populated in test env — check API contract
        assert "found" in data
        assert "furigana" in data

    def test_lookup_returns_reading(self):
        r = client.post("/lookup", json={"surface": "入って", "language": "ja"})
        assert r.status_code == 200
        data = r.json()
        # The furigana for 入 must show はい (correct reading for 入って context)
        # The dictionary `reading` field may be はいる or いる (both valid for 入る)
        furigana = data["furigana"]
        kanji_spans = [s for s in furigana if s["reading"]]
        assert any("はい" in s["reading"] for s in kanji_spans), (
            f"Furigana for 入 should include はい: {furigana}"
        )

    def test_lookup_furigana_structure(self):
        r = client.post("/lookup", json={"surface": "食べる", "language": "ja"})
        data = r.json()
        furigana = data["furigana"]
        assert isinstance(furigana, list)
        for span in furigana:
            assert "text" in span
            assert "reading" in span

    def test_lookup_unsupported_language(self):
        r = client.post("/lookup", json={"surface": "bonjour", "language": "fr"})
        assert r.status_code == 422

    def test_lookup_kana_only(self):
        """Pure kana input should still return a result."""
        r = client.post("/lookup", json={"surface": "たべる", "language": "ja"})
        assert r.status_code == 200
        assert r.json()["lemma"] != ""


class TestScoreText:

    def test_score_all_known(self):
        r = client.post("/score-text", json={
            "text": "食べる",
            "language": "ja",
            "known_lemmas": ["食べる"],
            "learning_lemmas": [],
        })
        assert r.status_code == 200
        data = r.json()
        assert data["comprehension_pct"] == 100.0
        assert data["recommended_mode"] == "flow_read"
        assert data["unknown_count"] == 0

    def test_score_all_unknown(self):
        r = client.post("/score-text", json={
            "text": "日本語を勉強しています",
            "language": "ja",
            "known_lemmas": [],
            "learning_lemmas": [],
        })
        data = r.json()
        assert data["comprehension_pct"] < 50.0
        assert data["unknown_count"] > 0
        assert data["recommended_mode"] in ("study_read", "too_hard", "mining_read")

    def test_score_returns_top_unknowns(self):
        r = client.post("/score-text", json={
            "text": "日本語を勉強しています",
            "language": "ja",
            "known_lemmas": [],
        })
        data = r.json()
        assert isinstance(data["top_unknown_lemmas"], list)

    def test_score_empty_text(self):
        r = client.post("/score-text", json={
            "text": "",
            "language": "ja",
        })
        assert r.status_code == 200
        assert r.json()["comprehension_pct"] == 100.0

    def test_score_mining_read_zone(self):
        """Content where ~10% words are unknown → mining_read."""
        # Known words = all common words in the sentence except one
        r = client.post("/score-text", json={
            "text": "毎日ご飯を食べています",
            "language": "ja",
            "known_lemmas": ["毎日", "食べる", "いる"],
            "learning_lemmas": ["ご飯"],
        })
        data = r.json()
        assert data["comprehension_pct"] > 80.0


class TestBatchLookup:

    def test_batch_lookup_returns_dict(self):
        r = client.post("/batch-lookup", json={
            "lemmas": ["食べる", "飲む", "日本語"],
            "language": "ja",
        })
        assert r.status_code == 200
        results = r.json()["results"]
        assert isinstance(results, dict)
        assert "食べる" in results
        assert "飲む" in results

    def test_batch_lookup_missing_word_returns_null(self):
        r = client.post("/batch-lookup", json={
            "lemmas": ["ZZZNONSENSEWORD999"],
            "language": "ja",
        })
        results = r.json()["results"]
        assert results.get("ZZZNONSENSEWORD999") is None

    def test_batch_lookup_empty(self):
        r = client.post("/batch-lookup", json={"lemmas": [], "language": "ja"})
        assert r.status_code == 200
        assert r.json()["results"] == {}
