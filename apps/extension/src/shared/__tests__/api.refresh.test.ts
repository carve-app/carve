/**
 * Tests the transparent access-token refresh in the extension API client:
 * on a 401, apiFetch exchanges the stored refresh token for a new access token
 * and retries the request once, so a long immersion session never silently
 * fails when the short-lived access token expires.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';

// In-memory chrome.storage.local backing the shared/storage helpers. Defined via
// vi.hoisted so the (hoisted) vi.mock factory can reference it.
const { store } = vi.hoisted(() => ({ store: {} as Record<string, unknown> }));
vi.mock('../browser', () => ({
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
        remove: async (key: string | string[]) => {
          for (const k of Array.isArray(key) ? key : [key]) delete store[k];
        },
      },
    },
  },
}));

import { nlpLookup } from '../api';

describe('extension apiFetch — token refresh on 401', () => {
  beforeEach(() => {
    for (const k of Object.keys(store)) delete store[k];
    store.apiBaseUrl = 'http://localhost:8080';
    store.accessToken = 'expired-token';
    store.refreshToken = 'valid-refresh';
    vi.restoreAllMocks();
  });

  it('refreshes and retries once on 401, then succeeds', async () => {
    const calls: string[] = [];
    global.fetch = vi.fn(async (url: any, init: any) => {
      const u = String(url);
      calls.push(`${init?.method ?? 'GET'} ${u.replace('http://localhost:8080', '')}`);
      if (u.includes('/v1/auth/refresh')) {
        return new Response(JSON.stringify({ access_token: 'fresh-token', refresh_token: 'rotated-refresh' }), { status: 200 });
      }
      // First lookup uses the expired token → 401; after refresh, the retry
      // carries the fresh token → 200.
      const auth = init?.headers?.['Authorization'];
      if (auth === 'Bearer expired-token') return new Response('unauthorized', { status: 401 });
      if (auth === 'Bearer fresh-token') return new Response(JSON.stringify({ entry: { lemma: 'gato' } }), { status: 200 });
      return new Response('unexpected', { status: 500 });
    }) as any;

    const result = await nlpLookup('gato', 'es');
    expect(result.entry.lemma).toBe('gato');
    // lookup(401) → refresh(200) → lookup retry(200)
    expect(calls).toEqual([
      'POST /v1/nlp/lookup',
      'POST /v1/auth/refresh',
      'POST /v1/nlp/lookup',
    ]);
    // The rotated tokens were persisted.
    expect(store.accessToken).toBe('fresh-token');
    expect(store.refreshToken).toBe('rotated-refresh');
  });

  it('clears the session and throws when the refresh token is rejected', async () => {
    let sawRefresh = false;
    global.fetch = vi.fn(async (url: any) => {
      if (String(url).includes('/v1/auth/refresh')) { sawRefresh = true; return new Response('bad', { status: 401 }); }
      return new Response('unauthorized', { status: 401 });
    }) as any;

    await expect(nlpLookup('gato', 'es')).rejects.toMatchObject({ status: 401 });
    expect(sawRefresh).toBe(true); // refresh was attempted
    // Session cleared so the UI prompts a real re-login.
    expect(store.accessToken).toBeUndefined();
    expect(store.refreshToken).toBeUndefined();
  });

  it('does not attempt refresh when there is no access token', async () => {
    delete store.accessToken;
    delete store.refreshToken;
    const fetchMock = vi.fn(async () => new Response('unauthorized', { status: 401 }));
    global.fetch = fetchMock as any;

    await expect(nlpLookup('gato', 'es')).rejects.toMatchObject({ status: 401 });
    // Only the original request — no refresh attempt without a token.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
