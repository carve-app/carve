"""
Correctness suite for the multilingual (Latin-script + Vietnamese) tokenizer.

Pedagogical perspective:
  - Lemmatize inflected forms so a learner sees one card per word (gatos→gato).
  - Flag function words (articles, prepositions, pronouns) as non-content so
    flashcards focus on real vocabulary.
  - Handle diacritics, apostrophes and hyphens without producing garbage tokens.

Lemmatization quality depends on `simplemma`; tests that assert specific lemmas
are skipped when it is not installed (the tokenizer still works, returning the
lowercased surface as the lemma — the function-word logic is independent of it).
"""
from __future__ import annotations

import sys
import pathlib

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))

import pytest

from carve_nlp.tokenizer_latin import (
    LatinTokenizer,
    lemmatize_word,
    is_function_word,
    _SIMPLEMMA_AVAILABLE,
    SUPPORTED_LANGUAGES,
)

requires_simplemma = pytest.mark.skipif(
    not _SIMPLEMMA_AVAILABLE, reason="simplemma not installed"
)


def lemmas(text: str, lang: str) -> list[str]:
    tok = LatinTokenizer(lang)
    return [t.lemma for t in tok.tokenize(text).tokens if t.pos != "punct"]


def content_lemmas(text: str, lang: str) -> list[str]:
    tok = LatinTokenizer(lang)
    return [t.lemma for t in tok.tokenize(text).tokens if t.is_content_word]


# ── Tokenization (independent of simplemma) ─────────────────────────────────────

class TestTokenization:
    def test_splits_words_and_punctuation(self):
        tok = LatinTokenizer("es")
        toks = tok.tokenize("Hola, mundo!")
        surfaces = [t.surface for t in toks.tokens]
        assert surfaces == ["Hola", ",", "mundo", "!"]

    def test_keeps_diacritics(self):
        tok = LatinTokenizer("fr")
        toks = tok.tokenize("café crème")
        assert [t.surface for t in toks.tokens] == ["café", "crème"]

    def test_handles_apostrophe_and_hyphen(self):
        tok = LatinTokenizer("fr")
        surfaces = [t.surface for t in tok.tokenize("l'avant-garde").tokens]
        assert surfaces == ["l'avant-garde"]

    def test_numbers_are_not_content(self):
        tok = LatinTokenizer("de")
        toks = {t.surface: t for t in tok.tokenize("3 Häuser").tokens}
        assert toks["3"].is_content_word is False
        assert toks["3"].pos == "num"

    def test_every_supported_language_constructs(self):
        for lang in SUPPORTED_LANGUAGES:
            assert LatinTokenizer(lang).tokenize("test 1").tokens

    def test_unsupported_language_raises(self):
        with pytest.raises(ValueError):
            LatinTokenizer("xx")


# ── Function-word detection (independent of simplemma) ──────────────────────────

class TestFunctionWords:
    def test_spanish_articles_are_function(self):
        for w in ("el", "la", "los", "un", "de", "y", "que"):
            assert is_function_word(w, "es")

    def test_german_articles_are_function(self):
        for w in ("der", "die", "das", "und", "in", "nicht"):
            assert is_function_word(w, "de")

    def test_french_articles_are_function(self):
        for w in ("le", "la", "les", "de", "et", "ne", "pas"):
            assert is_function_word(w, "fr")

    def test_content_words_are_not_function(self):
        assert not is_function_word("gato", "es")
        assert not is_function_word("maison", "fr")
        assert not is_function_word("haus", "de")

    def test_function_words_flagged_non_content(self):
        # "el gato" → el=function, gato=content
        tok = LatinTokenizer("es")
        toks = {t.surface: t for t in tok.tokenize("el gato").tokens}
        assert toks["el"].is_content_word is False
        assert toks["gato"].is_content_word is True


# ── Lemmatization (needs simplemma) ─────────────────────────────────────────────

class TestSpanishLemmas:
    @requires_simplemma
    def test_plural_noun(self):
        assert lemmatize_word("gatos", "es") == "gato"

    @requires_simplemma
    def test_conjugated_verb(self):
        assert lemmatize_word("corriendo", "es") == "correr"

    @requires_simplemma
    def test_sentence_content_lemmas(self):
        # "Los gatos corren rápido" → gato, correr, rápido (articles dropped)
        got = content_lemmas("Los gatos corren rápido", "es")
        assert "gato" in got
        assert "correr" in got


class TestGermanLemmas:
    @requires_simplemma
    def test_plural_noun(self):
        assert lemmatize_word("Häuser", "de") == "Haus"

    @requires_simplemma
    def test_conjugated_verb(self):
        assert lemmatize_word("gegangen", "de") == "gehen"


class TestFrenchLemmas:
    @requires_simplemma
    def test_conjugated_verb(self):
        assert lemmatize_word("mangé", "fr") == "manger"

    @requires_simplemma
    def test_plural_noun(self):
        assert lemmatize_word("chevaux", "fr") == "cheval"


class TestItalianPortuguese:
    @requires_simplemma
    def test_italian_plural(self):
        assert lemmatize_word("gatti", "it") == "gatto"

    @requires_simplemma
    def test_portuguese_plural(self):
        assert lemmatize_word("casas", "pt") == "casa"


class TestVietnamese:
    def test_identity_lemma(self):
        # Vietnamese is isolating: lemma == lowercased surface.
        assert lemmatize_word("Nhà", "vi") == "nhà"

    def test_function_words(self):
        assert is_function_word("và", "vi")
        assert not is_function_word("nhà", "vi")

    def test_tokenizes_and_marks_content(self):
        tok = LatinTokenizer("vi")
        toks = {t.surface: t for t in tok.tokenize("tôi yêu nhà").tokens}
        # tôi = pronoun (function), nhà = noun (content)
        assert toks["tôi"].is_content_word is False
        assert toks["nhà"].is_content_word is True


class TestFrequency:
    def test_content_words_get_a_rank_when_wordfreq_available(self):
        from carve_nlp.tokenizer_latin import _WORDFREQ_AVAILABLE
        if not _WORDFREQ_AVAILABLE:
            pytest.skip("wordfreq not installed")
        tok = LatinTokenizer("es")
        toks = {t.surface: t for t in tok.tokenize("casa").tokens}
        assert toks["casa"].frequency_rank is not None
        assert toks["casa"].frequency_rank >= 1
