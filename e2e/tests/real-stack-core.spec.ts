import { expect, test, type APIRequestContext } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { createTestCard, registerTestUser } from './helpers';

const API = process.env.API_BASE ?? 'http://127.0.0.1:8080';
const MAILPIT = process.env.MAILPIT_BASE ?? 'http://127.0.0.1:8025';

test.skip(process.env.E2E_USE_REAL !== '1', 'requires the isolated real stack');

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

test('real auth mail, multilingual NLP, and replay-safe review', async ({ request }) => {
  await request.delete(`${MAILPIT}/api/v1/messages`);
  const account = await registerTestUser(request, 'real-stack', 'Real Stack');
  const authHeader = { Authorization: `Bearer ${account.access_token}` };

  const verification = await latestMailText(request, 'Verify your Carve email');
  const verifyToken = verification.match(/verify-email\?token=([A-Za-z0-9]+)/)?.[1];
  expect(verifyToken).toBeTruthy();
  const verified = await request.post(`${API}/v1/auth/verify`, { data: { token: verifyToken } });
  expect(verified.ok()).toBe(true);

  for (const [language, text] of [
    ['ja', '日本語を勉強します。'], ['en', 'Language learning works.'],
    ['zh-cn', '我喜欢学习语言。'], ['ko', '언어를 공부합니다.'],
    ['es', 'Aprendo idiomas.'], ['de', 'Ich lerne Sprachen.'],
    ['fr', 'J’apprends les langues.'], ['it', 'Studio le lingue.'],
    ['pt', 'Eu estudo idiomas.'], ['vi', 'Tôi học ngôn ngữ.'],
  ] as const) {
    const response = await request.post(`${API}/v1/nlp/tokenize`, {
      headers: authHeader,
      data: { language, text, known_lemmas: [], learning_lemmas: [] },
    });
    expect(response.ok(), `${language} tokenize: ${response.status()}`).toBe(true);
    const body = await response.json() as { tokens?: unknown[] };
    expect(body.tokens?.length, `${language} should produce tokens`).toBeGreaterThan(0);
  }

  const card = await createTestCard(request, API, account.access_token, {
    lemma: `proof-${randomUUID()}`, languageCode: 'en', backText: 'proof',
  });
  const event = {
    event_id: randomUUID(), card_id: card.id, rating: 3,
    time_taken_ms: 900, reviewed_at: new Date().toISOString(),
  };
  const first = await request.post(`${API}/v1/review/events`, { headers: authHeader, data: event });
  const replay = await request.post(`${API}/v1/review/events`, { headers: authHeader, data: event });
  expect(first.ok()).toBe(true);
  expect(replay.ok()).toBe(true);
  expect(await replay.json()).toEqual(await first.json());

  await request.post(`${API}/v1/auth/forgot`, { data: { email: account.email } });
  const resetMail = await latestMailText(request, 'Reset your Carve password');
  const resetToken = resetMail.match(/reset-password\?token=([A-Za-z0-9]+)/)?.[1];
  expect(resetToken).toBeTruthy();
  const newPassword = 'new-super-secret-456';
  const reset = await request.post(`${API}/v1/auth/reset`, { data: { token: resetToken, password: newPassword } });
  expect(reset.ok()).toBe(true);
  const login = await request.post(`${API}/v1/auth/login`, {
    data: { email: account.email, password: newPassword },
  });
  expect(login.ok()).toBe(true);
});
