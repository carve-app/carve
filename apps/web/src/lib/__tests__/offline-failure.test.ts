import { afterEach, describe, expect, it, vi } from 'vitest';

describe('offline review queue failure handling', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it('rejects instead of silently losing a review when IndexedDB is unavailable', async () => {
    vi.stubGlobal('indexedDB', undefined);
    const { queueEvent } = await import('../offline');
    await expect(queueEvent({ event_id: crypto.randomUUID() })).rejects.toThrow('IndexedDB not available');
  });
});
