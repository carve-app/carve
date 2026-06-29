"""
Multilingual tokenizer for Latin-script European languages + Vietnamese.

Covers: es, de, fr, pt, it, vi.

Strategy (mirrors tokenizer_en.py so the Token interface stays identical):
  - Tokenize via a Unicode-aware regex (handles apostrophes, hyphens, diacritics).
  - Lemmatize with `simplemma` (lightweight, pip-installable, no model download)
    for the languages it supports (es/de/fr/pt/it). Vietnamese is an isolating
    language with essentially no inflection, so its lemma == its lowercased
    surface; simplemma has no `vi` dictionary, so we skip it for vi.
  - Frequency rank from `wordfreq` (Zipf scale -> rank), else None.
  - is_content_word via a per-language function-word (stopword) set.

Why this design: keeps the FastAPI process pure-Python with no heavyweight
model download in CI. simplemma + wordfreq together are a few MB and import
fast, unlike spaCy pipelines.

The exported Token interface matches tokenizer_en.EnglishToken:
    surface, lemma, reading, pos, is_content_word, frequency_rank
"""

from __future__ import annotations

import re
from dataclasses import dataclass


try:
    import simplemma
    from simplemma import lemmatize as _simplemma_lemmatize
    _SIMPLEMMA_AVAILABLE = True
except ImportError:  # pragma: no cover - exercised only without the dep
    _SIMPLEMMA_AVAILABLE = False

try:
    from wordfreq import zipf_frequency
    _WORDFREQ_AVAILABLE = True
except ImportError:  # pragma: no cover - exercised only without the dep
    _WORDFREQ_AVAILABLE = False


# Languages handled by this tokenizer.
SUPPORTED_LANGUAGES = ("es", "de", "fr", "pt", "it", "vi")

# simplemma supports these (vi has no simplemma dictionary).
_SIMPLEMMA_LANGS = frozenset(("es", "de", "fr", "pt", "it"))


@dataclass
class LatinToken:
    surface: str
    lemma: str
    reading: str            # lowercased orthographic form (no phonetics yet)
    pos: str                # coarse: 'func' | 'num' | 'punct' | 'word'
    is_content_word: bool
    frequency_rank: int | None = None


@dataclass
class LatinTokenizedText:
    tokens: list[LatinToken]

    @property
    def surface(self) -> str:
        return "".join(t.surface for t in self.tokens)


# ── Function words / stopwords per language ────────────────────────────────────
#
# These are closed-class words (articles, prepositions, pronouns, conjunctions,
# auxiliaries, common particles). A learner does not mine these as vocabulary, so
# they are flagged is_content_word=False. The lists are intentionally compact —
# they cover the highest-frequency function words, which is what matters for the
# comprehension-percentage heuristic.

