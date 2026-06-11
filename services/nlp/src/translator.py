"""
Fluent sentence machine translation via Google Cloud Translation v3 — the
Translation LLM (TLLM) model, which is best-on-market for the languages Carve
supports and auto-detects the source language.

This is the SINGLE translation engine. There is deliberately NO word-by-word
gloss / keyless / corpus fallback: a word-for-word gloss is worse than nothing,
so when translation is unavailable we return None and the card simply has no
translation rather than a bad one.

Auth: Google Cloud service account. Set GOOGLE_APPLICATION_CREDENTIALS to the
service-account JSON path (standard ADC). The project id is taken from the
credentials (or GOOGLE_CLOUD_PROJECT). When credentials are absent the engine is
disabled and translate_sentence() returns None.
"""
from __future__ import annotations

import json
import os
import threading
import urllib.error
import urllib.request

# Carve language code -> BCP-47 code understood by Cloud Translation.
_LANG = {
    "ja": "ja",
    "zh-cn": "zh-CN",
    "zh-tw": "zh-TW",
    "zh": "zh-CN",
    "ko": "ko",
    "en": "en",
    "es": "es",
    "de": "de",
    "fr": "fr",
    "it": "it",
    "pt": "pt",
    "vi": "vi",
}

# v3 TLLM endpoint (global location). Overridable in tests.
TRANSLATE_V3_URL = "https://translate.googleapis.com/v3/projects/{project}/locations/global:translateText"

_TIMEOUT = 15  # seconds
_MAX_CHARS = 5000  # Cloud Translation per-request limit; refuse longer up front.

_SCOPE = "https://www.googleapis.com/auth/cloud-platform"

# Lazily-built, process-wide credentials (thread-safe). google.auth handles
# token refresh internally once we hold the Credentials object.
_lock = threading.Lock()
_creds = None
_project: str | None = None
_init_done = False


def _bcp47(code: str) -> str | None:
    return _LANG.get(code.lower())


def _ensure_creds():
    """Load ADC service-account credentials once. Returns (creds, project) or
    (None, None) when unavailable. Never raises."""
    global _creds, _project, _init_done
    if _init_done:
        return _creds, _project
    with _lock:
        if _init_done:
            return _creds, _project
        _init_done = True
        try:
            import google.auth  # noqa: PLC0415

            creds, project = google.auth.default(scopes=[_SCOPE])
            _creds = creds
            _project = os.environ.get("GOOGLE_CLOUD_PROJECT") or project
        except Exception:
            _creds, _project = None, None
        return _creds, _project


def _token() -> str | None:
    """Return a fresh OAuth2 bearer token, or None if unavailable."""
    creds, _ = _ensure_creds()
    if creds is None:
        return None
    try:
        from google.auth.transport.requests import Request  # noqa: PLC0415

        if not creds.valid:
            creds.refresh(Request())
        return creds.token
    except Exception:
        return None


def is_enabled() -> bool:
    """True when service-account credentials + a project are available."""
    creds, project = _ensure_creds()
    return creds is not None and bool(project)


def translate_sentence(text: str, source_language: str, target_language: str) -> str | None:
    """
    Return a fluent TLLM translation of `text`, or None when translation is
    unavailable (no creds, unsupported language, identical languages, or any
    API error). Never raises, and never returns a degraded gloss.
    """
    text = text.strip()
    if not text or len(text) > _MAX_CHARS:
        return None

    tgt = _bcp47(target_language)
    if not tgt:
        return None
    src = _bcp47(source_language) if source_language else None
    if src and src == tgt:
        return None

    creds, project = _ensure_creds()
    if creds is None or not project:
        return None
    tok = _token()
    if not tok:
        return None

    body = {
        "contents": [text],
        "targetLanguageCode": tgt,
        "mimeType": "text/plain",
        "model": f"projects/{project}/locations/global/models/general/translation-llm",
    }
    # TLLM auto-detects when sourceLanguageCode is omitted; only pin it when we
    # have a confident mapping (avoids mis-tagging hurting quality).
    if src:
        body["sourceLanguageCode"] = src

    url = TRANSLATE_V3_URL.format(project=project)
    try:
        out = _post_json(url, body, tok, project)
    except Exception:
        return None
    return _parse_v3_response(out)


def _post_json(url: str, payload: dict, token: str, project: str) -> dict:
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        headers={
            "Authorization": "Bearer " + token,
            "Content-Type": "application/json",
            "x-goog-user-project": project,
        },
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=_TIMEOUT) as resp:
        return json.loads(resp.read().decode("utf-8"))


def _parse_v3_response(out) -> str | None:
    """Extract translatedText from a v3 translateText response. Pure, so tests
    can exercise it without the network.

    Shape: {"translations": [{"translatedText": "...", ...}]}
    """
    if not isinstance(out, dict):
        return None
    translations = out.get("translations") or []
    if not translations or not isinstance(translations[0], dict):
        return None
    translated = (translations[0].get("translatedText") or "").strip()
    return translated or None
