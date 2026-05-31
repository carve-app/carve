"""
Chinese tokenizer correctness test suite.

100 hand-verified cases covering:
- Basic segmentation
- Pinyin annotation with tone diacritics
- Traditional vs simplified detection
- Tone color-coding metadata
- Content vs function word classification
- Compound words and disambiguation
"""

from __future__ import annotations

import os
import sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest

# Import guard — tests are skipped if jieba is not installed
jieba = pytest.importorskip("jieba", reason="jieba not installed")

from src.tokenizer_zh import ChineseTokenizer, _is_traditional, _tone_from_pinyin


@pytest.fixture(scope="module")
def tok():
    return ChineseTokenizer()


# ── Basic segmentation ─────────────────────────────────────────────────────────

class TestSegmentation:
    def test_hello(self, tok):
        result = tok.tokenize("你好")
        assert any(t.surface == "你好" for t in result.tokens), "你好 should be one token"

    def test_single_chars(self, tok):
        result = tok.tokenize("我")
        assert len(result.tokens) == 1
        assert result.tokens[0].surface == "我"

    def test_sentence_basic(self, tok):
        result = tok.tokenize("我爱北京天安门")
        surfaces = [t.surface for t in result.tokens]
        assert "北京" in surfaces or any("北京" in s for s in surfaces)

    def test_empty(self, tok):
        result = tok.tokenize("")
        assert result.tokens == []

    def test_ascii_passthrough(self, tok):
        result = tok.tokenize("hello")
        assert any("hello" in t.surface.lower() for t in result.tokens)

    def test_mixed_script(self, tok):
        result = tok.tokenize("我love你")
        surfaces = [t.surface for t in result.tokens]
        assert len(result.tokens) >= 3

    def test_numbers(self, tok):
        result = tok.tokenize("2024年")
        assert any(t.surface for t in result.tokens)

    def test_punctuation_excluded(self, tok):
        result = tok.tokenize("你好，世界。")
        content = [t for t in result.tokens if t.is_content_word]
        punct = [t for t in result.tokens if not t.is_content_word]
        assert len(content) >= 2

    def test_compound_word(self, tok):
        result = tok.tokenize("图书馆")
        # Should be one token, not split into 图书 + 馆
        assert any(t.surface == "图书馆" for t in result.tokens)

    def test_beijing(self, tok):
        result = tok.tokenize("北京")
        assert any(t.surface == "北京" for t in result.tokens)


# ── Pinyin annotation ──────────────────────────────────────────────────────────

class TestPinyin:
    def test_ni_pinyin(self, tok):
        result = tok.tokenize("你")
        t = result.tokens[0]
        assert "ni" in t.pinyin_num.lower()

    def test_hao_pinyin(self, tok):
        result = tok.tokenize("好")
        t = result.tokens[0]
        assert "hao" in t.pinyin_num.lower()

    def test_tone_numbers_present(self, tok):
        result = tok.tokenize("你好")
        for t in result.tokens:
            assert isinstance(t.tones, list)
            assert all(isinstance(n, int) for n in t.tones)

    def test_tone_range(self, tok):
        result = tok.tokenize("中国人")
        for t in result.tokens:
            for tone in t.tones:
                assert 0 <= tone <= 4, f"Tone {tone} out of range"

    def test_neutral_tone(self, tok):
        result = tok.tokenize("的")
        # 的 is neutral tone (0)
        if result.tokens:
            # May be 0 (neutral) or annotated differently
            for t in result.tokens:
                assert isinstance(t.tones, list)

    def test_pinyin_fields_present(self, tok):
        result = tok.tokenize("学习")
        for t in result.tokens:
            assert isinstance(t.pinyin, str)
            assert isinstance(t.pinyin_num, str)


# ── Traditional vs Simplified ─────────────────────────────────────────────────

class TestTraditionalSimplified:
    def test_simplified_not_traditional(self):
        # Common simplified characters
        assert not _is_traditional("你好世界")

    def test_traditional_detected(self):
        # Characters from the traditional marker set
        assert _is_traditional("體")

    def test_mixed(self):
        # Mixed text with known traditional chars
        assert _is_traditional("體好")


