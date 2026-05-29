# API Design

Base URL: `https://api.carve.app/v1`

All endpoints require `Authorization: Bearer <jwt>` unless marked `[public]`.  
All request/response bodies are `application/json`.  
Errors follow RFC 9457 Problem Details.

---

## Authentication

### POST /auth/register `[public]`
```json
// Request
{
  "email": "user@example.com",
  "password": "...",
  "display_name": "Alex"
}

// Response 201
{
  "user": { "id": "...", "email": "...", "display_name": "..." },
  "access_token": "eyJ...",
  "expires_in": 900
}
// Refresh token set as HttpOnly cookie
```

### POST /auth/login `[public]`
```json
// Request
{ "email": "user@example.com", "password": "..." }

// Response 200
{
  "access_token": "eyJ...",
  "expires_in": 900,
  "user": { "id": "...", "email": "...", "display_name": "..." }
}
```

### POST /auth/refresh `[public]`
Uses `refresh_token` HttpOnly cookie.
```json
// Response 200
{ "access_token": "eyJ...", "expires_in": 900 }
```

### POST /auth/logout
Revokes current refresh token.
```json
// Response 204 (no body)
```

### GET /auth/oauth/:provider `[public]`
Redirects to OAuth provider (google, github). Callback at `/auth/oauth/:provider/callback`.

---

## Users

### GET /users/me
```json
// Response 200
{
  "id": "uuid",
  "email": "user@example.com",
  "display_name": "Alex",
  "avatar_url": null,
  "subscription": {
    "tier": "learner",
    "status": "active",
    "current_period_end": "2026-06-29T00:00:00Z"
  },
  "created_at": "2026-01-01T00:00:00Z"
}
```

### PATCH /users/me
```json
// Request (any subset)
{ "display_name": "Alex", "avatar_url": "https://..." }

// Response 200: updated user object
```

### DELETE /users/me
Schedules account for deletion in 30 days. Triggers full data export email.
```json
// Response 202
{ "message": "Account scheduled for deletion on 2026-06-28", "export_url": "..." }
```

---

## Language Configuration

### GET /languages `[public]`
```json
// Response 200
{
  "languages": [
    {
      "code": "ja",
      "name_en": "Japanese",
      "name_native": "日本語",
      "script": "cjk",
      "has_spaces": false,
      "wasm_tokenizer": "/wasm/ja-tokenizer.wasm"
    },
    ...
  ]
}
```

### GET /users/me/languages
```json
// Response 200
{
  "languages": [
    {
      "language_code": "ja",
      "is_active": true,
      "native_language": "en",
      "target_retention": 0.90,
      "daily_review_limit": null,
      "stats": {
        "known_words": 1842,
        "cards_due_today": 37,
        "streak_days": 14
      }
    }
  ]
}
```

### POST /users/me/languages
```json
// Request
{
  "language_code": "ja",
  "native_language": "en",
  "target_retention": 0.90,
  "daily_review_limit": 100
}
// Response 201: created language config object
```

### PATCH /users/me/languages/:code
```json
// Request (any subset)
{ "target_retention": 0.85, "daily_review_limit": 50 }
// Response 200: updated config object
```

---

## NLP

### POST /nlp/tokenize
Tokenize text server-side (for languages without WASM tokenizer or batch processing).
```json
// Request
{
  "text": "日本語を勉強しています",
  "language": "ja",
  "include_definitions": true,
  "user_context": true  // will annotate known/unknown status per user
}

// Response 200
{
  "tokens": [
    {
      "surface": "日本語",
      "lemma": "日本語",
      "reading": "にほんご",
      "pitch_accent": "にほんご (LHH)",
      "pos": "noun",
      "frequency_rank": 412,
      "word_id": "uuid",
      "user_status": "known",
      "definitions": [
        { "definition": "Japanese language", "pos": "noun", "source": "jmdict" }
      ]
    },
    {
      "surface": "を",
      "lemma": "を",
      "reading": "を",
      "pos": "particle",
      "frequency_rank": 5,
      "word_id": "uuid",
      "user_status": "known",
      "definitions": []
    },
    ...
  ],
  "comprehension_pct": 85.7,
  "unknown_tokens": ["勉強", "しています"]
}
```

### POST /nlp/lookup
Single word lookup (optimized path, used by extension popup).
```json
// Request
{ "surface": "食べる", "context": "毎日ご飯を食べる", "language": "ja" }

// Response 200
{
  "word_id": "uuid",
  "lemma": "食べる",
  "reading": "たべる",
  "pitch_accent": "たべる (LHL)",
  "pos": "verb (ichidan)",
  "frequency_rank": 287,
  "jlpt_level": "N5",
  "user_status": "learning",
  "definitions": [
    {
      "sense_index": 0,
      "definition": "to eat",
      "pos": "verb",
      "tags": [],
      "source": "jmdict"
    }
  ],
  "examples": [
    {
      "text": "朝ご飯を食べる",
      "translation": "I eat breakfast",
      "audio_url": "https://cdn.carve.app/audio/ja/..."
    }
  ],
  "kanji_components": [
    { "char": "食", "reading": "しょく", "meaning": "eat, food" }
  ]
}
```

