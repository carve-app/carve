import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { createTestCard, registerTestUser, seedAuthenticatedPage } from './helpers';

async function setup(page: any, request: any) {
  const { apiBase, access_token } = await registerTestUser(request, 'cards', 'Cards Tester');

  // Seed three cards
  const cardIds: string[] = [];
  for (const word of ['りんご', 'バナナ', 'さくらんぼ']) {
    const card = await createTestCard(request, apiBase, access_token, {
      lemma: word,
      backText: `${word} definition`,
    });
    cardIds.push(card.id);
  }
  await seedAuthenticatedPage(page, access_token);
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
