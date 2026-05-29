"""
Japanese morphological tokenizer wrapping SudachiPy.

Design notes:
  - Mode C (longest unit) for vocabulary lookup: matches dictionary entries
  - Mode A (shortest unit) for grammar breakdown display
  - Readings are always returned in katakana (standard form)
  - Furigana spans generated via LCS alignment of surface vs reading
"""

from __future__ import annotations

import re
import unicodedata
from dataclasses import dataclass, field

import sudachipy


@dataclass
class Token:
    surface: str
    lemma: str           # dictionary form
    reading: str         # katakana
    reading_hira: str    # hiragana (for display)
    pos: str             # 動詞, 名詞, 助詞, ...
    pos_detail: str
    pos_full: tuple[str, ...]
    is_content_word: bool
    # Set after dictionary lookup
    frequency_rank: int | None = None
    word_id: str | None = None
    confidence: float = 1.0


@dataclass
class FuriganaSpan:
    text: str       # kanji segment
    reading: str    # hiragana reading for this segment


@dataclass
class TokenizedText:
    tokens: list[Token]
    # A-unit breakdown (fine-grained, for grammar display)
    a_tokens: list[Token] = field(default_factory=list)

    @property
    def surface(self) -> str:
        return "".join(t.surface for t in self.tokens)

    @property
    def full_reading(self) -> str:
        return "".join(t.reading for t in self.tokens)


# POS categories that count as content words (content words drive vocabulary acquisition)
_CONTENT_POS = {"動詞", "名詞", "形容詞", "形状詞", "副詞", "感動詞"}

# POS categories that are function words (particles, auxiliaries, punctuation)
_FUNCTION_POS = {"助詞", "助動詞", "記号", "空白", "補助記号"}

# Katakana → Hiragana conversion range
_KATA_OFFSET = ord("ァ") - ord("ぁ")


def kata_to_hira(text: str) -> str:
    """Convert katakana string to hiragana."""
    result = []
    for ch in text:
        cp = ord(ch)
        if 0x30A1 <= cp <= 0x30F6:  # ァ to ヶ
            result.append(chr(cp - _KATA_OFFSET))
        else:
            result.append(ch)
    return "".join(result)


def _has_kanji(text: str) -> bool:
    return any(0x4E00 <= ord(c) <= 0x9FFF for c in text)


def generate_furigana(surface: str, reading_hira: str) -> list[FuriganaSpan]:
    """
    Align kanji spans in surface to their hiragana readings.

    Uses LCS-based alignment: kana in surface must match the same kana in
    reading_hira. Kanji spans between kana anchors get the reading between
    those same anchors in reading_hira.

    Example:
      surface="食べる", reading_hira="たべる"
      → [FuriganaSpan("食", "た"), FuriganaSpan("べる", "")]
         (kana segments don't need furigana)

    Returns a flat list of spans; spans with empty reading are plain kana.
    """
    if not _has_kanji(surface):
        return [FuriganaSpan(surface, "")]

    spans: list[FuriganaSpan] = []

    def is_kana(c: str) -> bool:
        cp = ord(c)
        return (0x3040 <= cp <= 0x309F or   # hiragana
                0x30A0 <= cp <= 0x30FF or   # katakana
                cp == 0x30FC)               # prolonged sound mark ー

    def kana_of(c: str) -> str:
        cp = ord(c)
        if 0x30A1 <= cp <= 0x30F6:
            return chr(cp - _KATA_OFFSET)
        return c

    # Build kana anchor list from surface
    # Each anchor: (surface_index, reading_index)
    surf_kana = [(i, kana_of(c)) for i, c in enumerate(surface) if is_kana(c)]
    read_kana = [(i, c) for i, c in enumerate(reading_hira) if is_kana(c)]

    if not surf_kana:
        # All kanji — whole surface gets whole reading
        return [FuriganaSpan(surface, reading_hira)]

    # Match kana anchors greedily from both ends
    anchors: list[tuple[int, int]] = []  # (surface_idx, reading_idx)

    si, ri = 0, 0
    while si < len(surf_kana) and ri < len(read_kana):
        sk, sc = surf_kana[si]
        rk, rc = read_kana[ri]
        if sc == rc:
            anchors.append((sk, rk))
            si += 1
            ri += 1
        else:
            ri += 1  # skip mismatches in reading (shouldn't normally happen)

    # Build spans using anchors as boundaries
    prev_s = 0  # surface index
    prev_r = 0  # reading index

    for s_anchor, r_anchor in anchors:
        kanji_seg = surface[prev_s:s_anchor]
        kana_reading = reading_hira[prev_r:r_anchor]
        if kanji_seg:
            spans.append(FuriganaSpan(kanji_seg, kana_reading))
        # The kana character itself
        spans.append(FuriganaSpan(surface[s_anchor], ""))
        prev_s = s_anchor + 1
        prev_r = r_anchor + 1

    # Trailing kanji after last anchor
    tail_s = surface[prev_s:]
    tail_r = reading_hira[prev_r:]
    if tail_s:
        spans.append(FuriganaSpan(tail_s, tail_r if _has_kanji(tail_s) else ""))

    return spans