_FUNCTION_WORDS: dict[str, frozenset[str]] = {
    "es": frozenset({
        "el", "la", "los", "las", "un", "una", "unos", "unas", "lo", "al", "del",
        "de", "a", "en", "con", "por", "para", "sin", "sobre", "entre", "hasta",
        "desde", "hacia", "según", "tras", "ante", "bajo", "contra",
        "y", "e", "o", "u", "ni", "pero", "sino", "que", "porque", "como", "cuando",
        "si", "aunque", "mientras", "pues",
        "yo", "tú", "él", "ella", "ello", "nosotros", "nosotras", "vosotros",
        "vosotras", "ellos", "ellas", "usted", "ustedes", "me", "te", "se", "nos",
        "os", "le", "les", "mi", "mis", "tu", "tus", "su", "sus", "nuestro",
        "nuestra", "nuestros", "nuestras", "vuestro", "vuestra",
        "este", "esta", "estos", "estas", "ese", "esa", "esos", "esas", "esto",
        "eso", "aquel", "aquella", "aquello",
        "ser", "estar", "haber", "es", "son", "está", "están", "fue", "fueron",
        "era", "han", "ha", "he", "has", "hay", "no", "sí", "más", "muy", "ya",
        "también", "tampoco", "todo", "toda", "todos", "todas",
    }),
    "de": frozenset({
        "der", "die", "das", "den", "dem", "des", "ein", "eine", "einen", "einem",
        "einer", "eines", "kein", "keine",
        "und", "oder", "aber", "denn", "sondern", "doch", "sowie", "weil", "dass",
        "wenn", "als", "ob", "damit", "obwohl", "während",
        "in", "an", "auf", "aus", "bei", "mit", "nach", "von", "zu", "zur", "zum",
        "vor", "über", "unter", "durch", "für", "gegen", "ohne", "um", "am", "im",
        "ins", "hinter", "neben", "zwischen",
        "ich", "du", "er", "sie", "es", "wir", "ihr", "mich", "dich", "sich", "uns",
        "euch", "mir", "dir", "ihm", "ihn", "ihnen", "mein", "meine", "dein",
        "sein", "seine", "unser", "euer",
        "ist", "sind", "war", "waren", "bin", "bist", "sein", "haben", "hat", "hatte",
        "habe", "wird", "werden", "wurde", "kann", "können", "muss", "müssen",
        "soll", "will", "nicht", "auch", "schon", "noch", "nur", "sehr", "mehr",
        "diese", "dieser", "dieses", "dieser", "alle", "dies", "man",
    }),
    "fr": frozenset({
        "le", "la", "les", "un", "une", "des", "du", "de", "au", "aux", "l", "d",
        "à", "en", "dans", "sur", "sous", "avec", "sans", "pour", "par", "chez",
        "vers", "entre", "depuis", "pendant", "contre", "avant", "après",
        "et", "ou", "mais", "donc", "car", "ni", "or", "que", "qui", "parce",
        "comme", "quand", "si", "lorsque", "puisque", "bien",
        "je", "tu", "il", "elle", "on", "nous", "vous", "ils", "elles", "me", "te",
        "se", "lui", "leur", "moi", "toi", "soi", "eux", "mon", "ma", "mes", "ton",
        "ta", "tes", "son", "sa", "ses", "notre", "votre", "nos", "vos", "leurs",
        "ce", "cet", "cette", "ces", "celui", "celle", "ceux",
        "est", "sont", "était", "été", "être", "suis", "es", "sommes", "êtes",
        "avoir", "ai", "as", "a", "avons", "avez", "ont", "avait", "fait", "faire",
        "ne", "pas", "plus", "très", "déjà", "encore", "aussi", "tout", "toute",
        "tous", "toutes", "y",
    }),
    "pt": frozenset({
        "o", "a", "os", "as", "um", "uma", "uns", "umas", "do", "da", "dos", "das",
        "no", "na", "nos", "nas", "ao", "aos", "à", "às", "pelo", "pela", "pelos",
        "pelas", "num", "numa",
        "de", "em", "com", "por", "para", "sem", "sobre", "entre", "até", "desde",
        "contra", "após", "perante", "trás",
        "e", "ou", "mas", "porque", "que", "como", "quando", "se", "embora",
        "enquanto", "pois", "porém", "logo",
        "eu", "tu", "ele", "ela", "nós", "vós", "eles", "elas", "você", "vocês",
        "me", "te", "se", "lhe", "lhes", "nos", "vos", "meu", "minha", "teu", "tua",
        "seu", "sua", "nosso", "nossa", "este", "esta", "esse", "essa", "isto",
        "isso", "aquele", "aquela", "aquilo",
        "ser", "estar", "ter", "haver", "é", "são", "está", "estão", "foi", "foram",
        "era", "tem", "têm", "tinha", "há", "não", "sim", "mais", "muito", "já",
        "também", "todo", "toda", "todos", "todas",
    }),
    "it": frozenset({
        "il", "lo", "la", "i", "gli", "le", "un", "uno", "una", "del", "dello",
        "della", "dei", "degli", "delle", "al", "allo", "alla", "ai", "agli", "alle",
        "dal", "dalla", "nel", "nella", "nei", "negli", "nelle", "sul", "sulla",
        "di", "a", "da", "in", "con", "su", "per", "tra", "fra", "senza", "sopra",
        "sotto", "verso", "presso", "contro", "dopo", "prima",
        "e", "ed", "o", "od", "ma", "però", "perché", "che", "come", "quando", "se",
        "anche", "benché", "mentre", "poiché", "dunque",
        "io", "tu", "lui", "lei", "noi", "voi", "loro", "egli", "ella", "mi", "ti",
        "si", "ci", "vi", "ne", "me", "te", "se", "mio", "mia", "tuo", "tua", "suo",
        "sua", "nostro", "vostro", "questo", "questa", "questi", "queste", "quello",
        "quella", "quelli", "quelle",
        "essere", "avere", "è", "sono", "era", "erano", "ho", "hai", "ha", "abbiamo",
        "avete", "hanno", "fare", "non", "sì", "più", "molto", "già", "tutto",
        "tutta", "tutti", "tutte", "ancora",
    }),
    "vi": frozenset({
        # Vietnamese is isolating; these are common particles/function words.
        "và", "là", "của", "có", "được", "cho", "với", "các", "những", "một",
        "này", "đó", "ấy", "kia", "ở", "trong", "ngoài", "trên", "dưới", "tại",
        "từ", "đến", "về", "theo", "bằng", "như", "thì", "mà", "nên", "nếu",
        "vì", "do", "bởi", "tuy", "nhưng", "hoặc", "hay", "rồi", "đã", "đang",
        "sẽ", "không", "chưa", "rất", "quá", "cũng", "đều", "vẫn", "còn", "lại",
        "tôi", "tao", "tớ", "mình", "bạn", "mày", "nó", "họ", "chúng", "ta",
        "anh", "chị", "em", "ông", "bà", "ai", "gì", "nào", "sao", "đâu", "khi",
        "để", "ra", "vào", "lên", "xuống", "thế", "vậy",
    }),
}

