"""
Property-based tests for the English tokenizer.

These assert invariants that every example test in test_correctness_en.py
implies but does not enumerate — e.g. "every irregular verb form, no matter
which one we generate, must lemmatize to its canonical lemma." Hypothesis
shrinks failures to a minimal example so regressions are easy to diagnose.
"""

from __future__ import annotations

import sys
import pathlib

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))

from hypothesis import given, strategies as st, settings, HealthCheck

from src.tokenizer_en import (
    EnglishTokenizer,
    lemmatize,
    _IRREGULAR_VERBS,
    _IRREGULAR_NOUNS,
    _FUNCTION_WORDS,
)

_tok = EnglishTokenizer()


# ── Irregular tables: every form lemmatizes to its declared canonical lemma ──


@given(st.sampled_from(sorted(_IRREGULAR_VERBS.keys())))
def test_every_irregular_verb_form_lemmatizes(form):
    assert lemmatize(form) == _IRREGULAR_VERBS[form]


@given(st.sampled_from(sorted(_IRREGULAR_NOUNS.keys())))
def test_every_irregular_noun_plural_lemmatizes(form):
    assert lemmatize(form) == _IRREGULAR_NOUNS[form]


@given(st.sampled_from(sorted(_FUNCTION_WORDS)))
def test_function_word_is_never_content(word):
    toks = _tok.tokenize(word).tokens
    # The single-word input must produce a non-content token.
    content_flags = [t.is_content_word for t in toks if t.pos != "punct"]
    assert content_flags == [False] or content_flags == []


# ── Tokenization invariants over arbitrary ASCII text ────────────────────────

text_strategy = st.text(
    alphabet=st.characters(whitelist_categories=("Ll", "Lu", "Nd", "Zs", "Po")),
    min_size=0,
    max_size=400,
)


@given(text_strategy)
@settings(max_examples=200, suppress_health_check=[HealthCheck.too_slow])
def test_tokens_preserve_alphabetic_content(text):
    """The concatenation of token surfaces preserves every alphabetic
    character from the input (whitespace and punctuation are allowed to
    drop, and letter↔digit transitions are allowed to split tokens, but
    no letter may vanish). Catches regressions where the tokenizer
    silently swallows input."""
    toks = _tok.tokenize(text).tokens
    in_letters = [c.lower() for c in text if c.isalpha()]
    out_letters = [c.lower() for c in "".join(t.surface for t in toks) if c.isalpha()]
    assert in_letters == out_letters, f"letters dropped: in={in_letters} out={out_letters}"


@given(text_strategy)
@settings(max_examples=200, suppress_health_check=[HealthCheck.too_slow])
def test_lemmatize_idempotent_on_function_words(text):
    """Lemmatizing a function word must return the function word; this is the
    only round-trip we can guarantee because lemmas are themselves words."""
    for w in text.lower().split():
        if w in _FUNCTION_WORDS:
            assert lemmatize(w) == w


@given(st.text(alphabet=st.characters(whitelist_categories=("Ll", "Lu")), min_size=1, max_size=30))
def test_lemmatize_returns_non_empty(word):
    """For any non-empty alphabetic input, lemmatize never returns empty."""
    result = lemmatize(word)
    assert isinstance(result, str)
    assert len(result) > 0


# ── POS classification stability ─────────────────────────────────────────────


@given(st.sampled_from(sorted(_IRREGULAR_VERBS.keys())))
def test_irregular_verb_classifies_as_verb_or_aux(form):
    toks = _tok.tokenize(form).tokens
    if not toks:
        return
    pos = toks[0].pos
    # "is", "was" etc. live in both _AUXILIARIES and _IRREGULAR_VERBS — both
    # are acceptable POS classifications.
    assert pos in {"verb", "aux"}, f"{form} classified as {pos}"
