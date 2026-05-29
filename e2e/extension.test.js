/**
 * Extension e2e test.
 *
 * Verifies the full content-script pipeline:
 *   content script → background SW → mock API → token spans rendered
 *
 * Also reproduces known regressions:
 *   - Ruby/furigana elements must not be modified (NHK Easy bug)
 *
 * Does NOT require the real backend — a minimal in-process HTTP server
 * handles /v1/nlp/tokenize and /v1/review/due-count.
 *
 * Run: node e2e/extension.test.js
 * Or:  make test-extension
 */

'use strict';

const http = require('http');
const path = require('path');
const { chromium } = require('playwright');

// ── Mock API server ───────────────────────────────────────────────────────────

function makeMockToken(surface, status = 'unknown', rank = 1000) {
  return {
    surface,
    lemma: surface,
    reading: surface,
    reading_hira: surface,
    pos: 'noun',
    is_content_word: true,
    user_status: status,
    frequency_rank: rank,
  };
}

// Main annotation test page
const TEST_PAGE_HTML = `<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>Carve E2E Test Page</title></head>
<body>
  <h1>テスト</h1>
  <p id="para">日本語の研究者たちが新しい人工知能を開発した。</p>
</body>
</html>`;

// Ruby regression test page — mirrors NHK Easy structure
// Before fix: extension walked into <ruby> and split 川柳→川+柳, breaking furigana
const RUBY_PAGE_HTML = `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>Ruby Test</title>
  <style>
    /* Ensure ruby renders normally so screenshots are useful */
    ruby { display: inline; }
    rt { font-size: 0.6em; }
  </style>
</head>
<body>
  <p id="ruby-para">
    <ruby id="ruby1">川柳<rt>せんりゅう</rt></ruby>のニュースです。
  </p>
  <p id="plain-para">
    災害時家庭のセルフケア。
  </p>
</body>
</html>`;

function startMockServer() {
  const server = http.createServer((req, res) => {
    res.setHeader('Access-Control-Allow-Origin', '*');
    res.setHeader('Access-Control-Allow-Headers', '*');
    res.setHeader('Access-Control-Allow-Methods', 'GET,POST,OPTIONS');

    if (req.method === 'OPTIONS') {
      res.writeHead(204);
      res.end();
      return;
    }

    if (req.url === '/test') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(TEST_PAGE_HTML);
      return;
    }

    if (req.url === '/ruby-test') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(RUBY_PAGE_HTML);
      return;
    }

    let body = '';
    req.on('data', c => { body += c; });
    req.on('end', () => {
      if (req.url === '/v1/nlp/tokenize' && req.method === 'POST') {
        let parsed = {};
        try { parsed = JSON.parse(body); } catch {}
        const text = parsed.text || '';

        // Return character-level tokens for any CJK text so the annotator
        // has to handle multi-token cases — this reproduces the ruby split bug.
        const tokens = [...text].flatMap(ch => {
          if (/[぀-ヿ一-鿿]/.test(ch)) {
            return [makeMockToken(ch, 'unknown', 3000)];
          }
          return [];
        });

        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ tokens, comprehension_pct: 0.5 }));
      } else if (req.url.startsWith('/v1/review/due-count')) {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ due_count: 5 }));
      } else {
        res.writeHead(404);
        res.end('{}');
      }
    });
  });

  return new Promise(resolve => {
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      resolve({ server, port });
    });
  });
}

// ── Test helpers ──────────────────────────────────────────────────────────────

function assert(condition, message) {
  if (!condition) throw new Error(`FAIL: ${message}`);
}

// ── Tests ─────────────────────────────────────────────────────────────────────

async function testAnnotation(context, mockBase) {
  console.log('\n── Test 1: basic annotation ─────────────────────────────────');
  const page = await context.newPage();
  await page.goto(`${mockBase}/test`);
  await page.waitForSelector('[data-carve="token"]', { timeout: 15_000 });

  const tokenCount = await page.$$eval('[data-carve="token"]', els => els.length);
  assert(tokenCount >= 3, `expected ≥3 token spans, got ${tokenCount}`);
  console.log(`[assert] ${tokenCount} token spans ✓`);

  const stylesInjected = await page.evaluate(() => !!document.getElementById('carve-styles'));
  assert(stylesInjected, 'expected #carve-styles injected');
  console.log('[assert] #carve-styles injected ✓');

  const borderColor = await page.$eval(
    '[data-carve="token"][data-status="unknown"][data-band]',
    el => window.getComputedStyle(el).borderBottomColor,
  );
  assert(
    borderColor !== 'rgba(0, 0, 0, 0)' && borderColor !== 'transparent',
    `expected colored border, got: ${borderColor}`,
  );
  console.log(`[assert] border color applied: ${borderColor} ✓`);

  await page.screenshot({ path: path.resolve(__dirname, 'extension-e2e.png'), fullPage: true });
  await page.close();
}

