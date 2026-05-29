# Browser Extension Architecture

The Carve browser extension is the primary interface for immersion learning. It must work on Chrome, Firefox, and Safari without diverging codebases.

---

## Cross-Browser Strategy

The extension is built against the **WebExtensions API** (MV3) using a unified TypeScript codebase. Browser-specific shims are isolated to a single `browser-compat.ts` module.

| Feature | Chrome | Firefox | Safari |
|---|---|---|---|
| Manifest version | MV3 | MV3 | MV3 (via Safari Web Extension converter) |
| Service worker | ✓ | ✓ (background script) | ✓ |
| `browser` API namespace | via polyfill | native | native |
| `offscreen` API | ✓ | N/A | N/A |
| `declarativeNetRequest` | ✓ | ✓ | ✓ |
| WASM in content scripts | ✓ | ✓ | ✓ |
| IndexedDB | ✓ | ✓ | ✓ |

Build tool: **Vite** with the `@webext-core/vite-plugin` plugin, producing separate zip artifacts for each browser target.

---

## Directory Structure

```
extension/
├── src/
│   ├── background/
│   │   ├── index.ts           # Service worker entry point
│   │   ├── auth.ts            # Token storage, refresh, API auth
│   │   ├── sync-queue.ts      # Offline review event queue
│   │   ├── badge.ts           # Extension badge (due card count)
│   │   └── context-menu.ts    # Right-click menu items
│   │
│   ├── content/
│   │   ├── index.ts           # Content script entry, init sequence
│   │   ├── page-analyzer.ts   # Detect language, page type
│   │   ├── overlay/
│   │   │   ├── injector.ts    # Wrap text nodes with token spans
│   │   │   ├── colorizer.ts   # Apply known/unknown CSS classes
│   │   │   └── styles.ts      # Injected CSS
│   │   ├── popup/
│   │   │   ├── PopupManager.ts # Mount/unmount word popup
│   │   │   ├── Popup.svelte   # Popup UI component
│   │   │   └── CardMiner.ts   # Orchestrate card creation from popup
│   │   ├── video/
│   │   │   ├── SubtitleHook.ts # Platform-specific subtitle capture
│   │   │   ├── platforms/
│   │   │   │   ├── netflix.ts
│   │   │   │   ├── youtube.ts
│   │   │   │   ├── viki.ts
│   │   │   │   └── disney.ts
│   │   │   └── SubtitleRenderer.ts # Re-render subtitles with annotations
│   │   └── tracker/
│   │       └── ImmersionTracker.ts # Active reading time measurement
│   │
│   ├── nlp/
│   │   ├── WasmTokenizer.ts   # Load and call WASM tokenizer
│   │   ├── DictCache.ts       # IndexedDB dictionary cache
│   │   ├── VocabCache.ts      # IndexedDB user vocab (known words)
│   │   ├── ComprehenScorer.ts # Calculate comprehension % for a page
│   │   └── wasm/
│   │       ├── ja/            # Japanese WASM tokenizer bundle
│   │       ├── zh/            # Chinese WASM tokenizer bundle
│   │       ├── ko/            # Korean WASM tokenizer bundle
│   │       └── latin/         # Spanish/French/German/etc. bundle
│   │
│   ├── popup-page/            # Extension toolbar popup (Svelte SPA)
│   │   ├── PopupApp.svelte
│   │   ├── views/
│   │   │   ├── StatsView.svelte
│   │   │   ├── MinerView.svelte
│   │   │   └── SettingsView.svelte
│   │   └── popup.html
│   │
│   ├── options-page/          # Options page (Svelte SPA)
│   │   ├── OptionsApp.svelte
│   │   └── options.html
│   │
│   ├── shared/
│   │   ├── api.ts             # Typed API client (fetch wrapper)
│   │   ├── browser-compat.ts  # Chrome/Firefox/Safari shims
│   │   ├── messages.ts        # Type-safe message bus definitions
│   │   ├── storage.ts         # browser.storage wrappers
│   │   └── types.ts           # Shared TypeScript types
│   │
│   └── manifest.template.json # Templated; build script fills version
│
├── wasm-src/                  # Rust source for WASM tokenizers
│   ├── ja-tokenizer/          # Japanese (SudachiRS-based)
│   ├── zh-tokenizer/          # Chinese (jieba-rs)
│   ├── ko-tokenizer/          # Korean (Rust MeCab-ko port)
│   └── latin-tokenizer/       # Latin scripts (rule-based + dict)
│
├── build/
│   ├── chrome/                # Chrome artifact
│   ├── firefox/               # Firefox artifact
│   └── safari/                # Safari Web Extension wrapper
│
├── vite.config.ts
├── tsconfig.json
└── package.json
```

---

## WASM Tokenizer Architecture

Each WASM module exposes a minimal API:

