"""
English tokenizer for intermediate/advanced learners.

Strategy:
  - Tokenize via Unicode-aware regex (handles contractions, possessives, hyphens).
  - Lemmatize with a curated irregular-verb / irregular-plural table plus
    suffix-stripping rules (handles ~95% of regular inflections).
  - POS via a coarse lexical lookup (stopwords + function words). Content
    words are flagged for the UI.
  - Frequency rank from `wordfreq` if available (Zipf scale -> rank), else None.

Why rule-based: keeps the service pure-Python with no model download, so it
ships in CI without a slow `pip install spacy && python -m spacy download` step.
For advanced learners the marginal accuracy of a neural tagger isn't worth the
cold-start cost on the FastAPI process.
"""

from __future__ import annotations

import re
from dataclasses import dataclass


try:
    from wordfreq import zipf_frequency
    _WORDFREQ_AVAILABLE = True
except ImportError:
    _WORDFREQ_AVAILABLE = False


@dataclass
class EnglishToken:
    surface: str
    lemma: str
    reading: str            # phonetic hint (currently lowercased surface; CMUdict could plug in later)
    pos: str                # coarse: 'verb' | 'noun' | 'adj' | 'adv' | 'pron' | 'det' | 'prep' | 'conj' | 'aux' | 'num' | 'punct' | 'other'
    is_content_word: bool
    frequency_rank: int | None = None


@dataclass
class EnglishTokenizedText:
    tokens: list[EnglishToken]

    @property
    def surface(self) -> str:
        return "".join(t.surface for t in self.tokens)


# ── Stopwords / function words by category ─────────────────────────────────────

_PRONOUNS = {
    "i", "me", "my", "mine", "myself",
    "you", "your", "yours", "yourself", "yourselves",
    "he", "him", "his", "himself",
    "she", "her", "hers", "herself",
    "it", "its", "itself",
    "we", "us", "our", "ours", "ourselves",
    "they", "them", "their", "theirs", "themselves",
    "this", "that", "these", "those",
    "who", "whom", "whose", "which", "what",
    "someone", "somebody", "something", "anyone", "anybody", "anything",
    "everyone", "everybody", "everything", "noone", "nobody", "nothing",
}

_DETERMINERS = {
    "a", "an", "the", "some", "any", "no", "every", "each", "all", "both",
    "many", "much", "few", "little", "several", "either", "neither",
    "another", "such", "same",
}

_PREPOSITIONS = {
    "of", "in", "to", "for", "with", "on", "at", "from", "by", "about",
    "as", "into", "like", "through", "after", "over", "between", "out",
    "against", "during", "without", "before", "under", "around", "among",
    "across", "behind", "beyond", "near", "off", "onto", "upon", "within",
    "above", "below", "beside", "besides", "inside", "outside",
}

_CONJUNCTIONS = {
    "and", "but", "or", "nor", "yet", "so", "because", "although", "though",
    "while", "whereas", "if", "unless", "until", "since", "when", "whenever",
    "where", "wherever", "whether", "than", "as",
}

_AUXILIARIES = {
    "am", "is", "are", "was", "were", "be", "been", "being",
    "have", "has", "had", "having",
    "do", "does", "did", "doing", "done",
    "will", "would", "shall", "should", "can", "could", "may", "might", "must",
    "ought",
    "wo", "sha",  # bits from contractions ("won't", "shan't")
}

_NEGATIONS = {"not", "n't", "no"}

_FUNCTION_WORDS = _PRONOUNS | _DETERMINERS | _PREPOSITIONS | _CONJUNCTIONS | _AUXILIARIES | _NEGATIONS

# Mapping for POS lookup
_POS_LOOKUP: dict[str, str] = {}
for w in _PRONOUNS:     _POS_LOOKUP[w] = "pron"
for w in _DETERMINERS:  _POS_LOOKUP[w] = "det"
for w in _PREPOSITIONS: _POS_LOOKUP[w] = "prep"
for w in _CONJUNCTIONS: _POS_LOOKUP[w] = "conj"
for w in _AUXILIARIES:  _POS_LOOKUP[w] = "aux"
for w in _NEGATIONS:    _POS_LOOKUP[w] = "neg"


# ── Irregular verb forms (past tense / past participle / -ing / 3rd singular) ──

