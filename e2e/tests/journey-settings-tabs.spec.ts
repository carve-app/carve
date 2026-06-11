import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { registerTestUser, seedAuthenticatedPage } from './helpers';

/**
 * L5/L8 — every Settings tab is reachable and accessible.
 *
 * Per the inventory:
 *   Account · Display · Review · Mining · Sites · Sync
 */
const TABS = ['account', 'display', 'review', 'mining', 'sites', 'sync'] as const;

async function seedAndLogin(page: any, request: any, apiBase: string, prefix: string) {
  const user = await registerTestUser(request, prefix, 'Settings Tester');
  expect(user.apiBase).toBe(apiBase);
  await seedAuthenticatedPage(page, user.access_token);
  return user.access_token;
}

for (const tab of TABS) {
  test(`settings: ${tab} tab is reachable and accessible`, async ({ page, request }) => {
    const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
    await seedAndLogin(page, request, apiBase, `set-${tab}`);

    await page.goto(`/settings#${tab}`);
    await page.waitForLoadState('networkidle').catch(() => {});

    // The tab button has aria-selected=true; its panel should be visible.
    const tabBtn = page.locator(`#tab-${tab}`);
    await expect(tabBtn).toBeVisible();
    await expect(tabBtn).toHaveAttribute('aria-selected', 'true');
    await expect(page.locator(`#panel-${tab}`)).toBeVisible();
    await expectNoSeriousA11y(page, { label: `settings/${tab}` });
  });
}

test('settings: account → delete confirms required text', async ({ page, request }) => {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  await seedAndLogin(page, request, apiBase, 'set-acct');

  await page.goto('/settings#account');
  await page.waitForLoadState('networkidle').catch(() => {});

  // Delete button is disabled until the confirmation phrase matches.
  const deleteBtn = page.locator('button:has-text("Delete my account")');
  await expect(deleteBtn).toBeDisabled();
  await page.fill('input[placeholder="delete my account"]', 'delete my account');
  await expect(deleteBtn).toBeEnabled();
});
