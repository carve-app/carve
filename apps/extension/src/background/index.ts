import { browser } from '../shared/browser';
import { nlpTokenize, nlpLookup, createCard, logImmersion, getDueCount, getReviewSession, submitReviewEvent, translateText } from '../shared/api';
import { getAccessToken, getApiBaseUrl, storageGet, storageSet, type OfflineReviewEvent, type CachedReviewCard } from '../shared/storage';
import type { Message, MessageResponse } from '../shared/messages';

// Handle messages from content scripts
browser.runtime.onMessage.addListener((message: Message, _sender, sendResponse) => {
  handleMessage(message)
    .then(sendResponse)
    .catch((err: Error) => {
      sendResponse({ error: err.message });
    });
  return true; // keep message channel open for async response
});

// Create alarms on install/update — not every service worker wake-up
browser.runtime.onInstalled.addListener(async () => {
  browser.alarms.create('refresh_due_count', { periodInMinutes: 30 });
  browser.alarms.create('sync_offline_queue', { periodInMinutes: 5 });
  browser.alarms.create('cache_review_cards', { periodInMinutes: 60 });
  await updateBadge();
  await cacheReviewCards();
});

browser.alarms.onAlarm.addListener(async (alarm) => {
  if (alarm.name === 'refresh_due_count') {
    await updateBadge();
  } else if (alarm.name === 'sync_offline_queue') {
    await syncOfflineQueue();
  } else if (alarm.name === 'cache_review_cards') {
    await cacheReviewCards();
  }
});

// Sync when the service worker wakes after connectivity is restored
self.addEventListener('online', () => {
  syncOfflineQueue().catch(() => {});
});

async function handleMessage(msg: Message): Promise<MessageResponse> {
  switch (msg.type) {
    case 'TOKENIZE': {
      try {
        const result = await nlpTokenize({
          text: msg.text,
          language: msg.language,
          knownLemmas: msg.knownLemmas,
          learningLemmas: msg.learningLemmas,
        });
        return {
          type: 'TOKENIZE_RESULT',
          tokens: result.tokens,
          comprehension_pct: result.comprehension_pct,
        };
      } catch {
        return { type: 'TOKENIZE_RESULT', tokens: [], comprehension_pct: null };
      }
    }

    case 'LOOKUP': {
      try {
        const entry = await nlpLookup(msg.surface, msg.language);
        return { type: 'LOOKUP_RESULT', entry };
      } catch {
        return { type: 'LOOKUP_RESULT', entry: null };
      }
    }

    case 'MINE_CARD': {
      try {
        const card = await createCard({
          language_code: msg.languageCode,
          lemma: msg.lemma,
          reading: msg.reading,
          definition: msg.definition,
          translation: msg.translation,
          sentence: msg.sentence,
          source_url: msg.sourceUrl,
        });
        return { type: 'MINE_CARD_RESULT', success: true, cardId: card.id };
      } catch (e: unknown) {
        const message = e instanceof Error ? e.message : 'Unknown error';
        return { type: 'MINE_CARD_RESULT', success: false, error: message };
      }
    }

    case 'IGNORE_WORD': {
      const ignored = (await storageGet('ignoredLemmas')) ?? [];
      if (!ignored.includes(msg.lemma)) {
        await storageSet('ignoredLemmas', [...ignored, msg.lemma]);
      }
      return { type: 'IGNORE_WORD_RESULT', success: true };
    }

    case 'LOG_IMMERSION': {
      try {
        await logImmersion({
          language_code: msg.languageCode,
          session_type: msg.sessionType,
          duration_sec: msg.durationSec,
          started_at: msg.startedAt,
          url: msg.url,
        });
        return { type: 'LOG_IMMERSION_RESULT', success: true };
      } catch {
        return { type: 'LOG_IMMERSION_RESULT', success: false };
      }
    }

    case 'GET_AUTH_STATE': {
      const token = await getAccessToken();
      return { type: 'AUTH_STATE', isLoggedIn: !!token };
    }

    case 'GET_DUE_COUNT': {
      const count = await updateBadge();
      return { type: 'DUE_COUNT', count };
    }

    case 'GET_CACHED_REVIEW_CARDS': {
      const cards = (await storageGet('cachedReviewCards')) ?? [];
      return { type: 'CACHED_REVIEW_CARDS', cards };
    }

    case 'QUEUE_REVIEW_EVENT': {
      await queueOfflineReviewEvent({
        card_id: msg.cardId,
        rating: msg.rating,
        time_taken_ms: msg.timeTakenMs,
        reviewed_at: new Date().toISOString(),
      });
      // Try immediate sync; if offline it stays queued
      syncOfflineQueue().catch(() => {});
      return { type: 'MINE_CARD_RESULT', success: true };
    }

    case 'TRANSLATE': {
      try {
        const result = await translateText(msg.text, msg.sourceLanguage);
        return { type: 'TRANSLATE_RESULT', translation: result.translation ?? null };
      } catch {
        return { type: 'TRANSLATE_RESULT', translation: null };
      }
    }

    case 'ATTACH_PAGE_SCREENSHOT': {
      try {
        const dataUrl: string = await new Promise((resolve, reject) => {
          browser.tabs.captureVisibleTab({ format: 'jpeg', quality: 80 }, (url) => {
            if (browser.runtime.lastError) reject(new Error(browser.runtime.lastError.message));
            else resolve(url);
          });
        });
        const blob = dataURLToBlob(dataUrl);
        const base = await getApiBaseUrl();
        const token = await getAccessToken();
        if (!token) return { type: 'ATTACH_SCREENSHOT_RESULT', success: false };

        const form = new FormData();
        form.append('image', blob, 'screenshot.jpg');
        const resp = await fetch(`${base}/v1/cards/${msg.cardId}/media`, {
          method: 'POST',
          headers: { Authorization: `Bearer ${token}` },
          body: form,
        });
        return { type: 'ATTACH_SCREENSHOT_RESULT', success: resp.ok };
      } catch {
        return { type: 'ATTACH_SCREENSHOT_RESULT', success: false };
      }
    }

    case 'CAPTURE_SCREENSHOT': {
      try {
        const dataUrl: string = await new Promise((resolve, reject) => {
          browser.tabs.captureVisibleTab({ format: 'png' }, (url) => {
            if (browser.runtime.lastError) reject(new Error(browser.runtime.lastError.message));
            else resolve(url);
          });
        });
        // Convert data URL to Blob and upload to media service.
        const blob = dataURLToBlob(dataUrl);
        const mediaBase = 'http://localhost:8002';
        const resp = await fetch(`${mediaBase}/screenshots`, {
          method: 'POST',
          headers: { 'Content-Type': 'image/png' },
          body: blob,
        });
        if (!resp.ok) throw new Error('upload failed');
        const { url } = await resp.json();
        return { type: 'SCREENSHOT_RESULT', success: true, url: `${mediaBase}${url}` };
      } catch (e: unknown) {
        return { type: 'SCREENSHOT_RESULT', success: false, error: e instanceof Error ? e.message : 'capture failed' };
      }
    }

    default:
      return { type: 'AUTH_STATE', isLoggedIn: false };
  }
}

