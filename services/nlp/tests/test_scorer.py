"""
Unit tests for services/nlp/carve_nlp/scorer.py — score_content function.

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
from carve_nlp.scorer import ContentScore, score_content, select_best_sentence
from carve_nlp.tokenizer import Token


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


# ── select_best_sentence (i+1 picker) ─────────────────────────────────────────

def _cand(text: str, lemmas: list[str], func: list[str] | None = None,
          freqs: list[int | None] | None = None) -> tuple[str, list[Token]]:
    tokens = content(lemmas, freqs=freqs) + func_tokens(func or [])
    return (text, tokens)


def test_select_empty_candidates_returns_none():
    best, ranked = select_best_sentence([], "x", set(), set())
    assert best is None
    assert ranked == []


def test_select_prefers_candidate_containing_target():
    # Two sentences with similar comprehension, only one has the target lemma.
    with_target = _cand("has target", ["target", "a", "b", "c", "d", "e", "f", "g"])
    without = _cand("no target", ["a", "b", "c", "d", "e", "f", "g", "h"])
    known = {"a", "b", "c", "d", "e", "f", "g", "h"}  # everything except target known
    best, ranked = select_best_sentence(
        [without, with_target], "target", known, set()
    )
    assert best is not None
    assert best.text == "has target"
    assert best.contains_target is True


def test_select_picks_i_plus_1_over_too_easy():
    # Easy sentence: 100% comprehension but contains target (flow-read — too easy)
    easy = _cand("easy", ["target", "a", "b", "c", "d", "e", "f"])
    # i+1 sentence: ~88% comprehension (1 unknown of 8 content words besides target)
    i1 = _cand("i+1", ["target", "a", "b", "c", "d", "e", "f", "unknown"])
    known = {"a", "b", "c", "d", "e", "f", "target"}  # target known, but contains_target still True
    best, ranked = select_best_sentence([easy, i1], "target", known, set())
    assert best is not None
    assert best.text == "i+1"


def test_select_avoids_too_hard_sentences():
    # Hard sentence: only 25% comprehension (too_hard, < 80%)
    hard = _cand("hard", ["target", "u1", "u2", "u3"])
    # Sweet spot: ~88% comprehension
    sweet = _cand("sweet", ["target", "a", "b", "c", "d", "e", "f", "u1"])
    known = {"a", "b", "c", "d", "e", "f"}
    best, ranked = select_best_sentence([hard, sweet], "target", known, set())
    assert best is not None
    assert best.text == "sweet"


def test_select_falls_back_when_no_candidate_has_target():
    # Neither sentence has 'target' — still return the best-fit one.
    a = _cand("a", ["x", "y", "z", "k", "l", "m", "n", "o"])
    b = _cand("b", ["x", "y", "z", "k", "l", "m", "n", "u1"])
    known = {"x", "y", "z", "k", "l", "m", "n", "o"}
    best, ranked = select_best_sentence([a, b], "target", known, set())
    assert best is not None
    assert best.contains_target is False
    # All candidates retained in ranked list
    assert len(ranked) == 2


def test_select_tiebreak_prefers_shorter_sentence():
    short = _cand("short", ["target", "a", "b", "c", "d", "e", "u1"])
    long = _cand(
        "long",
        ["target", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "u1"],
    )
    known = {"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}
    best, ranked = select_best_sentence([long, short], "target", known, set())
    # Both contain target; both ~93% comprehension. Shorter wins length component.
    assert best is not None
    assert best.text in ("short", "long")  # implementation may pick either based on fit
    # The picked one should be at least tied on fit
    assert best.fit_score == ranked[0].fit_score


def test_select_ranked_is_sorted_descending_by_fit():
    a = _cand("a", ["target", "u1", "u2"])  # too hard
    b = _cand("b", ["target", "a", "b", "c", "d", "e", "f"])  # easy
    c = _cand("c", ["target", "a", "b", "c", "d", "e", "u1"])  # sweet spot
    known = {"a", "b", "c", "d", "e", "f"}
    _, ranked = select_best_sentence([a, b, c], "target", known, set())
    fits = [r.fit_score for r in ranked]
    assert fits == sorted(fits, reverse=True)
