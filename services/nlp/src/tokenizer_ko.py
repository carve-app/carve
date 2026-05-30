"""
Korean tokenizer.

Uses python-mecab-ko when available; falls back to a rule-based eojeol
splitter with heuristic particle detection when it's not installed.

Korean grammar primer for implementation:
  - Text is written in eojeols (space-separated units, each = stem + suffix)
  - Content words: nouns (NNG, NNP), verbs (VV), adjectives (VA), adverbs (MAG)
  - Particles (조사, JX/JC/JKS/…) attach to the preceding noun eojeol
  - Endings (어미, EF/EC/…) attach to verb/adjective stems
  - Romanization: Revised Romanization of Korean (ISO/IEC 18005)
"""

from __future__ import annotations

import re
import unicodedata
from dataclasses import dataclass, field


try:
    import mecab  # python-mecab-ko
    _MECAB_AVAILABLE = True
except ImportError:
    _MECAB_AVAILABLE = False


@dataclass
class KoreanToken:
    surface: str
    lemma: str
    romanization: str     # Revised Romanization (approximate)
    pos: str              # MeCab POS tag (NNG, VV, JX, …)
    pos_category: str     # coarse: 'noun' | 'verb' | 'adjective' | 'particle' | 'other'
    is_content_word: bool
    particles: list[str] = field(default_factory=list)   # attached particles stripped off
    frequency_rank: int | None = None


@dataclass
class KoreanTokenizedText:
    tokens: list[KoreanToken]

    @property
    def surface(self) -> str:
        return "".join(t.surface for t in self.tokens)


# MeCab POS tag → coarse category
_POS_CATEGORY: dict[str, str] = {
    "NNG": "noun", "NNP": "noun", "NNB": "noun", "NP": "noun", "NR": "noun",
    "VV": "verb", "VA": "adjective", "VX": "verb", "VCP": "verb", "VCN": "verb",
    "MAG": "adverb", "MAJ": "adverb",
    "JX": "particle", "JC": "particle",
    "JKS": "particle", "JKC": "particle", "JKG": "particle", "JKO": "particle",
    "JKB": "particle", "JKV": "particle", "JKQ": "particle",
    "EF": "ending", "EC": "ending", "ETN": "ending", "ETM": "ending",
    "XSN": "suffix", "XSV": "suffix", "XSA": "suffix", "XR": "suffix",
    "IC": "interjection",
    "MM": "determiner",
    "SL": "foreign", "SH": "chinese", "SN": "number",
    "SF": "punctuation", "SP": "punctuation", "SS": "punctuation",
    "SE": "ellipsis", "SO": "symbol", "SW": "symbol",
}

_CONTENT_CATEGORIES = {"noun", "verb", "adjective", "adverb", "interjection"}

# Korean common particle endings (for rule-based fallback)
_PARTICLES = ["이가", "은는", "을를", "에서는", "에게서", "에서", "에게", "한테서", "한테", "로서는", "로서", "로써",
              "와과", "과와", "이나", "이고", "이라", "이면", "이지만",
              "가", "은", "는", "이", "을", "를", "의", "에", "서", "도", "만", "부터",
              "까지", "처럼", "보다", "마다", "조차", "밖에", "과", "와"]

# Hangul syllable block boundaries
_HANGUL_START = 0xAC00
_HANGUL_END = 0xD7A3


def _is_hangul(c: str) -> bool:
    cp = ord(c)
    return _HANGUL_START <= cp <= _HANGUL_END or 0x1100 <= cp <= 0x11FF or 0x3130 <= cp <= 0x318F


# Simplified Revised Romanization lookup (initial + vowel + final)
_INITIALS = ["g", "kk", "n", "d", "tt", "r", "m", "b", "pp", "s", "ss", "", "j", "jj", "ch", "k", "t", "p", "h"]
_VOWELS = ["a", "ae", "ya", "yae", "eo", "e", "yeo", "ye", "o", "wa", "wae", "oe", "yo", "u", "wo", "we", "wi", "yu", "eu", "ui", "i"]
_FINALS = ["", "g", "kk", "gs", "n", "nj", "nh", "d", "l", "rg", "rm", "rb", "rs", "rt", "rp", "rh", "m", "b", "bs", "s", "ss", "ng", "j", "ch", "k", "t", "p", "h"]


def _romanize_char(c: str) -> str:
    """Romanize a single Hangul syllable block."""
    cp = ord(c)
    if not (_HANGUL_START <= cp <= _HANGUL_END):
        return c
    idx = cp - _HANGUL_START
    final = idx % 28
    idx //= 28
    vowel = idx % 21
    initial = idx // 21
    return _INITIALS[initial] + _VOWELS[vowel] + _FINALS[final]


def romanize(text: str) -> str:
    """Approximate Revised Romanization of a Korean string."""
    return "".join(_romanize_char(c) if _is_hangul(c) else c for c in text)


def _strip_particles(eojeol: str) -> tuple[str, list[str]]:
    """Heuristic: strip trailing particles from a Korean eojeol."""
    for p in sorted(_PARTICLES, key=len, reverse=True):
        if eojeol.endswith(p) and len(eojeol) > len(p):
            return eojeol[: -len(p)], [p]
    return eojeol, []


class KoreanTokenizer:
    """
    Korean tokenizer.

    Uses MeCab (via python-mecab-ko) when available; otherwise falls back to
    a heuristic space-splitter with particle stripping.
    """

    def __init__(self) -> None:
        self._mecab = None
        if _MECAB_AVAILABLE:
            try:
                self._mecab = mecab.MeCab()
            except Exception:
                pass

    def tokenize(self, text: str) -> KoreanTokenizedText:
        if self._mecab is not None:
            return self._mecab_tokenize(text)
        return self._fallback_tokenize(text)

    def _mecab_tokenize(self, text: str) -> KoreanTokenizedText:
        tokens: list[KoreanToken] = []
        try:
            for item in self._mecab.parse(text):
                surface = item.surface
                pos_tag = item.pos.split("+")[0] if item.pos else "SW"
                lemma = getattr(item, "lemma", surface) or surface
                cat = _POS_CATEGORY.get(pos_tag, "other")
                tokens.append(KoreanToken(
                    surface=surface,
                    lemma=lemma,
                    romanization=romanize(surface),
                    pos=pos_tag,
                    pos_category=cat,
                    is_content_word=cat in _CONTENT_CATEGORIES,
                ))
        except Exception:
            return self._fallback_tokenize(text)
        return KoreanTokenizedText(tokens=tokens)

    def _fallback_tokenize(self, text: str) -> KoreanTokenizedText:
        """Space-split + heuristic particle stripping."""
        tokens: list[KoreanToken] = []
        for eojeol in text.split():
            if not eojeol:
                continue
            stem, particles = _strip_particles(eojeol)
            tokens.append(KoreanToken(
                surface=eojeol,
                lemma=stem,
                romanization=romanize(stem),
                pos="NNG",
                pos_category="noun",
                is_content_word=True,
                particles=particles,
            ))
        return KoreanTokenizedText(tokens=tokens)
