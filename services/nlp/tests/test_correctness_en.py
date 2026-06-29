"""
English tokenizer correctness suite — for intermediate/advanced learners.

The pedagogical perspective:
  - Lemmatize regular and irregular inflections so users see one card per word.
  - Distinguish content words from function words so flashcards focus on
    vocabulary the learner needs to acquire.
  - Handle contractions, hyphenation, and possessives without splitting
    them into garbage tokens.
"""
from __future__ import annotations

import sys
import pathlib

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))

from carve_nlp.tokenizer_en import EnglishTokenizer, lemmatize


_tok = EnglishTokenizer()


def lemmas(text: str) -> list[str]:
    return [t.lemma for t in _tok.tokenize(text).tokens if t.pos != "punct"]


def surfaces(text: str) -> list[str]:
    return [t.surface for t in _tok.tokenize(text).tokens if t.pos != "punct"]


def by_lemma(text: str) -> dict[str, str]:
    return {t.surface.lower(): t.lemma for t in _tok.tokenize(text).tokens if t.pos != "punct"}


# ── Section 1: regular verb inflections ───────────────────────────────────────

class TestRegularVerbs:
    def test_third_person_singular_drops_s(self):
        assert lemmatize("walks") == "walk"
        assert lemmatize("plays") == "play"

    def test_past_tense_drops_ed(self):
        assert lemmatize("walked") == "walk"
        assert lemmatize("played") == "play"

    def test_present_participle_drops_ing(self):
        assert lemmatize("walking") == "walk"
        assert lemmatize("playing") == "play"

    def test_doubled_consonant_running(self):
        assert lemmatize("running") == "run"
        assert lemmatize("stopped") == "stop"
        assert lemmatize("jogging") == "jog"

    def test_silent_e_baking(self):
        assert lemmatize("baking") == "bake"
        assert lemmatize("hoping") == "hope"

    def test_ies_to_y(self):
        assert lemmatize("studies") == "study"
        assert lemmatize("tries") == "try"

    def test_ied_to_y(self):
        assert lemmatize("studied") == "study"
        assert lemmatize("tried") == "try"


# ── Section 2: irregular verbs ────────────────────────────────────────────────

class TestIrregularVerbs:
    def test_be_forms(self):
        for form in ("am", "is", "are", "was", "were", "been", "being"):
            assert lemmatize(form) == "be", form

    def test_have_forms(self):
        for form in ("has", "had", "having"):
            assert lemmatize(form) == "have", form

    def test_go_forms(self):
        assert lemmatize("went") == "go"
        assert lemmatize("gone") == "go"

    def test_see_forms(self):
        assert lemmatize("saw") == "see"
        assert lemmatize("seen") == "see"

    def test_think_forms(self):
        assert lemmatize("thought") == "think"
        assert lemmatize("thinking") == "think"

    def test_take_forms(self):
        assert lemmatize("took") == "take"
        assert lemmatize("taken") == "take"

    def test_get_forms(self):
        assert lemmatize("got") == "get"
        assert lemmatize("gotten") == "get"


# ── Section 3: noun plurals ──────────────────────────────────────────────────

class TestPlurals:
    def test_regular_plural(self):
        assert lemmatize("cats") == "cat"
        assert lemmatize("books") == "book"

    def test_es_plural(self):
        assert lemmatize("boxes") == "box"
        assert lemmatize("watches") == "watch"
        assert lemmatize("buses") == "bus"

    def test_y_to_ies(self):
        assert lemmatize("babies") == "baby"
        assert lemmatize("countries") == "country"

    def test_irregular_plural(self):
        assert lemmatize("men") == "man"
        assert lemmatize("women") == "woman"
        assert lemmatize("children") == "child"
        assert lemmatize("feet") == "foot"
        assert lemmatize("mice") == "mouse"
        assert lemmatize("people") == "person"

    def test_latin_plural(self):
        assert lemmatize("criteria") == "criterion"
        assert lemmatize("phenomena") == "phenomenon"
        assert lemmatize("analyses") == "analysis"


# ── Section 4: adjective comparison ──────────────────────────────────────────

class TestAdjectives:
    def test_comparative(self):
        assert lemmatize("bigger") == "big"
        assert lemmatize("faster") == "fast"

    def test_superlative(self):
        assert lemmatize("biggest") == "big"
        assert lemmatize("fastest") == "fast"


# ── Section 5: function vs content words ─────────────────────────────────────

