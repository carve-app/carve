import { Page, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * Inject axe-core and assert zero serious or critical violations. L8 of
 * the testing strategy — every journey test calls this before the
 * positive assertions so accessibility regressions block PRs the same
 * way functional regressions do.
 *
 * Pass an array of CSS selectors to `exclude` for any third-party widget
 * we don't own (e.g. Stripe checkout iframes).
 */
export async function expectNoSeriousA11y(
  page: Page,
  { exclude = [], label = 'page' }: { exclude?: string[]; label?: string } = {},
) {
  const builder = new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'best-practice']);
  for (const e of exclude) builder.exclude(e);
  const results = await builder.analyze();
  const serious = results.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );
  if (serious.length > 0) {
    const summary = serious.map((v) => `${v.id} (${v.impact}): ${v.help}`).join('\n');
    throw new Error(`[${label}] axe found ${serious.length} serious violations:\n${summary}`);
  }
  expect(serious).toHaveLength(0);
}