### POST /nlp/score-content
Score a URL or text block for comprehension given the current user's vocabulary.
```json
// Request
{ "url": "https://nhk.or.jp/news/html/...", "language": "ja" }
// OR
{ "text": "...", "language": "ja" }

// Response 200
{
  "comprehension_pct": 91.3,
  "total_tokens": 847,
  "unknown_count": 74,
  "top_unknowns": [
    { "lemma": "条約", "frequency_rank": 3241, "definitions": [...] },
    ...
  ],
  "difficulty_estimate": "intermediate",
  "recommended_for": "mining_read"  // 'flow_read', 'mining_read', 'too_hard'
}
```

---

## Cards

### GET /cards
```
Query params:
  language  (required)
  deck_id   (optional)
  status    new|learning|review|relearning|suspended
  due_before ISO8601 datetime
  limit     default 50, max 200
  cursor    opaque pagination cursor
```
```json
// Response 200
{
  "cards": [
    {
      "id": "uuid",
      "card_type": "recognition",
      "front_text": "食べる",
      "front_audio_url": "https://cdn.carve.app/audio/ja/...",
      "back_text": "to eat",
      "sentence": "毎日ご飯を食べる",
      "sentence_audio_url": "...",
      "translation": "I eat rice every day",
      "source_url": "https://nhk.or.jp/...",
      "fsrs_stability": 12.3,
      "fsrs_difficulty": 4.2,
      "fsrs_due": "2026-05-30T08:00:00Z",
      "fsrs_state": "review",
      "fsrs_reps": 8,
      "word": {
        "id": "uuid",
        "lemma": "食べる",
        "reading": "たべる",
        "frequency_rank": 287
      }
    }
  ],
  "total": 142,
  "cursor": "eyJ..."
}
```

### POST /cards
Create a card (typically from extension mining).
```json
// Request
{
  "word_id": "uuid",               // or surface + language for auto-lookup
  "language_code": "ja",
  "card_type": "recognition",
  "sentence": "毎日ご飯を食べる",
  "translation": "I eat rice every day",
  "source_url": "https://nhk.or.jp/...",
  "source_timestamp": 142.5,       // video seconds
  "deck_id": "uuid",               // optional
  "capture_audio": true,           // trigger media service to fetch audio
  "capture_screenshot": true       // trigger media service to grab frame
}

// Response 201: full card object
```

### PATCH /cards/:id
Update card fields (suspend, unsuspend, change deck, edit content).
```json
// Request
{ "suspended": true }
// Response 200: updated card object
```

### DELETE /cards/:id
```json
// Response 204
```

### POST /cards/bulk
Bulk create from a deck import or pre-built deck.
```json
// Request
{
  "language_code": "ja",
  "deck_id": "uuid",
  "cards": [{ ... }, { ... }]   // up to 1000 per request
}
// Response 202: { "job_id": "uuid" }  (async; poll /jobs/:id)
```

---

## Review (SRS)

### GET /review/session
Get the next review session for a language.
```json
// Query: language=ja&limit=20&session_type=mixed

// Response 200
{
  "session_id": "uuid",
  "cards": [ /* array of card objects, shuffled/interleaved */ ],
  "due_count": 37,
  "new_count_today": 5,
  "session_stats": {
    "estimated_duration_min": 12,
    "new_cards": 5,
    "review_cards": 32
  }
}
```

### POST /review/events
Submit one or more review results. Designed for batching (submit after every card, or at session end).
```json
// Request
{
  "events": [
    {
      "card_id": "uuid",
      "rating": 3,                // 1=Again 2=Hard 3=Good 4=Easy
      "reviewed_at": "2026-05-29T10:04:33Z",
      "time_taken_ms": 2340
    },
    ...
  ]
}

// Response 200
{
  "processed": 20,
  "updated_cards": [
    {
      "id": "uuid",
      "fsrs_stability": 14.1,
      "fsrs_difficulty": 4.0,
      "fsrs_due": "2026-06-12T10:04:33Z",
      "fsrs_state": "review"
    }
  ]
}
```

### GET /review/forecast
Workload forecast for the next N days.
```json
// Query: language=ja&days=14

// Response 200
{
  "forecast": [
    { "date": "2026-05-29", "due": 37, "new": 5 },
    { "date": "2026-05-30", "due": 18, "new": 5 },
    ...
  ],
  "total_due_next_7_days": 183
}
```

---

## Decks

### GET /decks `[public for is_public=true]`
```
Query: language, is_official, tags, search, sort=downloads|rating|created, limit, cursor
```
```json
// Response 200
{
  "decks": [
    {
      "id": "uuid",
      "name": "JLPT N5 Core Vocabulary",
      "description": "...",
      "language_code": "ja",
      "is_official": true,
      "tags": ["jlpt", "n5", "beginner"],
      "card_count": 800,
      "download_count": 42103,
      "avg_rating": 4.7
    }
  ],
  "total": 234,
  "cursor": "eyJ..."
}
```

