"""
Chinese (Mandarin) tokenizer using jieba.

Provides pinyin annotation with tone diacritics, traditional/simplified
detection, and tone metadata for color-coding in the UI.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

try:
    import jieba
    import jieba.posseg as pseg
    _JIEBA_AVAILABLE = True
except ImportError:
    _JIEBA_AVAILABLE = False

try:
    from pypinyin import lazy_pinyin, Style, pinyin as get_pinyin
    _PINYIN_AVAILABLE = True
except ImportError:
    _PINYIN_AVAILABLE = False


@dataclass
class ChineseToken:
    surface: str
    lemma: str
    pinyin: str          # space-separated pinyin with tone diacritics (e.g. "nǐ hǎo")
    pinyin_num: str      # space-separated with tone numbers (e.g. "ni3 hao3")
    tones: list[int]     # per-syllable tone numbers (0=neutral, 1–4)
    pos: str             # jieba POS tag (n, v, a, …)
    is_content_word: bool
    frequency_rank: int | None = None


@dataclass
class ChineseTokenizedText:
    tokens: list[ChineseToken]

    @property
    def surface(self) -> str:
        return "".join(t.surface for t in self.tokens)


# Content-word POS tags in jieba's Penn-Treebank-like tagset
_CONTENT_POS = {"n", "nr", "ns", "nt", "nz", "v", "vd", "vn", "a", "an", "ad", "d", "i", "l"}

# Traditional Chinese character range markers (simplified check)
# True detection requires a lookup table; we use a heuristic based on codepoints
# that are exclusively in the traditional set.
_TRADITIONAL_ONLY = frozenset(
    "體標來說樣區時間過動會學實際關現問題點種個產業面經已發展"
    "從於與無為對這邊請問還讓給"
)


def _is_traditional(text: str) -> bool:
    """Heuristic: text is traditional if it contains chars only in Traditional set."""
    return any(c in _TRADITIONAL_ONLY for c in text)


def _tone_from_pinyin(py: str) -> int:
    """Extract tone number from a pinyin syllable with diacritics."""
    tone_map = {
        "ā": 1, "á": 2, "ǎ": 3, "à": 4,
        "ē": 1, "é": 2, "ě": 3, "è": 4,
        "ī": 1, "í": 2, "ǐ": 3, "ì": 4,
        "ō": 1, "ó": 2, "ǒ": 3, "ò": 4,
        "ū": 1, "ú": 2, "ǔ": 3, "ù": 4,
        "ǖ": 1, "ǘ": 2, "ǚ": 3, "ǜ": 4,
    }
    # Diacritics indicate tones 1-4; neutral tone has no diacritic
    for ch, t in tone_map.items():
        if ch in py:
            return t
    return 0  # neutral (5th) tone or punctuation


def _diacritic_to_num(py: str) -> str:
    """Convert pinyin with diacritic to numbered form (e.g. nǐ → ni3)."""
    tone_chars = {
        "ā": ("a", 1), "á": ("a", 2), "ǎ": ("a", 3), "à": ("a", 4),
        "ē": ("e", 1), "é": ("e", 2), "ě": ("e", 3), "è": ("e", 4),
        "ī": ("i", 1), "í": ("i", 2), "ǐ": ("i", 3), "ì": ("i", 4),
        "ō": ("o", 1), "ó": ("o", 2), "ǒ": ("o", 3), "ò": ("o", 4),
        "ū": ("u", 1), "ú": ("u", 2), "ǔ": ("u", 3), "ù": ("u", 4),
        "ǖ": ("ü", 1), "ǘ": ("ü", 2), "ǚ": ("ü", 3), "ǜ": ("ü", 4),
    }
    result = py
    tone = 0
    for ch, (base, t) in tone_chars.items():
        if ch in result:
            result = result.replace(ch, base)
            tone = t
            break
    return result + (str(tone) if tone else "5")


class ChineseTokenizer:
    """
    Chinese tokenizer wrapping jieba with pypinyin annotation.

    Falls back gracefully if jieba/pypinyin are not installed.
    """

    def __init__(self) -> None:
        if _JIEBA_AVAILABLE:
            jieba.initialize()
        if _JIEBA_AVAILABLE:
            jieba.setLogLevel("ERROR")

    def tokenize(self, text: str) -> ChineseTokenizedText:
        if not _JIEBA_AVAILABLE:
            return self._fallback_tokenize(text)

        words = pseg.cut(text)
        tokens: list[ChineseToken] = []
        for word, flag in words:
            py_list, py_num_list, tones = self._get_pinyin(word)
            tokens.append(ChineseToken(
                surface=word,
                lemma=word,
                pinyin=" ".join(py_list),
                pinyin_num=" ".join(py_num_list),
                tones=tones,
                pos=flag,
                is_content_word=flag in _CONTENT_POS,
            ))
        return ChineseTokenizedText(tokens=tokens)

    def _get_pinyin(self, word: str) -> tuple[list[str], list[str], list[int]]:
        if not _PINYIN_AVAILABLE:
            return [word], [word], [0]
        try:
            py_diac = [syl[0] for syl in get_pinyin(word, style=Style.TONE)]
            py_num = [_diacritic_to_num(p) for p in py_diac]
            tones = [_tone_from_pinyin(p) for p in py_diac]
            return py_diac, py_num, tones
        except Exception:
            return [word], [word], [0] * len(word)

    def _fallback_tokenize(self, text: str) -> ChineseTokenizedText:
        """Character-level fallback when jieba is not available."""
        tokens = []
        for ch in text:
            tokens.append(ChineseToken(
                surface=ch, lemma=ch,
                pinyin=ch, pinyin_num=ch, tones=[0],
                pos="n", is_content_word=True,
            ))
        return ChineseTokenizedText(tokens=tokens)

    def is_traditional(self, text: str) -> bool:
        return _is_traditional(text)