```typescript
// TypeScript interface for all WASM tokenizer modules
interface WasmTokenizerModule {
  // Initialize with dictionary data (called once, lazy)
  init(dictBytes: Uint8Array): void;

  // Tokenize text; returns JSON string of Token[]
  tokenize(text: string): string;

  // Quick word lookup by exact surface form
  lookup(surface: string): string;  // JSON string of LookupResult
}
```

```rust
// Rust (compiled to WASM) — simplified ja-tokenizer
#[wasm_bindgen]
pub fn init(dict_bytes: &[u8]) { /* load SudachiRS with embedded dict */ }

#[wasm_bindgen]
pub fn tokenize(text: &str) -> String {
    let tokens = tokenizer::analyze(text, Granularity::C); // longest unit
    serde_json::to_string(&tokens).unwrap()
}
```

**Token structure:**
```typescript
interface Token {
  surface: string;      // text as it appears
  lemma: string;        // dictionary form
  reading: string;      // pronunciation (kana / pinyin / romanization)
  pos: string;          // part of speech
  pos_detail: string;   // subcategory
  frequency_rank: number | null;
  is_content_word: boolean;  // false for particles, punctuation
}
```

**Loading strategy:**
1. On first extension install, user selects target language
2. Corresponding WASM module is fetched from CDN and stored in `chrome.storage.local` (5 MB limit) or cached in service worker Cache API
3. WASM is initialized lazily on first content script activation
4. Subsequent pages load WASM from memory (already initialized in content script context)

---

## Content Script Flow

### Initialization

```
content/index.ts:
  1. Check if extension is enabled for this domain
  2. Detect page language (html[lang], meta[http-equiv], heuristics)
  3. If target language matches user config: proceed
  4. Load user's vocabulary cache from IndexedDB (VocabCache)
  5. Initialize WASM tokenizer for detected language
  6. If video page: mount SubtitleHook
  7. If article page: mount PageAnalyzer (reads text nodes)
  8. Mount ImmersionTracker (measures active focus time)
  9. Set up mouse event listener for hover-to-lookup
```

### Text Annotation (Article Mode)

```
PageAnalyzer:
  1. Walk DOM text nodes (exclude scripts, styles, already-processed)
  2. For each text node with > 2 content characters:
     a. Tokenize via WasmTokenizer.tokenize(text)
     b. For each token: lookup user status in VocabCache
     c. Wrap tokens in <span data-carve-token data-status="known|learning|unknown">
     d. Set CSS class for colorization
  3. Throttle: process max 500 tokens per frame (requestIdleCallback)

Colorizer applies CSS:
  --carve-known:    transparent (no highlight)
  --carve-learning: rgba(var(--accent), 0.15)  (faint)
  --carve-unknown:  mapped to frequency band:
    rank 1-1000:    rgba(255, 200, 0, 0.3)    (high frequency = yellow)
    rank 1001-5000: rgba(255, 140, 0, 0.3)    (medium = orange)
    rank 5001+:     rgba(200, 60, 60, 0.3)    (low frequency = red)
```

### Hover Popup

```
PopupManager:
  1. Listen for mouseover / touchstart on [data-carve-token]
  2. On trigger:
     a. Get token surface and lemma from data attributes
     b. Lookup in DictCache (IndexedDB) — < 2ms if cached
     c. If cache miss: POST /nlp/lookup (async, show loading state)
     d. Position popup: prefer above token, flip if near viewport edge
     e. Render Popup.svelte with token data
  3. Popup contains:
     - Reading / pronunciation
     - Pitch accent visualization (Japanese)
     - Top 3 definitions
     - Status badge (unknown / learning / known)
     - Frequency rank
     - "Mine" button (one-click card creation)
     - "Ignore" button (mark as known/ignore)
     - "Examples" tab (sentence examples)
     - "Grammar" tab (morphological breakdown)
  4. Popup dismisses on: click outside, Escape key, scroll > 100px
```

### Card Mining

```
CardMiner (triggered from Popup "Mine" button):
  1. Capture:
     - word_id from lookup result
     - sentence: surrounding sentence text (±2 sentences)
     - source_url: window.location.href
     - source_timestamp: if video, current video.currentTime
  2. POST /cards with capture_audio=true
  3. Show confirmation toast: "Added to [DeckName]"
  4. Update VocabCache: word status = 'learning'
  5. Update token span CSS class to 'learning'
```

---

## Subtitle Hook (Video Mode)

### Platform Detection

```typescript
// Detected by URL pattern
const PLATFORM_PATTERNS = {
  netflix: /netflix\.com\/watch/,
  youtube: /youtube\.com\/watch|youtu\.be\//,
  viki:    /viki\.com\/videos/,
  disney:  /disneyplus\.com\/video/,
  amazon:  /primevideo\.com\/detail/,
};
```

### Netflix Implementation