# A token is one of:
#   - word with diacritics, optional apostrophe (l'amour, c'est) and hyphens
#     (avant-garde); apostrophe-split clitics are kept attached to the surface
#     but the lemmatizer strips them where relevant.
#   - number
#   - whitespace
#   - single punctuation char
_TOKEN_RE = re.compile(
    r"""
      ([^\W\d_]+(?:['’][^\W\d_]+)*(?:-[^\W\d_]+(?:['’][^\W\d_]+)*)*)   # word (Unicode letters + diacritics)
    | (\d+(?:[.,]\d+)*)                                                 # number
    | (\s+)                                                             # whitespace
    | (\S)                                                              # single non-letter, non-digit char
    """,
    re.VERBOSE | re.UNICODE,
)


def lemmatize_word(word: str, lang: str) -> str:
    """
    Return the lemma of `word` in `lang`. Lowercases first.

    Uses simplemma for the languages it supports; for Vietnamese (and as a
    fallback when simplemma is unavailable or errors) returns the lowercased
    surface, which is correct for isolating languages.

    Pure function — safe to unit test directly.
    """
    w = word.lower()
    if lang in _SIMPLEMMA_LANGS and _SIMPLEMMA_AVAILABLE:
        try:
            return _simplemma_lemmatize(w, lang=lang)
        except Exception:
            return w
    return w


def is_function_word(word_lower: str, lang: str) -> bool:
    """True if `word_lower` is a closed-class function word in `lang`."""
    return word_lower in _FUNCTION_WORDS.get(lang, frozenset())


def _frequency_rank(word: str, lang: str) -> int | None:
    """
    Approximate frequency rank from `wordfreq`'s Zipf scale.

    rank ≈ 10 ** (8 - zipf), capped at 1_000_000. (Same mapping as the EN
    tokenizer so ranks are comparable across languages.)
    """
    if not _WORDFREQ_AVAILABLE:
        return None
    try:
        z = zipf_frequency(word, lang)
    except Exception:
        return None
    if z <= 0:
        return None
    rank = int(10 ** (8 - z))
    return max(1, min(rank, 1_000_000))


class LatinTokenizer:
    """Multilingual tokenizer/lemmatizer for es/de/fr/pt/it/vi."""

    def __init__(self, lang: str) -> None:
        if lang not in SUPPORTED_LANGUAGES:
            raise ValueError(f"Unsupported language for LatinTokenizer: {lang}")
        self.lang = lang

    def tokenize(self, text: str) -> LatinTokenizedText:
        tokens: list[LatinToken] = []
        lang = self.lang
        for m in _TOKEN_RE.finditer(text):
            word, num, ws, punct = m.group(1), m.group(2), m.group(3), m.group(4)
            if ws is not None:
                continue
            if punct is not None:
                tokens.append(LatinToken(
                    surface=punct, lemma=punct, reading=punct,
                    pos="punct", is_content_word=False,
                ))
                continue
            if num is not None:
                tokens.append(LatinToken(
                    surface=num, lemma=num, reading=num,
                    pos="num", is_content_word=False,
                ))
                continue
            if word is None:
                continue
            wl = word.lower()
            is_func = is_function_word(wl, lang)
            lemma = wl if is_func else lemmatize_word(word, lang)
            is_content = not is_func
            tokens.append(LatinToken(
                surface=word,
                lemma=lemma,
                reading=wl,
                pos="func" if is_func else "word",
                is_content_word=is_content,
                frequency_rank=_frequency_rank(lemma, lang) if is_content else None,
            ))
        return LatinTokenizedText(tokens=tokens)
