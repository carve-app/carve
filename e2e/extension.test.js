/**
 * Extension e2e test.
 *
 * Verifies the full content-script pipeline:
 *   content script → background SW → mock API → token spans rendered
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

const TEST_PAGE_HTML = `<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>Carve E2E Test Page</title></head>
<body>
  <h1>テスト</h1>
  <p id="para">日本語の研究者たちが新しい人工知能を開発した。</p>
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

    // Serve the test page itself
    if (req.url === '/' || req.url === '/test') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(TEST_PAGE_HTML);
      return;
    }

    let body = '';
    req.on('data', c => { body += c; });
    req.on('end', () => {
      if (req.url === '/v1/nlp/tokenize' && req.method === 'POST') {
        const tokens = [
          makeMockToken('日本語', 'unknown', 500),
          makeMockToken('の', 'known', 1),
          makeMockToken('研究者', 'learning', 3200),
          makeMockToken('たち', 'known', 200),
          makeMockToken('が', 'known', 1),
          makeMockToken('新しい', 'known', 400),
          makeMockToken('人工知能', 'unknown', 3840),
          makeMockToken('を', 'known', 1),
          makeMockToken('開発', 'learning', 1500),
          makeMockToken('した', 'known', 50),
        ];
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ tokens, comprehension_pct: 0.6 }));
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

// ── Main ──────────────────────────────────────────────────────────────────────

async function run() {
  const EXTENSION_DIR = path.resolve(__dirname, '../apps/extension/dist');

  // 1. Start mock API
  const { server, port } = await startMockServer();
  const mockBase = `http://127.0.0.1:${port}`;
  console.log(`[mock-api] listening on ${mockBase}`);

  // 2. Launch Chromium with extension loaded
  const userDataDir = require('os').tmpdir() + '/carve-e2e-' + Date.now();
  // Extensions don't load in Playwright's headless mode; use Chrome's --headless=new instead.
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
    // 3. Find service worker and point it at the mock API
    // Give the SW a moment to register if it isn't listed yet
    let sw = context.serviceWorkers().find(w => w.url().includes('background'));
    if (!sw) {
      sw = await Promise.race([
        context.waitForEvent('serviceworker', {
          predicate: w => w.url().includes('background'),
          timeout: 10_000,
        }),
        new Promise((_, reject) =>
          setTimeout(() => reject(new Error('Service worker not found after 10s — extension may not have loaded')), 10_000)
        ),
      ]);
    }
    console.log(`[sw] found: ${sw.url()}`);

    await sw.evaluate((base) => {
      chrome.storage.local.set({ apiBaseUrl: base });
    }, mockBase);
    console.log(`[sw] apiBaseUrl set to ${mockBase}`);

    // 4. Navigate to the test page served over http (content scripts don't fire on about:blank)
    const page = await context.newPage();
    await page.goto(`${mockBase}/test`);

    // 5. Wait for content script to annotate the paragraph
    // The annotator wraps content-word tokens in <span data-carve="token">
    await page.waitForSelector('[data-carve="token"]', { timeout: 15_000 });
    console.log('[page] [data-carve="token"] spans appeared');

    // 6. Assertions
    const tokenCount = await page.$$eval('[data-carve="token"]', els => els.length);
    assert(tokenCount >= 3, `expected ≥3 token spans, got ${tokenCount}`);
    console.log(`[assert] ${tokenCount} token spans ✓`);

    const processedCount = await page.$$eval('[data-carve="processed"]', els => els.length);
    assert(processedCount >= 1, `expected ≥1 processed element, got ${processedCount}`);
    console.log(`[assert] ${processedCount} processed element(s) ✓`);

    // Check styles were injected
    const stylesInjected = await page.evaluate(() => !!document.getElementById('carve-styles'));
    assert(stylesInjected, 'expected #carve-styles to be injected into document.head');
    console.log('[assert] #carve-styles injected ✓');

    // Check unknown words have frequency-band attribute
    const hasUnknown = await page.$('[data-carve="token"][data-status="unknown"][data-band]');
    assert(hasUnknown, 'expected at least one unknown token with data-band set');
    console.log('[assert] data-band on unknown token ✓');

    // Check border color is actually applied (not transparent)
    const borderColor = await page.$eval(
      '[data-carve="token"][data-status="unknown"][data-band]',
      el => window.getComputedStyle(el).borderBottomColor,
    );
    assert(
      borderColor !== 'rgba(0, 0, 0, 0)' && borderColor !== 'transparent',
      `expected colored border on unknown token, got: ${borderColor}`,
    );
    console.log(`[assert] border color applied: ${borderColor} ✓`);

    // 7. Screenshot for visual confirmation
    const screenshotPath = path.resolve(__dirname, 'extension-e2e.png');
    await page.screenshot({ path: screenshotPath, fullPage: true });
    console.log(`[screenshot] saved to ${screenshotPath}`);

    console.log('\n✓ PASS — extension loaded and annotated Japanese text correctly');
  } finally {
    await context.close();
    server.close();
  }
}

run().catch(err => {
  console.error('\n✗ FAIL —', err.message);
  process.exit(1);
});