### POST /decks
Create a deck.

### PATCH /decks/:id
Update deck metadata (owner only).

### DELETE /decks/:id
Soft delete.

### POST /decks/:id/clone
Clone a public deck into the user's own library for customization.
```json
// Response 201: new deck object with owner = current user
```

### POST /decks/:id/subscribe
Subscribe to a pre-built deck (adds cards to user's review queue).
```json
// Response 201: user_deck_subscription object
```

---

## Library & Content

### GET /library
User's saved content items.
```json
// Query: language, content_type, min_comprehension, max_comprehension, sort, limit, cursor

// Response 200
{
  "items": [
    {
      "id": "uuid",
      "content": {
        "id": "uuid",
        "title": "NHK Web Easy Article",
        "url": "https://nhk.or.jp/...",
        "content_type": "article",
        "thumbnail_url": "..."
      },
      "comprehension_pct": 91.3,
      "unknown_word_count": 12,
      "progress_pct": 0,
      "added_at": "2026-05-29T09:00:00Z"
    }
  ]
}
```

### POST /library
Add a URL to the library. Triggers async content indexing.
```json
// Request
{ "url": "https://nhk.or.jp/...", "language_code": "ja" }

// Response 202
{ "library_item_id": "uuid", "job_id": "uuid" }
```

### PATCH /library/:id
Update progress, notes.

### DELETE /library/:id
Remove from library.

---

## Immersion Log

### GET /immersion/sessions
```
Query: language, date_from, date_to, session_type, limit, cursor
```
```json
// Response 200
{
  "sessions": [
    {
      "id": "uuid",
      "language_code": "ja",
      "session_type": "reading",
      "duration_sec": 1842,
      "words_mined": 12,
      "started_at": "2026-05-29T08:00:00Z",
      "content": { "title": "NHK Article", "url": "..." }
    }
  ]
}
```

### POST /immersion/sessions
Create a manual log entry (for listening/watching done outside the extension).
```json
// Request
{
  "language_code": "ja",
  "session_type": "listening",
  "duration_sec": 3600,
  "started_at": "2026-05-29T07:00:00Z",
  "source": "manual"
}
// Response 201: session object
```

---

## Statistics

### GET /stats/summary
```
Query: language, period=week|month|year|all
```
```json
// Response 200
{
  "language_code": "ja",
  "period": "week",
  "immersion": {
    "total_sec": 18420,
    "reading_sec": 7200,
    "listening_sec": 9000,
    "review_sec": 2220,
    "days_active": 5
  },
  "vocabulary": {
    "known": 1842,
    "mature": 1203,
    "learning": 312,
    "new_this_period": 87
  },
  "reviews": {
    "completed": 483,
    "correct_pct": 91.2,
    "retention_pct": 90.1
  },
  "streak": {
    "current": 14,
    "longest": 31
  }
}
```

### GET /stats/history
Time series for charts.
```
Query: language, metric=known_words|immersion_sec|retention, granularity=day|week, from, to
```
```json
// Response 200
{
  "metric": "known_words",
  "granularity": "day",
  "series": [
    { "date": "2026-05-01", "value": 1720 },
    { "date": "2026-05-02", "value": 1733 },
    ...
  ]
}
```

---

## Export

### POST /export/request
Triggers async generation of a full data export.
```json
// Response 202
{ "job_id": "uuid", "estimated_seconds": 30 }
```

### GET /export/:job_id
```json
// Response 200 (when complete)
{
  "status": "complete",
  "download_url": "https://cdn.carve.app/exports/user-uuid-2026-05-29.zip",
  "expires_at": "2026-05-30T09:00:00Z",
  "size_bytes": 2048400
}
```

Export archive contains:
- `vocabulary.json` — all words and knowledge state
- `cards.json` — all cards with full content
- `review_history.json` — all review events
- `immersion_log.json` — all sessions
- `decks/` — custom deck definitions
- `media/` — all audio and image attachments

---

## WebSocket Events

Endpoint: `wss://api.carve.app/v1/sync`

Used for real-time card sync when multiple clients are active.

### Client → Server

```json
{ "type": "subscribe", "language": "ja" }
{ "type": "review_event", "card_id": "uuid", "rating": 3, "reviewed_at": "..." }
{ "type": "ping" }
```

### Server → Client

```json
{ "type": "card_updated", "card": { "id": "uuid", "fsrs_due": "...", ... } }
{ "type": "cards_added", "cards": [...] }
{ "type": "sync_complete" }
{ "type": "pong" }
```

---

## Error Format (RFC 9457)

```json
{
  "type": "https://api.carve.app/errors/not-found",
  "title": "Card Not Found",
  "status": 404,
  "detail": "No card with ID 'abc123' exists for this user.",
  "instance": "/cards/abc123"
}
```

Common error types:
- `validation-error` (400) — request body failed validation
- `unauthorized` (401) — missing or invalid JWT
- `forbidden` (403) — user lacks permission for the resource
- `not-found` (404)
- `rate-limited` (429) — `Retry-After` header included
- `internal-error` (500)
