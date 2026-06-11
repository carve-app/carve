"""
Carve NLP Service — FastAPI application.

Endpoints:
  POST /tokenize        Tokenize text, optionally overlay user vocabulary status
  POST /lookup          Single word dictionary lookup
  POST /batch-lookup    Multi-word dictionary lookup (one DB round-trip)
  POST /score-text      Comprehension score for text + user vocabulary
  POST /select-sentence Pick the best i+1 candidate sentence for mining
  GET  /health          Health check
"""

from __future__ import annotations

import hmac
import logging
import os
from contextlib import asynccontextmanager
from typing import Annotated

from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel, Field

from .dictionary import DictionaryService
from .grammar_ja import PATTERNS as JA_PATTERNS, pattern_summary
from .scorer import CandidateScore, score_content, select_best_sentence
from .tokenizer import JapaneseTokenizer
from .tokenizer_zh import ChineseTokenizer
from .tokenizer_ko import KoreanTokenizer
from .tokenizer_en import EnglishTokenizer

logger = logging.getLogger(__name__)

_dict_service = DictionaryService()
_ja_tokenizer = JapaneseTokenizer()
_zh_tokenizer = ChineseTokenizer()
_ko_tokenizer = KoreanTokenizer()
_en_tokenizer = EnglishTokenizer()


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Warm up SudachiPy — first tokenize call loads the dictionary into memory
    # and can take 5-30s. Do it at startup so the first user request is instant.
    logger.info("warming up tokenizer...")
    try:
        _ja_tokenizer.tokenize("日本語", mode="C")
        logger.info("tokenizer warm-up complete")
    except Exception as e:
        logger.warning("tokenizer warm-up failed: %s", e)
    yield


app = FastAPI(title="Carve NLP Service", version="0.1.0", lifespan=lifespan)

# Internal-only: requests must carry this header when deployed
_INTERNAL_SECRET = os.environ.get("NLP_INTERNAL_SECRET", "")


def _check_auth(x_internal_secret: str | None) -> None:
    # Constant-time compare so the shared secret can't be recovered byte-by-byte
    # via response-timing. hmac.compare_digest raises on None, hence the guard.
    if _INTERNAL_SECRET and not (
        x_internal_secret and hmac.compare_digest(x_internal_secret, _INTERNAL_SECRET)
    ):
        raise HTTPException(status_code=401, detail="Unauthorized")


# ── Request / Response models ─────────────────────────────────────────────────

class TokenizeRequest(BaseModel):
    text: str = Field(..., max_length=50_000)
    language: str = Field("ja", pattern=r"^[a-z]{2}(-[a-z]{2,4})?$")
    mode: str = Field("C", pattern=r"^[ABC]$")
    include_definitions: bool = False
    known_lemmas: list[str] = []
    learning_lemmas: list[str] = []
    known_pattern_ids: list[str] = []
    include_patterns: bool = False


class TokenOut(BaseModel):
    surface: str
    lemma: str
    reading: str          # katakana
    reading_hira: str     # hiragana
    pos: str
    is_content_word: bool
    user_status: str      # 'known' | 'learning' | 'unknown'
    frequency_rank: int | None
    definitions: list[dict] | None = None


class TokenizeResponse(BaseModel):
    tokens: list[TokenOut]
    comprehension_pct: float | None
    unknown_count: int | None
    recommended_mode: str | None
    detected_patterns: list[dict] | None = None
    grammar_pct: float | None = None
    unknown_patterns: list[dict] | None = None


class LookupRequest(BaseModel):
    surface: str = Field(..., max_length=200)
    language: str = Field("ja", pattern=r"^[a-z]{2}(-[a-z]{2,4})?$")
    target_lang: str = Field("en", max_length=5)
    context: str | None = Field(None, max_length=500)


class LookupResponse(BaseModel):
    lemma: str
    reading: str | None
    frequency_rank: int | None
    jlpt_level: str | None
    definitions: list[dict]
    furigana: list[dict]  # [{text, reading}]
    is_exact_match: bool
    found: bool
    pitch_accent: str | None