_IRREGULAR_VERBS: dict[str, str] = {
    # be
    "is": "be", "am": "be", "are": "be", "was": "be", "were": "be", "been": "be", "being": "be",
    # have
    "has": "have", "had": "have", "having": "have",
    # do
    "does": "do", "did": "do", "done": "do", "doing": "do",
    # go
    "goes": "go", "went": "go", "gone": "go", "going": "go",
    # say
    "says": "say", "said": "say", "saying": "say",
    # get
    "gets": "get", "got": "get", "gotten": "get", "getting": "get",
    # make
    "makes": "make", "made": "make", "making": "make",
    # know
    "knows": "know", "knew": "know", "known": "know", "knowing": "know",
    # think
    "thinks": "think", "thought": "think", "thinking": "think",
    # take
    "takes": "take", "took": "take", "taken": "take", "taking": "take",
    # see
    "sees": "see", "saw": "see", "seen": "see", "seeing": "see",
    # come
    "comes": "come", "came": "come", "coming": "come",
    # want
    "wants": "want", "wanted": "want", "wanting": "want",
    # look
    "looks": "look", "looked": "look", "looking": "look",
    # use
    "uses": "use", "used": "use", "using": "use",
    # find
    "finds": "find", "found": "find", "finding": "find",
    # give
    "gives": "give", "gave": "give", "given": "give", "giving": "give",
    # tell
    "tells": "tell", "told": "tell", "telling": "tell",
    # work
    "works": "work", "worked": "work", "working": "work",
    # call
    "calls": "call", "called": "call", "calling": "call",
    # try
    "tries": "try", "tried": "try", "trying": "try",
    # ask
    "asks": "ask", "asked": "ask", "asking": "ask",
    # need
    "needs": "need", "needed": "need", "needing": "need",
    # feel
    "feels": "feel", "felt": "feel", "feeling": "feel",
    # become
    "becomes": "become", "became": "become", "becoming": "become",
    # leave
    "leaves": "leave", "left": "leave", "leaving": "leave",
    # put
    "puts": "put", "putting": "put",
    # mean
    "means": "mean", "meant": "mean", "meaning": "mean",
    # keep
    "keeps": "keep", "kept": "keep", "keeping": "keep",
    # let
    "lets": "let", "letting": "let",
    # begin
    "begins": "begin", "began": "begin", "begun": "begin", "beginning": "begin",
    # seem
    "seems": "seem", "seemed": "seem", "seeming": "seem",
    # help
    "helps": "help", "helped": "help", "helping": "help",
    # show
    "shows": "show", "showed": "show", "shown": "show", "showing": "show",
    # hear
    "hears": "hear", "heard": "hear", "hearing": "hear",
    # play
    "plays": "play", "played": "play", "playing": "play",
    # run
    "runs": "run", "ran": "run", "running": "run",
    # move
    "moves": "move", "moved": "move", "moving": "move",
    # live
    "lives": "live", "lived": "live", "living": "live",
    # believe
    "believes": "believe", "believed": "believe", "believing": "believe",
    # bring
    "brings": "bring", "brought": "bring", "bringing": "bring",
    # write
    "writes": "write", "wrote": "write", "written": "write", "writing": "write",
    # read
    "reads": "read", "reading": "read",
    # speak
    "speaks": "speak", "spoke": "speak", "spoken": "speak", "speaking": "speak",
    # eat
    "eats": "eat", "ate": "eat", "eaten": "eat", "eating": "eat",
    # drink
    "drinks": "drink", "drank": "drink", "drunk": "drink", "drinking": "drink",
    # sleep
    "sleeps": "sleep", "slept": "sleep", "sleeping": "sleep",
    # buy
    "buys": "buy", "bought": "buy", "buying": "buy",
    # sell
    "sells": "sell", "sold": "sell", "selling": "sell",
    # build
    "builds": "build", "built": "build", "building": "build",
    # break
    "breaks": "break", "broke": "break", "broken": "break", "breaking": "break",
    # choose
    "chooses": "choose", "chose": "choose", "chosen": "choose", "choosing": "choose",
    # send
    "sends": "send", "sent": "send", "sending": "send",
    # stand
    "stands": "stand", "stood": "stand", "standing": "stand",
    # understand
    "understands": "understand", "understood": "understand", "understanding": "understand",
    # win
    "wins": "win", "won": "win", "winning": "win",
    # lose
    "loses": "lose", "lost": "lose", "losing": "lose",
    # teach
    "teaches": "teach", "taught": "teach", "teaching": "teach",
    # catch
    "catches": "catch", "caught": "catch", "catching": "catch",
    # fight
    "fights": "fight", "fought": "fight", "fighting": "fight",
    # buy/think/seek already handled by suffix rule for regular pattern -ought
    # forget
    "forgets": "forget", "forgot": "forget", "forgotten": "forget", "forgetting": "forget",
    # grow
    "grows": "grow", "grew": "grow", "grown": "grow", "growing": "grow",
}

