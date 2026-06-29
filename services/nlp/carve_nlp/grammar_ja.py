"""
Japanese grammar pattern library.

Detects common N5-N3 grammar patterns over a tokenized sentence and surfaces
them alongside vocabulary. Patterns are intentionally conservative — false
negatives are preferable to over-tagging.

A pattern is a sequence of constraints matched left-to-right against the
token list. Each constraint is one of:
  * lemma="X"            — exact lemma match
  * surface="X"          — exact surface match
  * pos="X"              — exact pos tag match
  * pos_prefix="X"       — pos starts with prefix (e.g. "動詞")
  * any=True             — any token

The library is exported as a list of `Pattern` objects with stable IDs so
clients can mark them known/unknown.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from .tokenizer import Token


@dataclass(frozen=True)
class Constraint:
    lemma: str | None = None
    surface: str | None = None
    pos: str | None = None
    pos_prefix: str | None = None
    any_token: bool = False

    def matches(self, t: Token) -> bool:
        if self.any_token:
            return True
        if self.lemma is not None and t.lemma != self.lemma:
            return False
        if self.surface is not None and t.surface != self.surface:
            return False
        if self.pos is not None and t.pos != self.pos:
            return False
        if self.pos_prefix is not None and not t.pos.startswith(self.pos_prefix):
            return False
        return True


@dataclass(frozen=True)
class Pattern:
    id: str                       # stable slug, e.g. "te-iru"
    name: str                     # display, e.g. "～ている"
    jlpt: str                     # "N5"|"N4"|"N3"
    sequence: tuple[Constraint, ...]
    description: str


def _c(**kw: Any) -> Constraint:
    return Constraint(**kw)


# Curated set — 30 patterns spanning N5/N4/N3. Each one is a deliberately
# narrow detector; the goal is precision over recall.
PATTERNS: tuple[Pattern, ...] = (
    # ── N5 ───────────────────────────────────────────────────────────────────
    Pattern("te-iru", "～ている", "N5",
            (_c(surface="て"), _c(lemma="いる")),
            "Progressive / resultant state"),
    Pattern("te-imasu", "～ています", "N5",
            (_c(surface="て"), _c(lemma="います")),
            "Progressive (polite)"),
    Pattern("masu",    "～ます", "N5",
            (_c(pos_prefix="動詞"), _c(lemma="ます")),
            "Polite verb ending"),
    Pattern("masen",   "～ません", "N5",
            (_c(pos_prefix="動詞"), _c(surface="ませ"), _c(lemma="ん")),
            "Negative polite"),
    Pattern("desu",    "～です", "N5",
            (_c(lemma="です"),),
            "Copula (polite)"),
    Pattern("janai",   "～じゃない", "N5",
            (_c(lemma="じゃ"), _c(lemma="ない")),
            "Negative copula"),
    Pattern("tai",     "～たい", "N5",
            (_c(pos_prefix="動詞"), _c(lemma="たい")),
            "Want to ～"),
    Pattern("nai",     "～ない", "N5",
            (_c(pos_prefix="動詞"), _c(lemma="ない")),
            "Negative plain"),
    Pattern("kara",    "～から", "N5",
            (_c(lemma="から"),),
            "Because / from"),
    Pattern("made",    "～まで", "N5",
            (_c(lemma="まで"),),
            "Until / up to"),
    Pattern("ga-suki", "～が好き", "N5",
            (_c(lemma="が"), _c(lemma="好き")),
            "To like ～"),

    # ── N4 ───────────────────────────────────────────────────────────────────
    Pattern("te-shimau", "～てしまう", "N4",
            (_c(surface="て"), _c(lemma="しまう")),
            "To finish doing / regrettably"),
    Pattern("nakereba-naranai", "～なければならない", "N4",
            (_c(surface="なけれ"), _c(lemma="ば"), _c(lemma="ならない")),
            "Must do"),
    Pattern("nakute-mo-ii", "～なくてもいい", "N4",
            (_c(surface="なく"), _c(lemma="て"), _c(lemma="も"), _c(lemma="いい")),
            "Don't have to ～"),
    Pattern("you-ni-naru", "～ようになる", "N4",
            (_c(lemma="よう"), _c(lemma="に"), _c(lemma="なる")),
            "To reach the point of ～"),
    Pattern("koto-ga-dekiru", "～ことができる", "N4",
            (_c(lemma="こと"), _c(lemma="が"), _c(lemma="できる")),
            "Can do ～"),
    Pattern("rareru-passive", "～られる (passive)", "N4",
            (_c(pos_prefix="動詞"), _c(lemma="られる")),
            "Passive / potential / honorific"),
    Pattern("saseru-causative", "～させる", "N4",
            (_c(pos_prefix="動詞"), _c(lemma="させる")),
            "Causative"),
    Pattern("tara",  "～たら", "N4",
            (_c(pos_prefix="動詞"), _c(lemma="たら")),
            "If / when ～"),
    Pattern("temo",  "～ても", "N4",
            (_c(surface="て"), _c(lemma="も")),
            "Even if ～"),
    Pattern("te-aru", "～てある", "N4",
            (_c(surface="て"), _c(lemma="ある")),
            "Resultant state (transitive)"),

    # ── N3 ───────────────────────────────────────────────────────────────────
    Pattern("you-ni",  "～ように", "N3",
            (_c(lemma="よう"), _c(lemma="に")),
            "So that / in order to"),
    Pattern("hou-ga-ii", "～ほうがいい", "N3",
            (_c(lemma="ほう"), _c(lemma="が"), _c(lemma="いい")),
            "It is better to ～"),
    Pattern("kamoshirenai", "～かもしれない", "N3",
            (_c(lemma="かも"), _c(lemma="しれる"), _c(lemma="ない")),
            "Might / may"),
    Pattern("you-da",   "～ようだ", "N3",
            (_c(lemma="よう"), _c(lemma="だ")),
            "Seems like / looks like"),
    Pattern("souda-hearsay", "～そうだ (hearsay)", "N3",
            (_c(lemma="そう"), _c(lemma="だ")),
            "I heard that ～"),
    Pattern("tsumori", "～つもり", "N3",
            (_c(lemma="つもり"),),
            "Intend to / plan to"),
    Pattern("bakari",  "～ばかり", "N3",
            (_c(lemma="ばかり"),),
            "Just / nothing but"),
    Pattern("noni",    "～のに", "N3",
            (_c(lemma="のに"),),
            "Even though"),
    Pattern("nagara",  "～ながら", "N3",
            (_c(pos_prefix="動詞"), _c(lemma="ながら")),
            "While doing ～"),
)


def _matches_at(tokens: list[Token], idx: int, p: Pattern) -> bool:
    if idx + len(p.sequence) > len(tokens):
        return False
    for j, c in enumerate(p.sequence):
        if not c.matches(tokens[idx + j]):
            return False
    return True


@dataclass
class DetectedPattern:
    pattern_id: str
    name: str
    jlpt: str
    start: int            # token index
    length: int           # token span


def detect_patterns(tokens: list[Token]) -> list[DetectedPattern]:
    """
    Run all patterns over `tokens`, left-to-right, non-overlapping.

    When two patterns could match at the same starting position, the longer
    sequence wins; ties are broken by lower JLPT level (more advanced first)
    so e.g. ～なければならない is preferred over its inner ～ば fragment.
    """
    out: list[DetectedPattern] = []
    n = len(tokens)
    i = 0
    while i < n:
        best: Pattern | None = None
        for p in PATTERNS:
            if _matches_at(tokens, i, p):
                if best is None:
                    best = p
                    continue
                if len(p.sequence) > len(best.sequence):
                    best = p
                elif len(p.sequence) == len(best.sequence) and p.jlpt < best.jlpt:
                    best = p
        if best is not None:
            out.append(DetectedPattern(best.id, best.name, best.jlpt,
                                       start=i, length=len(best.sequence)))
            i += len(best.sequence)
        else:
            i += 1
    return out


def pattern_summary(
    tokens: list[Token],
    known_pattern_ids: set[str],
) -> dict:
    """
    Composite of detect_patterns + comprehension over grammar.

    Returns:
      detected_patterns: list of {id, name, jlpt, start, length}
      grammar_pct: % of detected patterns the user already knows
      total_patterns: distinct detected patterns
      unknown_patterns: list of pattern dicts the user doesn't yet know
    """
    detected = detect_patterns(tokens)
    seen_ids: set[str] = set()
    distinct: list[DetectedPattern] = []
    for d in detected:
        if d.pattern_id in seen_ids:
            continue
        seen_ids.add(d.pattern_id)
        distinct.append(d)

    known = [d for d in distinct if d.pattern_id in known_pattern_ids]
    unknown = [d for d in distinct if d.pattern_id not in known_pattern_ids]

    grammar_pct = 100.0 if not distinct else round(len(known) / len(distinct) * 100.0, 1)

    return {
        "detected_patterns": [
            {"id": d.pattern_id, "name": d.name, "jlpt": d.jlpt,
             "start": d.start, "length": d.length}
            for d in detected
        ],
        "grammar_pct": grammar_pct,
        "total_patterns": len(distinct),
        "unknown_patterns": [
            {"id": d.pattern_id, "name": d.name, "jlpt": d.jlpt}
            for d in unknown
        ],
    }
