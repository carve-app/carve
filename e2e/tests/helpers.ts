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
  expect(body.access_token, `register ${email} should return access_token`).toBeTruthy();
  return { apiBase, email, password, access_token: body.access_token as string };
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
