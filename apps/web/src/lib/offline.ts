/**
 * Tiny IndexedDB-backed offline queue for review events.
 *
 * Mirrors the extension's shared/storage.ts behavior:
 *   - When `submitReviewEvent` fails or the browser is offline, the event
 *     gets queued.
 *   - When the page comes back online, `flushQueue()` retries every queued
 *     event in order and removes those that succeed.
 *
 * Stored objects look like:
 *   { id: number; payload: ReviewEventPayload; queuedAt: number }
 */

export interface QueuedEvent {
  id?: number;
  payload: Record<string, unknown>;
  queuedAt: number;
}

const DB_NAME = 'carve_offline';
const STORE = 'review_events';
const VERSION = 1;

let dbPromise: Promise<IDBDatabase> | null = null;

function openDb(): Promise<IDBDatabase> {
  if (typeof indexedDB === 'undefined') {
    return Promise.reject(new Error('IndexedDB not available'));
  }
  if (dbPromise) return dbPromise;
  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) {
        db.createObjectStore(STORE, { keyPath: 'id', autoIncrement: true });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
  return dbPromise;
}

export async function queueEvent(payload: Record<string, unknown>): Promise<void> {
  const db = await openDb();
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite');
    tx.objectStore(STORE).add({ payload, queuedAt: Date.now() });
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error ?? new Error('Failed to persist offline review'));
    tx.onabort = () => reject(tx.error ?? new Error('Offline review transaction aborted'));
  });
}

export async function listQueued(): Promise<QueuedEvent[]> {
  const db = await openDb();
  return new Promise<QueuedEvent[]>((resolve, reject) => {
    const out: QueuedEvent[] = [];
    const tx = db.transaction(STORE, 'readonly');
    const req = tx.objectStore(STORE).openCursor();
    req.onsuccess = () => {
      const cursor = req.result;
      if (cursor) {
        out.push(cursor.value as QueuedEvent);
        cursor.continue();
      } else {
        resolve(out);
      }
    };
    req.onerror = () => reject(req.error ?? new Error('Failed to read offline reviews'));
  });
}

export async function removeQueued(id: number): Promise<void> {
  const db = await openDb();
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite');
    tx.objectStore(STORE).delete(id);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error ?? new Error('Failed to remove offline review'));
  });
}

export async function flushQueue(
  submit: (payload: Record<string, unknown>) => Promise<void>,
): Promise<{ flushed: number; failed: number }> {
  const events = await listQueued();
  let flushed = 0;
  let failed = 0;
  for (const e of events) {
    try {
      await submit(e.payload);
      if (e.id != null) await removeQueued(e.id);
      flushed++;
    } catch {
      failed++;
    }
  }
  return { flushed, failed };
}
