import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

async function seed(page: any, request: any) {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `out+${Date.now()}@example.com`;
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'Output Tester' },
  });
  const { access_token } = await reg.json();
  await page.goto('/');
  await page.evaluate((t: string) => localStorage.setItem('carve_access_token', t), access_token);
  return { access_token, apiBase };
}

test('output landing page renders + a11y', async ({ page, request }) => {
  await seed(page, request);
  await page.goto('/output');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/output' });
});

test('shadowing page renders prompt or empty state', async ({ page, request }) => {
  await seed(page, request);
  await page.goto('/output/shadowing/some-id');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/output/shadowing' });
  await expect(page.locator('button:has-text("Start recording")').first()).toBeVisible();
});

test('speaking page renders prompt or empty state', async ({ page, request }) => {
  await seed(page, request);
  await page.goto('/output/speaking/some-id');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/output/speaking' });
  await expect(page.locator('button:has-text("Start speaking")').first()).toBeVisible();
});
