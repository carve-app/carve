# NLP Pipeline

The NLP pipeline is what separates Carve from Migaku at the most fundamental level. Migaku's parser produces incorrect morpheme splits and wrong furigana — problems that undermine everything built on top. Carve invests heavily in correctness here, with per-language test suites and confidence scoring.

---

## Design Principles

1. **Correctness over coverage** — a word with no annotation is better than a word with wrong annotation. Low-confidence results are flagged to the user.
2. **Language-specific, not generic** — each language gets its own purpose-built tokenizer, not a one-size-fits-all model.
3. **Two execution contexts** — in-browser (WASM, fast, private) and server-side (Python, higher quality, richer features). Server is used for pre-indexing and when WASM isn't available.
4. **Verifiable** — every language has a curated test suite of tricky cases (common errors from Migaku are the starting point).

---

## Pipeline Stages

```
Raw text input
     │
     ▼
1. Language Detection
     │
     ▼
2. Pre-processing (normalization)
     │
     ▼
3. Tokenization / Segmentation
     │
     ▼
4. Morphological Analysis (POS, lemma, reading)
     │
     ▼
5. Dictionary Lookup
     │
     ▼
6. Frequency Annotation
     │
     ▼
7. Difficulty Estimation
     │
     ▼
8. User Vocabulary Overlay (known/unknown status)
     │
     ▼
Annotated token array
```

---

## Stage 1: Language Detection

Used when the user hasn't explicitly configured a language, or for mixed-language content.

```python
from langdetect import detect_langs
from charset_normalizer import from_bytes

def detect_language(text: str) -> list[tuple[str, float]]:
    # Map langdetect codes to BCP-47
    CODE_MAP = {"ja": "ja", "zh-cn": "zh-cn", "zh-tw": "zh-tw",
                "ko": "ko", "es": "es", "de": "de", "fr": "fr", ...}
    results = detect_langs(text)
    return [(CODE_MAP.get(r.lang, r.lang), r.prob) for r in results]
```

For Japanese/Chinese disambiguation (both use CJK characters):
- Count hiragana/katakana characters → if > 0, almost certainly Japanese
- Otherwise: use character frequency models for Simplified vs Traditional Chinese

---

## Stage 2: Pre-processing

```python
import unicodedata

def normalize_text(text: str, language: str) -> str:
    # NFC normalization (canonical composition)
    text = unicodedata.normalize("NFC", text)

    if language == "ja":
        # Convert full-width ASCII to half-width
        text = text.translate(FULLWIDTH_TO_HALFWIDTH)
        # Normalize wave dashes etc.
        text = text.replace("〜", "～").replace("―", "—")

    if language in ("zh-cn", "zh-tw"):
        # For Traditional, optionally convert to Simplified for lookup
        # (controlled by user setting)
        if language == "zh-tw":
            text = opencc.convert(text)  # opencc: Traditional → Simplified

    return text
```

---

## Stage 3–4: Tokenization & Morphological Analysis

### Japanese

**In-browser (WASM)**: A Rust port of Sudachi's dictionary-based tokenizer.

**Server-side (Python)**: SudachiPy with system dictionary (full dictionary, ~200 MB).

SudachiPy provides three segmentation granularities:
- **A (short unit)**: finest granularity, matches grammar analysis needs
- **B (medium unit)**: intermediate
- **C (long unit)**: compound words as single tokens (closest to dictionary form)

Carve uses **C-unit** for vocabulary lookup (matches dictionary entries) and **A-unit** for grammatical breakdown display.

```python
import sudachipy

class JapaneseTokenizer:
    def __init__(self):
        self.dict = sudachipy.Dictionary(dict="full")
        self.tokenizer = self.dict.create()

    def analyze(self, text: str, mode: str = "C") -> list[Token]:
        mode_map = {"A": sudachipy.SplitMode.A,
                    "B": sudachipy.SplitMode.B,
                    "C": sudachipy.SplitMode.C}
        morphemes = self.tokenizer.tokenize(text, mode_map[mode])
        return [
            Token(
                surface=m.surface(),
                lemma=m.dictionary_form(),
                reading=m.reading_form(),           # katakana
                normalized_form=m.normalized_form(),
                pos=m.part_of_speech()[0],          # 名詞, 動詞, 助詞...
                pos_detail=m.part_of_speech()[1],
                is_content_word=m.part_of_speech()[0] not in
                    {"助詞", "助動詞", "記号", "空白"},
            )
            for m in morphemes
        ]
```

**Pitch Accent**: Fetched from the OJAD (Online Japanese Accent Dictionary) database, stored in our `words` table as JSON. Not computed by the tokenizer.

**Furigana generation**:
```python
def generate_furigana(surface: str, reading: str) -> list[FuriganaSpan]:
    # Convert reading from katakana to hiragana
    reading_hira = kata_to_hira(reading)
    # Align kanji spans to their readings using LCS algorithm
    # (handles words with mixed kanji/kana like 食べる)
    return align_reading(surface, reading_hira)
```