class BatchLookupRequest(BaseModel):
    lemmas: list[str] = Field(..., max_length=200)
    language: str = Field("ja", pattern=r"^[a-z]{2}(-[a-z]{2,4})?$")
    target_lang: str = Field("en", max_length=5)


class ScoreRequest(BaseModel):
    text: str = Field(..., max_length=100_000)
    language: str = Field("ja", pattern=r"^[a-z]{2}(-[a-z]{2,4})?$")
    known_lemmas: list[str] = []
    learning_lemmas: list[str] = []


class ScoreResponse(BaseModel):
    comprehension_pct: float
    difficulty_score: float
    total_content_words: int
    unknown_count: int
    recommended_mode: str
    top_unknown_lemmas: list[str]


# ── Endpoints ─────────────────────────────────────────────────────────────────

@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "nlp", "version": "0.1.0"}


@app.get("/grammar/patterns")
def list_grammar_patterns(language: str = "ja") -> dict:
    """List all grammar patterns the detector knows about. Used by Settings
    so the user can mark patterns as known/unknown."""
    if language != "ja":
        raise HTTPException(status_code=422, detail=f"Grammar patterns not yet supported for '{language}'")
    return {
        "patterns": [
            {"id": p.id, "name": p.name, "jlpt": p.jlpt, "description": p.description}
            for p in JA_PATTERNS
        ],
    }


