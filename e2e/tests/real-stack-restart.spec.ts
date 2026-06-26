import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { basename, resolve } from 'node:path';
import { createTestCard, registerTestUser } from './helpers';

const API = process.env.API_BASE ?? 'http://127.0.0.1:8080';
const ROOT = basename(process.cwd()) === 'e2e' ? resolve(process.cwd(), '..') : process.cwd();

test.skip(process.env.E2E_USE_REAL !== '1', 'requires the isolated real stack');

test('committed review remains exactly-once across an API process restart', async ({ request }) => {
  test.setTimeout(90_000);
  const account = await registerTestUser(request, 'restart-proof', 'Restart Proof');
  const card = await createTestCard(request, API, account.access_token, {
    lemma: `restart-${randomUUID()}`,
    languageCode: 'en',
    backText: 'restart persistence proof',
  });
  const event = {
    event_id: randomUUID(),
    card_id: card.id,
    rating: 3,
    time_taken_ms: 900,
    reviewed_at: new Date().toISOString(),
  };
  const headers = { Authorization: `Bearer ${account.access_token}` };
  const first = await request.post(`${API}/v1/review/events`, { headers, data: event });
  expect(first.ok(), await first.text()).toBe(true);
  const firstBody = await first.json();

  execFileSync('docker', ['compose', 'restart', 'api'], {
    cwd: ROOT,
    env: process.env,
    stdio: 'pipe',
    timeout: 60_000,
  });
  await expect.poll(async () => {
    const ready = await request.get(`${API}/health/ready`);
    return ready.status();
  }, { timeout: 45_000 }).toBe(200);

  const replay = await request.post(`${API}/v1/review/events`, { headers, data: event });
  expect(replay.ok(), await replay.text()).toBe(true);
  expect(await replay.json()).toEqual(firstBody);

  const detail = await request.get(`${API}/v1/cards/${card.id}`, { headers });
  expect(detail.ok()).toBe(true);
  expect((await detail.json()).reps).toBe(1);
  const stats = await request.get(`${API}/v1/stats?language=en`, { headers });
  expect(stats.ok()).toBe(true);
  expect((await stats.json()).total_ever_reviews).toBe(1);
});
