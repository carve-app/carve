"""
Japanese morphological tokenizer wrapping SudachiPy.

Design notes:
  - Mode C (longest unit) for vocabulary lookup: matches dictionary entries
  - Mode A (shortest unit) for grammar breakdown display
  - Readings are always returned in katakana (standard form)
  - Furigana spans generated via LCS alignment of surface vs reading

Post-processing corrections:
  - Arabic numerals: SudachiPy reads each digit individually (24 → にし);
    we replace with proper Japanese number readings (24 → にじゅうし).
  - Compound rendaku: SudachiPy splits 大-prefix + head but gives the
    unvoiced reading (大相撲 → おお+すもう); we merge and apply the
    correct reading (おおずもう) for known cases.
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


def hira_to_kata(text: str) -> str:
    """Convert hiragana string to katakana."""
    result = []
    for ch in text:
        cp = ord(ch)
        if 0x3041 <= cp <= 0x3096:  # ぁ to ゖ
            result.append(chr(cp + _KATA_OFFSET))
        else:
            result.append(ch)
    return "".join(result)


# ── Arabic numeral → Japanese reading ────────────────────────────────────────
# SudachiPy reads each digit individually (24 → によん / にし). We replace
# the reading with proper positional Japanese number words.

_ONES = ["", "いち", "に", "さん", "よん", "ご", "ろく", "なな", "はち", "きゅう"]
_SPECIAL_H = {3: "さんびゃく", 6: "ろっぴゃく", 8: "はっぴゃく"}   # 百 sandhi
_SPECIAL_K = {3: "さんぜん",   8: "はっせん"}                       # 千 sandhi


def _reading_under_10000(n: int) -> str:
    parts: list[str] = []
    sen = n // 1000
    if sen:
        if sen in _SPECIAL_K:
            parts.append(_SPECIAL_K[sen])
        elif sen == 1:
            parts.append("せん")
        else:
            parts.append(_ONES[sen] + "せん")
    hyaku = (n % 1000) // 100
    if hyaku:
        if hyaku in _SPECIAL_H:
            parts.append(_SPECIAL_H[hyaku])
        elif hyaku == 1:
            parts.append("ひゃく")
        else:
            parts.append(_ONES[hyaku] + "ひゃく")
    juu = (n % 100) // 10
    if juu:
        parts.append(("" if juu == 1 else _ONES[juu]) + "じゅう")
    ones = n % 10
    if ones:
        parts.append(_ONES[ones])
    return "".join(parts)


def _numeral_reading_int(n: int) -> str | None:
    """Core reading for a non-negative integer < 1_000_000_000_000 (one trillion)."""
    if n < 0:
        return None
    if n == 0:
        return "ぜろ"
    parts: list[str] = []
    chou = n // 1_000_000_000_000
    if chou:
        parts.append(_reading_under_10000(chou) + "ちょう")
    oku = (n % 1_000_000_000_000) // 100_000_000
    if oku:
        parts.append(_reading_under_10000(oku) + "おく")
    man = (n % 100_000_000) // 10_000
    if man:
        parts.append(_reading_under_10000(man) + "まん")
    rem = n % 10_000
    if rem:
        parts.append(_reading_under_10000(rem))
    return "".join(parts) if parts else None


def arabic_numeral_reading(s: str) -> str | None:
    """Return hiragana reading for an ASCII numeral string, or None if unsupported."""
    try:
        n = int(s)
    except ValueError:
        return None
    if n < 0 or n >= 1_000_000_000_000:
        return None
    return _numeral_reading_int(n)


# Mixed large-unit patterns: 100万, 5万4000, 1億2500万, 12兆3000億…
# Must contain at least one large unit character; allows chained units.
_LARGE_UNIT_CHARS = frozenset("万億兆")
_HYBRID_NUM_RE = re.compile(
    r'^(?:(\d+)兆)?(?:(\d+)億)?(?:(\d+)万)?(\d+)?$'
)
_LARGE_UNIT_RE = re.compile(r'[万億兆]')  # kept for the _tokenize_raw guard


def mixed_numeral_reading(surface: str) -> str | None:
    """Return hiragana reading for hybrid numeral patterns like 1億2500万, 100万, 5万4000."""
    if not any(c in _LARGE_UNIT_CHARS for c in surface):
        return None
    m = _HYBRID_NUM_RE.match(surface)
    if not m or not any(m.group(i) for i in range(1, 4)):
        return None
    chou = int(m.group(1)) if m.group(1) else 0
    oku  = int(m.group(2)) if m.group(2) else 0
    man  = int(m.group(3)) if m.group(3) else 0
    rem  = int(m.group(4)) if m.group(4) else 0
    total = chou * 1_000_000_000_000 + oku * 100_000_000 + man * 10_000 + rem
    if total == 0:
        return None
    return _numeral_reading_int(total)


# ── Compound reading corrections (rendaku etc.) ───────────────────────────────
# When SudachiPy splits a prefix from its head, the concatenated readings can
# miss phonetic changes. We merge known pairs into a single corrected token.
# Each entry: compound_surface → (reading_hira, lemma, pos)

_COMPOUND_CORRECTIONS: dict[str, tuple[str, str, str]] = {
    "大相撲": ("おおずもう", "大相撲", "名詞"),
    # 来る causative: SudachiPy splits 来させ as 来さ+せ with wrong lemma/reading
    "来させ": ("こさせ", "来る", "動詞"),
    # Kanji date ordinals — merge into a single vocabulary token with correct reading
    "二日":   ("ふつか",    "二日",   "名詞"),
    "三日":   ("みっか",    "三日",   "名詞"),
    "四日":   ("よっか",    "四日",   "名詞"),
    "五日":   ("いつか",    "五日",   "名詞"),
    "六日":   ("むいか",    "六日",   "名詞"),
    "七日":   ("なのか",    "七日",   "名詞"),
    "八日":   ("ようか",    "八日",   "名詞"),
    "九日":   ("ここのか",  "九日",   "名詞"),
    "十日":   ("とおか",    "十日",   "名詞"),
    "二十日": ("はつか",    "二十日", "名詞"),
    # 一昨日/一昨年: SudachiPy splits as 一昨+日/年, giving wrong readings
    "一昨日": ("おととい", "一昨日", "名詞"),
    "一昨年": ("おととし", "一昨年", "名詞"),
}


# ── Japanese date native readings ─────────────────────────────────────────────
# 日 as a date counter uses native Japanese readings for 2-10 and a few specials.
# SudachiPy splits e.g. "2日" into numeral + 日 and gives にち (wrong).
_DATE_NATIVE: dict[str, str] = {
    "2": "ふつか",   "3": "みっか",   "4": "よっか",   "5": "いつか",
    "6": "むいか",   "7": "なのか",   "8": "ようか",   "9": "ここのか",
    "10": "とおか",  "14": "じゅうよっか", "20": "はつか", "24": "にじゅうよっか",
}


def _date_reading(numeral_str: str) -> str | None:
    """Return hiragana reading for N日 (e.g. '2'→'ふつか'). Falls back to N+にち."""
    clean = numeral_str.replace(",", "")
    if clean in _DATE_NATIVE:
        return _DATE_NATIVE[clean]
    r = arabic_numeral_reading(clean)
    if r is None:
        return None
    return r + "にち"


# ── Counter sandhi ────────────────────────────────────────────────────────────
# Explicit table for 1-10 (irregular forms); rule-based for higher numbers.
# Tuple: (plain_base, geminate_base, voiced_base | None)
#   geminate: used after 1, 6, 8 (いち→いっ, ろく→ろっ, はち→はっ)
#   voiced:   used after 3 (さん + voiced form)
_COUNTER_TABLE: dict[str, dict[str, str]] = {
    "本": {"1": "いっぽん", "2": "にほん",   "3": "さんぼん",  "4": "よんほん",
           "5": "ごほん",   "6": "ろっぽん", "7": "ななほん",  "8": "はっぽん",
           "9": "きゅうほん", "10": "じゅっぽん"},
    "冊": {"1": "いっさつ", "2": "にさつ",   "3": "さんさつ",  "4": "よんさつ",
           "5": "ごさつ",   "6": "ろくさつ", "7": "ななさつ",  "8": "はっさつ",
           "9": "きゅうさつ", "10": "じゅっさつ"},
    "匹": {"1": "いっぴき", "2": "にひき",   "3": "さんびき",  "4": "よんひき",
           "5": "ごひき",   "6": "ろっぴき", "7": "ななひき",  "8": "はっぴき",
           "9": "きゅうひき", "10": "じゅっぴき"},
    "階": {"1": "いっかい", "2": "にかい",   "3": "さんがい",  "4": "よんかい",
           "5": "ごかい",   "6": "ろっかい", "7": "ななかい",  "8": "はっかい",
           "9": "きゅうかい", "10": "じゅっかい"},
    "個": {"1": "いっこ",   "2": "にこ",     "3": "さんこ",    "4": "よんこ",
           "5": "ごこ",     "6": "ろっこ",   "7": "ななこ",    "8": "はっこ",
           "9": "きゅうこ", "10": "じゅっこ"},
    "杯": {"1": "いっぱい", "2": "にはい",   "3": "さんばい",  "4": "よんはい",
           "5": "ごはい",   "6": "ろっぱい", "7": "ななはい",  "8": "はっぱい",
           "9": "きゅうはい", "10": "じゅっぱい"},
    "回": {"1": "いっかい", "2": "にかい",   "3": "さんかい",  "4": "よんかい",
           "5": "ごかい",   "6": "ろっかい", "7": "ななかい",  "8": "はっかい",
           "9": "きゅうかい", "10": "じゅっかい"},
    "枚": {"1": "いちまい", "2": "にまい",   "3": "さんまい",  "4": "よんまい",
           "5": "ごまい",   "6": "ろくまい", "7": "ななまい",  "8": "はちまい",
           "9": "きゅうまい", "10": "じゅうまい"},
    "台": {"1": "いちだい", "2": "にだい",   "3": "さんだい",  "4": "よんだい",
           "5": "ごだい",   "6": "ろくだい", "7": "ななだい",  "8": "はちだい",
           "9": "きゅうだい", "10": "じゅうだい"},
    # 分 (minutes): 1→いっぷん, 3→さんぷん (p-voicing), 6,8,10 geminate
    "分": {"1": "いっぷん", "2": "にふん",   "3": "さんぷん",  "4": "よんふん",
           "5": "ごふん",   "6": "ろっぷん", "7": "ななふん",  "8": "はっぷん",
           "9": "きゅうふん", "10": "じゅっぷん"},
    # 人 (people): 1人=ひとり, 2人=ふたり handled by SudachiPy; 4→よにん (not しにん)
    "人": {"1": "ひとり",   "2": "ふたり",   "3": "さんにん",  "4": "よにん",
           "5": "ごにん",   "6": "ろくにん", "7": "ななにん",  "8": "はちにん",
           "9": "きゅうにん", "10": "じゅうにん"},
    # 時 (o'clock): 4→よじ, 7→しちじ, 9→くじ (irregular clock-time readings)
    "時": {"1": "いちじ",   "2": "にじ",     "3": "さんじ",    "4": "よじ",
           "5": "ごじ",     "6": "ろくじ",   "7": "しちじ",    "8": "はちじ",
           "9": "くじ",     "10": "じゅうじ", "11": "じゅういちじ", "12": "じゅうにじ"},
    # 月 (months): 4→しがつ (し is CORRECT for months), 7→しちがつ, 9→くがつ
    "月": {"1": "いちがつ",  "2": "にがつ",   "3": "さんがつ",  "4": "しがつ",
           "5": "ごがつ",   "6": "ろくがつ",  "7": "しちがつ",  "8": "はちがつ",
           "9": "くがつ",   "10": "じゅうがつ", "11": "じゅういちがつ", "12": "じゅうにがつ"},
    # 年 (years): 4→よねん, others regular
    "年": {"1": "いちねん",  "2": "にねん",   "3": "さんねん",  "4": "よねん",
           "5": "ごねん",   "6": "ろくねん",  "7": "ななねん",  "8": "はちねん",
           "9": "きゅうねん", "10": "じゅうねん"},
    # 歳 (age): 1→いっさい, 8→はっさい; 4→よんさい (not しさい)
    "歳": {"1": "いっさい",  "2": "にさい",   "3": "さんさい",  "4": "よんさい",
           "5": "ごさい",   "6": "ろくさい",  "7": "ななさい",  "8": "はっさい",
           "9": "きゅうさい", "10": "じゅっさい"},
    # 点 (points/scores): 1→いってん, 8→はってん
    "点": {"1": "いってん",  "2": "にてん",   "3": "さんてん",  "4": "よんてん",
           "5": "ごてん",   "6": "ろくてん",  "7": "ななてん",  "8": "はってん",
           "9": "きゅうてん", "10": "じゅってん"},
    # 泊 (nights): 1→いっぱく, 3→さんぱく, 6→ろっぱく, 8→はっぱく
    "泊": {"1": "いっぱく",  "2": "にはく",   "3": "さんぱく",  "4": "よんはく",
           "5": "ごはく",   "6": "ろっぱく",  "7": "ななはく",  "8": "はっぱく",
           "9": "きゅうはく", "10": "じゅっぱく"},
    # 発 (shots/launches): 1→いっぱつ, 3→さんぱつ, 6→ろっぱつ, 8→はっぱつ
    "発": {"1": "いっぱつ",  "2": "にはつ",   "3": "さんぱつ",  "4": "よんはつ",
           "5": "ごはつ",   "6": "ろっぱつ",  "7": "ななはつ",  "8": "はっぱつ",
           "9": "きゅうはつ", "10": "じゅっぱつ"},
    # か月/ヶ月 (months of duration): 1→いっかげつ, 6→ろっかげつ, 8→はっかげつ
    "か月": {"1": "いっかげつ", "2": "にかげつ",   "3": "さんかげつ", "4": "よんかげつ",
             "5": "ごかげつ",   "6": "ろっかげつ", "7": "ななかげつ", "8": "はっかげつ",
             "9": "きゅうかげつ", "10": "じゅっかげつ"},
    "ヶ月": {"1": "いっかげつ", "2": "にかげつ",   "3": "さんかげつ", "4": "よんかげつ",
             "5": "ごかげつ",   "6": "ろっかげつ", "7": "ななかげつ", "8": "はっかげつ",
             "9": "きゅうかげつ", "10": "じゅっかげつ"},
    # 週間 (weeks of duration): 1→いっしゅうかん, 8→はっしゅうかん (gemination before しゅ)
    "週間": {"1": "いっしゅうかん", "2": "にしゅうかん",   "3": "さんしゅうかん", "4": "よんしゅうかん",
             "5": "ごしゅうかん",   "6": "ろくしゅうかん", "7": "ななしゅうかん", "8": "はっしゅうかん",
             "9": "きゅうしゅうかん", "10": "じゅっしゅうかん"},
    # か所/ヶ所 (locations): 1→いっかしょ, 6→ろっかしょ, 8→はっかしょ
    "か所": {"1": "いっかしょ",  "2": "にかしょ",   "3": "さんかしょ",  "4": "よんかしょ",
             "5": "ごかしょ",   "6": "ろっかしょ", "7": "ななかしょ",  "8": "はっかしょ",
             "9": "きゅうかしょ", "10": "じゅっかしょ"},
    "ヶ所": {"1": "いっかしょ",  "2": "にかしょ",   "3": "さんかしょ",  "4": "よんかしょ",
             "5": "ごかしょ",   "6": "ろっかしょ", "7": "ななかしょ",  "8": "はっかしょ",
             "9": "きゅうかしょ", "10": "じゅっかしょ"},
}

# Tuple: (plain_base, geminate_base | None, voiced_base | None)
#   gem=None → no consonant gemination (年, 月, 時, 人…)
#   gem=str  → geminate numeral before this form (1,6,8 rule)
_COUNTER_SANDHI: dict[str, tuple[str, str | None, str | None]] = {
    "本": ("ほん", "ぽん", "ぼん"),   "冊": ("さつ", "さつ", None),
    "匹": ("ひき", "ぴき", "びき"),   "階": ("かい", "かい", "がい"),
    "個": ("こ",   "こ",   None),     "杯": ("はい", "ぱい", "ばい"),
    "回": ("かい", "かい", None),     "枚": ("まい", None,   None),
    "台": ("だい", None,   None),     "分": ("ふん", "ぷん", "ぷん"),
    "人": ("にん", None,   None),    "時": ("じ",   None,   None),
    "月": ("がつ", None,   None),    "年": ("ねん", None,   None),
    "歳": ("さい", "さい", None),   "点": ("てん", "てん", None),
    "泊": ("はく", "ぱく", "ぱく"), "発": ("はつ", "ぱつ", "ぱつ"),
    "か月": ("かげつ", "かげつ", None), "ヶ月": ("かげつ", "かげつ", None),
    "週間": ("しゅうかん", "しゅうかん", None),
    "か所": ("かしょ",   "かしょ",   None), "ヶ所": ("かしょ", "かしょ", None),
}


def _geminate(reading: str) -> str:
    """Apply consonant gemination before a counter (いち→いっ, ろく→ろっ, はち→はっ, じゅう→じゅっ)."""
    for suffix in ("ち", "く", "う"):
        if reading.endswith(suffix):
            return reading[:-1] + "っ"
    return reading


def counter_reading(numeral_str: str, counter_char: str) -> str | None:
    """Return hiragana reading for N+counter, applying sandhi rules."""
    clean = numeral_str.replace(",", "")
    # Explicit table wins for 1-10
    if counter_char in _COUNTER_TABLE and clean in _COUNTER_TABLE[counter_char]:
        return _COUNTER_TABLE[counter_char][clean]
    if counter_char not in _COUNTER_SANDHI:
        return None
    try:
        n = int(clean)
    except ValueError:
        return None
    if n <= 0 or n >= 100_000_000:
        return None
    num_read = arabic_numeral_reading(clean)
    if not num_read:
        return None
    ones = n % 10
    base, gem, voice_3 = _COUNTER_SANDHI[counter_char]
    if ones in (1, 6, 8) and gem is not None:
        return _geminate(num_read) + gem
    elif ones == 3 and voice_3:
        return num_read + voice_3
    else:
        return num_read + base


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

        def _correct(raw: list[Token]) -> list[Token]:
            raw = self._apply_compound_corrections(raw)
            raw = self._apply_date_corrections(raw)
            raw = self._apply_counter_corrections(raw)
            return raw

        c_tokens = _correct(self._tokenize_raw(text, sudachipy.SplitMode.C))
        a_tokens = (
            _correct(self._tokenize_raw(text, sudachipy.SplitMode.A))
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
            surface = m.surface()
            reading_kata = m.reading_form() or surface

            # Correct SudachiPy's per-digit/mixed reading of numerals
            if re.fullmatch(r'\d{1,3}(?:,\d{3})*|\d+', surface):
                corrected = arabic_numeral_reading(surface.replace(",", ""))
                if corrected is not None:
                    reading_kata = hira_to_kata(corrected)
            elif _LARGE_UNIT_RE.search(surface):
                corrected = mixed_numeral_reading(surface)
                if corrected is not None:
                    reading_kata = hira_to_kata(corrected)

            reading_hira = kata_to_hira(reading_kata)
            tokens.append(Token(
                surface=surface,
                lemma=m.dictionary_form() or surface,
                reading=reading_kata,
                reading_hira=reading_hira,
                pos=pos,
                pos_detail=pos_detail,
                pos_full=tuple(pos_tuple),
                is_content_word=pos in _CONTENT_POS,
            ))
        return tokens

    @staticmethod
    def _apply_compound_corrections(tokens: list[Token]) -> list[Token]:
        """Merge adjacent tokens that form a known compound with corrected reading."""
        if len(tokens) < 2:
            return tokens
        result: list[Token] = []
        i = 0
        while i < len(tokens):
            if i + 1 < len(tokens):
                compound = tokens[i].surface + tokens[i + 1].surface
                if compound in _COMPOUND_CORRECTIONS:
                    r_hira, lemma_str, pos_str = _COMPOUND_CORRECTIONS[compound]
                    r_kata = hira_to_kata(r_hira)
                    result.append(Token(
                        surface=compound,
                        lemma=lemma_str,
                        reading=r_kata,
                        reading_hira=r_hira,
                        pos=pos_str,
                        pos_detail=tokens[i + 1].pos_detail,
                        pos_full=tokens[i + 1].pos_full,
                        is_content_word=True,
                    ))
                    i += 2
                    continue
            result.append(tokens[i])
            i += 1
        return result

    @staticmethod
    def _apply_date_corrections(tokens: list[Token]) -> list[Token]:
        """Merge [numeral] + [日 as date counter] into a single token with native reading."""
        if len(tokens) < 2:
            return tokens
        result: list[Token] = []
        i = 0
        while i < len(tokens):
            if i + 1 < len(tokens):
                t_num = tokens[i]
                t_day = tokens[i + 1]
                is_numeral = re.fullmatch(r'\d{1,3}(?:,\d{3})*|\d+', t_num.surface)
                # 日 appears as 接尾辞/助数詞 (small numbers) or 名詞/助数詞可能 (larger)
                is_day_counter = (
                    t_day.surface == "日"
                    and t_day.reading_hira in ("にち", "か")
                    and (
                        t_day.pos == "接尾辞"
                        or (t_day.pos == "名詞" and len(t_day.pos_full) > 2
                            and t_day.pos_full[2] == "助数詞可能")
                    )
                )
                if is_numeral and is_day_counter:
                    r_hira = _date_reading(t_num.surface)
                    if r_hira is not None:
                        result.append(Token(
                            surface=t_num.surface + "日",
                            lemma=t_num.surface + "日",
                            reading=hira_to_kata(r_hira),
                            reading_hira=r_hira,
                            pos="名詞",
                            pos_detail=t_num.pos_detail,
                            pos_full=t_num.pos_full,
                            is_content_word=True,
                        ))
                        i += 2
                        continue
            result.append(tokens[i])
            i += 1
        return result

    @staticmethod
    def _apply_counter_corrections(tokens: list[Token]) -> list[Token]:
        """Merge [numeral] + [接尾辞/助数詞 counter] into a single token with correct sandhi reading."""
        if len(tokens) < 2:
            return tokens
        result: list[Token] = []
        i = 0
        while i < len(tokens):
            if i + 1 < len(tokens):
                t_num = tokens[i]
                t_ctr = tokens[i + 1]
                is_numeral = re.fullmatch(r'\d{1,3}(?:,\d{3})*|\d+', t_num.surface)
                # Counter is接尾辞/助数詞 (冊) or 名詞/助数詞可能 (杯, 階, 回, 台…).
                # Counters already merged by SudachiPy (本, 匹, 個, 枚) won't
                # appear as a split pair so no false-merge risk.
                is_counter = (
                    t_ctr.surface in _COUNTER_SANDHI
                    and t_ctr.pos in ("接尾辞", "名詞")
                )
                if is_numeral and is_counter:
                    r_hira = counter_reading(t_num.surface, t_ctr.surface)
                    if r_hira is not None:
                        result.append(Token(
                            surface=t_num.surface + t_ctr.surface,
                            lemma=t_num.surface + t_ctr.surface,
                            reading=hira_to_kata(r_hira),
                            reading_hira=r_hira,
                            pos="名詞",
                            pos_detail=t_ctr.pos_detail,
                            pos_full=t_ctr.pos_full,
                            is_content_word=True,
                        ))
                        i += 2
                        continue
            result.append(tokens[i])
            i += 1
        return result

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
