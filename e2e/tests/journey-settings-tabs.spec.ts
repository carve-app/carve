import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

/**
 * L5/L8 — every Settings tab is reachable and accessible.
 *
 * Per the inventory:
 *   Account · Display · Review · Mining · Sites · Sync · Billing
 */
const TABS = ['account', 'display', 'review', 'mining', 'sites', 'sync', 'billing'] as const;

async function seedAndLogin(page: any, request: any, apiBase: string, prefix: string) {
  const email = `${prefix}+${Date.now()}@example.com`;
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'Settings Tester' },
  });
  const { access_token } = await reg.json();
  await page.goto('/');
  await page.evaluate((t: string) => localStorage.setItem('carve_access_token', t), access_token);
  return access_token;
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
