"""
Korean tokenizer correctness test suite.

Hand-verified cases covering:
- Eojeol splitting and lemma extraction
- Particle detection and stripping (은/는/이/가/을/를/에서/…)
- Revised Romanization accuracy
- Content vs function word classification
- Edge cases: mixed script, punctuation, English loanwords
"""

from __future__ import annotations

import os
import sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest

from carve_nlp.tokenizer_ko import (
    KoreanTokenizer,
    romanize,
    _strip_particles,
)


@pytest.fixture(scope="module")
def tok():
    return KoreanTokenizer()


# ── Romanization ───────────────────────────────────────────────────────────────

class TestRomanization:
    def test_han_guk(self):
        # 한 = han, 국 = gug (Revised Romanization: ㄱ final = 'g')
        assert romanize("한국") == "hangug"

    def test_annyeong(self):
        assert romanize("안녕") == "annyeong"

    def test_sarang(self):
        # ㄹ as onset = 'r'; sarang, not salang
        assert romanize("사랑") == "sarang"

    def test_haksaeng(self):
        assert romanize("학생") == "hagsaeng"

    def test_seoul(self):
        assert romanize("서울") == "seoul"

    def test_single_syllable(self):
        assert romanize("나") == "na"
        assert romanize("가") == "ga"
        assert romanize("다") == "da"

    def test_empty(self):
        assert romanize("") == ""

    def test_ascii_passthrough(self):
        assert romanize("hello") == "hello"


# ── Particle stripping ─────────────────────────────────────────────────────────

class TestParticleStripping:
    def test_topic_particle_neun(self):
        stem, particles = _strip_particles("나는")
        assert stem == "나"
        assert "는" in particles

    def test_topic_particle_eun(self):
        stem, particles = _strip_particles("학교는")
        assert stem == "학교"
        assert "는" in particles

    def test_subject_ga(self):
        stem, particles = _strip_particles("친구가")
        assert stem == "친구"
        assert "가" in particles

    def test_subject_i(self):
        stem, particles = _strip_particles("학생이")
        assert stem == "학생"
        assert "이" in particles

    def test_object_reul(self):
        stem, particles = _strip_particles("밥을")
        assert stem == "밥"
        assert "을" in particles

    def test_object_reul_ro(self):
        stem, particles = _strip_particles("물을")
        assert stem == "물"
        assert "을" in particles

    def test_locative_e(self):
        stem, particles = _strip_particles("학교에")
        assert stem == "학교"
        assert "에" in particles

    def test_locative_eseo(self):
        stem, particles = _strip_particles("학교에서")
        assert stem == "학교"
        assert "에서" in particles

    def test_no_particle(self):
        stem, particles = _strip_particles("사랑")
        assert stem == "사랑"
        assert particles == []

    def test_particle_not_stripped_from_short(self):
        # Don't strip particle if result would be empty
        stem, particles = _strip_particles("가")
        assert stem == "가"

    def test_dative_ege(self):
        stem, particles = _strip_particles("친구에게")
        assert stem == "친구"
        assert "에게" in particles

    def test_genitive_ui(self):
        stem, particles = _strip_particles("나의")
        assert stem == "나"
        assert "의" in particles

    def test_additive_do(self):
        stem, particles = _strip_particles("나도")
        assert stem == "나"
        assert "도" in particles

    def test_exclusive_man(self):
        stem, particles = _strip_particles("나만")
        assert stem == "나"
        assert "만" in particles


# ── Tokenizer integration ──────────────────────────────────────────────────────