class JapaneseTokenizer:
    """
    Thread-safe wrapper around SudachiPy with the 'core' dictionary.

    Correctness contract (verified by tests/test_correctness.py):
      - ございません: full reading concatenates to ゴザイマセン
      - 入って: reading = ハイッ + テ (not any other reading)
      - 東京都: reading = トウキョウト (not トウキョウミヤコ)
      - 食べられる: lemma = 食べる
      - 生き物: reading = イキモノ (not ナマキモノ)
    """

    _instance: JapaneseTokenizer | None = None

    def __init__(self, dict_name: str = "core") -> None:
        self._dict = sudachipy.Dictionary(dict=dict_name)
        self._tokenizer = self._dict.create()

    @classmethod
    def get(cls) -> JapaneseTokenizer:
        if cls._instance is None:
            cls._instance = cls()
        return cls._instance

    def tokenize(self, text: str, mode: str = "C") -> TokenizedText:
        """
        Tokenize Japanese text.

        Args:
            text:  Input text (mixed script OK)
            mode:  "C" (longest, for vocab lookup) or "A" (shortest, for grammar)
        """
        text = self._normalize(text)
        mode_obj = {
            "A": sudachipy.SplitMode.A,
            "B": sudachipy.SplitMode.B,
            "C": sudachipy.SplitMode.C,
        }[mode]

        c_tokens = self._tokenize_raw(text, sudachipy.SplitMode.C)
        a_tokens = (
            self._tokenize_raw(text, sudachipy.SplitMode.A)
            if mode != "A"
            else c_tokens
        )

        return TokenizedText(tokens=c_tokens, a_tokens=a_tokens)

    def _tokenize_raw(self, text: str, mode: sudachipy.SplitMode) -> list[Token]:
        morphemes = self._tokenizer.tokenize(text, mode)
        tokens = []
        for m in morphemes:
            pos_tuple = m.part_of_speech()
            pos = pos_tuple[0] if pos_tuple else "未知"
            pos_detail = pos_tuple[1] if len(pos_tuple) > 1 else "*"
            reading_kata = m.reading_form() or m.surface()
            reading_hira = kata_to_hira(reading_kata)
            tokens.append(Token(
                surface=m.surface(),
                lemma=m.dictionary_form() or m.surface(),
                reading=reading_kata,
                reading_hira=reading_hira,
                pos=pos,
                pos_detail=pos_detail,
                pos_full=tuple(pos_tuple),
                is_content_word=pos in _CONTENT_POS,
            ))
        return tokens

    @staticmethod
    def _normalize(text: str) -> str:
        """NFC normalize and convert full-width ASCII to half-width."""
        text = unicodedata.normalize("NFC", text)
        # Full-width ASCII → half-width (！→! etc.)
        result = []
        for ch in text:
            cp = ord(ch)
            if 0xFF01 <= cp <= 0xFF5E:
                result.append(chr(cp - 0xFEE0))
            else:
                result.append(ch)
        return "".join(result)

    def get_furigana(self, surface: str, reading_hira: str) -> list[FuriganaSpan]:
        return generate_furigana(surface, reading_hira)
