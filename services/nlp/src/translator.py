"""
Fluent sentence machine translation via Google Translate.

This produces an actual fluent target-language sentence (e.g.
"los gatos negros duermen" -> "the black cats are sleeping"), as opposed to the
word-by-word gloss the dictionary path builds. It mirrors the audio TTS provider
pattern: env-selectable, best-effort, and it NEVER raises — on any failure it
returns None so the caller falls back to the gloss.

Provider selection (MT_PROVIDER overrides; otherwise inferred):
  - "google_cloud": official Cloud Translation API v2 (needs GOOGLE_TRANSLATE_API_KEY)
  - "google":       key-less translate.googleapis.com endpoint (dev/best-effort,
                    undocumented — not an SLA'd API)
  - "off":          disabled (default)
Inference when MT_PROVIDER is unset/blank: GOOGLE_TRANSLATE_API_KEY set ->
google_cloud; else MT_ENABLED truthy -> google; else off.
"""
from __future__ import annotations

import json
import os
import urllib.parse
import urllib.request

# Carve language code -> Google Translate language code. Google expects the
# mixed-case region tags for Chinese; everything else passes through.
_GOOGLE_LANG = {
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

# Overridable in tests so they never hit the network.
GOOGLE_FREE_URL = "https://translate.googleapis.com/translate_a/single"
GOOGLE_CLOUD_URL = "https://translation.googleapis.com/language/translate/v2"

_TIMEOUT = 8  # seconds
_MAX_CHARS = 5000  # Google's per-request limit; refuse longer up front.


def _google_lang(code: str) -> str | None:
    return _GOOGLE_LANG.get(code.lower())


def _is_truthy(v: str) -> bool:
    return v.strip().lower() in ("1", "true", "yes", "on")


def mt_mode() -> str:
    """Which MT provider to use. See module docstring for the matrix."""
    explicit = os.environ.get("MT_PROVIDER", "").strip().lower()
    if explicit in ("google", "google_cloud", "off"):
        return explicit
    if os.environ.get("GOOGLE_TRANSLATE_API_KEY", "").strip():
        return "google_cloud"
    if _is_truthy(os.environ.get("MT_ENABLED", "")):
        return "google"
    return "off"


def translate_sentence(text: str, source_language: str, target_language: str) -> str | None:
    """
    Return a fluent MT translation of `text`, or None on any failure / when MT
    is disabled / when the language pair is unsupported. Never raises.
    """
    text = text.strip()
    if not text or len(text) > _MAX_CHARS:
        return None

    src = _google_lang(source_language)
    tgt = _google_lang(target_language)
    if not src or not tgt or src == tgt:
        return None

    mode = mt_mode()
    try:
        if mode == "google_cloud":
            return _translate_cloud(text, src, tgt)
        if mode == "google":
            return _translate_free(text, src, tgt)
    except Exception:
        # Best-effort: any network/parse error falls back to the gloss.
        return None
    return None


def _http_post_json(url: str, payload: dict, headers: dict) -> dict:
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    with urllib.request.urlopen(req, timeout=_TIMEOUT) as resp:
        return json.loads(resp.read().decode("utf-8"))


def _translate_cloud(text: str, src: str, tgt: str) -> str | None:
    """Official Cloud Translation API v2 (key-authenticated)."""
    key = os.environ["GOOGLE_TRANSLATE_API_KEY"]
    url = GOOGLE_CLOUD_URL + "?key=" + urllib.parse.quote(key)
    body = {"q": text, "source": src, "target": tgt, "format": "text"}
    out = _http_post_json(url, body, {"Content-Type": "application/json"})
    translations = (out.get("data") or {}).get("translations") or []
    if not translations:
        return None
    translated = translations[0].get("translatedText")
    return translated or None


def _translate_free(text: str, src: str, tgt: str) -> str | None:
    """
    Key-less translate.googleapis.com endpoint. Returns a nested JSON array;
    the translated sentence is the concatenation of segment[0] for each segment
    in result[0].
    """
    params = urllib.parse.urlencode(
        {"client": "gtx", "sl": src, "tl": tgt, "dt": "t", "q": text}
    )
    url = GOOGLE_FREE_URL + "?" + params
    req = urllib.request.Request(
        url, headers={"User-Agent": "Mozilla/5.0 (compatible; CarveBot/1.0)"}
    )
    with urllib.request.urlopen(req, timeout=_TIMEOUT) as resp:
        out = json.loads(resp.read().decode("utf-8"))
    return _parse_free_response(out)


def _parse_free_response(out) -> str | None:
    """Extract the translated text from the key-less endpoint's array shape.

    Shape: [ [ [translated, original, ...], [translated, original, ...] ], ... ].
    Join the first element of each segment. Pure so tests can exercise it.
    """
    if not isinstance(out, list) or not out or not isinstance(out[0], list):
        return None
    parts = []
    for seg in out[0]:
        if isinstance(seg, list) and seg and isinstance(seg[0], str):
            parts.append(seg[0])
    result = "".join(parts).strip()
    return result or None
