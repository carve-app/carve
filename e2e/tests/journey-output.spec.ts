import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { registerTestUser, seedAuthenticatedPage } from './helpers';

async function seed(page: any, request: any) {
  const user = await registerTestUser(request, 'out', 'Output Tester');
  await seedAuthenticatedPage(page, user.access_token);
  return { access_token: user.access_token, apiBase: user.apiBase };
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