Netflix uses a proprietary subtitle renderer. The hook intercepts at the CSS/DOM level:

```typescript
// Netflix subtitle elements appear in:
// .player-timedtext-text-container span

const observer = new MutationObserver((mutations) => {
  for (const m of mutations) {
    if (m.type === 'childList') {
      const subtitleEl = document.querySelector('.player-timedtext');
      if (subtitleEl) processSubtitleElement(subtitleEl);
    }
  }
});
observer.observe(document.body, { childList: true, subtree: true });

function processSubtitleElement(el: Element) {
  const text = el.textContent ?? '';
  const tokens = wasmTokenizer.tokenize(text);
  // Replace text content with annotated spans (preserving Netflix layout)
  el.innerHTML = '';
  for (const token of tokens) {
    const span = document.createElement('span');
    span.setAttribute('data-carve-token', 'true');
    span.setAttribute('data-status', vocabCache.getStatus(token.lemma));
    span.setAttribute('data-lemma', token.lemma);
    span.textContent = token.surface;
    el.appendChild(span);
  }
}
```

### YouTube Implementation

YouTube provides `youtube.com/api/timedtext` and renders subtitles in `.ytp-caption-segment`. Additionally, for auto-generated subtitles with word timing, we intercept the timedtext API response to get word-level timestamps:

```typescript
// Intercept XMLHttpRequest to capture word-timing data
const originalOpen = XMLHttpRequest.prototype.open;
XMLHttpRequest.prototype.open = function(method, url) {
  if (typeof url === 'string' && url.includes('timedtext')) {
    this.addEventListener('load', () => captureTimedtext(this.responseText));
  }
  return originalOpen.apply(this, arguments);
};
```

Word-level timing enables: highlight the exact word being spoken as audio plays (karaoke mode).

---

## Immersion Time Tracking

```typescript
class ImmersionTracker {
  private sessionStart: number | null = null;
  private lastActivity: number = Date.now();
  private IDLE_THRESHOLD = 30_000; // 30 seconds

  constructor(private lang: string) {
    // Page focus/blur tracking
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) this.pause();
      else this.resume();
    });

    // Activity signals (scroll, click, keypress)
    ['scroll', 'click', 'keypress', 'mousemove'].forEach(event => {
      document.addEventListener(event, () => this.recordActivity(), { passive: true });
    });

    // Flush on page unload
    window.addEventListener('beforeunload', () => this.flush());

    // Periodic flush every 30s
    setInterval(() => this.maybeFlush(), 30_000);
  }

  private resume() { this.sessionStart = Date.now(); }

  private pause() { this.flush(); this.sessionStart = null; }

  private recordActivity() {
    this.lastActivity = Date.now();
    if (!this.sessionStart) this.resume();
  }

  private maybeFlush() {
    if (Date.now() - this.lastActivity > this.IDLE_THRESHOLD) {
      this.pause();
    }
  }

  private async flush() {
    if (!this.sessionStart) return;
    const duration = Math.round((Date.now() - this.sessionStart) / 1000);
    if (duration < 5) return;  // ignore micro-sessions
    await chrome.runtime.sendMessage({
      type: 'log_immersion',
      payload: {
        language_code: this.lang,
        session_type: this.detectSessionType(),
        duration_sec: duration,
        started_at: new Date(this.sessionStart).toISOString(),
        url: window.location.href,
      }
    });
    this.sessionStart = null;
  }
}
```

---

## Background Service Worker

The service worker handles:
- **Auth**: Store JWT in `chrome.storage.session` (cleared on browser close), refresh token in `chrome.storage.local` (encrypted)
- **Sync queue**: Offline review events queued in `chrome.storage.local`, flushed when online
- **Badge**: Shows due card count, fetched every 30 minutes
- **Periodic alarm**: Fetch due card count, trigger background sync

```typescript
// Sync queue (offline-first)
chrome.alarms.create('sync', { periodInMinutes: 1 });

chrome.alarms.onAlarm.addListener(async (alarm) => {
  if (alarm.name === 'sync') {
    await flushReviewQueue();
    await flushImmersionQueue();
    await updateBadge();
  }
});
```

---

## Privacy Design

| Data | Where stored | Leaves device? |
|---|---|---|
| Page text | RAM only (content script) | Never |
| Dictionary lookups | IndexedDB cache | Only on cache miss: sent to /nlp/lookup |
| Card mining data | Queued locally, synced to API | Yes (by user action) |
| Immersion time | Queued locally, synced to API | Yes (in aggregate) |
| Review events | Queued locally, synced to API | Yes |

- The extension **never** sends page URLs to the server without user action (the `/nlp/score-content` API is only called when the user explicitly clicks "Score this page")
- Subtitle text is processed entirely by WASM in-browser; never transmitted
- Users can disable all server sync and use the extension in local-only mode (SRS reviews backed up to local IndexedDB only)