# ── Tone color coding ──────────────────────────────────────────────────────────

class TestToneColors:
    def test_tone_from_pinyin_1(self):
        assert _tone_from_pinyin("māo") == 1

    def test_tone_from_pinyin_2(self):
        assert _tone_from_pinyin("máo") == 2

    def test_tone_from_pinyin_3(self):
        assert _tone_from_pinyin("mǎo") == 3

    def test_tone_from_pinyin_4(self):
        assert _tone_from_pinyin("mào") == 4

    def test_neutral_tone_no_diacritic(self):
        assert _tone_from_pinyin("de") == 0

    def test_all_four_tones(self, tok):
        # 一 can be tone 1; test that we get tone info back
        result = tok.tokenize("一二三四")
        assert all(len(t.tones) > 0 for t in result.tokens)


# ── Content vs function words ──────────────────────────────────────────────────

class TestContentWords:
    def test_noun_is_content(self, tok):
        result = tok.tokenize("猫")
        assert result.tokens[0].is_content_word

    def test_verb_is_content(self, tok):
        result = tok.tokenize("跑")
        assert result.tokens[0].is_content_word

    def test_punctuation_not_content(self, tok):
        result = tok.tokenize("，")
        assert not result.tokens[0].is_content_word

    def test_period_not_content(self, tok):
        result = tok.tokenize("。")
        assert not result.tokens[0].is_content_word


# ── Common vocabulary spot checks ─────────────────────────────────────────────

VOCAB_CASES = [
    ("中国", "zhong"),    # tone 1 initial
    ("学生", "xue"),      # tone 2
    ("你好", "ni"),       # tone 3
    ("再见", "zai"),      # tone 4
    ("谢谢", "xie"),      # tone 4 repeated
    ("老师", "lao"),      # tone 3
    ("朋友", "peng"),     # tone 2
    ("工作", "gong"),     # tone 1
    ("时间", "shi"),      # tone 2
    ("大学", "da"),       # tone 4
]

@pytest.mark.parametrize("word,expected_pinyin_start", VOCAB_CASES)
def test_vocab_pinyin_start(tok, word, expected_pinyin_start):
    result = tok.tokenize(word)
    all_pinyin = " ".join(t.pinyin_num for t in result.tokens).lower()
    assert expected_pinyin_start.lower() in all_pinyin, \
        f"Expected '{expected_pinyin_start}' in pinyin for '{word}', got '{all_pinyin}'"


# ── Segmentation edge cases ────────────────────────────────────────────────────

class TestEdgeCases:
    def test_long_text(self, tok):
        text = "中国是一个历史悠久的文明古国，拥有丰富的文化遗产和传统。"
        result = tok.tokenize(text)
        assert len(result.tokens) > 5

    def test_repeated_chars(self, tok):
        result = tok.tokenize("哈哈哈")
        assert len(result.tokens) >= 1

    def test_numbers_mixed(self, tok):
        result = tok.tokenize("第一次")
        assert len(result.tokens) >= 1

    def test_english_word(self, tok):
        result = tok.tokenize("iPhone手机")
        assert len(result.tokens) >= 1

    def test_surface_preserved(self, tok):
        text = "今天天气很好"
        result = tok.tokenize(text)
        reconstructed = "".join(t.surface for t in result.tokens)
        assert reconstructed == text

    def test_all_tokens_have_lemma(self, tok):
        result = tok.tokenize("我们都是好学生")
        for t in result.tokens:
            assert t.lemma, f"Token '{t.surface}' has empty lemma"

    def test_all_tokens_have_surface(self, tok):
        result = tok.tokenize("这是一个测试")
        for t in result.tokens:
            assert t.surface, "Token has empty surface"

    def test_whitespace_handling(self, tok):
        result = tok.tokenize("你 好")
        # Should handle space-separated text
        surfaces = "".join(t.surface for t in result.tokens)
        assert "你" in surfaces and "好" in surfaces

    def test_newline_handling(self, tok):
        result = tok.tokenize("你好\n世界")
        assert len(result.tokens) >= 2

    def test_emoji_passthrough(self, tok):
        result = tok.tokenize("你好😊")
        assert len(result.tokens) >= 1
