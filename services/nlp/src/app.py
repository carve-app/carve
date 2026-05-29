"""
Carve NLP Service — FastAPI application.

Endpoints:
  POST /tokenize        Tokenize text, optionally overlay user vocabulary status
  POST /lookup          Single word dictionary lookup
  POST /batch-lookup    Multi-word dictionary lookup (one DB round-trip)
  POST /score-text      Comprehension score for text + user vocabulary
  GET  /health          Health check
"""

from __future__ import annotations

import os
from typing import Annotated

from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel, Field

from .dictionary import DictionaryService
from .scorer import score_content
from .tokenizer import JapaneseTokenizer

app = FastAPI(title="Carve NLP Service", version="0.1.0")

_dict_service = DictionaryService()
_ja_tokenizer = JapaneseTokenizer()

# Internal-only: requests must carry this header when deployed
_INTERNAL_SECRET = os.environ.get("NLP_INTERNAL_SECRET", "")


def _check_auth(x_internal_secret: str | None) -> None:
    if _INTERNAL_SECRET and x_internal_secret != _INTERNAL_SECRET:
        raise HTTPException(status_code=401, detail="Unauthorized")


# ── Request / Response models ─────────────────────────────────────────────────

class TokenizeRequest(BaseModel):
    text: str = Field(..., max_length=50_000)
    language: str = Field("ja", pattern=r"^[a-z]{2}(-[a-z]{2,4})?$")
    mode: str = Field("C", pattern=r"^[ABC]$")
    include_definitions: bool = False
    known_lemmas: list[str] = []
    learning_lemmas: list[str] = []


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


@app.post("/tokenize", response_model=TokenizeResponse)
def tokenize(
    req: TokenizeRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> TokenizeResponse:
    _check_auth(x_internal_secret)

    if req.language != "ja":
        raise HTTPException(status_code=422, detail=f"Language '{req.language}' not yet supported")

    result = _ja_tokenizer.tokenize(req.text, mode=req.mode)

    known = set(req.known_lemmas)
    learning = set(req.learning_lemmas)

    token_outs = []
    for t in result.tokens:
        status = (
            "known" if t.lemma in known else
            "learning" if t.lemma in learning else
            "unknown"
        )
        defs = None
        if req.include_definitions and status == "unknown" and t.is_content_word:
            entry = _dict_service.lookup(t.lemma, target_lang="en")
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
            surface=t.surface,
            lemma=t.lemma,
            reading=t.reading,
            reading_hira=t.reading_hira,
            pos=t.pos,
            is_content_word=t.is_content_word,
            user_status=status,
            frequency_rank=t.frequency_rank,
            definitions=defs,
        ))

    score = None
    if req.known_lemmas or req.learning_lemmas:
        s = score_content(result.tokens, known, learning)
        return TokenizeResponse(
            tokens=token_outs,
            comprehension_pct=s.comprehension_pct,
            unknown_count=s.unknown_count,
            recommended_mode=s.recommended_mode,
        )

    return TokenizeResponse(
        tokens=token_outs,
        comprehension_pct=None,
        unknown_count=None,
        recommended_mode=None,
    )


@app.post("/lookup", response_model=LookupResponse)
def lookup(
    req: LookupRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> LookupResponse:
    _check_auth(x_internal_secret)

    if req.language != "ja":
        raise HTTPException(status_code=422, detail=f"Language '{req.language}' not yet supported")

    # Tokenize the surface to get the lemma
    result = _ja_tokenizer.tokenize(req.surface)
    tokens = result.tokens
    # Use the first content token's lemma, or first token if all function words
    content = [t for t in tokens if t.is_content_word]
    canonical = (content[0] if content else tokens[0]) if tokens else None

    lemma = canonical.lemma if canonical else req.surface
    reading = canonical.reading if canonical else None
    reading_hira = canonical.reading_hira if canonical else None

    entry = _dict_service.lookup(lemma, target_lang=req.target_lang)

    # Generate furigana for the surface form
    furigana: list[dict] = []
    if reading_hira:
        spans = _ja_tokenizer.get_furigana(req.surface, reading_hira)
        furigana = [{"text": s.text, "reading": s.reading} for s in spans]

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
    )


@app.post("/batch-lookup")
def batch_lookup(
    req: BatchLookupRequest,
    x_internal_secret: Annotated[str | None, Header()] = None,
) -> dict:
    _check_auth(x_internal_secret)

    if req.language != "ja":
        raise HTTPException(status_code=422, detail=f"Language '{req.language}' not yet supported")

    results = _dict_service.batch_lookup(req.lemmas, target_lang=req.target_lang)
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

    if req.language != "ja":
        raise HTTPException(status_code=422, detail=f"Language '{req.language}' not yet supported")

    result = _ja_tokenizer.tokenize(req.text)
    s = score_content(result.tokens, set(req.known_lemmas), set(req.learning_lemmas))

    return ScoreResponse(
        comprehension_pct=s.comprehension_pct,
        difficulty_score=s.difficulty_score,
        total_content_words=s.total_content_words,
        unknown_count=s.unknown_count,
        recommended_mode=s.recommended_mode,
        top_unknown_lemmas=s.top_unknown_lemmas,
    )
