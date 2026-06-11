import { describe, it, expect, beforeEach, vi } from 'vitest';

const { store } = vi.hoisted(() => ({ store: {} as Record<string, unknown> }));

vi.mock('../../shared/browser', () => ({
  browser: {
    storage: {
      local: {
        get: async (key: string | string[]) => {
          const keys = Array.isArray(key) ? key : [key];
          const out: Record<string, unknown> = {};
          for (const k of keys) if (k in store) out[k] = store[k];
          return out;
        },
        set: async (obj: Record<string, unknown>) => { Object.assign(store, obj); },
      },
    },
  },
}));

import { VocabCache } from '../VocabCache';

describe('VocabCache', () => {
  beforeEach(() => {
    for (const key of Object.keys(store)) delete store[key];
  });

  it('markKnown writes to known lemmas, not ignored lemmas', async () => {
    store.learningLemmas = ['conversation'];
    store.ignoredLemmas = ['conversation'];
    const cache = new VocabCache();
    await cache.load();

    await cache.markKnown('conversation');

    expect(store.knownLemmas).toEqual(['conversation']);
    expect(store.learningLemmas).toEqual([]);
    expect(store.ignoredLemmas).toEqual([]);
    expect(cache.getKnownLemmas()).toEqual(['conversation']);
    expect(cache.getStatus('conversation')).toBe('known');
  });

  it('markIgnored writes only to ignored lemmas', async () => {
    store.knownLemmas = ['badparse'];
    store.learningLemmas = ['badparse'];
    const cache = new VocabCache();
    await cache.load();

    await cache.markIgnored('badparse');

    expect(store.knownLemmas).toEqual([]);
    expect(store.learningLemmas).toEqual([]);
    expect(store.ignoredLemmas).toEqual(['badparse']);
    expect(cache.getKnownLemmas()).toEqual([]);
    expect(cache.getStatus('badparse')).toBe('known');
  });
});
