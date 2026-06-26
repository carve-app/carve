/**
 * Real, end-to-end verification of the video sentence-mining flow.
 *
 * This is the test that guarantees the headline feature works for real:
 *   press "m" on a subtitle → a flashcard is created carrying
 *     (1) a SCREENSHOT of the video frame at the mined moment,
 *     (2) AUDIO of the exact mined sentence (real Opus bytes, not silence),
 *     (3) the SENTENCE text,
 *     (4) a real TRANSLATION,
 *     (5) the exact subtitle timing + source timestamp.
 *
 * Unlike a mock-server test, this drives the ACTUAL extension in a REAL
 * Chromium against the REAL Go api + media binaries and a REAL Postgres, then
 * fetches the stored media back over HTTP and checks the bytes. The video
 * fixture carries a genuine audio track (so captureStream → MediaRecorder
 * produces real audio) and a native-language <track> (so the translation comes
 * from a real human subtitle line, exercising getNativeCueText).
 *
 * It is fully self-contained: it boots its own Postgres container, runs
 * migrations, and starts api + media on ephemeral ports. Nothing else needs to
 * be running. Skips (exit 0) if Docker or the built artifacts are unavailable,
 * so it never blocks a machine that can't run it.
 *
 * Run: node e2e/video-mining.test.js
 */

'use strict';

const http = require('http');
const path = require('path');
const fs = require('fs');
const os = require('os');
const net = require('net');
const { execSync, spawn, spawnSync } = require('child_process');
const { chromium } = require('playwright');

const ROOT = path.resolve(__dirname, '..');
const EXTENSION_DIR = path.join(ROOT, 'apps', 'extension', 'dist', 'chrome');
const FIXTURE_VIDEO = path.join(ROOT, 'e2e', 'fixtures', 'media', 'sample.webm');
const BIN = path.join(ROOT, 'bin');

const procs = [];
let pgContainer = null;
let fakeNlpServer = null;
let mediaDir = null;
let userDataDir = null;

function log(msg) { console.log(`[video-mining] ${msg}`); }
function assert(cond, msg) { if (!cond) throw new Error(`FAIL: ${msg}`); }

function freePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
    srv.on('error', reject);
  });
}

async function waitForHealth(url, timeoutMs = 30_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const r = await fetch(url);
      if (r.ok) return;
    } catch {/* not up yet */}
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`service at ${url} never became healthy`);
}

// ── The YouTube-like fixture page ─────────────────────────────────────────────
//
// lang="ja" + plenty of kana → the content script's detectLanguage() returns
// 'ja' and mounts the subtitle hook. The <video> has class html5-main-video
// (matched by getVideoElement) and a real audio track. A native-language
// <track srclang="en"> provides the translation source. The caption container
// (.ytp-caption-window-container / .ytp-caption-segment) is what the YouTube
// hook's MutationObserver watches.
function fixturePage(videoSrc) {
  return `<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>YT</title></head>
<body style="margin:0;background:#111">
  <div style="position:absolute;left:40px;top:50px;width:480px;height:270px">
    <video class="html5-main-video" id="vid" src="${videoSrc}"
           width="480" height="270" crossorigin="anonymous" playsinline></video>
  </div>
  <div class="ytp-caption-window-container">
    <span class="ytp-caption-segment"></span>
  </div>
  <p>日本語のテキストです。これはテストページであり、映画を見ながら日本語を勉強するためのものです。
     ひらがな も カタカナ も たくさん あります。今日 は とても いい 天気 ですね。</p>
  <script>
    // Add a native-language subtitle track and an active cue spanning the cue
    // window we mine [2s, 5s], so getNativeCueText() finds a real translation.
    const v = document.getElementById('vid');
    const track = v.addTextTrack('subtitles', 'English', 'en');
    track.mode = 'showing';
    track.addCue(new VTTCue(2.0, 5.0, 'I study Japanese while watching movies.'));
    // Surface the active cue's timing through textTracks for the JA target line
    // too — the YouTube hook reads showing tracks for [startMs,endMs].
    const ja = v.addTextTrack('subtitles', 'Japanese', 'ja');
    ja.mode = 'showing';
    ja.addCue(new VTTCue(2.0, 5.0, '映画を見ながら日本語を勉強する'));
  </script>
</body>
</html>`;
}