@app.post("/tokenize", response_model=TokenizeResponse)
def tokenize(
    req: TokenizeRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> TokenizeResponse:
    _check_auth(x_internal_secret)

    known = set(req.known_lemmas)
    learning = set(req.learning_lemmas)

    if req.language == "ja":
        result = _ja_tokenizer.tokenize(req.text, mode=req.mode)
        raw_tokens = result.tokens
    elif req.language in ("zh-cn", "zh-tw", "zh"):
        zh_result = _zh_tokenizer.tokenize(req.text)
        raw_tokens = zh_result.tokens
    elif req.language == "ko":
        ko_result = _ko_tokenizer.tokenize(req.text)
        raw_tokens = ko_result.tokens
    elif req.language == "en":
        en_result = _en_tokenizer.tokenize(req.text)
        raw_tokens = en_result.tokens
    else:
        raise HTTPException(status_code=422, detail=f"Language '{req.language}' not yet supported")

    token_outs = []
    for t in raw_tokens:
        lemma = t.lemma
        surface = t.surface
        pos = t.pos
        is_content = t.is_content_word
        freq = getattr(t, "frequency_rank", None)

        # Language-specific reading fields
        if req.language == "ja":
            reading = t.reading
            reading_hira = t.reading_hira
        elif req.language in ("zh-cn", "zh-tw", "zh"):
            reading = t.pinyin_num
            reading_hira = t.pinyin
        elif req.language == "ko":
            reading = t.romanization
            reading_hira = t.romanization
        elif req.language == "en":
            # English: 'reading' is the lowercased orthographic form
            reading = t.reading
            reading_hira = t.reading
        else:
            reading = surface
            reading_hira = surface

        status = (
            "known" if lemma in known else
            "learning" if lemma in learning else
            "unknown"
        )
        defs = None
        if req.include_definitions and status == "unknown" and is_content:
            entry = _dict_service.lookup(lemma, language=req.language, target_lang="en")
            if entry:
                defs = [
                    {
                        "sense_index": d.sense_index,
                        "definition": d.definition,
                        "pos": d.part_of_speech,
                        "confidence": d.confidence,
                    }
                    for d in entry.definitions[:3]
                ]
        token_outs.append(TokenOut(
            surface=surface,
            lemma=lemma,
            reading=reading,
            reading_hira=reading_hira,
            pos=pos,
            is_content_word=is_content,
            user_status=status,
            frequency_rank=freq,
            definitions=defs,
        ))

    # Optional grammar layer (JA only for now).
    grammar_payload: dict | None = None
    if req.language == "ja" and req.include_patterns:
        grammar_payload = pattern_summary(raw_tokens, set(req.known_pattern_ids))

    if req.known_lemmas or req.learning_lemmas:
        scored = score_content(raw_tokens, known, learning)
        return TokenizeResponse(
            tokens=token_outs,
            comprehension_pct=scored.comprehension_pct,
            unknown_count=scored.unknown_count,
            recommended_mode=scored.recommended_mode,
            detected_patterns=grammar_payload["detected_patterns"] if grammar_payload else None,
            grammar_pct=grammar_payload["grammar_pct"] if grammar_payload else None,
            unknown_patterns=grammar_payload["unknown_patterns"] if grammar_payload else None,
        )

    return TokenizeResponse(
        tokens=token_outs,
        comprehension_pct=None,
        unknown_count=None,
        recommended_mode=None,
        detected_patterns=grammar_payload["detected_patterns"] if grammar_payload else None,
        grammar_pct=grammar_payload["grammar_pct"] if grammar_payload else None,
        unknown_patterns=grammar_payload["unknown_patterns"] if grammar_payload else None,
    )


@app.post("/lookup", response_model=LookupResponse)
def lookup(
    req: LookupRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> LookupResponse:
    _check_auth(x_internal_secret)

    furigana: list[dict] = []
    lemma = req.surface
    reading: str | None = None
    reading_hira: str | None = None
    pitch_accent: str | None = None

    if req.language == "ja":
        result = _ja_tokenizer.tokenize(req.surface)
        tokens = result.tokens
        content = [t for t in tokens if t.is_content_word]
        canonical = (content[0] if content else tokens[0]) if tokens else None
        if canonical:
            lemma = canonical.lemma
            reading = canonical.reading
            reading_hira = canonical.reading_hira
        if reading_hira:
            spans = _ja_tokenizer.get_furigana(req.surface, reading_hira)
            furigana = [{"text": s.text, "reading": s.reading} for s in spans]
        from .pitch_accent import PITCH_ACCENT
        pitch_accent = PITCH_ACCENT.get(lemma)
    elif req.language in ("zh-cn", "zh-tw", "zh"):
        zh_result = _zh_tokenizer.tokenize(req.surface)
        if zh_result.tokens:
            t = zh_result.tokens[0]
            lemma = t.lemma
            reading = t.pinyin_num
            reading_hira = t.pinyin
        furigana = [{"text": req.surface, "reading": reading_hira or ""}]
    elif req.language == "ko":
        ko_result = _ko_tokenizer.tokenize(req.surface)
        if ko_result.tokens:
            t = ko_result.tokens[0]
            lemma = t.lemma
            reading = t.romanization
            reading_hira = t.romanization
        furigana = [{"text": req.surface, "reading": reading or ""}]
    elif req.language == "en":
        en_result = _en_tokenizer.tokenize(req.surface)
        if en_result.tokens:
            t = en_result.tokens[0]
            lemma = t.lemma
            reading = t.reading
            reading_hira = t.reading
        furigana = [{"text": req.surface, "reading": ""}]
    else:
        raise HTTPException(status_code=422, detail=f"Language '{req.language}' not yet supported")

    entry = _dict_service.lookup(lemma, language=req.language, target_lang=req.target_lang)

    if not entry:
        return LookupResponse(
            lemma=lemma,
            reading=reading,
            frequency_rank=None,
            jlpt_level=None,
            definitions=[],
            furigana=furigana,
            is_exact_match=False,
            found=False,
            pitch_accent=pitch_accent,
        )

    return LookupResponse(
        lemma=entry.lemma,
        reading=entry.reading,
        frequency_rank=entry.frequency_rank,
        jlpt_level=entry.jlpt_level,
        definitions=[
            {
                "sense_index": d.sense_index,
                "definition": d.definition,
                "pos": d.part_of_speech,
                "tags": d.tags,
                "source": d.source,
                "confidence": d.confidence,
            }
            for d in entry.definitions
        ],
        furigana=furigana,
        is_exact_match=entry.is_exact_match,
        found=True,
        pitch_accent=pitch_accent if pitch_accent else entry.pitch_accent,
    )


@app.post("/batch-lookup")
def batch_lookup(
    req: BatchLookupRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> dict:
    _check_auth(x_internal_secret)

    results = _dict_service.batch_lookup(req.lemmas, language=req.language, target_lang=req.target_lang)
    return {
        "results": {
            lemma: (
                {
                    "lemma": entry.lemma,
                    "reading": entry.reading,
                    "frequency_rank": entry.frequency_rank,
                    "definitions": [
                        {"definition": d.definition, "pos": d.part_of_speech}
                        for d in entry.definitions[:3]
                    ],
                }
                if entry else None
            )
            for lemma, entry in results.items()
        }
    }


@app.post("/score-text", response_model=ScoreResponse)
def score_text(
    req: ScoreRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> ScoreResponse:
    _check_auth(x_internal_secret)

    known = set(req.known_lemmas)
    learning = set(req.learning_lemmas)

    if req.language == "ja":
        result = _ja_tokenizer.tokenize(req.text)
        s = score_content(result.tokens, known, learning)
    elif req.language in ("zh-cn", "zh-tw", "zh"):
        zh_result = _zh_tokenizer.tokenize(req.text)
        content = [t for t in zh_result.tokens if t.is_content_word]
        known_ct = sum(1 for t in content if t.lemma in known or t.lemma in learning)
        total = len(content) or 1
        pct = round(known_ct / total * 100, 1)
        from .scorer import ContentScore
        s = ContentScore(
            comprehension_pct=pct,
            difficulty_score=round(1.0 - pct / 100, 2),
            total_content_words=total,
            unknown_count=total - known_ct,
            learning_count=0,
            recommended_mode="mining_read" if pct >= 90 else "study_read" if pct >= 80 else "too_hard",
            top_unknown_lemmas=[t.lemma for t in content if t.lemma not in known and t.lemma not in learning][:10],
        )
    elif req.language == "ko":
        ko_result = _ko_tokenizer.tokenize(req.text)
        content = [t for t in ko_result.tokens if t.is_content_word]
        known_ct = sum(1 for t in content if t.lemma in known or t.lemma in learning)
        total = len(content) or 1
        pct = round(known_ct / total * 100, 1)
        from .scorer import ContentScore
        s = ContentScore(
            comprehension_pct=pct,
            difficulty_score=round(1.0 - pct / 100, 2),
            total_content_words=total,
            unknown_count=total - known_ct,
            learning_count=0,
            recommended_mode="mining_read" if pct >= 90 else "study_read" if pct >= 80 else "too_hard",
            top_unknown_lemmas=[t.lemma for t in content if t.lemma not in known and t.lemma not in learning][:10],
        )
    elif req.language == "en":
        en_result = _en_tokenizer.tokenize(req.text)
        content = [t for t in en_result.tokens if t.is_content_word]
        known_ct = sum(1 for t in content if t.lemma in known or t.lemma in learning)
        total = len(content) or 1
        pct = round(known_ct / total * 100, 1)
        from .scorer import ContentScore
        s = ContentScore(
            comprehension_pct=pct,
            difficulty_score=round(1.0 - pct / 100, 2),
            total_content_words=total,
            unknown_count=total - known_ct,
            learning_count=0,
            recommended_mode="mining_read" if pct >= 90 else "study_read" if pct >= 80 else "too_hard",
            top_unknown_lemmas=[t.lemma for t in content if t.lemma not in known and t.lemma not in learning][:10],
        )
    else:
        raise HTTPException(status_code=422, detail=f"Language '{req.language}' not yet supported")

    return ScoreResponse(
        comprehension_pct=s.comprehension_pct,
        difficulty_score=s.difficulty_score,
        total_content_words=s.total_content_words,
        unknown_count=s.unknown_count,
        recommended_mode=s.recommended_mode,
        top_unknown_lemmas=s.top_unknown_lemmas,
    )


def _tokenize_for_language(text: str, language: str):
    """Tokenize a single sentence in the requested language, returning the
    raw token list (with .lemma, .is_content_word, .frequency_rank)."""
    if language == "ja":
        return _ja_tokenizer.tokenize(text).tokens
    if language in ("zh-cn", "zh-tw", "zh"):
        return _zh_tokenizer.tokenize(text).tokens
    if language == "ko":
        return _ko_tokenizer.tokenize(text).tokens
    if language == "en":
        return _en_tokenizer.tokenize(text).tokens
    raise HTTPException(status_code=422, detail=f"Language '{language}' not yet supported")


class SelectSentenceRequest(BaseModel):
    candidates: list[str] = Field(..., min_length=1, max_length=20)
    target_lemma: str = Field(..., min_length=1, max_length=200)
    language: str = Field("ja", pattern=r"^[a-z]{2}(-[a-z]{2,4})?$")
    known_lemmas: list[str] = []
    learning_lemmas: list[str] = []


class CandidateOut(BaseModel):
    index: int
    text: str
    comprehension_pct: float
    content_word_count: int
    unknown_count: int
    contains_target: bool
    fit_score: float


class SelectSentenceResponse(BaseModel):
    best: CandidateOut | None
    ranked: list[CandidateOut]


def _candidate_to_out(c: CandidateScore) -> CandidateOut:
    return CandidateOut(
        index=c.index,
        text=c.text,
        comprehension_pct=c.comprehension_pct,
        content_word_count=c.content_word_count,
        unknown_count=c.unknown_count,
        contains_target=c.contains_target,
        fit_score=c.fit_score,
    )


@app.post("/select-sentence", response_model=SelectSentenceResponse)
def select_sentence(
    req: SelectSentenceRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> SelectSentenceResponse:
    """
    Pick the best i+1 candidate sentence for mining `target_lemma`.

    The caller (extension content script, web mine form) gathers nearby
    candidate sentences — adjacent subtitle cues, sibling sentences in the
    current paragraph — and we pick the one closest to ~93% comprehension that
    actually contains the target lemma.
    """
    _check_auth(x_internal_secret)

    cleaned = [c.strip() for c in req.candidates if c and c.strip()]
    if not cleaned:
        return SelectSentenceResponse(best=None, ranked=[])

    scored: list[tuple[str, list]] = []
    for text in cleaned:
        tokens = _tokenize_for_language(text, req.language)
        scored.append((text, tokens))

    best, ranked = select_best_sentence(
        scored,
        target_lemma=req.target_lemma,
        known_lemmas=set(req.known_lemmas),
        learning_lemmas=set(req.learning_lemmas),
    )
    return SelectSentenceResponse(
        best=_candidate_to_out(best) if best else None,
        ranked=[_candidate_to_out(c) for c in ranked],
    )


class TranslateRequest(BaseModel):
    # Bound the input like TokenizeRequest/ScoreRequest — a sentence translation
    # is never this long; the cap keeps gloss-building from unbounded work.
    text: str = Field(..., max_length=50_000)
    source_language: str = "ja"
    target_language: str = "en"


class TranslateResponse(BaseModel):
    translation: str | None = None
    source_language: str
    target_language: str


@app.post("/translate", response_model=TranslateResponse)
def translate(
    req: TranslateRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> TranslateResponse:
    """
    Translate a sentence to the target language.

    Strategy (best available, no external network call):
      1. Tatoeba corpus — a real human translation when the sentence (or a
         punctuation-normalized form) is in the imported JA→EN pairs.
      2. Word-gloss fallback — the top definition of each content word, clearly
         bracketed so it reads as a gloss, not a fluent translation.

    Returns None (never a fabricated sentence) when neither is available, so the
    client can leave the field blank rather than show a wrong translation.
    """
    _check_auth(x_internal_secret)

    if not req.text.strip():
        return TranslateResponse(
            translation=None,
            source_language=req.source_language,
            target_language=req.target_language,
        )

    translation: str | None = None

    if req.source_language == "ja":
        # 1) Real corpus translation, if we have one.
        translation = _dict_service.translate_sentence(req.text, req.target_language)

        # 2) Fall back to a word gloss built from the dictionary.
        if not translation:
            gloss_parts: list[str] = []
            result = _ja_tokenizer.tokenize(req.text)
            for tok in result.tokens:
                if not tok.is_content_word:
                    continue
                entry = _dict_service.lookup(
                    tok.lemma,
                    language=req.source_language,
                    target_lang=req.target_language,
                )
                if entry and entry.definitions:
                    top_def = entry.definitions[0].definition
                    gloss_parts.append(f"{tok.surface}[{top_def}]")
            translation = " ".join(gloss_parts) if gloss_parts else None
    # Other source languages: no MT corpus yet → None.

    return TranslateResponse(
        translation=translation,
        source_language=req.source_language,
        target_language=req.target_language,
    )
