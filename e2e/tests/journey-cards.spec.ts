import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

async function setup(page: any, request: any) {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `cards+${Date.now()}@example.com`;
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'Cards Tester' },
  });
  const { access_token } = await reg.json();

  // Seed three cards
  const cardIds: string[] = [];
  for (const word of ['apple', 'banana', 'cherry']) {
    const r = await request.post(`${apiBase}/v1/cards`, {
      headers: { Authorization: `Bearer ${access_token}` },
      data: { front_text: word, back_text: word.toUpperCase(), language_code: 'en' },
    });
    const card = await r.json();
    cardIds.push(card.id);
  }
  await page.goto('/');
  await page.evaluate((t: string) => localStorage.setItem('carve_access_token', t), access_token);
  return { access_token, cardIds, apiBase };
}

test('cards list — renders + a11y clean', async ({ page, request }) => {
  await setup(page, request);
  await page.goto('/cards');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/cards' });
});

test('card detail edit form — every label is bound', async ({ page, request }) => {
  const { cardIds } = await setup(page, request);
  await page.goto(`/cards/${cardIds[0]}`);
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/cards/[id]' });

  const edit = page.locator('button:has-text("Edit")').first();
  if (await edit.count()) {
    await edit.click();
    // Every label must associate with an input (svelte-check now blocks regressions).
    const orphanLabels = await page.locator('label:not([for]):not(:has(input)):not(:has(textarea)):not(:has(select))').count();
    expect(orphanLabels).toBe(0);
  }
});
