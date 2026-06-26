/**
 * Mock /v1 API for Playwright journey tests.
 *
 * Implements just enough of the carve API surface so the SvelteKit web app
 * can complete a full register → onboard → cards → review journey. State
 * is kept in-memory per process; each Playwright worker boots a fresh
 * instance via the webServer config.
 *
 * The nightly "real-services" run replaces this with the actual Go API
 * against a testcontainers Postgres; the contract this file implements is
 * a strict subset of docs/openapi.yaml.
 */

const http = require('http');
const PORT = process.env.PORT ?? 8080;

const state = {
  users: new Map(),       // email -> { id, password, display_name }
  tokens: new Map(),      // token -> userId
  cards: new Map(),       // id -> card row
  reviewEvents: [],       // { user_id, card_id, rating, time_taken_ms, at }
  reviewEventsById: new Map(),
  passwordResetTokens: new Map(),
};

function uuid() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

function applyCors(req, res) {
  const origin = req.headers.origin || 'http://localhost:5173';
  res.setHeader('Access-Control-Allow-Origin', origin);
  res.setHeader('Access-Control-Allow-Credentials', 'true');
  res.setHeader('Access-Control-Allow-Methods', 'GET,POST,PATCH,PUT,DELETE,OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Authorization,Content-Type');
  res.setHeader('Vary', 'Origin');
}

function send(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(body));
}

function readBody(req) {
  return new Promise((resolve) => {
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => {
      const buf = Buffer.concat(chunks).toString('utf8');
      try { resolve(JSON.parse(buf)); }
      catch { resolve({}); }
    });
  });
}

function auth(req) {
  const h = req.headers.authorization;
  if (!h?.startsWith('Bearer ')) return null;
  const token = h.slice(7);
  return state.tokens.get(token) ?? null;
}