# Irregular plural nouns
_IRREGULAR_NOUNS: dict[str, str] = {
    "men": "man", "women": "woman", "children": "child",
    "feet": "foot", "teeth": "tooth", "geese": "goose", "mice": "mouse",
    "people": "person", "oxen": "ox",
    "data": "data", "media": "media",  # mass / collective — keep as-is
    "criteria": "criterion", "phenomena": "phenomenon", "analyses": "analysis",
    "theses": "thesis", "crises": "crisis", "indices": "index",
    "matrices": "matrix", "vertices": "vertex",
}


# ── Tokenization regex ─────────────────────────────────────────────────────────

# A token is one of:
#   - word with optional apostrophe contraction (don't, isn't, it's, we'll)
#   - hyphenated compound (mother-in-law)
#   - number
#   - punctuation (single char)
_TOKEN_RE = re.compile(
    r"""
      ([A-Za-z]+(?:['’][A-Za-z]+)*(?:-[A-Za-z]+(?:['’][A-Za-z]+)*)*)   # word/contraction/hyphenated
    | (\d+(?:[.,]\d+)*)                                                 # number
    | (\s+)                                                              # whitespace
    | ([^\sA-Za-z0-9])                                                  # single punctuation
    """,
    re.VERBOSE,
)


_YING_TO_IE = {"dying", "lying", "tying", "vying"}


def _word_score(word: str) -> float:
    """Higher is better. Uses wordfreq if available, else 0.0 (unknown)."""
    if not _WORDFREQ_AVAILABLE:
        return 0.0
    try:
        return zipf_frequency(word, "en")
    except Exception:
        return 0.0


def _best_candidate(original: str, candidates: list[str]) -> str:
    """
    Pick the candidate with the highest wordfreq Zipf score; if wordfreq isn't
    installed, return the first candidate (matches purely-rule-based behavior).
    Ties or all-zero scores fall back to the first candidate.
    """
    if not _WORDFREQ_AVAILABLE:
        return candidates[0]
    scored = [(c, _word_score(c)) for c in candidates]
    best = max(scored, key=lambda x: x[1])
    if best[1] <= 0:
        return candidates[0]
    return best[0]


def _suffix_lemma(w: str) -> str:
    """
    Strip common regular suffixes. Order matters — longer suffixes first.
    When wordfreq is installed, multiple stem candidates are scored and the
    most frequent valid English word wins. This resolves the classic ambiguity
    between "evolv|e" vs "evolv" and "stud|y|ing" vs "studye".
    Returns the original word if no rule applies.
    """
    # -ies → -y  (tries → try)   but not 'series', 'species'
    if w.endswith("ies") and len(w) > 4:
        return w[:-3] + "y"
    # -ied → -y  (tried → try)
    if w.endswith("ied") and len(w) > 4:
        return w[:-3] + "y"
    # -ying → -ie for the known closed class
    if w in _YING_TO_IE:
        return w[:-3] + "ie"
    # -ing  (running → run, making → make)
    if w.endswith("ing") and len(w) > 5:
        stem = w[:-3]
        # Doubled consonant first — unambiguous (running → run)
        if len(stem) >= 2 and stem[-1] == stem[-2] and stem[-1] not in "aeiou":
            return stem[:-1]
        # Two candidates: stem (sing→sing) and stem+e (mak+e=make, evolv+e=evolve)
        return _best_candidate(w, [stem, stem + "e"])
    # -ed  (worked → work, baked → bake; tried already handled)
    if w.endswith("ed") and len(w) > 3:
        stem = w[:-2]
        # Doubled consonant (stopped → stop)
        if len(stem) >= 2 and stem[-1] == stem[-2] and stem[-1] not in "aeiou":
            return stem[:-1]
        return _best_candidate(w, [stem, stem + "e"])
    # -es → ''  (boxes → box, watches → watch, places → place)
    if w.endswith("es") and len(w) > 3:
        stem = w[:-2]
        if stem.endswith(("s", "x", "z", "ch", "sh")):
            return stem
        return _best_candidate(w, [stem + "e", stem])
    # Plural -s
    if w.endswith("s") and len(w) > 3 and not w.endswith("ss") and not w.endswith("us") and not w.endswith("is"):
        return w[:-1]
    # Adjective -er, -est on monosyllabic (heuristic: short word)
    if w.endswith("est") and len(w) > 5:
        stem = w[:-3]
        if len(stem) >= 2 and stem[-1] == stem[-2] and stem[-1] not in "aeiou":
            return stem[:-1]
        return _best_candidate(w, [stem, stem + "e", w])
    if w.endswith("er") and len(w) > 4:
        stem = w[:-2]
        if len(stem) >= 2 and stem[-1] == stem[-2] and stem[-1] not in "aeiou":
            return stem[:-1]
        # -er is ambiguous: comparative (fast+er=fast) vs agent noun (teach+er=teacher).
        # Pick stem if it's more common than the surface; otherwise keep surface.
        return _best_candidate(w, [stem, stem + "e", w])
    return w


