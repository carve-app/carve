import { afterEach, describe, expect, it, vi } from 'vitest';
import { updateCard } from '../api';

describe('API no-content responses', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('treats a successful 204 mutation as success instead of parsing JSON', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(updateCard('card-1', { back_text: 'updated' })).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
