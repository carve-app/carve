import {
  expect,
  request as playwrightRequest,
  test,
  type APIRequestContext,
  type APIResponse,
  type Page,
} from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { strToU8, zipSync } from 'fflate';
import { createTestCard, registerTestUser, seedAuthenticatedPage } from './helpers';

const API = process.env.API_BASE ?? 'http://127.0.0.1:8080';
const MAILPIT = process.env.MAILPIT_BASE ?? 'http://127.0.0.1:8025';

test.skip(process.env.E2E_USE_REAL !== '1', 'requires the isolated real stack');
test.describe.configure({ mode: 'serial' });

function bearer(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` };
}

async function jsonOK(response: APIResponse, label: string): Promise<any> {
  const text = await response.text();
  expect(response.ok(), `${label}: ${response.status()} ${text}`).toBe(true);
  return text ? JSON.parse(text) : null;
}

async function latestMailText(request: APIRequestContext, subject: string): Promise<string> {
  let text = '';
  await expect.poll(async () => {
    const list = await request.get(`${MAILPIT}/api/v1/messages`);
    if (!list.ok()) return false;
    const body = await list.json() as { messages?: Array<{ ID: string; Subject: string }> };
    const message = body.messages?.find((candidate) => candidate.Subject === subject);
    if (!message) return false;
    const detail = await request.get(`${MAILPIT}/api/v1/message/${message.ID}`);
    if (!detail.ok()) return false;
    const payload = await detail.json() as { Text?: string; HTML?: string };
    text = payload.Text ?? payload.HTML ?? '';
    return text.length > 0;
  }, { timeout: 10_000 }).toBe(true);
  return text;
}

async function submitReview(
  request: APIRequestContext,
  token: string,
  cardID: string,
  rating: number,
  reviewedAt: string,
  eventID: string = randomUUID(),
): Promise<{ eventID: string; body: any }> {
  const response = await request.post(`${API}/v1/review/events`, {
    headers: bearer(token),
    data: {
      event_id: eventID,
      card_id: cardID,
      rating,
      time_taken_ms: 750,
      reviewed_at: reviewedAt,
    },
  });
  return { eventID, body: await jsonOK(response, `review ${cardID}`) };
}

async function offlineQueueLength(page: Page): Promise<number> {
  return page.evaluate(async () => {
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const open = indexedDB.open('carve_offline', 1);
      open.onsuccess = () => resolve(open.result);
      open.onerror = () => reject(open.error);
    });
    return new Promise<number>((resolve, reject) => {
      const request = db.transaction('review_events', 'readonly').objectStore('review_events').count();
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
  });
}

test('register, verification, login, concurrent refresh, logout, and password reset', async ({ request }) => {
  await request.delete(`${MAILPIT}/api/v1/messages`);
  const account = await registerTestUser(request, 'auth-proof', 'Auth Proof');

  const verification = await latestMailText(request, 'Verify your Carve email');
  const verifyToken = verification.match(/verify-email\?token=([A-Za-z0-9_-]+)/)?.[1];
  expect(verifyToken).toBeTruthy();
  await jsonOK(await request.post(`${API}/v1/auth/verify`, {
    data: { token: verifyToken },
  }), 'verify email');

  const badLogin = await request.post(`${API}/v1/auth/login`, {
    data: { email: account.email, password: 'incorrect-password' },
  });
  expect(badLogin.status()).toBe(401);
  await jsonOK(await request.post(`${API}/v1/auth/login`, {
    data: { email: account.email, password: account.password },
  }), 'valid login');

  // Two callers rotate the same token concurrently. Exactly one wins, and the
  // winning replacement remains usable (the loser must not revoke it).
  const firstClient = await playwrightRequest.newContext();
  const secondClient = await playwrightRequest.newContext();
  const refreshCalls = await Promise.all([
    firstClient.post(`${API}/v1/auth/refresh`, { data: { refresh_token: account.refresh_token } }),
    secondClient.post(`${API}/v1/auth/refresh`, { data: { refresh_token: account.refresh_token } }),
  ]);
  expect(refreshCalls.map((response) => response.status()).sort()).toEqual([200, 401]);
  const winningResponse = refreshCalls.find((response) => response.ok());
  expect(winningResponse).toBeTruthy();
  const winningPair = await winningResponse!.json() as { refresh_token: string };
  await firstClient.dispose();
  await secondClient.dispose();

  const thirdClient = await playwrightRequest.newContext();
  const nextPair = await jsonOK(await thirdClient.post(`${API}/v1/auth/refresh`, {
    data: { refresh_token: winningPair.refresh_token },
  }), 'winning refresh remains valid') as { refresh_token: string };
  expect(nextPair.refresh_token).toBeTruthy();

  const logout = await thirdClient.post(`${API}/v1/auth/logout`, {
    data: { refresh_token: nextPair.refresh_token },
  });
  expect(logout.status()).toBe(204);
  const afterLogout = await thirdClient.post(`${API}/v1/auth/refresh`, {
    data: { refresh_token: nextPair.refresh_token },
  });
  expect(afterLogout.status()).toBe(401);
  await thirdClient.dispose();

  await request.delete(`${MAILPIT}/api/v1/messages`);
  await jsonOK(await request.post(`${API}/v1/auth/forgot`, {
    data: { email: account.email },
  }), 'request password reset');
  const resetMail = await latestMailText(request, 'Reset your Carve password');
  const resetToken = resetMail.match(/reset-password\?token=([A-Za-z0-9_-]+)/)?.[1];
  expect(resetToken).toBeTruthy();
  const newPassword = 'new-super-secret-456';
  await jsonOK(await request.post(`${API}/v1/auth/reset`, {
    data: { token: resetToken, password: newPassword },
  }), 'reset password');
  await jsonOK(await request.post(`${API}/v1/auth/login`, {
    data: { email: account.email, password: newPassword },
  }), 'login with reset password');
});

test('onboarding knowledge affects every tokenizer, lookup is shaped, starter deck is idempotent, and grammar persists', async ({ request }) => {
  const account = await registerTestUser(request, 'language-proof', 'Language Proof');
  const headers = bearer(account.access_token);
  const samples = [
    ['ja', '猫が好きです。'],
    ['en', 'Language learning works.'],
    ['zh-cn', '我喜欢学习语言。'],
    ['zh-tw', '我喜歡學習語言。'],
    ['ko', '언어를 공부합니다.'],
    ['es', 'Aprendo idiomas en casa.'],
    ['de', 'Ich lerne Sprachen im Haus.'],
    ['fr', 'J’apprends les langues à la maison.'],
    ['it', 'Studio le lingue a casa.'],
    ['pt', 'Eu estudo idiomas em casa.'],
    ['vi', 'Tôi học ngôn ngữ ở nhà.'],
  ] as const;

  for (const [language, text] of samples) {
    const first = await jsonOK(await request.post(`${API}/v1/nlp/tokenize`, {
      headers,
      data: { language, text, known_lemmas: [], learning_lemmas: [] },
    }), `${language} cold tokenize`) as { tokens: Array<Record<string, any>> };
    const content = first.tokens.find((token) => token.is_content_word && token.lemma);
    expect(content, `${language} should produce a content token`).toBeTruthy();

    const marked = await jsonOK(await request.post(`${API}/v1/onboarding/known-words`, {
      headers,
      data: { language, lemmas: [content!.lemma] },
    }), `${language} mark known`) as { marked: number };
    expect(marked.marked).toBe(1);

    const second = await jsonOK(await request.post(`${API}/v1/nlp/tokenize`, {
      headers,
      data: { language, text, known_lemmas: [], learning_lemmas: [] },
    }), `${language} knowledge-aware tokenize`) as { tokens: Array<Record<string, any>> };
    expect(
      second.tokens.some((token) => token.lemma === content!.lemma && token.user_status === 'known'),
      `${language} persisted known word should be reflected by the tokenizer`,
    ).toBe(true);

    const lookup = await jsonOK(await request.post(`${API}/v1/nlp/lookup`, {
      headers,
      data: { surface: content!.lemma, language },
    }), `${language} lookup`) as Record<string, any>;
    expect(typeof lookup.found).toBe('boolean');
    expect(Array.isArray(lookup.definitions)).toBe(true);
    if (language === 'vi') {
      expect(lookup.found, 'Vietnamese dictionary content is an explicit limitation').toBe(false);
    }
  }

  const subscribed = await jsonOK(await request.post(`${API}/v1/onboarding/starter-deck`, {
    headers,
    data: { language: 'ja' },
  }), 'subscribe starter deck') as { status: string; deck_id: string };
  expect(subscribed.status).toBe('subscribed');
  expect(subscribed.deck_id).toBeTruthy();
  const firstCards = await jsonOK(await request.get(`${API}/v1/cards?language=ja&limit=100`, { headers }), 'starter cards');
  expect(firstCards.total).toBeGreaterThan(0);
  await jsonOK(await request.post(`${API}/v1/onboarding/starter-deck`, {
    headers,
    data: { language: 'ja' },
  }), 'repeat starter subscription');
  const repeatedCards = await jsonOK(await request.get(`${API}/v1/cards?language=ja&limit=100`, { headers }), 'repeat starter cards');
  expect(repeatedCards.total).toBe(firstCards.total);

  const catalog = await jsonOK(await request.get(`${API}/v1/nlp/grammar/patterns?language=ja`, { headers }), 'grammar catalog');
  expect(catalog.patterns.length).toBeGreaterThan(0);
  const patternID = catalog.patterns[0].id as string;
  await jsonOK(await request.post(`${API}/v1/grammar/known`, {
    headers,
    data: { language_code: 'ja', pattern_id: patternID },
  }), 'mark grammar known');
  const known = await jsonOK(await request.get(`${API}/v1/grammar/known?language=ja`, { headers }), 'list known grammar');
  expect(known.pattern_ids).toContain(patternID);
  await jsonOK(await request.delete(`${API}/v1/grammar/known`, {
    headers,
    data: { language_code: 'ja', pattern_id: patternID },
  }), 'unmark grammar known');
  const unmarked = await jsonOK(await request.get(`${API}/v1/grammar/known?language=ja`, { headers }), 'list unmarked grammar');
  expect(unmarked.pattern_ids).not.toContain(patternID);
});

test('card mining/media/lifecycle, daily limits, replay, exact undo, leeches, output, immersion, and stats', async ({ request }) => {
  const account = await registerTestUser(request, 'card-proof', 'Card Proof');
  const headers = bearer(account.access_token);
  await jsonOK(await request.put(`${API}/v1/settings/fsrs`, {
    headers,
    data: { language_code: 'en', daily_new_limit: 2, leech_threshold: 1, target_retention: 0.9 },
  }), 'save FSRS settings');

  const cards: Array<{ id: string }> = [];
  for (const word of ['proof-alpha', 'proof-beta', 'proof-gamma']) {
    cards.push(await createTestCard(request, API, account.access_token, {
      lemma: word,
      languageCode: 'en',
      backText: `${word} definition`,
      sentence: `A sentence containing ${word}.`,
    }));
  }

  const due = await jsonOK(await request.get(`${API}/v1/review/due-count?language=en`, { headers }), 'daily due count');
  expect(due.due_count).toBe(2);
  const session = await jsonOK(await request.get(`${API}/v1/review/session?language=en&limit=50`, { headers }), 'daily session');
  expect(session.total).toBe(2);
  await jsonOK(await request.put(`${API}/v1/settings/fsrs`, {
    headers,
    data: { language_code: 'en', daily_new_limit: 3, leech_threshold: 1, target_retention: 0.9 },
  }), 'raise daily cap for lifecycle proof');

  const duplicate = await jsonOK(await request.post(`${API}/v1/cards`, {
    headers,
    data: { language_code: 'en', lemma: 'proof-alpha', back_text: 'duplicate' },
  }), 'duplicate mine');
  expect(duplicate.existing).toBe(true);
  expect(duplicate.id).toBe(cards[0].id);

  const updatedText = 'updated, Unicode café 日本語';
  const update = await request.patch(`${API}/v1/cards/${cards[0].id}`, {
    headers,
    data: { back_text: updatedText, notes: 'proof note', tags: ['audit', 'core'] },
  });
  expect(update.status()).toBe(204);
  let detail = await jsonOK(await request.get(`${API}/v1/cards/${cards[0].id}`, { headers }), 'updated card');
  expect(detail.back_text).toBe(updatedText);
  expect(detail.tags).toEqual(['audit', 'core']);

  const similar = await jsonOK(await request.post(`${API}/v1/cards/find-similar`, {
    headers,
    data: { language_code: 'en', sentence: 'A sentence containing proof-alpha.', threshold: 0.8 },
  }), 'find similar');
  expect(similar.matches.some((candidate: { id: string }) => candidate.id === cards[0].id)).toBe(true);

  const media = await jsonOK(await request.post(`${API}/v1/cards/${cards[0].id}/media`, {
    headers,
    multipart: {
      image: { name: 'proof.png', mimeType: 'image/png', buffer: Buffer.from('proof-image') },
      audio: { name: 'proof.webm', mimeType: 'audio/webm', buffer: Buffer.from('proof-audio') },
      subtitle_translation: 'translated proof',
      video_source_url: 'https://example.com/video',
      subtitle_start_ms: '1000',
      subtitle_end_ms: '2500',
    },
  }), 'attach media');
  expect(media.image_url).toMatch(/^http:\/\/127\.0\.0\.1:\d+\/screenshots\//);
  expect(media.audio_url).toMatch(/^http:\/\/127\.0\.0\.1:\d+\/audio\//);
  expect((await request.get(media.image_url)).ok()).toBe(true);
  expect((await request.get(media.audio_url)).ok()).toBe(true);

  await jsonOK(await request.post(`${API}/v1/cards/${cards[1].id}/suspend`, { headers }), 'suspend');
  detail = await jsonOK(await request.get(`${API}/v1/cards/${cards[1].id}`, { headers }), 'suspended card');
  expect(detail.suspended).toBe(true);
  await jsonOK(await request.post(`${API}/v1/cards/${cards[1].id}/unsuspend`, { headers }), 'unsuspend');
  await jsonOK(await request.post(`${API}/v1/cards/${cards[1].id}/bury`, { headers }), 'bury');
  let buriedSession = await jsonOK(await request.get(`${API}/v1/review/session?language=en&limit=50`, { headers }), 'buried session');
  expect(buriedSession.cards.map((card: { id: string }) => card.id).includes(cards[1].id)).toBe(false);
  await jsonOK(await request.post(`${API}/v1/cards/${cards[1].id}/unbury`, { headers }), 'unbury');
  await jsonOK(await request.post(`${API}/v1/cards/bulk`, {
    headers,
    data: { action: 'suspend', ids: [cards[1].id, cards[2].id] },
  }), 'bulk suspend');
  await jsonOK(await request.post(`${API}/v1/cards/bulk`, {
    headers,
    data: { action: 'unsuspend', ids: [cards[1].id, cards[2].id] },
  }), 'bulk unsuspend');
  buriedSession = await jsonOK(await request.get(`${API}/v1/review/session?language=en&limit=50`, { headers }), 'unburied session');
  expect(buriedSession.cards.map((card: { id: string }) => card.id).includes(cards[1].id)).toBe(true);

  const t0 = new Date(Date.now() - 120_000).toISOString();
  const first = await submitReview(request, account.access_token, cards[0].id, 3, t0);
  expect(first.body.state).toBe('review');
  const replay = await submitReview(request, account.access_token, cards[0].id, 3, t0, first.eventID);
  expect(replay.body).toEqual(first.body);
  const stateAfterFirst = await jsonOK(await request.get(`${API}/v1/cards/${cards[0].id}`, { headers }), 'state after first review');

  const t1 = new Date(Date.now() - 60_000).toISOString();
  const second = await submitReview(request, account.access_token, cards[0].id, 3, t1);
  await jsonOK(await request.post(`${API}/v1/review/undo`, { headers }), 'undo second review');
  const stateAfterUndo = await jsonOK(await request.get(`${API}/v1/cards/${cards[0].id}`, { headers }), 'state after exact undo');
  for (const key of ['fsrs_state', 'due', 'stability', 'difficulty', 'reps', 'lapses']) {
    expect(stateAfterUndo[key], `undo should restore ${key}`).toEqual(stateAfterFirst[key]);
  }
  const repeatedSecond = await submitReview(request, account.access_token, cards[0].id, 3, t1);
  expect(repeatedSecond.body).toEqual(second.body);
  await jsonOK(await request.post(`${API}/v1/review/undo`, { headers }), 'undo repeated second review');

  const lapse = await submitReview(request, account.access_token, cards[0].id, 1, new Date().toISOString());
  expect(lapse.body.is_leech).toBe(true);
  detail = await jsonOK(await request.get(`${API}/v1/cards/${cards[0].id}`, { headers }), 'leech card');
  expect(detail.suspended).toBe(true);
  expect(detail.is_leech).toBe(true);
  const notifications = await jsonOK(await request.get(`${API}/v1/review/notifications`, { headers }), 'leech notifications');
  expect(notifications.notifications.some((item: { type: string }) => item.type === 'leech_suspended')).toBe(true);

  // A manually unsuspended leech can be reviewed again. Undoing that later
  // review must preserve the pre-existing leech flag rather than clearing it.
  await jsonOK(await request.post(`${API}/v1/cards/${cards[0].id}/unsuspend`, { headers }), 'unsuspend leech');
  await submitReview(request, account.access_token, cards[0].id, 3, new Date().toISOString());
  await jsonOK(await request.post(`${API}/v1/review/undo`, { headers }), 'undo review of existing leech');
  detail = await jsonOK(await request.get(`${API}/v1/cards/${cards[0].id}`, { headers }), 'restored existing leech');
  expect(detail.suspended).toBe(false);
  expect(detail.is_leech).toBe(true);

  await jsonOK(await request.post(`${API}/v1/review/undo`, { headers }), 'undo lapse');
  detail = await jsonOK(await request.get(`${API}/v1/cards/${cards[0].id}`, { headers }), 'unleech card');
  expect(detail.suspended).toBe(false);
  expect(detail.is_leech).toBe(false);
  const notificationsAfterUndo = await jsonOK(await request.get(`${API}/v1/review/notifications`, { headers }), 'notifications after undo');
  expect(notificationsAfterUndo.notifications).toHaveLength(0);

  const intervals = await jsonOK(await request.get(`${API}/v1/review/intervals?card_id=${cards[0].id}`, { headers }), 'interval preview');
  expect(new Date(intervals.easy).getTime()).toBeGreaterThanOrEqual(new Date(intervals.good).getTime());
  const forecast = await jsonOK(await request.get(`${API}/v1/review/forecast?language=en&days=14`, { headers }), 'review forecast');
  expect(forecast.forecast).toHaveLength(14);

  const exercises = await jsonOK(await request.get(`${API}/v1/output/exercises?language=en`, { headers }), 'output exercises');
  expect(exercises.exercises.length).toBeGreaterThan(0);
  const submission = await jsonOK(await request.post(`${API}/v1/output/submit`, {
    headers,
    data: { exercise_id: exercises.exercises[0].id, answer_text: 'A proof answer.' },
  }), 'output submission');
  expect(submission.submission_id).toBeTruthy();
  expect(typeof submission.feedback).toBe('string');

  await jsonOK(await request.post(`${API}/v1/immersion`, {
    headers,
    data: {
      language_code: 'en', session_type: 'reading', duration_sec: 180,
      started_at: new Date().toISOString(), url: 'https://example.com/article',
    },
  }), 'immersion session');
  const stats = await jsonOK(await request.get(`${API}/v1/stats?language=en`, { headers }), 'stats');
  expect(stats.total_ever_reviews).toBe(1);
  expect(stats.reading_minutes).toBe(3);
});

test('Anki/Migaku/Yomitan/JPDB imports, JSON/CSV/APKG exports, reader imports, and round-trip scheduling', async ({ request }) => {
  const producer = await registerTestUser(request, 'export-proof', 'Export Proof');
  const producerHeaders = bearer(producer.access_token);
  const source = await createTestCard(request, API, producer.access_token, {
    lemma: '日本語 café',
    languageCode: 'ja',
    backText: 'Japanese language, coffee',
    sentence: '日本語と café を勉強する。',
  });
  await submitReview(request, producer.access_token, source.id, 3, new Date().toISOString());

  const jsonExportResponse = await request.get(`${API}/v1/export`, { headers: producerHeaders });
  const jsonExport = await jsonOK(jsonExportResponse, 'JSON export');
  expect(jsonExport.cards.some((card: { front_text: string }) => card.front_text === '日本語 café')).toBe(true);
  expect(jsonExport.review_events).toHaveLength(1);

  const csvExport = await request.get(`${API}/v1/export/csv?language=ja`, { headers: producerHeaders });
  expect(csvExport.ok()).toBe(true);
  const csvText = await csvExport.text();
  expect(csvText).toContain('日本語 café');
  expect(csvText).toContain('Japanese language, coffee');

  const apkgExport = await request.get(`${API}/v1/export/apkg?language=ja`, { headers: producerHeaders });
  expect(apkgExport.ok()).toBe(true);
  const apkg = await apkgExport.body();
  expect(apkg.byteLength).toBeGreaterThan(1_000);

  const consumer = await registerTestUser(request, 'import-proof', 'Import Proof');
  const headers = bearer(consumer.access_token);
  const anki = await jsonOK(await request.post(`${API}/v1/import/anki`, {
    headers,
    multipart: {
      language: 'ja',
      file: { name: 'roundtrip.apkg', mimeType: 'application/octet-stream', buffer: apkg },
    },
  }), 'Anki round-trip import');
  expect(anki.imported).toBe(1);

  const migaku = await jsonOK(await request.post(`${API}/v1/import/migaku-csv`, {
    headers,
    multipart: {
      language: 'ja',
      file: {
        name: 'migaku.csv', mimeType: 'text/csv',
        buffer: Buffer.from('word,reading,definition,sentence,status\n移行,いこう,"migration, transition",移行を確認する,learning\n'),
      },
    },
  }), 'Migaku import');
  expect(migaku.imported).toBe(1);

  const yomitanArchive = Buffer.from(zipSync({
    'index.json': strToU8(JSON.stringify({ title: 'Proof dictionary' })),
    'term_bank_1.json': strToU8(JSON.stringify([['猫', 'ねこ', '', '', 1, ['cat']]])),
  }));
  const yomitan = await jsonOK(await request.post(`${API}/v1/import/yomitan`, {
    headers,
    multipart: {
      language: 'ja',
      file: { name: 'yomitan.zip', mimeType: 'application/zip', buffer: yomitanArchive },
    },
  }), 'Yomitan import');
  expect(yomitan).toMatchObject({ imported: 1, skipped: 0, type: 'known_words' });

  const jpdb = await jsonOK(await request.post(`${API}/v1/import/jpdb-csv`, {
    headers,
    multipart: {
      language: 'ja',
      file: {
        name: 'jpdb.csv', mimeType: 'text/csv',
        buffer: Buffer.from('vocabulary,reading,status\n犬,いぬ,known\n鳥,とり,learning\n'),
      },
    },
  }), 'JPDB import');
  expect(jpdb).toMatchObject({ imported: 2, skipped: 0, type: 'known_words' });

  const knowledge = await jsonOK(await request.post(`${API}/v1/nlp/tokenize`, {
    headers,
    data: { language: 'ja', text: '猫と犬と鳥', known_lemmas: [], learning_lemmas: [] },
  }), 'imported knowledge reaches tokenizer');
  const statusByLemma = new Map(knowledge.tokens.map((token: Record<string, string>) => [token.lemma, token.user_status]));
  expect(statusByLemma.get('猫')).toBe('known');
  expect(statusByLemma.get('犬')).toBe('known');
  expect(statusByLemma.get('鳥')).toBe('learning');

  const importedCards = await jsonOK(await request.get(`${API}/v1/cards?language=ja&limit=100`, { headers }), 'imported cards');
  expect(importedCards.total).toBe(2);
  expect(importedCards.cards.some((card: { fsrs_state: string }) => card.fsrs_state === 'review')).toBe(true);
  expect(importedCards.cards.some((card: { front_text: string }) => card.front_text === '移行')).toBe(true);

  const invalidAnki = await request.post(`${API}/v1/import/anki`, {
    headers,
    multipart: {
      language: 'ja',
      file: { name: 'broken.apkg', mimeType: 'application/octet-stream', buffer: Buffer.from('not a zip') },
    },
  });
  expect(invalidAnki.status()).toBe(400);
  const invalidYomitan = await request.post(`${API}/v1/import/yomitan`, {
    headers,
    multipart: {
      language: 'ja',
      file: { name: 'broken.zip', mimeType: 'application/zip', buffer: Buffer.from('not a zip') },
    },
  });
  expect(invalidYomitan.status()).toBe(400);

  for (const fixture of [
    {
      name: 'reader.txt',
      mimeType: 'text/plain',
      body: '猫と犬について日本語で読む。',
    },
    {
      name: 'subtitles.srt',
      mimeType: 'application/x-subrip',
      body: '1\n00:00:01,000 --> 00:00:03,000\n<i>鳥を見ました。</i>\n',
    },
  ]) {
    const imported = await jsonOK(await request.post(`${API}/v1/library/import`, {
      headers,
      multipart: {
        language: 'ja',
        file: { name: fixture.name, mimeType: fixture.mimeType, buffer: Buffer.from(fixture.body) },
      },
    }), `${fixture.name} library import`);
    const reader = await jsonOK(await request.get(`${API}/v1/library/${imported.id}/reader`, { headers }), `${fixture.name} reader`);
    expect(reader.tokens.length).toBeGreaterThan(0);
  }

  const urlItem = await jsonOK(await request.post(`${API}/v1/library`, {
    headers,
    data: { url: 'https://example.com', language: 'en' },
  }), 'URL library import');
  expect(urlItem.id).toBeTruthy();
  const library = await jsonOK(await request.get(`${API}/v1/library?language=ja`, { headers }), 'library list');
  expect(library.items).toHaveLength(2);

  const epub = await request.post(`${API}/v1/library/import`, {
    headers,
    multipart: {
      language: 'ja',
      file: { name: 'unsupported.epub', mimeType: 'application/epub+zip', buffer: Buffer.from('epub') },
    },
  });
  expect(epub.status()).toBe(400);
});

test('50 browser-offline reviews survive a lost response and drain exactly once', async ({ page, request }) => {
  test.setTimeout(120_000);
  const account = await registerTestUser(request, 'offline-proof', 'Offline Proof');
  const headers = bearer(account.access_token);
  await jsonOK(await request.put(`${API}/v1/settings/fsrs`, {
    headers,
    data: { language_code: 'ja', daily_new_limit: 50 },
  }), 'allow 50 daily cards');
  for (let index = 0; index < 50; index++) {
    await createTestCard(request, API, account.access_token, {
      lemma: `offline-${index.toString().padStart(2, '0')}-${randomUUID()}`,
      languageCode: 'ja',
      backText: `offline proof ${index}`,
    });
  }

  await seedAuthenticatedPage(page, account.access_token);
  await page.goto('/review');
  await expect(page.locator('.show-btn')).toBeVisible();
  await page.context().setOffline(true);

  for (let index = 0; index < 50; index++) {
    await page.locator('.show-btn').click();
    await page.getByRole('button', { name: /Good/ }).click();
    await expect.poll(() => offlineQueueLength(page), { timeout: 5_000 }).toBe(index + 1);
  }
  await expect(page.getByText('Session complete')).toBeVisible();
  expect(await offlineQueueLength(page)).toBe(50);

  // Commit the first event at the server, then drop its response. The browser
  // must retain that one queue item while draining the other 49.
  let droppedCommittedResponse = false;
  await page.route('**/v1/review/events', async (route) => {
    if (!droppedCommittedResponse) {
      const upstream = await route.fetch();
      expect(upstream.ok()).toBe(true);
      droppedCommittedResponse = true;
      await route.abort('failed');
      return;
    }
    await route.continue();
  });
  await page.context().setOffline(false);
  await expect.poll(() => offlineQueueLength(page), { timeout: 30_000 }).toBe(1);
  expect(droppedCommittedResponse).toBe(true);

  await page.unroute('**/v1/review/events');
  await page.evaluate(() => window.dispatchEvent(new Event('online')));
  await expect.poll(() => offlineQueueLength(page), { timeout: 15_000 }).toBe(0);

  const stats = await jsonOK(await request.get(`${API}/v1/stats?language=ja`, { headers }), 'offline review stats');
  expect(stats.total_ever_reviews).toBe(50);
});