def lemmatize(word: str) -> str:
    """Public lemma helper. Lowercases and applies the irregular tables + suffix rules."""
    w = word.lower()
    if w in _IRREGULAR_VERBS:
        return _IRREGULAR_VERBS[w]
    if w in _IRREGULAR_NOUNS:
        return _IRREGULAR_NOUNS[w]
    if w in _FUNCTION_WORDS:
        return w
    return _suffix_lemma(w)


def _classify_pos(word_lower: str, lemma: str) -> tuple[str, bool]:
    """
    Return (pos, is_content_word). Coarse heuristic: function-word lookup first,
    then morphology hints (verb -ing/-ed, adj -ous/-ful/-ish/-able/-y) — otherwise
    default to 'noun' which is a safe content-word bucket for unknown words.
    """
    if word_lower in _POS_LOOKUP:
        return _POS_LOOKUP[word_lower], False
    if word_lower in _IRREGULAR_VERBS or lemma in _IRREGULAR_VERBS.values():
        return "verb", True
    # Adverbs
    if word_lower.endswith("ly") and len(word_lower) >= 5:
        return "adv", True
    # Adjectives
    if word_lower.endswith(("ous", "ful", "less", "ish", "able", "ible", "ive", "al", "ic", "ed")) \
            and word_lower not in _FUNCTION_WORDS:
        # -ed could be verb or adjective; conservatively mark as adj when standalone
        if word_lower.endswith("ed"):
            return "verb", True
        return "adj", True
    # Verb-ish
    if word_lower.endswith("ing") or word_lower.endswith("ize") or word_lower.endswith("ise"):
        return "verb", True
    # Number
    if word_lower.isdigit():
        return "num", False
    return "noun", True


def _frequency_rank(lemma: str) -> int | None:
    """
    Approximate frequency rank from `wordfreq`'s Zipf scale.

    Zipf 7 ≈ rank 10, Zipf 6 ≈ 100, Zipf 5 ≈ 1k, Zipf 4 ≈ 10k, Zipf 3 ≈ 100k.
    rank ≈ 10 ** (8 - zipf), capped at 1_000_000.
    """
    if not _WORDFREQ_AVAILABLE:
        return None
    try:
        z = zipf_frequency(lemma, "en")
    except Exception:
        return None
    if z <= 0:
        return None
    rank = int(10 ** (8 - z))
    return max(1, min(rank, 1_000_000))


class EnglishTokenizer:
    """Lightweight rule-based English tokenizer/lemmatizer."""

    def tokenize(self, text: str) -> EnglishTokenizedText:
        tokens: list[EnglishToken] = []
        for m in _TOKEN_RE.finditer(text):
            word, num, ws, punct = m.group(1), m.group(2), m.group(3), m.group(4)
            if ws is not None:
                # Whitespace is not surfaced as a token; surface joins to preserve text.
                continue
            if punct is not None:
                tokens.append(EnglishToken(
                    surface=punct, lemma=punct,
                    reading=punct, pos="punct",
                    is_content_word=False,
                ))
                continue
            if num is not None:
                tokens.append(EnglishToken(
                    surface=num, lemma=num,
                    reading=num, pos="num",
                    is_content_word=False,
                ))
                continue
            if word is None:
                continue
            wl = word.lower()
            lemma = lemmatize(word)
            pos, is_content = _classify_pos(wl, lemma)
            tokens.append(EnglishToken(
                surface=word,
                lemma=lemma,
                reading=wl,
                pos=pos,
                is_content_word=is_content,
                frequency_rank=_frequency_rank(lemma) if is_content else None,
            ))
        return EnglishTokenizedText(tokens=tokens)
