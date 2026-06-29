"""
Unit tests for services/nlp/carve_nlp/grammar_ja.py — the JA grammar pattern detector.

Synthetic tokens are used (no SudachiPy dependency) so tests stay fast.
"""

from __future__ import annotations

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from carve_nlp.grammar_ja import (
    PATTERNS,
    detect_patterns,
    pattern_summary,
)
from carve_nlp.tokenizer import Token


def tok(surface: str, lemma: str | None = None, pos: str = "名詞") -> Token:
    return Token(
        surface=surface,
        lemma=lemma if lemma is not None else surface,
        reading=surface, reading_hira=surface,
        pos=pos, pos_detail="", pos_full=(pos,),
        is_content_word=pos.startswith("名詞") or pos.startswith("動詞") or pos.startswith("形容"),
    )


def test_patterns_are_unique_ids():
    ids = [p.id for p in PATTERNS]
    assert len(ids) == len(set(ids)), "pattern IDs must be unique"


def test_patterns_have_jlpt_levels():
    for p in PATTERNS:
        assert p.jlpt in {"N5", "N4", "N3"}, f"{p.id} jlpt={p.jlpt}"


def test_detect_te_iru():
    tokens = [
        tok("食べ", "食べる", "動詞"),
        tok("て", pos="助詞"),
        tok("いる", pos="動詞"),
    ]
    got = detect_patterns(tokens)
    ids = [d.pattern_id for d in got]
    assert "te-iru" in ids


def test_detect_te_shimau():
    tokens = [
        tok("食べ", "食べる", "動詞"),
        tok("て", pos="助詞"),
        tok("しまう", pos="動詞"),
    ]
    got = detect_patterns(tokens)
    assert any(d.pattern_id == "te-shimau" for d in got)


def test_detect_tai():
    tokens = [
        tok("食べ", "食べる", "動詞"),
        tok("たい", pos="助動詞"),
    ]
    got = detect_patterns(tokens)
    assert any(d.pattern_id == "tai" for d in got)


def test_detect_nakereba_naranai_prefers_longer_over_inner_ba():
    tokens = [
        tok("食べ", "食べる", "動詞"),
        tok("なけれ", pos="助動詞"),
        tok("ば", pos="助詞"),
        tok("ならない", pos="助動詞"),
    ]
    got = detect_patterns(tokens)
    ids = [d.pattern_id for d in got]
    assert "nakereba-naranai" in ids


def test_detect_koto_ga_dekiru():
    tokens = [
        tok("する", pos="動詞"),
        tok("こと", pos="名詞"),
        tok("が", pos="助詞"),
        tok("できる", pos="動詞"),
    ]
    got = detect_patterns(tokens)
    assert any(d.pattern_id == "koto-ga-dekiru" for d in got)


def test_no_false_positive_on_unrelated_text():
    tokens = [
        tok("猫", pos="名詞"),
        tok("が", pos="助詞"),
        tok("いる", pos="動詞"),
    ]
    got = detect_patterns(tokens)
    # 'いる' alone (no preceding て) must not match te-iru
    assert all(d.pattern_id != "te-iru" for d in got)


def test_non_overlapping_matches():
    tokens = [
        tok("食べ", "食べる", "動詞"),
        tok("て", pos="助詞"),
        tok("いる", pos="動詞"),
        tok("飲ん", "飲む", "動詞"),
        tok("て", pos="助詞"),
        tok("いる", pos="動詞"),
    ]
    got = detect_patterns(tokens)
    te_iru = [d for d in got if d.pattern_id == "te-iru"]
    assert len(te_iru) == 2


def test_pattern_summary_grammar_pct():
    tokens = [
        tok("食べ", "食べる", "動詞"),
        tok("て", pos="助詞"),
        tok("いる", pos="動詞"),
    ]
    summary = pattern_summary(tokens, {"te-iru"})
    assert summary["grammar_pct"] == 100.0
    assert summary["total_patterns"] == 1
    assert summary["unknown_patterns"] == []


def test_pattern_summary_unknown_partial():
    tokens = [
        tok("食べ", "食べる", "動詞"),
        tok("て", pos="助詞"),
        tok("いる", pos="動詞"),
        tok("飲ん", "飲む", "動詞"),
        tok("て", pos="助詞"),
        tok("しまう", pos="動詞"),
    ]
    summary = pattern_summary(tokens, {"te-iru"})
    assert summary["total_patterns"] == 2
    assert summary["grammar_pct"] == 50.0
    unknown_ids = {p["id"] for p in summary["unknown_patterns"]}
    assert "te-shimau" in unknown_ids


def test_pattern_summary_empty():
    summary = pattern_summary([], set())
    assert summary["total_patterns"] == 0
    assert summary["grammar_pct"] == 100.0
