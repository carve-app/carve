import { expect, type APIRequestContext, type Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';

export const TEST_PASSWORD = 'super-secret-123';

export function uniqueEmail(prefix: string): string {
  return `${prefix}+${Date.now()}-${randomUUID()}@example.com`;
}

export async function registerTestUser(
  request: APIRequestContext,
  prefix: string,
  displayName: string,
  password = TEST_PASSWORD,
) {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = uniqueEmail(prefix);
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password, display_name: displayName },
  });
  expect(reg.ok(), `register ${email}`).toBe(true);
  const body = await reg.json();

  if (!body.access_token) {
    let verificationToken = body.verification_token_test as string | undefined;
    const mailpitBase = process.env.MAILPIT_BASE;
    if (!verificationToken && mailpitBase) {
      await expect.poll(async () => {
        const list = await request.get(`${mailpitBase}/api/v1/messages`);
        if (!list.ok()) return false;
        const payload = await list.json() as { messages?: Array<{ ID: string; Subject: string; To?: Array<{ Address: string }> }> };
        const candidate = payload.messages?.find(message =>
          message.Subject === 'Verify your Carve email' &&
          (!message.To || message.To.some(recipient => recipient.Address === email))
        );
        if (!candidate) return false;
        const detail = await request.get(`${mailpitBase}/api/v1/message/${candidate.ID}`);
        if (!detail.ok()) return false;
        const message = await detail.json() as { Text?: string; HTML?: string };
        verificationToken = (message.Text ?? message.HTML ?? '').match(/verify-email\?token=([A-Za-z0-9_-]+)/)?.[1];
        return Boolean(verificationToken);
      }, { timeout: 10_000 }).toBe(true);
    }
    expect(verificationToken, `register ${email} should deliver a verification token`).toBeTruthy();
    const verify = await request.post(`${apiBase}/v1/auth/verify`, { data: { token: verificationToken } });
    expect(verify.ok(), `verify ${email}`).toBe(true);
    const login = await request.post(`${apiBase}/v1/auth/login`, { data: { email, password } });
    expect(login.ok(), `login ${email} after verification`).toBe(true);
    Object.assign(body, await login.json());
  }

  expect(body.access_token, `login ${email} should return access_token`).toBeTruthy();
  expect(body.refresh_token, `login ${email} should return refresh_token`).toBeTruthy();
  return {
    apiBase,
    email,
    password,
    access_token: body.access_token as string,
    refresh_token: body.refresh_token as string,
  };
}

export async function createTestCard(
  request: APIRequestContext,
  apiBase: string,
  accessToken: string,
  {
    lemma,
    backText = lemma,
    languageCode = 'ja',
    reading = '',
    sentence,
  }: {
    lemma: string;
    backText?: string;
    languageCode?: string;
    reading?: string;
    sentence?: string;
  },
) {
  const res = await request.post(`${apiBase}/v1/cards`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    data: {
      language_code: languageCode,
      lemma,
      reading,
      front_text: lemma,
      back_text: backText,
      sentence,
    },
  });
  const text = await res.text();
  expect(res.ok(), `create card ${lemma}: ${res.status()} ${text}`).toBe(true);
  return JSON.parse(text) as { id: string };
}

export async function seedAuthenticatedPage(page: Page, accessToken: string) {
  await page.goto('/');
  await page.evaluate((token: string) => localStorage.setItem('carve_access_token', token), accessToken);
}