async function testRubyPreservation(context, mockBase) {
  console.log('\n── Test 2: ruby/furigana preservation ───────────────────────');
  const page = await context.newPage();
  await page.goto(`${mockBase}/ruby-test`);

  // Wait for the plain paragraph to get annotated (it has no ruby)
  await page.waitForSelector('#plain-para [data-carve="token"]', { timeout: 15_000 });
  console.log('[page] plain paragraph annotated');

  // Ruby element must NOT have been modified
  const rubyInnerHTML = await page.$eval('#ruby1', el => el.innerHTML);
  assert(
    !rubyInnerHTML.includes('data-carve'),
    `ruby element was modified by extension: ${rubyInnerHTML}`,
  );
  console.log('[assert] <ruby> element untouched ✓');

  // The <rt> text must still read correctly
  const rtText = await page.$eval('#ruby1 rt', el => el.textContent);
  assert(rtText === 'せんりゅう', `<rt> text corrupted: "${rtText}"`);
  console.log(`[assert] <rt> text intact: "${rtText}" ✓`);

  // The base ruby text must be intact (no spans injected inside)
  const rubyBase = await page.$eval('#ruby1', el => {
    // textContent of ruby includes rt; get just the first text node
    for (const n of el.childNodes) {
      if (n.nodeType === Node.TEXT_NODE) return n.textContent;
    }
    return null;
  });
  assert(rubyBase === '川柳', `ruby base text corrupted — expected "川柳", got "${rubyBase}"`);
  console.log(`[assert] ruby base text intact: "${rubyBase}" ✓`);

  // The ruby parent paragraph must NOT be marked data-carve="processed"
  const rubyParaProcessed = await page.$eval('#ruby-para', el => el.getAttribute('data-carve'));
  // processed is OK at the paragraph level, but the ruby child must be intact
  // (we only check the ruby element itself wasn't processed)
  const rubyProcessed = await page.$eval('#ruby1', el => el.getAttribute('data-carve'));
  assert(rubyProcessed !== 'processed', `<ruby> itself was marked processed`);
  console.log('[assert] <ruby> not marked as processed ✓');

  await page.screenshot({ path: path.resolve(__dirname, 'extension-ruby-e2e.png'), fullPage: true });
  console.log('[screenshot] saved extension-ruby-e2e.png');
  await page.close();
}

// ── Main ──────────────────────────────────────────────────────────────────────

async function run() {
  const EXTENSION_DIR = path.resolve(__dirname, '../apps/extension/dist');

  const { server, port } = await startMockServer();
  const mockBase = `http://127.0.0.1:${port}`;
  console.log(`[mock-api] listening on ${mockBase}`);

  const userDataDir = require('os').tmpdir() + '/carve-e2e-' + Date.now();
  const context = await chromium.launchPersistentContext(userDataDir, {
    headless: false,
    args: [
      '--headless=new',
      `--disable-extensions-except=${EXTENSION_DIR}`,
      `--load-extension=${EXTENSION_DIR}`,
      '--no-sandbox',
    ],
  });

  try {
    let sw = context.serviceWorkers().find(w => w.url().includes('background'));
    if (!sw) {
      sw = await Promise.race([
        context.waitForEvent('serviceworker', {
          predicate: w => w.url().includes('background'),
          timeout: 10_000,
        }),
        new Promise((_, reject) =>
          setTimeout(() => reject(new Error('Service worker not found after 10s')), 10_000)
        ),
      ]);
    }
    console.log(`[sw] found: ${sw.url()}`);
    await sw.evaluate((base) => { chrome.storage.local.set({ apiBaseUrl: base }); }, mockBase);
    console.log(`[sw] apiBaseUrl set to ${mockBase}`);

    await testAnnotation(context, mockBase);
    await testRubyPreservation(context, mockBase);

    console.log('\n✓ PASS — all tests passed');
  } finally {
    await context.close();
    server.close();
  }
}

run().catch(err => {
  console.error('\n✗ FAIL —', err.message);
  process.exit(1);
});
