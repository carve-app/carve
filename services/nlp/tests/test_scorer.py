"""
Unit tests for services/nlp/src/scorer.py — score_content function.

Tests cover:
  - Empty token list
  - All known / all unknown / mixed
  - recommended_mode thresholds (flow_read / mining_read / study_read / too_hard)
  - Learning words count as 50% known
  - top_unknown_lemmas: sorted by frequency_rank, deduplicated, capped at 10
  - frequency penalty effect on difficulty_score
  - difficulty_score range (0.0 – 1.0)
  - comprehension_pct rounding
"""

from __future__ import annotations

import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest
from src.scorer import ContentScore, score_content
from src.tokenizer import Token


# ── helpers ───────────────────────────────────────────────────────────────────

def make_token(
    lemma: str,
    is_content: bool = True,
    freq: int | None = None,
) -> Token:
    return Token(
        surface=lemma,
        lemma=lemma,
        reading=lemma,
        reading_hira=lemma,
        pos="名詞",
        pos_detail="普通名詞",
        pos_full=("名詞", "普通名詞", "一般"),
        is_content_word=is_content,
        frequency_rank=freq,
    )


def content(lemmas: list[str], freqs: list[int | None] | None = None) -> list[Token]:
    if freqs is None:
        freqs = [None] * len(lemmas)
    return [make_token(l, is_content=True, freq=f) for l, f in zip(lemmas, freqs)]


def func_tokens(lemmas: list[str]) -> list[Token]:
    return [make_token(l, is_content=False) for l in lemmas]


# ── empty ─────────────────────────────────────────────────────────────────────

def test_empty_tokens_returns_flow_read():
    s = score_content([], set(), set())
    assert s.comprehension_pct == 100.0
    assert s.recommended_mode == "flow_read"
    assert s.total_content_words == 0
    assert s.unknown_count == 0


def test_only_function_words_treated_as_empty():
    tokens = func_tokens(["は", "が", "を", "で"])
    s = score_content(tokens, set(), set())
    assert s.comprehension_pct == 100.0
    assert s.recommended_mode == "flow_read"


# ── all known ─────────────────────────────────────────────────────────────────

def test_all_known_is_100_pct():
    tokens = content(["食べる", "飲む", "行く"])
    s = score_content(tokens, {"食べる", "飲む", "行く"}, set())
    assert s.comprehension_pct == 100.0
    assert s.recommended_mode == "flow_read"
    assert s.unknown_count == 0


# ── all unknown ──────────────────────────────────────────────────────────────

def test_all_unknown_is_0_pct():
    tokens = content(["食べる", "飲む", "行く"])
    s = score_content(tokens, set(), set())
    assert s.comprehension_pct == 0.0
    assert s.recommended_mode == "too_hard"
    assert s.unknown_count == 3


# ── recommended_mode thresholds ───────────────────────────────────────────────

def test_exactly_98pct_is_flow_read():
    # 98 of 100 content words known
    lemmas = [f"w{i}" for i in range(100)]
    tokens = content(lemmas)
    known = set(lemmas[:98])
    s = score_content(tokens, known, set())
    assert s.comprehension_pct >= 98.0
    assert s.recommended_mode == "flow_read"


def test_95pct_is_mining_read():
    lemmas = [f"w{i}" for i in range(100)]
    tokens = content(lemmas)
    known = set(lemmas[:95])
    s = score_content(tokens, known, set())
    assert 90.0 <= s.comprehension_pct < 98.0
    assert s.recommended_mode == "mining_read"


def test_85pct_is_study_read():
    lemmas = [f"w{i}" for i in range(100)]
    tokens = content(lemmas)
    known = set(lemmas[:85])
    s = score_content(tokens, known, set())
    assert 80.0 <= s.comprehension_pct < 90.0
    assert s.recommended_mode == "study_read"


def test_70pct_is_too_hard():
    lemmas = [f"w{i}" for i in range(100)]
    tokens = content(lemmas)
    known = set(lemmas[:70])
    s = score_content(tokens, known, set())
    assert s.comprehension_pct < 80.0
    assert s.recommended_mode == "too_hard"


# ── learning words count as 50% known ────────────────────────────────────────

def test_learning_words_half_credit():
    # 2 words: 1 learning, 1 unknown. Learning counts 50%.
    # effective_known = (2 - 1) - 0.5*1 = 0.5. comprehension = 0.5/2 * 100 = 25%
    tokens = content(["word_l", "word_u"])
    s = score_content(tokens, set(), {"word_l"})
    assert s.comprehension_pct == pytest.approx(25.0)
    assert s.learning_count == 1
    assert s.unknown_count == 1