function dataURLToBlob(dataUrl: string): Blob {
  const [header, data] = dataUrl.split(',');
  const mime = header.match(/:(.*?);/)?.[1] ?? 'image/png';
  const binary = atob(data);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return new Blob([bytes], { type: mime });
}

async function getTargetLanguage(): Promise<string> {
  const r = await browser.storage.local.get('targetLanguage');
  return (r['targetLanguage'] as string | undefined) ?? 'ja';
}

async function updateBadge(): Promise<number> {
  try {
    const lang = await getTargetLanguage();
    const count = await getDueCount(lang);
    await storageSet('dueCount', count);
    browser.action.setBadgeText({ text: count > 0 ? String(count) : '' });
    browser.action.setBadgeBackgroundColor({ color: '#4CAF50' });
    return count;
  } catch {
    return 0;
  }
}

/**
 * Add a review event to the offline queue.
 */
async function queueOfflineReviewEvent(event: OfflineReviewEvent): Promise<void> {
  const queue = (await storageGet('offlineReviewQueue')) ?? [];
  queue.push(event);
  await storageSet('offlineReviewQueue', queue);
}

/**
 * Attempt to flush all queued offline review events to the server.
 * Events that succeed are removed; failed events remain for next retry.
 */
async function syncOfflineQueue(): Promise<void> {
  const token = await getAccessToken();
  if (!token) return;

  const queue = (await storageGet('offlineReviewQueue')) ?? [];
  if (queue.length === 0) return;

  const remaining: OfflineReviewEvent[] = [];
  for (const event of queue) {
    try {
      await submitReviewEvent(event);
    } catch {
      remaining.push(event); // keep for next retry
    }
  }
  await storageSet('offlineReviewQueue', remaining);

  if (remaining.length < queue.length) {
    await updateBadge();
  }
}

/**
 * Pre-fetch review cards into local storage so offline review is possible.
 */
async function cacheReviewCards(): Promise<void> {
  const token = await getAccessToken();
  if (!token) return;
  try {
    const lang = await getTargetLanguage();
    const session = await getReviewSession(lang, 50);
    const cards: CachedReviewCard[] = (session.cards ?? []).map((c: any) => ({
      id: c.id,
      front_text: c.front_text,
      back_text: c.back_text ?? null,
      sentence: c.sentence ?? null,
      source_url: c.source_url ?? null,
      fsrs_state: c.fsrs_state,
      stability: c.stability ?? null,
      difficulty: c.difficulty ?? null,
      reps: c.reps ?? 0,
      lapses: c.lapses ?? 0,
    }));
    await storageSet('cachedReviewCards', cards);
    await storageSet('cachedReviewAt', Date.now());
  } catch {
    // Network unavailable — keep stale cache
  }
}