const server = http.createServer(async (req, res) => {
  applyCors(req, res);
  if (req.method === 'OPTIONS') { send(res, 204, {}); return; }
  const url = new URL(req.url, 'http://localhost');
  const path = url.pathname;

  // ── Health / metrics ───────────────────────────────────────────────────
  if (req.method === 'GET' && path === '/health') {
    return send(res, 200, { status: 'ok', service: 'mock', version: 'test' });
  }
  if (req.method === 'GET' && path === '/metrics') {
    res.writeHead(200, { 'Content-Type': 'text/plain' });
    return res.end('# mock metrics\n');
  }
  if (req.method === 'GET' && path === '/__test/review-events') {
    const userId = auth(req);
    if (!userId) return send(res, 401, { error: 'unauthorized' });
    const events = state.reviewEvents.filter((event) => event.user_id === userId);
    return send(res, 200, {
      count: events.length,
      unique_event_ids: new Set(events.map((event) => event.event_id)).size,
    });
  }

  // ── Auth ───────────────────────────────────────────────────────────────
  if (req.method === 'POST' && path === '/v1/auth/register') {
    const body = await readBody(req);
    if (!body.email || !body.password || !body.display_name) return send(res, 400, { error: 'missing fields' });
    if (state.users.has(body.email)) return send(res, 409, { error: 'email taken' });
    const user = { id: uuid(), email: body.email, password: body.password, display_name: body.display_name };
    state.users.set(body.email, user);
    const token = uuid();
    state.tokens.set(token, user.id);
    return send(res, 200, {
      access_token: token, refresh_token: uuid(),
      user: { id: user.id, email: user.email, display_name: user.display_name },
    });
  }
  if (req.method === 'POST' && path === '/v1/auth/login') {
    const body = await readBody(req);
    const u = state.users.get(body.email);
    if (!u || u.password !== body.password) return send(res, 401, { error: 'invalid' });
    const token = uuid();
    state.tokens.set(token, u.id);
    return send(res, 200, {
      access_token: token, refresh_token: uuid(),
      user: { id: u.id, email: u.email, display_name: u.display_name },
    });
  }
  if (req.method === 'POST' && path === '/v1/auth/forgot') {
    const body = await readBody(req);
    const u = state.users.get(body.email);
    if (u) state.passwordResetTokens.set('test-reset-token', u.id);
    return send(res, 200, { ok: true });
  }
  if (req.method === 'POST' && path === '/v1/auth/reset') {
    const body = await readBody(req);
    const userId = state.passwordResetTokens.get(body.token);
    if (!userId) return send(res, 400, { error: 'invalid token' });
    return send(res, 200, { ok: true });
  }

  // ── Onboarding ─────────────────────────────────────────────────────────
  if (req.method === 'POST' && path === '/v1/onboarding/known-words') {
    if (!auth(req)) return send(res, 401, { error: 'unauthorized' });
    await readBody(req);
    return send(res, 200, { marked: 0 });
  }
  if (req.method === 'POST' && path === '/v1/onboarding/starter-deck') {
    if (!auth(req)) return send(res, 401, { error: 'unauthorized' });
    await readBody(req);
    return send(res, 200, { status: 'no_deck' });
  }

  // ── Users ──────────────────────────────────────────────────────────────
  if (req.method === 'GET' && path === '/v1/users/me') {
    const userId = auth(req);
    if (!userId) return send(res, 401, { error: 'unauthorized' });
    const u = [...state.users.values()].find((x) => x.id === userId);
    if (!u) return send(res, 404, { error: 'no user' });
    return send(res, 200, { id: u.id, email: u.email, display_name: u.display_name });
  }

  // ── Cards ──────────────────────────────────────────────────────────────
  if (req.method === 'GET' && path === '/v1/cards') {
    const userId = auth(req);
    if (!userId) return send(res, 401, { error: 'unauthorized' });
    const cards = [...state.cards.values()].filter((c) => c.user_id === userId);
    return send(res, 200, { cards });
  }
  if (req.method === 'POST' && path === '/v1/cards') {
    const userId = auth(req);
    if (!userId) return send(res, 401, { error: 'unauthorized' });
    const body = await readBody(req);
    const card = {
      id: uuid(),
      user_id: userId,
      front_text: body.front_text ?? '',
      back_text: body.back_text ?? null,
      sentence: body.sentence ?? null,
      fsrs_state: 'new', fsrs_reps: 0, fsrs_lapses: 0,
      fsrs_stability: null, fsrs_difficulty: null, fsrs_due: null,
      language_code: body.language_code ?? 'en',
    };
    state.cards.set(card.id, card);
    return send(res, 200, card);
  }
  if (req.method === 'GET' && /^\/v1\/cards\/[\w-]+$/.test(path)) {
    const userId = auth(req);
    if (!userId) return send(res, 401, { error: 'unauthorized' });
    const id = path.split('/').pop();
    const card = state.cards.get(id);
    if (!card) return send(res, 404, { error: 'not found' });
    return send(res, 200, card);
  }

  // ── Review ─────────────────────────────────────────────────────────────
  if (req.method === 'GET' && path === '/v1/review/due-count') {
    const userId = auth(req);
    if (!userId) return send(res, 401, { error: 'unauthorized' });
    const count = [...state.cards.values()].filter((c) => c.user_id === userId).length;
    return send(res, 200, { due_count: count });
  }
  if (req.method === 'GET' && path === '/v1/review/session') {
    const userId = auth(req);
    if (!userId) return send(res, 401, { error: 'unauthorized' });
    const cards = [...state.cards.values()].filter((c) => c.user_id === userId).slice(0, 50);
    return send(res, 200, { cards });
  }
  if (req.method === 'POST' && path === '/v1/review/events') {
    const userId = auth(req);
    if (!userId) return send(res, 401, { error: 'unauthorized' });
    const body = await readBody(req);
    if (!body.card_id || ![1, 2, 3, 4].includes(body.rating)) {
      return send(res, 400, { error: 'card_id and rating 1-4 are required' });
    }
    const replayKey = body.event_id ? `${userId}:${body.event_id}` : null;
    if (replayKey && state.reviewEventsById.has(replayKey)) {
      return send(res, 200, state.reviewEventsById.get(replayKey));
    }
    state.reviewEvents.push({ ...body, user_id: userId, at: Date.now() });
    const response = {
      state: 'review', stability: 1, difficulty: 5,
      due: new Date(Date.now() + 86400000).toISOString(),
      reps: 1, lapses: 0, is_leech: false,
    };
    if (replayKey) state.reviewEventsById.set(replayKey, response);
    return send(res, 200, response);
  }
  if (req.method === 'GET' && path === '/v1/review/intervals') {
    if (!auth(req)) return send(res, 401, { error: 'unauthorized' });
    const now = Date.now();
    return send(res, 200, {
      again: new Date(now + 60_000).toISOString(),
      hard:  new Date(now + 600_000).toISOString(),
      good:  new Date(now + 86400_000).toISOString(),
      easy:  new Date(now + 4 * 86400_000).toISOString(),
    });
  }
  if (req.method === 'GET' && path === '/v1/review/forecast') {
    if (!auth(req)) return send(res, 401, { error: 'unauthorized' });
    return send(res, 200, { forecast: [] });
  }

  // ── NLP ────────────────────────────────────────────────────────────────
  if (req.method === 'POST' && path === '/v1/nlp/tokenize') {
    if (!auth(req)) return send(res, 401, { error: 'unauthorized' });
    const body = await readBody(req);
    const tokens = String(body.text ?? '').split(/(\s+)/).filter(Boolean).map((s) => ({
      surface: s, lemma: s.toLowerCase(), reading: '', reading_hira: '',
      pos: 'noun', is_content_word: /\w/.test(s), user_status: 'unknown',
      frequency_rank: null,
    }));
    return send(res, 200, { tokens });
  }
  if (req.method === 'POST' && path === '/v1/nlp/translate') {
    if (!auth(req)) return send(res, 401, { error: 'unauthorized' });
    await readBody(req);
    return send(res, 200, { translation: 'mock translation', source_language: 'en', target_language: 'en' });
  }

  // ── Stats / library / settings stubs ──────────────────────────────────
  if (req.method === 'GET' && (path === '/v1/stats' || path.startsWith('/v1/stats/'))) {
    if (!auth(req)) return send(res, 401, { error: 'unauthorized' });
    return send(res, 200, {
      reviews_today: state.reviewEvents.length,
      known_cards: 0,
      learning_cards: 0,
      retention_30d: 0.9,
      retention: 0.9,
      total_reviews: 0,
      total_ever_reviews: 0,
      streak_days: 0,
      reading_minutes: 0,
      listening_minutes: 0,
      word_growth: [],
      reviews_by_day: [],
    });
  }

  // Explicit empty-state contracts used by route/shell journeys. Keeping this
  // list concrete prevents a new frontend call from silently receiving a fake
  // success for an endpoint the mock does not implement.
  const explicitGetStubs = new Map([
    ['/v1/decks', { decks: [] }],
    ['/v1/settings/fsrs', {
      language_code: url.searchParams.get('language') ?? 'ja',
      weights: Array(19).fill(0), target_retention: 0.9,
      leech_threshold: 8, daily_new_limit: 20, is_customized: false,
    }],
    ['/v1/settings/workload-preview', { target_retention: 0.9, avg_interval_days: 0, total_review_cards: 0 }],
    ['/v1/library', { items: [] }],
    ['/v1/output/exercises', { exercises: [] }],
    ['/v1/output/shadowing', { items: [] }],
    ['/v1/discover/feed', { items: [] }],
    ['/v1/review/notifications', { notifications: [] }],
    ['/v1/grammar/known', { pattern_ids: [] }],
    ['/v1/nlp/grammar/patterns', { patterns: [] }],
  ]);
  if (req.method === 'GET' && explicitGetStubs.has(path)) {
    if (!auth(req)) return send(res, 401, { error: 'unauthorized' });
    return send(res, 200, explicitGetStubs.get(path));
  }

  send(res, 501, { error: `mock route not implemented: ${req.method} ${path}` });
});

server.listen(PORT, () => {
  // eslint-disable-next-line no-console
  console.log(`mock API listening on http://localhost:${PORT}`);
});
