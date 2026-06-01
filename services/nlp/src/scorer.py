"""
Content difficulty and comprehension scorer.

Implements the i+1 scoring from the architecture design:
  - flow_read:    comprehension >= 98% (barely need lookups)
  - mining_read:  90-97% (sweet spot for vocabulary acquisition)
  - study_read:   80-89% (effortful, still productive)
  - too_hard:     < 80%  (comprehension too low for acquisition)
"""

from __future__ import annotations

from dataclasses import dataclass

from .tokenizer import Token


@dataclass
class ScoredToken:
    token: Token
    user_status: str   # 'known', 'learning', 'unknown'


@dataclass
class ContentScore:
    comprehension_pct: float       # 0-100
    difficulty_score: float        # 0-1 (0=easy)
    total_content_words: int
    unknown_count: int
    learning_count: int
    recommended_mode: str          # flow_read | mining_read | study_read | too_hard
    top_unknown_lemmas: list[str]  # sorted by frequency rank (most common first)


def score_content(
    tokens: list[Token],
    known_lemmas: set[str],
    learning_lemmas: set[str],
) -> ContentScore:
    """
    Compute comprehension score given tokenized content and user vocabulary.

    Only content words (nouns, verbs, adjectives, adverbs) count toward the
    score — function words (particles, auxiliaries) are excluded because they
    are acquired implicitly and shouldn't penalize comprehension %.
    """
    content_tokens = [t for t in tokens if t.is_content_word]
    total = len(content_tokens)
    if total == 0:
        return ContentScore(
            comprehension_pct=100.0,
            difficulty_score=0.0,
            total_content_words=0,
            unknown_count=0,
            learning_count=0,
            recommended_mode="flow_read",
            top_unknown_lemmas=[],
        )

    unknown = [t for t in content_tokens if t.lemma not in known_lemmas and t.lemma not in learning_lemmas]
    learning = [t for t in content_tokens if t.lemma in learning_lemmas]

    # Learning words count as 50% known (they're partially internalized)
    effective_known = (total - len(unknown)) - len(learning) * 0.5
    comprehension_pct = (effective_known / total) * 100

    # Frequency penalty: unknowns in top 2000 words are more concerning
    freq_penalty = 0.0
    for t in unknown:
        if t.frequency_rank and t.frequency_rank <= 2000:
            freq_penalty += (1 / t.frequency_rank) * 100
    freq_penalty = min(freq_penalty / max(total, 1), 0.3)

    difficulty_score = (1 - comprehension_pct / 100) * 0.7 + freq_penalty * 0.3

    mode = (
        "flow_read" if comprehension_pct >= 98 else
        "mining_read" if comprehension_pct >= 90 else
        "study_read" if comprehension_pct >= 80 else
        "too_hard"
    )

    # Top unknown words by frequency (most common unknowns first)
    unknown_sorted = sorted(
        unknown,
        key=lambda t: t.frequency_rank if t.frequency_rank else 999_999,
    )
    # Deduplicate by lemma while preserving order
    seen: set[str] = set()
    top_unknown: list[str] = []
    for t in unknown_sorted:
        if t.lemma not in seen:
            seen.add(t.lemma)
            top_unknown.append(t.lemma)
        if len(top_unknown) >= 10:
            break

    return ContentScore(
        comprehension_pct=round(comprehension_pct, 1),
        difficulty_score=round(difficulty_score, 3),
        total_content_words=total,
        unknown_count=len(unknown),
        learning_count=len(learning),
        recommended_mode=mode,
        top_unknown_lemmas=top_unknown,
    )


# ── i+1 candidate selection ──────────────────────────────────────────────────

@dataclass
class CandidateScore:
    index: int                     # position in input candidates list
    text: str
    comprehension_pct: float
    content_word_count: int
    unknown_count: int
    contains_target: bool
    fit_score: float               # 0-1, higher = better i+1 fit


def _fit_score(comprehension_pct: float, content_word_count: int) -> float:
    """
    Score how close a sentence is to the i+1 ideal.

    Optimum is ~93% comprehension with 8-20 content words. Too easy (>=98%,
    flow-read) or too hard (<70%) get penalised; very short (<4) or very long
    (>30) sentences are penalised because they make poor mining cards.
    """
    if comprehension_pct < 70.0 or comprehension_pct > 100.0:
        comp = 0.0
    elif comprehension_pct <= 93.0:
        comp = (comprehension_pct - 70.0) / 23.0
    else:
        comp = max(0.0, 1.0 - (comprehension_pct - 93.0) / 7.0)

    if content_word_count <= 0:
        length = 0.0
    elif content_word_count < 4:
        length = content_word_count / 4.0
    elif content_word_count <= 20:
        length = 1.0
    elif content_word_count <= 30:
        length = 1.0 - (content_word_count - 20) / 20.0
    else:
        length = 0.5

    return round(comp * 0.75 + length * 0.25, 4)


def select_best_sentence(
    scored_candidates: list[tuple[str, list[Token]]],
    target_lemma: str,
    known_lemmas: set[str],
    learning_lemmas: set[str],
) -> tuple[CandidateScore | None, list[CandidateScore]]:
    """
    Pick the candidate that's the best i+1 mining card for `target_lemma`.

    `scored_candidates` is (sentence_text, tokens) pairs — the caller tokenizes
    in the right language. Returns (best, all_ranked). Candidates that don't
    contain the target lemma get a heavy penalty but stay in the list as a
    fallback for when nothing contains the lemma (e.g. tokenizer mis-segments).
    """
    ranked: list[CandidateScore] = []
    for i, (text, tokens) in enumerate(scored_candidates):
        score = score_content(tokens, known_lemmas, learning_lemmas)
        contains = any(
            t.is_content_word and t.lemma == target_lemma for t in tokens
        )
        fit = _fit_score(score.comprehension_pct, score.total_content_words)
        if not contains:
            fit *= 0.05
        ranked.append(CandidateScore(
            index=i,
            text=text,
            comprehension_pct=score.comprehension_pct,
            content_word_count=score.total_content_words,
            unknown_count=score.unknown_count,
            contains_target=contains,
            fit_score=round(fit, 4),
        ))

    if not ranked:
        return None, []

    ranked_sorted = sorted(
        ranked,
        key=lambda c: (-c.fit_score, c.content_word_count, c.index),
    )
    return ranked_sorted[0], ranked_sorted
