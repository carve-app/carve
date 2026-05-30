/**
 * Runtime verification for the Japanese WASM tokenizer.
 *
 * Uses the wasm-pack pkg directly (import.meta.url works in Node ESM).
 * Run with: node test-wasm.mjs
 */
import { createRequire } from 'module';
import { fileURLToPath } from 'url';
import { readFileSync } from 'fs';
import { join, dirname } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));

// Load and initialize the wasm-pack pkg
const pkgDir = join(__dirname, 'wasm-src/ja-tokenizer/pkg');
const { default: init, init: initTokenizer, tokenize, lookup, reading_of }
  = await import(join(pkgDir, 'ja_tokenizer.js'));

// Initialize with explicit wasm path (same pattern as WasmTokenizer.ts).
// Pass as { module_or_path } to avoid wasm-pack deprecation warning.
const wasmBytes = readFileSync(join(pkgDir, 'ja_tokenizer_bg.wasm'));
await init({ module_or_path: wasmBytes });
initTokenizer(new Uint8Array()); // empty dict → built-in minimal vocab

// ── Assertion helper ──────────────────────────────────────────────────────────

let passed = 0;
let failed = 0;

function assert(label, condition, extra = '') {
  if (condition) {
    console.log(`  ✅ ${label}`);
    passed++;
  } else {
    console.error(`  ❌ FAIL: ${label}${extra ? ' — ' + extra : ''}`);
    failed++;
  }
}

function assertEq(label, actual, expected) {
  const ok = JSON.stringify(actual) === JSON.stringify(expected);
  assert(label, ok, `got ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`);
}

// ── Tests ─────────────────────────────────────────────────────────────────────

console.log('\n── tokenize() ────────────────────────────────────────────────────');

{
  const raw = tokenize('日本語');
  const tokens = JSON.parse(raw);
  assert('returns non-empty array for Japanese text', tokens.length > 0, `got ${raw}`);
  const t = tokens[0];
  assert('each token has surface', typeof t.surface === 'string' && t.surface.length > 0);
  assert('each token has lemma', typeof t.lemma === 'string' && t.lemma.length > 0);
  assert('each token has reading_hira', typeof t.reading_hira === 'string');
  assert('each token has reading_kata', typeof t.reading_kata === 'string');
  assert('each token has pos', typeof t.pos === 'string' && t.pos.length > 0);
  assert('each token has is_content_word (boolean)', typeof t.is_content_word === 'boolean');
  assert('frequency_rank is number or null', t.frequency_rank === null || typeof t.frequency_rank === 'number');
}

{
  const tokens = JSON.parse(tokenize('私は学生です'));
  assert('sentence tokenizes to multiple tokens', tokens.length >= 2, `got ${tokens.length}`);
  const surfaces = tokens.map(t => t.surface).join('');
  assert('surfaces reconstruct original (minus spaces)', surfaces.includes('私') && surfaces.includes('学生'));
}

{
  // Function words (particles, auxiliaries) should not be content words
  const tokens = JSON.parse(tokenize('は'));
  if (tokens.length > 0) {
    assert('particle は is not content word', tokens[0].is_content_word === false,
      `is_content_word=${tokens[0].is_content_word}`);
  }
}

{
  // Verb should be a content word
  const tokens = JSON.parse(tokenize('食べる'));
  assert('verb 食べる tokenizes', tokens.length > 0);
  if (tokens.length > 0) {
    assert('verb 食べる is content word', tokens[0].is_content_word === true,
      `is_content_word=${tokens[0].is_content_word}, pos=${tokens[0].pos}`);
  }
}

{
  const tokens = JSON.parse(tokenize(''));
  assertEq('empty string → empty array', tokens, []);
}

{
  const tokens = JSON.parse(tokenize('hello world'));
  assert('ASCII passthrough does not crash', Array.isArray(tokens));
}

{
  const tokens = JSON.parse(tokenize('東京に行く'));
  assert('mixed content (noun + particle + verb) tokenizes', tokens.length >= 2);
  const lemmas = tokens.map(t => t.lemma);
  assert('東京 recognized as lemma', lemmas.some(l => l.includes('東京') || l.includes('東')));
}

console.log('\n── reading_of() ──────────────────────────────────────────────────');

{
  const r = reading_of('日本語');
  assert('reading_of returns non-empty string', typeof r === 'string' && r.length > 0, `got "${r}"`);
  assert('reading_of returns hiragana/katakana', /[぀-ヿ]/.test(r), `got "${r}"`);
}

{
  const r = reading_of('');
  assertEq('reading_of empty string', r, '');
}

console.log('\n── lookup() ──────────────────────────────────────────────────────');

{
  const raw = lookup('食べる');
  assert('lookup returns string', typeof raw === 'string');
  // Result is JSON null or a Token object
  const result = JSON.parse(raw);
  assert('lookup result is null or object', result === null || typeof result === 'object');
}

{
  // lookup() tokenizes the input and returns the first token — never null for non-empty input.
  // Unknown/unseen words get frequency_rank: null.
  const raw = lookup('zzzznotaword');
  const result = JSON.parse(raw);
  assert('lookup unknown word returns a token (not null)', result !== null && typeof result === 'object');
  assertEq('lookup unknown word has null frequency_rank', result?.frequency_rank, null);
}

{
  // Known word with frequency data
  const raw = lookup('食べる');
  const result = JSON.parse(raw);
  assert('lookup known verb 食べる returns result', result !== null);
  assert('lookup 食べる has frequency_rank', typeof result?.frequency_rank === 'number',
    `frequency_rank=${result?.frequency_rank}`);
}

console.log('\n── WasmTokenizer token shape ─────────────────────────────────────');

{
  // Verify the token shape matches what WasmTokenizer.ts expects
  const tokens = JSON.parse(tokenize('猫が好きです'));
  if (tokens.length > 0) {
    const t = tokens[0];
    const requiredFields = ['surface', 'lemma', 'reading_hira', 'reading_kata', 'pos', 'is_content_word', 'frequency_rank'];
    for (const field of requiredFields) {
      assert(`token has field '${field}'`, field in t, `fields: ${Object.keys(t).join(', ')}`);
    }
  }
}

{
  // WasmTokenizer.ts maps reading_kata → reading; verify kata exists
  const tokens = JSON.parse(tokenize('猫'));
  if (tokens.length > 0) {
    const t = tokens[0];
    assert('reading_kata is non-empty for kanji', typeof t.reading_kata === 'string' && t.reading_kata.length > 0, `got "${t.reading_kata}"`);
    assert('reading_hira is non-empty for kanji', typeof t.reading_hira === 'string' && t.reading_hira.length > 0, `got "${t.reading_hira}"`);
  }
}

// ── Summary ───────────────────────────────────────────────────────────────────

console.log(`\n${'─'.repeat(60)}`);
console.log(`Results: ${passed} passed, ${failed} failed`);
if (failed > 0) {
  process.exit(1);
}
