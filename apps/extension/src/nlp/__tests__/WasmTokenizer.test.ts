/**
 * Unit tests for WasmTokenizer.ts.
 *
 * Stubs chrome.runtime.getURL and fetch so the tests load the real wasm binary
 * without needing a browser or HTTP server. WebAssembly.instantiate runs the
 * actual compiled module, so tokenization behavior is real, not mocked.
 */
import { describe, it, expect, vi, beforeAll } from 'vitest';
import { fileURLToPath } from 'url';
import { join, dirname } from 'path';
import { readFileSync } from 'fs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const wasmBytes = readFileSync(
  join(__dirname, '../../../wasm-src/ja-tokenizer/pkg/ja_tokenizer_bg.wasm'),
);

// Stub chrome.runtime.getURL — returns a sentinel URL the mocked fetch resolves.
const WASM_SENTINEL = 'https://extension-wasm/ja_tokenizer_bg.wasm';
vi.stubGlobal('chrome', {
  runtime: { getURL: vi.fn(() => WASM_SENTINEL) },
});

// Stub fetch — intercept the sentinel URL and return the real wasm bytes.
vi.stubGlobal('fetch', vi.fn(async (url: string) => {
  if (url === WASM_SENTINEL) {
    return new Response(wasmBytes, {
      status: 200,
      headers: { 'Content-Type': 'application/wasm' },
    });
  }
  throw new Error(`unexpected fetch: ${url}`);
}));

// Dynamically import after stubs are in place so ensureInit() sees them.
let wasmTokenize: (text: string) => Promise<Array<{
  surface: string;
  lemma: string;
  reading: string;
  reading_hira: string;
  pos: string;
  is_content_word: boolean;
  frequency_rank: number | null;
  user_status: 'known' | 'learning' | 'unknown';
}>>;

beforeAll(async () => {
  const mod = await import('../WasmTokenizer');
  wasmTokenize = mod.wasmTokenize;
});

// ── Token shape ────────────────────────────────────────────────────────────────

describe('wasmTokenize — token shape', () => {
  it('returns an array for Japanese text', async () => {
    const tokens = await wasmTokenize('日本語');
    expect(Array.isArray(tokens)).toBe(true);
    expect(tokens.length).toBeGreaterThan(0);
  });

  it('every token has the TypeScript Token fields', async () => {
    const tokens = await wasmTokenize('猫が好き');
    for (const t of tokens) {
      expect(typeof t.surface).toBe('string');
      expect(typeof t.lemma).toBe('string');
      expect(typeof t.reading).toBe('string');       // mapped from WASM reading_kata
      expect(typeof t.reading_hira).toBe('string');
      expect(typeof t.pos).toBe('string');
      expect(typeof t.is_content_word).toBe('boolean');
      expect(t.frequency_rank === null || typeof t.frequency_rank === 'number').toBe(true);
      expect(['known', 'learning', 'unknown']).toContain(t.user_status);
    }
  });

  it('reading field is mapped from WASM reading_kata (katakana for known word)', async () => {
    // 食べる is in the built-in minimal vocab (frequency_rank 287) so has a real reading.
    const tokens = await wasmTokenize('食べる');
    expect(tokens.length).toBeGreaterThan(0);
    const t = tokens.find(tk => tk.frequency_rank !== null) ?? tokens[0];
    // Katakana range: U+30A0–U+30FF
    expect(/[゠-ヿ]/.test(t.reading)).toBe(true);
  });

  it('reading_hira field contains hiragana for known word', async () => {
    // 食べる has reading_hira='たべる' in the built-in vocab.
    const tokens = await wasmTokenize('食べる');
    expect(tokens.length).toBeGreaterThan(0);
    const t = tokens.find(tk => tk.frequency_rank !== null) ?? tokens[0];
    // Hiragana range: U+3040–U+309F
    expect(/[぀-ゟ]/.test(t.reading_hira)).toBe(true);
  });

  it('user_status defaults to unknown (WasmTokenizer has no vocab context)', async () => {
    const tokens = await wasmTokenize('猫');
    expect(tokens[0]?.user_status).toBe('unknown');
  });
});

// ── Content word classification ────────────────────────────────────────────────

describe('wasmTokenize — content word classification', () => {
  it('noun 猫 is a content word', async () => {
    const tokens = await wasmTokenize('猫');
    expect(tokens.some(t => t.is_content_word)).toBe(true);
  });

  it('verb 食べる is a content word', async () => {
    const tokens = await wasmTokenize('食べる');
    const verb = tokens.find(t => t.surface.includes('食') || t.lemma.includes('食'));
    expect(verb?.is_content_word).toBe(true);
  });

  it('particle は is not a content word', async () => {
    const tokens = await wasmTokenize('は');
    expect(tokens.every(t => !t.is_content_word)).toBe(true);
  });
});

// ── Frequency rank ────────────────────────────────────────────────────────────

describe('wasmTokenize — frequency rank', () => {
  it('common verb 食べる has numeric frequency_rank', async () => {
    const tokens = await wasmTokenize('食べる');
    const hasRank = tokens.some(t => typeof t.frequency_rank === 'number');
    expect(hasRank).toBe(true);
  });

  it('ASCII junk has null frequency_rank', async () => {
    const tokens = await wasmTokenize('zzz');
    expect(tokens.every(t => t.frequency_rank === null)).toBe(true);
  });
});

// ── Edge cases ────────────────────────────────────────────────────────────────

describe('wasmTokenize — edge cases', () => {
  it('empty string returns empty array', async () => {
    expect(await wasmTokenize('')).toEqual([]);
  });

  it('ASCII text does not crash', async () => {
    expect(Array.isArray(await wasmTokenize('hello world'))).toBe(true);
  });

  it('multi-token sentence produces ≥2 tokens', async () => {
    const tokens = await wasmTokenize('東京に行く');
    expect(tokens.length).toBeGreaterThanOrEqual(2);
  });

  it('results are idempotent across calls', async () => {
    const a = await wasmTokenize('日本語');
    const b = await wasmTokenize('日本語');
    expect(JSON.stringify(a)).toBe(JSON.stringify(b));
  });
});

// ── Singleton init (ensureInit caching) ───────────────────────────────────────

describe('wasmTokenize — singleton init', () => {
  it('concurrent calls do not double-initialise', async () => {
    const mockFetch = vi.mocked(globalThis.fetch as ReturnType<typeof vi.fn>);
    const callsBefore = mockFetch.mock.calls.length;

    await Promise.all([
      wasmTokenize('日本'),
      wasmTokenize('語'),
      wasmTokenize('猫'),
    ]);

    // fetch must not have been called again — WASM was already initialised
    expect(mockFetch.mock.calls.length).toBe(callsBefore);
  });

  it('fetch was called exactly once total across all tests', () => {
    const mockFetch = vi.mocked(globalThis.fetch as ReturnType<typeof vi.fn>);
    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(mockFetch.mock.calls[0][0]).toBe(WASM_SENTINEL);
  });
});
