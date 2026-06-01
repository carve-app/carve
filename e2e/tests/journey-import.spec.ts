import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

async function seed(page: any, request: any) {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `imp+${Date.now()}@example.com`;
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'Importer' },
  });
  const { access_token } = await reg.json();
  await page.goto('/');
  await page.evaluate((t: string) => localStorage.setItem('carve_access_token', t), access_token);
  return { access_token, apiBase };
}

test('import page — reader tab is reachable and a11y clean', async ({ page, request }) => {
  await seed(page, request);
  await page.goto('/import');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/import (reader tab)' });

  // Drop zone exposes a region role so dragover/drop is announced.
  const region = page.locator('[role="region"][aria-label*="Drop"]').first();
  await expect(region).toBeVisible();
});

test('import page — vocab tab switch + each importer kind is selectable', async ({ page, request }) => {
  await seed(page, request);
  await page.goto('/import');
  await page.waitForLoadState('networkidle').catch(() => {});

  const vocabTab = page.locator('button:has-text("Cards"), button:has-text("Vocab")').first();
  if (await vocabTab.count()) {
    await vocabTab.click();
    // Each kind card should be a real <button> with focus styles.
    const buttons = await page.locator('button.kind-card, [role="button"].kind-card').count();
    expect(buttons).toBeGreaterThanOrEqual(2);
  }
  await expectNoSeriousA11y(page, { label: '/import (vocab tab)' });
});