The alignment uses a longest-common-subsequence approach that correctly handles:
- Mixed kanji/kana words: `食べる` → `食(た)べる`
- Okurigana: `書き込む` → `書(か)き込(こ)む`
- Pure kanji compounds: `学校` → `学校(がっこう)`

**Correctness test suite (Japanese)** — these are cases where Migaku was reported to fail:

```python
TEST_CASES = [
    # Migaku failure: splits ございません incorrectly
    ("ございません", "ございません", "ご-ざ-い-ま-せ-ん"),
    # Migaku failure: wrong reading for 入って
    ("入って", "はいって", None),
    # Common tricky cases
    ("食べられる", "たべられる", None),   # potential form
    ("勉強しなければならない", None, None),  # long grammar chain
    ("東京都", "とうきょうと", None),    # vs 東京都(みやこ)
    ("生き物", "いきもの", None),        # vs 生(なま)き物
]

def run_correctness_tests(tokenizer):
    for surface, expected_reading, expected_furigana in TEST_CASES:
        tokens = tokenizer.analyze(surface)
        lemma_reading = "".join(t.reading for t in tokens)
        assert hiragana(lemma_reading) == expected_reading, \
            f"FAIL: {surface} → {lemma_reading}, expected {expected_reading}"
```

---

### Chinese (Mandarin)

**In-browser (WASM)**: jieba-rs (Rust port of jieba-python)
**Server-side**: HanLP or jieba-python with custom word frequency dictionary

```python
import jieba
import jieba.posseg as pseg

class ChineseTokenizer:
    def analyze(self, text: str) -> list[Token]:
        words = pseg.cut(text)
        return [
            Token(
                surface=word,
                lemma=word,
                reading=pinyin(word, style=Style.TONE3),  # e.g., "nǐ hǎo"
                pos=flag,
                is_content_word=flag not in {"x", "u", "p", "c", "e"},
            )
            for word, flag in words
        ]
```

Tone annotation stored as both numeric (nǐ=ni3) and diacritic (nǐ) for display flexibility.

---

### Korean

**In-browser (WASM)**: Custom Rust implementation based on MeCab-ko rules
**Server-side**: KoNLPy with Okt (Open Korean Text) analyzer

Korean morphology is highly agglutinative. The key challenge is separating the **stem** from **grammatical suffixes** (particles, endings).

```python
from konlpy.tag import Okt

class KoreanTokenizer:
    def __init__(self):
        self.okt = Okt()

    def analyze(self, text: str) -> list[Token]:
        morphs = self.okt.pos(text, norm=True, stem=True)
        return [
            Token(
                surface=surface,
                lemma=surface,  # Okt stem=True gives lemmatized form
                reading=surface,  # Korean reads as written (no furigana needed)
                pos=pos,
                is_content_word=pos in {"Noun", "Verb", "Adjective", "Adverb"},
            )
            for surface, pos in morphs
        ]
```

---

### Latin-Script Languages (Spanish, French, German, etc.)

**In-browser (WASM)**: Rule-based tokenizer + dictionary lookup (no ML needed)
**Server-side**: spaCy with language-specific models (es_core_news_md, fr_core_news_md, de_core_news_md)

```python
import spacy

MODELS = {
    "es": "es_core_news_md",
    "fr": "fr_core_news_md",
    "de": "de_core_news_md",
    "pt": "pt_core_news_md",
    "it": "it_core_news_md",
}

class LatinTokenizer:
    def __init__(self, language: str):
        self.nlp = spacy.load(MODELS[language])

    def analyze(self, text: str) -> list[Token]:
        doc = self.nlp(text)
        return [
            Token(
                surface=token.text,
                lemma=token.lemma_,
                reading=token.text,  # Latin script reads as written
                pos=token.pos_,
                is_content_word=token.pos_ in
                    {"NOUN", "VERB", "ADJ", "ADV", "PROPN"},
            )
            for token in doc
            if not token.is_space
        ]
```

---

## Stage 5: Dictionary Lookup

### Dictionary Sources

| Language | Primary Dictionary | License | Format |
|---|---|---|---|
| Japanese | JMdict/EDICT | CC BY-SA 4.0 | XML → SQLite |
| Japanese | JMnedict (names) | CC BY-SA 4.0 | XML → SQLite |
| Chinese | CC-CEDICT | CC BY-SA 4.0 | Plain text → SQLite |
| Korean | KDE4 Localization | LGPL | — |
| Multi-lingual | Wiktionary dumps | CC BY-SA 3.0 | XML → SQLite |
| Spanish | dict.cc exports | Custom (educational) | TSV → SQLite |
| Multi-lingual | Tatoeba (examples) | CC BY 2.0 | CSV → SQLite |

### Lookup Strategy