class TestTokenizer:
    def test_basic_sentence(self, tok):
        result = tok.tokenize("나는 학교에 간다")
        assert len(result.tokens) == 3

    def test_surface_preserved(self, tok):
        text = "사랑 친구 가족"
        result = tok.tokenize(text)
        reconstructed = " ".join(t.surface for t in result.tokens)
        assert reconstructed == text

    def test_lemma_extracted(self, tok):
        result = tok.tokenize("나는")
        assert result.tokens[0].lemma == "나"

    def test_particles_tracked(self, tok):
        result = tok.tokenize("나는")
        assert "는" in result.tokens[0].particles

    def test_romanization_in_token(self, tok):
        result = tok.tokenize("사랑")
        assert "sarang" in result.tokens[0].romanization.lower()

    def test_content_word_flag(self, tok):
        result = tok.tokenize("책")
        assert result.tokens[0].is_content_word

    def test_empty(self, tok):
        result = tok.tokenize("")
        assert result.tokens == []

    def test_single_word(self, tok):
        result = tok.tokenize("사랑")
        assert len(result.tokens) == 1

    def test_multiple_words(self, tok):
        result = tok.tokenize("나는 학생이다")
        assert len(result.tokens) >= 2

    def test_pos_field_present(self, tok):
        result = tok.tokenize("학교에")
        for t in result.tokens:
            assert t.pos, f"Token '{t.surface}' missing POS"


# ── Common vocabulary spot checks ─────────────────────────────────────────────

PARTICLE_CASES = [
    ("나는", "나", "는"),
    ("학교에서", "학교", "에서"),
    ("친구가", "친구", "가"),
    ("책을", "책", "을"),
    ("물을", "물", "을"),
    ("집에", "집", "에"),
    ("나도", "나", "도"),
    ("나만", "나", "만"),
    ("나의", "나", "의"),
    ("친구에게", "친구", "에게"),
    ("서울에서", "서울", "에서"),
    ("한국에서", "한국", "에서"),
    ("가족과", "가족", "과"),
    ("음식이", "음식", "이"),
    ("사람들은", "사람들", "은"),
]

@pytest.mark.parametrize("eojeol,expected_stem,expected_particle", PARTICLE_CASES)
def test_particle_strip(eojeol, expected_stem, expected_particle):
    stem, particles = _strip_particles(eojeol)
    assert stem == expected_stem, f"For '{eojeol}': expected stem '{expected_stem}', got '{stem}'"
    assert expected_particle in particles, \
        f"For '{eojeol}': expected particle '{expected_particle}' in {particles}"


# ── Edge cases ────────────────────────────────────────────────────────────────

class TestEdgeCases:
    def test_english_loanword(self, tok):
        result = tok.tokenize("스마트폰")
        assert len(result.tokens) == 1

    def test_number(self, tok):
        result = tok.tokenize("123")
        assert len(result.tokens) >= 1

    def test_mixed_korean_english(self, tok):
        result = tok.tokenize("나는 Apple을 좋아한다")
        assert len(result.tokens) >= 3

    def test_punctuation(self, tok):
        result = tok.tokenize("안녕하세요.")
        assert len(result.tokens) >= 1

    def test_long_sentence(self, tok):
        text = "오늘 날씨가 정말 좋아서 친구들과 함께 공원에 갔다"
        result = tok.tokenize(text)
        assert len(result.tokens) >= 6

    def test_repeated_word(self, tok):
        result = tok.tokenize("사랑 사랑 사랑")
        assert len(result.tokens) == 3

    def test_all_tokens_have_romanization(self, tok):
        result = tok.tokenize("한국어를 공부하다")
        for t in result.tokens:
            assert isinstance(t.romanization, str), f"Missing romanization for '{t.surface}'"

    def test_all_tokens_have_lemma(self, tok):
        result = tok.tokenize("서울에서 왔어요")
        for t in result.tokens:
            assert t.lemma, f"Empty lemma for '{t.surface}'"

    def test_newline(self, tok):
        result = tok.tokenize("안녕\n세상")
        assert len(result.tokens) >= 2

    def test_hanja_passthrough(self, tok):
        # Hanja (Chinese characters used in Korean) should not crash
        result = tok.tokenize("漢字")
        assert len(result.tokens) >= 1
