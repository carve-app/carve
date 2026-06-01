/**
 * Extension e2e test.
 *
 * Verifies the full content-script pipeline:
 *   content script → background SW → mock API → token spans rendered
 *
 * Also reproduces known regressions:
 *   - Ruby/furigana elements must not be modified (NHK Easy bug)
 *
 * And verifies the full mining workflow:
 *   click token → popup → Mine form → Save card → POST /v1/cards
 *
 * Does NOT require the real backend — a minimal in-process HTTP server
 * handles /v1/nlp/tokenize, /v1/nlp/lookup, /v1/cards, /v1/review/due-count.
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
// Includes navigation text (症状・病名から探す) to catch cross-segment token
// contamination: if the annotator batches text nodes and misaligns token
// boundaries, navigation text can appear inside the article paragraph.
const RUBY_PAGE_HTML = `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>Ruby Test</title>
  <style>
    ruby { display: inline; }
    rt { font-size: 0.6em; }
  </style>
</head>
<body>
  <nav id="nav">
    <a id="nav-link" href="#">症状・病名から探す</a>
  </nav>
  <p id="ruby-para">
    <ruby id="ruby1">川柳<rt>せんりゅう</rt></ruby>のニュースです。
  </p>
  <p id="plain-para">
    災害時家庭のセルフケア。
  </p>
</body>
</html>`;

function startMockServer() {
  // Capture card creation requests for Test 3 assertions
  const capturedCards = [];

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

        // Return character-level tokens for any CJK text.
        // Every 3rd token gets frequency_rank=null to reproduce the pink-background
        // bug (annotator must still set data-band so the fallback CSS never fires).
        const tokens = [...text].flatMap((ch, i) => {
          if (/[぀-ヿ一-鿿]/.test(ch)) {
            return [makeMockToken(ch, 'unknown', i % 3 === 0 ? null : 3000)];
          }
          return [];
        });

        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ tokens, comprehension_pct: 0.5 }));

      } else if (req.url === '/v1/nlp/lookup' && req.method === 'POST') {
        let parsed = {};
        try { parsed = JSON.parse(body); } catch {}
        const surface = parsed.surface || '';
        // Return a minimal DictEntry so the popup renders correctly
        const entry = {
          surface,
          reading: surface,
          jlpt_level: 'N5',
          frequency_rank: 500,
          pitch_accent: null,
          furigana: [],
          definitions: [
            { pos: 'noun', definition: 'test definition' },
          ],
        };
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ entry }));

      } else if (req.url === '/v1/cards' && req.method === 'POST') {
        let parsed = {};
        try { parsed = JSON.parse(body); } catch {}
        capturedCards.push(parsed);
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ id: 'test-card-e2e-001', lemma: parsed.lemma ?? '' }));

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
      resolve({ server, port, capturedCards });
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

  // Wait for both paragraphs to be annotated.
  // Use state:'attached' because whitespace-only token spans are not 'visible'.
  await page.waitForSelector('#plain-para [data-carve="token"]', { timeout: 15_000, state: 'attached' });
  await page.waitForSelector('#ruby1 [data-carve="token"]', { timeout: 15_000, state: 'attached' });
  console.log('[page] both paragraphs annotated');

  // ── Ruby base text MUST be annotated (highlighted) ──────────────────────────
  const rubyTokenCount = await page.$$eval('#ruby1 [data-carve="token"]', els => els.length);
  assert(rubyTokenCount >= 1, `expected ≥1 token span inside <ruby>, got ${rubyTokenCount}`);
  console.log(`[assert] ${rubyTokenCount} token span(s) inside <ruby> ✓`);

  // ── <rt> (furigana reading) must NOT be annotated ───────────────────────────
  const rtTokenCount = await page.$$eval('#ruby1 rt [data-carve="token"]', els => els.length);
  assert(rtTokenCount === 0, `expected 0 token spans inside <rt>, got ${rtTokenCount}`);
  console.log('[assert] <rt> not annotated ✓');

  // ── <rt> text content must be intact ────────────────────────────────────────
  const rtText = await page.$eval('#ruby1 rt', el => el.textContent);
  assert(rtText === 'せんりゅう', `<rt> text corrupted: "${rtText}"`);
  console.log(`[assert] <rt> text intact: "${rtText}" ✓`);

  // ── Ruby base kanji text must be preserved (concatenation of span texts) ────
  const rubyBaseText = await page.$eval('#ruby1', el => {
    // Sum text from all child nodes except <rt>/<rp>/<rb>
    let t = '';
    for (const n of el.childNodes) {
      if (n.nodeType === Node.TEXT_NODE) t += n.textContent;
      else if (n.nodeType === Node.ELEMENT_NODE) {
        const tag = n.tagName.toUpperCase();
        if (tag !== 'RT' && tag !== 'RP' && tag !== 'RB') t += n.textContent;
      }
    }
    return t;
  });
  assert(rubyBaseText === '川柳', `ruby base text wrong — expected "川柳", got "${rubyBaseText}"`);
  console.log(`[assert] ruby base text intact: "${rubyBaseText}" ✓`);

  // ── Text content must not be corrupted (cross-segment token contamination) ───
  // The annotator used to batch text nodes joined by a separator. If the
  // tokenizer skipped characters (e.g. 。), chars < text.length kept consuming
  // tokens from the NEXT segment, injecting wrong text into the wrong node.
  const plainText = await page.$eval('#plain-para', el => el.textContent?.trim() ?? '');
  assert(
    plainText.includes('災害時'),
    `#plain-para missing expected text. Got: "${plainText}"`,
  );
  assert(
    !plainText.includes('症状') && !plainText.includes('病名'),
    `nav text leaked into #plain-para: "${plainText}"`,
  );
  console.log(`[assert] #plain-para text intact: "${plainText.slice(0, 30)}" ✓`);

  const navText = await page.$eval('#nav-link', el => el.textContent?.trim() ?? '');
  assert(navText.includes('症状'), `#nav-link text wrong. Got: "${navText}"`);
  assert(
    !navText.includes('川柳') && !navText.includes('災害'),
    `article text leaked into nav: "${navText}"`,
  );
  console.log(`[assert] nav text intact: "${navText}" ✓`);

  // ── No annotated token should have a pink/red background ────────────────────
  // Tokens without a frequency rank must still get data-band (not fall through
  // to a background-color CSS rule). Any non-transparent background is a bug.
  const badTokenBg = await page.evaluate(() => {
    for (const el of document.querySelectorAll('[data-carve="token"]')) {
      const bg = window.getComputedStyle(el).backgroundColor;
      // transparent = rgba(0,0,0,0) or 'transparent'
      if (bg !== 'rgba(0, 0, 0, 0)' && bg !== 'transparent') return bg;
    }
    return null;
  });
  assert(badTokenBg === null, `annotated token has unexpected background: ${badTokenBg}`);
  console.log('[assert] no token has a background color ✓');

  // ── Every unknown content-word token must have data-band set ────────────────
  const missingBand = await page.evaluate(() => {
    for (const el of document.querySelectorAll('[data-carve="token"][data-status="unknown"][data-content="1"]')) {
      if (!el.hasAttribute('data-band')) return el.textContent;
    }
    return null;
  });
  assert(missingBand === null, `unknown content-word token "${missingBand}" is missing data-band`);
  console.log('[assert] all unknown content-word tokens have data-band ✓');

  await page.screenshot({ path: path.resolve(__dirname, 'extension-ruby-e2e.png'), fullPage: true });
  console.log('[screenshot] saved extension-ruby-e2e.png');
  await page.close();
}

async function testMiningWorkflow(context, mockBase, capturedCards) {
  console.log('\n── Test 3: mining workflow (click → popup → mine → save) ────');
  const page = await context.newPage();
  await page.goto(`${mockBase}/test`);

  // Wait for at least one content-word token to be annotated
  await page.waitForSelector('[data-carve="token"][data-content="1"]', { timeout: 15_000, state: 'attached' });
  console.log('[page] content-word tokens annotated');

  // Click the first content-word token
  const tokenEl = page.locator('[data-carve="token"][data-content="1"]').first();
  const tokenSurface = await tokenEl.getAttribute('data-lemma');
  await tokenEl.click();
  console.log(`[action] clicked token: "${tokenSurface}"`);

  // Popup should appear (loading state first, then full popup with Mine button)
  await page.waitForSelector('#carve-popup', { timeout: 10_000, state: 'visible' });
  console.log('[page] #carve-popup appeared');

  // Wait for the Mine button (appears after LOOKUP response)
  await page.waitForSelector('#carve-popup .btn-mine', { timeout: 10_000, state: 'visible' });
  console.log('[page] .btn-mine visible');

  // ── Popup content checks ──────────────────────────────────────────────────
  const popupText = await page.$eval('#carve-popup', el => el.textContent ?? '');
  assert(popupText.includes('Mine'), `popup should have Mine button, got: "${popupText.slice(0, 100)}"`);
  console.log('[assert] Mine button present in popup ✓');

  // Click Mine to open the mine form
  await page.click('#carve-popup .btn-mine');
  console.log('[action] clicked Mine button');

  // Mine form should appear with Save card button.
  const saveBtn = page.locator('#carve-popup .btn-mine-save');
  await saveBtn.waitFor({ timeout: 5_000, state: 'visible' });
  console.log('[page] mine form appeared');

  // ── Mine form field checks ────────────────────────────────────────────────
  const lemmaValue = await page.$eval('#mine-lemma', el => el.value);
  assert(lemmaValue.length > 0, `mine lemma field should be populated, got empty`);
  console.log(`[assert] mine lemma field: "${lemmaValue}" ✓`);

  const sentenceValue = await page.$eval('#mine-sentence', el => el.value);
  console.log(`[assert] mine sentence field: "${sentenceValue.slice(0, 40)}" ✓`);

  // ── Save card and verify API call ─────────────────────────────────────────
  const cardCountBefore = capturedCards.length;
  await saveBtn.click();
  console.log('[action] clicked Save card');

  // Wait for the card to be captured by mock server (poll up to 3s)
  const deadline = Date.now() + 3000;
  while (capturedCards.length === cardCountBefore && Date.now() < deadline) {
    await new Promise(r => setTimeout(r, 50));
  }

  assert(
    capturedCards.length > cardCountBefore,
    `expected POST /v1/cards to be called, but capturedCards is still ${capturedCards.length}`,
  );
  console.log(`[assert] POST /v1/cards received ✓`);

  const card = capturedCards[capturedCards.length - 1];
  assert(card.lemma === lemmaValue, `card.lemma "${card.lemma}" !== token lemma "${lemmaValue}"`);
  assert(card.language_code, `card.language_code should be set, got: ${JSON.stringify(card.language_code)}`);
  assert(card.source_url, `card.source_url should be set, got: ${JSON.stringify(card.source_url)}`);
  console.log(`[assert] card fields: lemma="${card.lemma}" lang="${card.language_code}" source_url="${card.source_url}" ✓`);

  // Popup should close or show "✓ Saved" status after successful save
  // Give it up to 2s (the popup auto-hides after 900ms on success)
  await new Promise(r => setTimeout(r, 1000));
  const popupVisible = await page.isVisible('#carve-popup');
  // Either the popup is gone or shows the saved confirmation
  if (popupVisible) {
    const statusText = await page.$eval('#mine-status', el => el.textContent ?? '').catch(() => '');
    assert(
      statusText.includes('Saved') || statusText.includes('✓'),
      `expected popup closed or "Saved" status, got popup visible with status: "${statusText}"`,
    );
    console.log(`[assert] popup shows saved status: "${statusText}" ✓`);
  } else {
    console.log('[assert] popup closed after save ✓');
  }

  // ── Token status updated to learning ─────────────────────────────────────
  const updatedStatus = await tokenEl.getAttribute('data-status');
  assert(
    updatedStatus === 'learning',
    `expected token data-status="learning" after mining, got "${updatedStatus}"`,
  );
  console.log(`[assert] token status updated to "learning" ✓`);

  await page.screenshot({ path: path.resolve(__dirname, 'extension-mining-e2e.png'), fullPage: true });
  console.log('[screenshot] saved extension-mining-e2e.png');
  await page.close();
}

// ── Manifest validation ───────────────────────────────────────────────────────

function testManifestFiles() {
  console.log('\n── Test 0: manifest validation ──────────────────────────────');
  const fs = require('fs');
  const DIST = path.resolve(__dirname, '../apps/extension/dist');

  for (const browser of ['chrome', 'firefox', 'safari']) {
    const manifestPath = path.join(DIST, browser, 'manifest.json');
    if (!fs.existsSync(manifestPath)) {
      console.log(`[skip] ${browser} dist not found — skipping manifest check`);
      continue;
    }
    const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
    assert(manifest.manifest_version === 3, `${browser}: expected manifest_version 3`);
    assert(manifest.background, `${browser}: missing background field`);
    if (browser === 'firefox') {
      assert(manifest.background.scripts, `firefox: background must use 'scripts', not service_worker`);
      assert(!manifest.background.service_worker, `firefox: must not use service_worker`);
      assert(manifest.browser_specific_settings?.gecko?.id, `firefox: missing gecko.id`);
    } else {
      assert(manifest.background.service_worker, `${browser}: background must use service_worker`);
    }
    console.log(`[assert] ${browser} manifest valid ✓`);
  }
}

// ── Main ──────────────────────────────────────────────────────────────────────

async function run() {
  const EXTENSION_DIR = path.resolve(__dirname, '../apps/extension/dist/chrome');

  const { server, port, capturedCards } = await startMockServer();
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

  testManifestFiles();

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
    await testMiningWorkflow(context, mockBase, capturedCards);

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