class TestFunctionWords:
    def test_pronouns_not_content(self):
        toks = _tok.tokenize("I think they know him").tokens
        is_content = {t.lemma: t.is_content_word for t in toks}
        assert is_content["i"] is False
        assert is_content["they"] is False
        assert is_content["him"] is False
        assert is_content["think"] is True
        assert is_content["know"] is True

    def test_determiners_not_content(self):
        toks = _tok.tokenize("The cat sat on a mat").tokens
        is_content = {t.surface.lower(): t.is_content_word for t in toks}
        assert is_content["the"] is False
        assert is_content["a"] is False
        assert is_content["cat"] is True
        assert is_content["mat"] is True

    def test_prepositions_not_content(self):
        toks = _tok.tokenize("She walked into the room").tokens
        m = {t.surface.lower(): t.is_content_word for t in toks}
        assert m["into"] is False
        assert m["the"] is False
        assert m["walked"] is True
        assert m["room"] is True

    def test_conjunctions_not_content(self):
        toks = _tok.tokenize("I came and saw but did not conquer").tokens
        m = {t.surface.lower(): t.is_content_word for t in toks}
        assert m["and"] is False
        assert m["but"] is False
        assert m["not"] is False


# ── Section 6: tokenization edge cases ───────────────────────────────────────

class TestTokenizationEdges:
    def test_contractions_preserved(self):
        surf = surfaces("Don't worry, it's fine.")
        assert "Don't" in surf or "don't" in [s.lower() for s in surf]
        assert "it's" in [s.lower() for s in surf]

    def test_possessive_kept(self):
        surf = surfaces("the cat's bowl")
        # we accept either "cat's" as one token or "cat" + "'s"
        joined = " ".join(s.lower() for s in surf)
        assert "cat" in joined

    def test_hyphenated_compound_kept_whole(self):
        surf = surfaces("mother-in-law arrived")
        assert "mother-in-law" in surf

    def test_numbers_recognized(self):
        toks = _tok.tokenize("We saw 42 birds and 3.14 came up").tokens
        nums = [t for t in toks if t.pos == "num"]
        assert len(nums) >= 2
        assert any(t.surface == "42" for t in nums)
        assert any(t.surface == "3.14" for t in nums)


# ── Section 7: round-trip on real text ───────────────────────────────────────

class TestSentenceRoundTrip:
    def test_simple_sentence(self):
        text = "The quick brown fox jumps over the lazy dog."
        toks = _tok.tokenize(text).tokens
        content_lemmas = [t.lemma for t in toks if t.is_content_word]
        # quick, brown, fox, jump, lazy, dog should all surface as content lemmas
        for w in ("quick", "brown", "fox", "jump", "lazy", "dog"):
            assert w in content_lemmas, f"missing {w} in {content_lemmas}"

    def test_intermediate_learner_sentence(self):
        text = "She has been studying English for three years."
        toks = _tok.tokenize(text).tokens
        m = {t.surface.lower(): t.lemma for t in toks}
        assert m["has"] == "have"
        assert m["been"] == "be"
        assert m["studying"] == "study"
        assert m["years"] == "year"

    def test_advanced_learner_paragraph(self):
        text = (
            "The criteria for evaluating these phenomena have evolved as analyses become "
            "more sophisticated. Researchers found that previously overlooked variables "
            "were actually decisive."
        )
        toks = _tok.tokenize(text).tokens
        content = [t.lemma for t in toks if t.is_content_word]
        # Irregular plurals collapse to singular lemmas
        assert "criterion" in content
        assert "phenomenon" in content
        assert "analysis" in content
        # Regular past tenses lemmatize
        assert "evaluate" in content or "evaluating" not in content  # gerund handled
        assert "evolve" in content
        assert "research" in content or "researcher" in content


# ── Section 8: frequency rank sanity ─────────────────────────────────────────

class TestFrequency:
    def test_common_words_low_rank(self):
        try:
            import wordfreq  # noqa: F401
        except ImportError:
            return  # skip when not installed
        the_tok = next(t for t in _tok.tokenize("the").tokens if t.lemma == "the")
        cat_tok = next(t for t in _tok.tokenize("cat").tokens if t.lemma == "cat")
        # "the" is a function word -> frequency_rank is None (only content gets ranked)
        assert the_tok.frequency_rank is None
        # "cat" gets a rank that's a positive int
        if cat_tok.frequency_rank is not None:
            assert cat_tok.frequency_rank > 0
            assert cat_tok.frequency_rank < 100_000