// A minimal stand-in for the Python NLP service. The REAL api binary proxies
// /v1/nlp/* straight through to NLP_SERVICE_URL, so this lets the e2e exercise
// the extension → api-proxy → translate path (the realistic no-dual-subs case)
// without building the multi-hundred-MB JMdict/Tatoeba image. The NLP
// translation logic itself is covered by services/nlp/tests/test_translate.py.
async function startFakeNlp() {
  const server = http.createServer((req, res) => {
    let body = '';
    req.on('data', (c) => { body += c; });
    req.on('end', () => {
      const url = (req.url || '').split('?')[0];
      const reply = (obj) => { res.writeHead(200, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(obj)); };
      if (url === '/health') return reply({ status: 'ok' });
      if (url === '/translate') {
        // Return a real sentence translation (what the Tatoeba corpus would).
        return reply({ translation: 'I study Japanese while watching movies.', source_language: 'ja', target_language: 'en' });
      }
      if (url === '/tokenize') return reply({ tokens: [], comprehension_pct: null });
      if (url === '/select-sentence') return reply({ best: null, ranked: [] });
      reply({});
    });
  });
  const port = await freePort();
  await new Promise((r) => server.listen(port, '127.0.0.1', r));
  return { server, port };
}

async function main() {
  // ── Preconditions ───────────────────────────────────────────────────────────
  if (!fs.existsSync(EXTENSION_DIR)) {
    log('SKIP: extension dist not built (run `npm --prefix apps/extension run build:chrome`)');
    return;
  }
  if (!fs.existsSync(FIXTURE_VIDEO)) {
    log(`SKIP: fixture video missing at ${FIXTURE_VIDEO}`);
    return;
  }
  for (const b of ['api', 'media', 'migrate']) {
    if (!fs.existsSync(path.join(BIN, b))) {
      log(`SKIP: bin/${b} not built (run \`make build\` + build media). Missing real stack.`);
      return;
    }
  }
  const dockerOk = spawnSync('docker', ['version'], { stdio: 'ignore' }).status === 0;
  if (!dockerOk) { log('SKIP: docker not available'); return; }

  // ── 1. Postgres ──────────────────────────────────────────────────────────────
  const pgPort = await freePort();
  pgContainer = `carve-vidmine-${Date.now()}`;
  log(`starting postgres (${pgContainer}) on :${pgPort}`);
  execSync(
    `docker run -d --name ${pgContainer} -e POSTGRES_USER=carve -e POSTGRES_PASSWORD=carve ` +
    `-e POSTGRES_DB=carve -p ${pgPort}:5432 postgres:16-alpine`,
    { stdio: 'ignore' },
  );
  // Wait until Postgres actually answers a real query. `pg_isready` flips true
  // during the bootstrap phase (a throwaway init server) BEFORE the real server
  // accepts connections, so we run an actual `SELECT 1` inside the container.
  {
    const deadline = Date.now() + 40_000;
    let ready = false;
    while (Date.now() < deadline) {
      const q = spawnSync('docker', ['exec', pgContainer, 'psql', '-U', 'carve', '-d', 'carve', '-tAc', 'SELECT 1'], { stdio: 'ignore' });
      if (q.status === 0) { ready = true; break; }
      await new Promise((r) => setTimeout(r, 500));
    }
    assert(ready, 'postgres never accepted a real connection');
  }
  const dbUrl = `postgres://carve:carve@localhost:${pgPort}/carve?sslmode=disable`;

  // ── 2. Migrations (retry a few times to ride out the final restart) ─────────
  log('running migrations');
  {
    let migrated = false;
    let lastErr = null;
    for (let i = 0; i < 5 && !migrated; i++) {
      try {
        execSync(`${path.join(BIN, 'migrate')} --dir ${path.join(ROOT, 'services', 'api', 'migrations')}`, {
          stdio: 'ignore',
          env: { ...process.env, DATABASE_URL: dbUrl },
        });
        migrated = true;
      } catch (e) {
        lastErr = e;
        await new Promise((r) => setTimeout(r, 1000));
      }
    }
    assert(migrated, `migrations never succeeded: ${lastErr && lastErr.message}`);
  }

  // ── 3. fake NLP + media + api (local storage backend) ────────────────────────
  const { server: nlpServer, port: nlpPort } = await startFakeNlp();
  fakeNlpServer = nlpServer;
  log(`fake NLP on :${nlpPort}`);

  const mediaPort = await freePort();
  const apiPort = await freePort();
  mediaDir = fs.mkdtempSync(path.join(os.tmpdir(), 'carve-media-'));

  log(`starting media :${mediaPort} (local storage at ${mediaDir})`);
  const media = spawn(path.join(BIN, 'media'), [], {
    env: { ...process.env, PORT: String(mediaPort), STORAGE_BACKEND: 'local', MEDIA_STORAGE_DIR: mediaDir },
    stdio: 'ignore',
  });
  procs.push(media);

  log(`starting api :${apiPort} (media → :${mediaPort}, nlp → :${nlpPort})`);
  const api = spawn(path.join(BIN, 'api'), [], {
    env: {
      ...process.env,
      PORT: String(apiPort),
      DATABASE_URL: dbUrl,
      JWT_SECRET: 'video-mining-e2e-secret-at-least-32-chars-long',
      MEDIA_SERVICE_URL: `http://localhost:${mediaPort}`,
      NLP_SERVICE_URL: `http://localhost:${nlpPort}`,
      EXPOSE_VERIFY_TOKENS: '1',
    },
    stdio: 'ignore',
  });
  procs.push(api);

  await waitForHealth(`http://localhost:${mediaPort}/health`);
  await waitForHealth(`http://localhost:${apiPort}/health`);
  const API = `http://localhost:${apiPort}`;
  const MEDIA = `http://localhost:${mediaPort}`;
  log('api + media healthy');

  // ── 4. Register a real user ───────────────────────────────────────────────────
  const email = `vidmine+${Date.now()}@example.com`;
  const reg = await fetch(`${API}/v1/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password: 'alphapassword123', display_name: 'Vid Mine' }),
  });
  assert(reg.ok, `register failed: ${reg.status}`);
  const registration = await reg.json();
  const verified = await fetch(`${API}/v1/auth/verify`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token: registration.verification_token_test }),
  });
  assert(verified.ok, `verify failed: ${verified.status}`);
  const login = await fetch(`${API}/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password: 'alphapassword123' }),
  });
  assert(login.ok, `login failed: ${login.status}`);
  const token = (await login.json()).access_token;
  assert(token, 'no access token from verified login');

  // ── 5. Launch real Chromium with the built extension ──────────────────────────
  userDataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'carve-vidmine-profile-'));
  // headless:false + --headless=new is the documented combo for loading an
  // unpacked extension under Chromium's new headless mode (plain headless:true
  // silently drops --load-extension and no service worker ever registers).
  const ctx = await chromium.launchPersistentContext(userDataDir, {
    headless: false,
    args: [
      '--headless=new',
      '--no-sandbox',
      '--autoplay-policy=no-user-gesture-required',
      `--disable-extensions-except=${EXTENSION_DIR}`,
      `--load-extension=${EXTENSION_DIR}`,
    ],
  });

  try {
    // Seed auth + API base into the extension's storage via the service worker.
    let sw = ctx.serviceWorkers().find((w) => w.url().includes('background'));
    if (!sw) {
      sw = await ctx.waitForEvent('serviceworker', { timeout: 15_000 });
    }
    await sw.evaluate(async ([t, base]) => {
      await chrome.storage.local.set({ accessToken: t, apiBaseUrl: base, targetLanguage: 'ja' });
    }, [token, API]);
    log('seeded extension storage (token + apiBaseUrl)');

    const page = await ctx.newPage();
    // The <video> src must be SAME-ORIGIN as the page (youtube.com): a
    // cross-origin media element's captureStream() is tainted and yields no
    // audio tracks. We fulfill both the page and the video from the youtube.com
    // origin via routing, keeping the youtube.com/watch URL so the platform
    // hook's regex matches and mounts the overlay.
    const videoBytes = fs.readFileSync(FIXTURE_VIDEO);
    const fixtureHtml = fixturePage('https://www.youtube.com/carve-sample.webm');
    await page.route('**/*', (route) => {
      const url = route.request().url();
      if (url.includes('youtube.com/carve-sample.webm')) {
        return route.fulfill({ status: 200, contentType: 'video/webm', body: videoBytes });
      }
      if (url.includes('youtube.com/watch')) {
        return route.fulfill({ status: 200, contentType: 'text/html; charset=utf-8', body: fixtureHtml });
      }
      return route.continue();
    });
    await page.goto('https://www.youtube.com/watch?v=carvetest');

    // Overlay must mount (proves URL routing + language detection + hook).
    await page.waitForSelector('#carve-sub-overlay', { timeout: 20_000 });
    log('subtitle overlay mounted');

    // Play THROUGH into the cue window [2s, 5s] so the video's textTracks
    // populate activeCues (programmatic VTTCues only activate during real
    // playback in headless Chromium) — this is what lets the YouTube hook read
    // the cue's true timing and the native-language line.
    await page.evaluate(async () => {
      const v = document.getElementById('vid');
      v.muted = false;
      try { await v.play(); } catch {/* autoplay policy disabled via launch args */}
    });
    await page.waitForFunction(() => {
      const v = document.getElementById('vid');
      return v && v.currentTime >= 2.2 && v.currentTime < 4.8;
    }, { timeout: 15_000 });

    // Fire a subtitle cue: set the caption text → the YouTube hook's observer
    // calls overlay.onCue() with the active cue timing ([2000, 5000]).
    await page.evaluate(() => {
      const seg = document.querySelector('.ytp-caption-segment');
      if (seg) seg.textContent = '映画を見ながら日本語を勉強する';
    });
    await page.waitForFunction(() => {
      const t = document.getElementById('cso-target');
      return !!t && (t.textContent ?? '').length > 0 && !t.textContent.includes('Subtitles will appear here');
    }, { timeout: 10_000 });
    log('cue rendered in overlay');

    // Press "m" to mine.
    await page.bringToFront();
    await page.keyboard.press('m');
    log('pressed "m" to mine');

    // Wait for a TERMINAL mine status — i.e. media capture finished, not the
    // transient "Mined! Capturing media…". A terminal message reports what
    // landed (+image/+audio), unavailability, or a media error.
    await page.waitForFunction(() => {
      const el = document.getElementById('cso-mine-status');
      const t = el?.textContent ?? '';
      return /Mined/i.test(t) && !/Capturing media/i.test(t);
    }, { timeout: 40_000 });
    const status = await page.locator('#cso-mine-status').textContent();
    log(`mine status: "${status}"`);
    assert(/Mined/i.test(status), `mine status was: ${status}`);
    assert(/\+image|\+audio/i.test(status), `expected captured media in status, got: "${status}"`);

    // ── 6. Assert the card exists with all info ───────────────────────────────
    let cardId = null;
    {
      const deadline = Date.now() + 10_000;
      while (Date.now() < deadline && !cardId) {
        const list = await fetch(`${API}/v1/cards?language=ja`, {
          headers: { Authorization: `Bearer ${token}` },
        }).then((r) => r.json());
        if ((list.cards ?? []).length > 0) cardId = list.cards[0].id;
        else await new Promise((r) => setTimeout(r, 300));
      }
    }
    assert(cardId, 'card never appeared in backend');
    log(`card created: ${cardId}`);

    // Poll the card until media lands (upload is async via the worker).
    let card = null;
    {
      const deadline = Date.now() + 20_000;
      while (Date.now() < deadline) {
        card = await fetch(`${API}/v1/cards/${cardId}`, {
          headers: { Authorization: `Bearer ${token}` },
        }).then((r) => r.json());
        if (card.image_url || card.audio_url) break;
        await new Promise((r) => setTimeout(r, 400));
      }
    }

    // (1) sentence
    assert(card.sentence && card.sentence.length > 0, `card has no sentence: ${JSON.stringify(card.sentence)}`);
    assert(/勉強/.test(card.sentence), `sentence missing mined word: "${card.sentence}"`);
    log(`✓ sentence: "${card.sentence}"`);

    // (2) translation — from the native <track srclang="en"> (real human line)
    assert(
      card.subtitle_translation && /study/i.test(card.subtitle_translation),
      `card translation missing/wrong: ${JSON.stringify(card.subtitle_translation)}`,
    );
    log(`✓ translation: "${card.subtitle_translation}"`);

    // (3) exact subtitle timing — the cue window [2000, 5000], NOT a
    //     playhead-relative guess.
    assert(card.subtitle_start_ms === 2000, `subtitle_start_ms = ${card.subtitle_start_ms}, want 2000`);
    assert(card.subtitle_end_ms === 5000, `subtitle_end_ms = ${card.subtitle_end_ms}, want 5000`);
    log(`✓ subtitle timing: [${card.subtitle_start_ms}, ${card.subtitle_end_ms}]`);

    // (4) screenshot — fetchable, non-trivial bytes
    assert(card.image_url, 'card has no image_url (screenshot)');
    const imgResp = await fetch(card.image_url.replace('media:8002', new URL(MEDIA).host));
    assert(imgResp.status === 200, `screenshot not retrievable: ${imgResp.status}`);
    const imgBytes = (await imgResp.arrayBuffer()).byteLength;
    assert(imgBytes > 1000, `screenshot too small (${imgBytes} bytes) — likely blank/failed crop`);
    log(`✓ screenshot: ${imgBytes} bytes, ${imgResp.headers.get('content-type')}`);

    // (5) audio — fetchable, real Opus webm of the sentence (NOT silence/empty)
    assert(card.audio_url, 'card has no audio_url — the exact-sentence audio was lost');
    const audResp = await fetch(card.audio_url.replace('media:8002', new URL(MEDIA).host));
    assert(audResp.status === 200, `audio not retrievable: ${audResp.status}`);
    const audBuf = Buffer.from(await audResp.arrayBuffer());
    assert(audBuf.length > 2000, `audio too small (${audBuf.length} bytes) — likely silent/empty capture`);
    // EBML magic 0x1A45DFA3 → it's a real webm/matroska container.
    assert(
      audBuf[0] === 0x1a && audBuf[1] === 0x45 && audBuf[2] === 0xdf && audBuf[3] === 0xa3,
      `audio is not a valid webm container (first bytes: ${audBuf.slice(0, 4).toString('hex')})`,
    );
    log(`✓ audio: ${audBuf.length} bytes, valid webm, ${audResp.headers.get('content-type')}`);

    log('');
    log('✓ PASS — mined card has screenshot + exact-sentence audio + sentence + translation + timing');
  } finally {
    await ctx.close();
  }
}

function cleanup() {
  for (const p of procs) { try { p.kill('SIGKILL'); } catch {/* ignore */} }
  if (fakeNlpServer) { try { fakeNlpServer.close(); } catch {/* ignore */} }
  if (pgContainer) {
    try { execSync(`docker rm -f ${pgContainer}`, { stdio: 'ignore' }); } catch {/* ignore */}
  }
  // Remove the temp dirs (Chromium profile + local media store) on every exit
  // path so repeated runs don't accumulate /tmp cruft.
  for (const dir of [userDataDir, mediaDir]) {
    if (dir) { try { fs.rmSync(dir, { recursive: true, force: true }); } catch {/* ignore */} }
  }
}

main()
  .then(() => { cleanup(); process.exit(0); })
  .catch((err) => {
    console.error('\n✗ FAIL —', err.message);
    cleanup();
    process.exit(1);
  });
