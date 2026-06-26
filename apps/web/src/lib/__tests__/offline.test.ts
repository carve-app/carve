import 'fake-indexeddb/auto';
import { describe, expect, it } from 'vitest';

import { flushQueue, listQueued, queueEvent } from '../offline';

describe('offline review queue', () => {
  it('persists event IDs and removes only acknowledged events', async () => {
    const event = {
      event_id: crypto.randomUUID(),
      card_id: crypto.randomUUID(),
      rating: 3,
      reviewed_at: new Date().toISOString(),
    };
    await queueEvent(event);
    const queued = await listQueued();
    expect(queued).toHaveLength(1);
    expect(queued[0].payload).toEqual(event);

    const submitted: Record<string, unknown>[] = [];
    const result = await flushQueue(async (payload) => {
      submitted.push(payload);
    });
    expect(result).toEqual({ flushed: 1, failed: 0 });
    expect(submitted).toEqual([event]);
    expect(await listQueued()).toEqual([]);
  });
});