def test_all_learning_gives_half():
    # 4 words all learning: effective_known = 4 - 0 - 4*0.5 = 2. comp = 50%
    lemmas = ["a", "b", "c", "d"]
    tokens = content(lemmas)
    s = score_content(tokens, set(), set(lemmas))
    assert s.comprehension_pct == pytest.approx(50.0)


# ── top_unknown_lemmas ────────────────────────────────────────────────────────

def test_top_unknowns_sorted_by_frequency():
    # Lower freq rank = more common = should appear first
    tokens = content(["rare", "common", "medium"], freqs=[5000, 100, 1000])
    s = score_content(tokens, set(), set())
    assert s.top_unknown_lemmas == ["common", "medium", "rare"]


def test_top_unknowns_unranked_go_last():
    tokens = content(["unranked", "ranked"], freqs=[None, 500])
    s = score_content(tokens, set(), set())
    assert s.top_unknown_lemmas[0] == "ranked"
    assert s.top_unknown_lemmas[-1] == "unranked"


def test_top_unknowns_capped_at_10():
    lemmas = [f"w{i}" for i in range(20)]
    tokens = content(lemmas, freqs=list(range(1, 21)))
    s = score_content(tokens, set(), set())
    assert len(s.top_unknown_lemmas) == 10


def test_top_unknowns_deduplicated():
    # Duplicate lemmas in tokens — should only appear once in top_unknown
    tokens = [
        make_token("食べる", freq=100),
        make_token("食べる", freq=100),  # duplicate
        make_token("飲む", freq=200),
    ]
    s = score_content(tokens, set(), set())
    assert s.top_unknown_lemmas.count("食べる") == 1


def test_top_unknowns_excludes_known_and_learning():
    tokens = content(["known", "learning", "unknown"], freqs=[100, 200, 300])
    s = score_content(tokens, {"known"}, {"learning"})
    assert "known" not in s.top_unknown_lemmas
    assert "learning" not in s.top_unknown_lemmas
    assert "unknown" in s.top_unknown_lemmas


# ── difficulty_score ─────────────────────────────────────────────────────────

def test_difficulty_score_all_known_is_near_zero():
    lemmas = ["a", "b", "c"]
    tokens = content(lemmas)
    s = score_content(tokens, set(lemmas), set())
    assert s.difficulty_score >= 0.0
    assert s.difficulty_score < 0.05


def test_difficulty_score_all_unknown_is_less_than_one():
    lemmas = [f"w{i}" for i in range(10)]
    tokens = content(lemmas)
    s = score_content(tokens, set(), set())
    assert 0.0 <= s.difficulty_score <= 1.0


def test_difficulty_score_increases_with_unknowns():
    lemmas = [f"w{i}" for i in range(10)]
    tokens = content(lemmas)
    s_all_known = score_content(tokens, set(lemmas), set())
    s_all_unknown = score_content(tokens, set(), set())
    assert s_all_unknown.difficulty_score > s_all_known.difficulty_score


def test_frequency_penalty_for_common_unknowns():
    # Top-2000 unknowns should increase difficulty more than rare unknowns
    tokens_common = content(["common"], freqs=[500])
    tokens_rare = content(["rare"], freqs=[50000])
    s_common = score_content(tokens_common, set(), set())
    s_rare = score_content(tokens_rare, set(), set())
    # Both have 100% unknown, but common should be harder
    assert s_common.difficulty_score >= s_rare.difficulty_score


# ── comprehension_pct rounding ────────────────────────────────────────────────

def test_comprehension_pct_rounded_to_1_decimal():
    lemmas = [f"w{i}" for i in range(3)]
    tokens = content(lemmas)
    known = {lemmas[0]}
    s = score_content(tokens, known, set())
    # 1/3 known = 33.3...% — should be rounded to 1 decimal place
    assert isinstance(s.comprehension_pct, float)
    pct_str = str(s.comprehension_pct)
    decimal_part = pct_str.split(".")[-1] if "." in pct_str else "0"
    assert len(decimal_part) <= 1, f"Expected at most 1 decimal place, got {s.comprehension_pct}"


# ── mixed content and function words ─────────────────────────────────────────

def test_function_words_excluded_from_score():
    # 1 content word (known) + 5 function words → 100% comprehension
    tokens = content(["word"]) + func_tokens(["は", "が", "を", "で", "に"])
    s = score_content(tokens, {"word"}, set())
    assert s.comprehension_pct == 100.0
    assert s.total_content_words == 1