```python
class DictionaryService:
    def lookup(self, lemma: str, language: str,
               target_lang: str = "en") -> LookupResult:
        # 1. Try exact lemma match in primary dictionary
        result = self.db.query(
            "SELECT * FROM word_definitions "
            "WHERE lemma = ? AND language_code = ? AND target_language = ?",
            lemma, language, target_lang
        )
        if result:
            return self._format(result)

        # 2. Try normalized form (for languages with spelling variants)
        normalized = normalize(lemma, language)
        result = self.db.query(..., normalized, ...)
        if result:
            return self._format(result, confidence=0.9)

        # 3. Fall back to Wiktionary
        result = self.wiktionary_lookup(lemma, language)
        if result:
            return self._format(result, confidence=0.7)

        # 4. Return None (no definition found) — do NOT fabricate
        return None
```

**Confidence scoring**:
- 1.0: Exact match in primary dictionary (JMdict, CEDICT)
- 0.9: Normalized form match
- 0.7: Wiktionary match
- < 0.7: Flagged to user as "uncertain" with a visual indicator

Low-confidence definitions are shown with a warning icon in the popup: "This definition is uncertain — please verify." This directly addresses Migaku's problem of showing misleading translations without any warning.

---

## Stage 6: Frequency Annotation

Frequency lists are pre-compiled from large corpora and stored in the `words` table.

| Language | Corpus | Size |
|---|---|---|
| Japanese | BCCWJ (Balanced Corpus of Contemporary Written Japanese) | 100M tokens |
| Chinese | Chinese Web Corpus | 500M tokens |
| Korean | Sejong Corpus | 100M tokens |
| Spanish | SUBTLEX-ES | 41M tokens |
| German | SUBTLEX-DE | 22M tokens |
| French | SUBTLEX-FR | 51M tokens |

Frequency rank is stored as an integer (1 = most frequent). The extension uses frequency bands for color-coding:

```
Band 1: rank 1–500     → core vocabulary
Band 2: rank 501–2000  → high frequency
Band 3: rank 2001–5000 → medium frequency
Band 4: rank 5001+     → low frequency / specialized
```

---

## Stage 7: Difficulty Estimation

Content difficulty is a composite score:

```python
def estimate_difficulty(tokens: list[Token], user_id: str) -> DifficultyScore:
    known_words = fetch_user_known_words(user_id)  # from Redis cache

    content_tokens = [t for t in tokens if t.is_content_word]
    total = len(content_tokens)
    unknown = [t for t in content_tokens if t.lemma not in known_words]

    # Vocabulary coverage
    vocab_coverage = 1 - (len(unknown) / max(total, 1))

    # Frequency penalty: unknown high-frequency words are more concerning
    # than unknown rare words (user should know them but doesn't)
    freq_penalty = sum(
        1 / max(t.frequency_rank, 1) * 1000
        for t in unknown
        if t.frequency_rank and t.frequency_rank < 2000
    ) / max(total, 1)

    # Grammar complexity estimate (approximated by average sentence length
    # and ratio of particles/function words to content words)
    sentence_lengths = get_sentence_lengths(tokens)
    avg_sentence_length = sum(sentence_lengths) / max(len(sentence_lengths), 1)
    grammar_complexity = min(avg_sentence_length / 30, 1.0)

    # Composite score: 0 = very easy, 1 = very hard
    difficulty = (
        (1 - vocab_coverage) * 0.6 +
        freq_penalty * 0.2 +
        grammar_complexity * 0.2
    )

    # Map to comprehension %
    comprehension_pct = vocab_coverage * 100

    # Recommendation
    if comprehension_pct >= 98:
        mode = "flow_read"
    elif comprehension_pct >= 90:
        mode = "mining_read"
    elif comprehension_pct >= 80:
        mode = "study_read"
    else:
        mode = "too_hard"

    return DifficultyScore(
        comprehension_pct=round(comprehension_pct, 1),
        difficulty_score=round(difficulty, 3),
        unknown_count=len(unknown),
        total_content_words=total,
        recommended_mode=mode,
        top_unknown_words=sorted(unknown, key=lambda t: t.frequency_rank or 99999)[:10]
    )
```

---

## WASM Build Pipeline

The Rust tokenizers are compiled to WASM using `wasm-pack`:

```toml
# wasm-src/ja-tokenizer/Cargo.toml
[package]
name = "ja-tokenizer"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
wasm-bindgen = "0.2"
serde = { version = "1", features = ["derive"] }
serde-wasm-bindgen = "0.6"
# Lightweight Sudachi-compatible dict parser (custom, no C deps)
sudachi-lite = { git = "https://github.com/carve-app/sudachi-lite" }
```

```bash
# Build script
cd wasm-src/ja-tokenizer
wasm-pack build --target web --out-dir ../../extension/src/nlp/wasm/ja
```

The compiled WASM module + JS glue is checked into the repo (pre-built) to avoid requiring Rust in CI/CD for the extension build. WASM is rebuilt only when the Rust source changes.

---

## NLP Service API

Internal HTTP API consumed by the Core API (not exposed externally).

```
POST /tokenize      { text, language, granularity }
POST /lookup        { lemma, language, target_lang }
POST /batch-lookup  { lemmas: string[], language, target_lang }
POST /score-text    { text, language, user_vocabulary: string[] }
GET  /health
```

Deployed as a separate service to allow independent scaling (GPU nodes for heavier models in the future).
