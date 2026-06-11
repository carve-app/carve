import { browser } from '../shared/browser';
import { nlpTokenize, nlpLookup, createCard, markKnownWords, logImmersion, getDueCount, getReviewSession, submitReviewEvent, translateText, selectMiningSentence, findSimilarCards, explainWord, getWordAudio, getWordImage } from '../shared/api';
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
          source_timestamp: msg.sourceTimestamp,
        });
        return { type: 'MINE_CARD_RESULT', success: true, cardId: card.id };
      } catch (e: unknown) {
        const message = e instanceof Error ? e.message : 'Unknown error';
        return { type: 'MINE_CARD_RESULT', success: false, error: message };
      }
    }

    case 'MARK_KNOWN_WORD': {
      await persistLemmaMembership(msg.lemma, 'known');
      try {
        await markKnownWords({ language: msg.languageCode, lemmas: [msg.lemma] });
      } catch {
        // Local knowledge still matters immediately; server sync is best-effort.
      }
      return { type: 'MARK_KNOWN_WORD_RESULT', success: true };
    }

    case 'IGNORE_WORD': {
      await persistLemmaMembership(msg.lemma, 'ignored');
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

    case 'FIND_SIMILAR_CARDS': {
      try {
        const result = await findSimilarCards({
          languageCode: msg.languageCode,
          sentence: msg.sentence,
        });
        return { type: 'FIND_SIMILAR_CARDS_RESULT', matches: result.matches ?? [] };
      } catch {
        return { type: 'FIND_SIMILAR_CARDS_RESULT', matches: [] };
      }
    }

    case 'SELECT_SENTENCE': {
      try {
        const result = await selectMiningSentence({
          candidates: msg.candidates,
          targetLemma: msg.targetLemma,
          language: msg.language,
          knownLemmas: msg.knownLemmas,
          learningLemmas: msg.learningLemmas,
        });
        return {
          type: 'SELECT_SENTENCE_RESULT',
          bestText: result.best?.text ?? null,
          bestComprehensionPct: result.best?.comprehension_pct ?? null,
          bestContainsTarget: result.best?.contains_target ?? false,
        };
      } catch {
        return {
          type: 'SELECT_SENTENCE_RESULT',
          bestText: null,
          bestComprehensionPct: null,
          bestContainsTarget: false,
        };
      }
    }

    case 'EXPLAIN_WORD': {
      try {
        const result = await explainWord({
          word: msg.word,
          sentence: msg.sentence,
          language: msg.language,
        });
        return { type: 'EXPLAIN_WORD_RESULT', explanation: result.explanation ?? null };
      } catch {
        return { type: 'EXPLAIN_WORD_RESULT', explanation: null };
      }
    }

    case 'WORD_AUDIO': {
      try {
        const result = await getWordAudio({
          language: msg.language,
          lemma: msg.lemma,
          reading: msg.reading,
        });
        return { type: 'WORD_AUDIO_RESULT', audioUrl: result.audio_url ?? null };
      } catch {
        return { type: 'WORD_AUDIO_RESULT', audioUrl: null };
      }
    }

    case 'WORD_IMAGE': {
      // Best-effort dictionary image. Any failure → null so the popup just
      // hides its image slot.
      try {
        const result = await getWordImage({ word: msg.word, language: msg.language });
        return { type: 'WORD_IMAGE_RESULT', imageUrl: result.image_url ?? null };
      } catch {
        return { type: 'WORD_IMAGE_RESULT', imageUrl: null };
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

    case 'CAPTURE_VIDEO_FRAME': {
      // Screenshot the visible tab and crop to the video rect. Run from the
      // worker (not a content-script canvas) because captureVisibleTab
      // composites the *rendered* page and so captures DRM/EME video
      // (Netflix, Disney+, Prime) that a <video> canvas drawImage would
      // render black. Called at mine-time so the frame matches the moment the
      // user mined; the result is base64 handed back to the page, which pairs
      // it with the concurrently-recorded audio for a single upload.
      try {
        const dataUrl: string = await new Promise((resolve, reject) => {
          browser.tabs.captureVisibleTab({ format: 'jpeg', quality: 85 }, (url) => {
            if (browser.runtime.lastError) reject(new Error(browser.runtime.lastError.message));
            else resolve(url);
          });
        });
        const blob = await cropDataUrl(dataUrl, msg.rect, msg.dpr);
        if (!blob) {
          // Degenerate / off-screen rect — better to report no frame than to
          // attach a screenshot of unrelated page chrome.
          return { type: 'CAPTURE_VIDEO_FRAME_RESULT', imageBase64: null };
        }
        const imageBase64 = await blobToBase64Worker(blob);
        return { type: 'CAPTURE_VIDEO_FRAME_RESULT', imageBase64 };
      } catch {
        // DRM with HDCP, or a transient capture failure — no frame. The card
        // still gets its audio (if any) and subtitle text.
        return { type: 'CAPTURE_VIDEO_FRAME_RESULT', imageBase64: null };
      }
    }

    case 'ATTACH_VIDEO_MEDIA': {
      // Upload the already-captured frame (from CAPTURE_VIDEO_FRAME) + audio.
      // Routed through the worker so the upload uses the worker's host
      // permission, sidestepping the CORS wall a content-script cross-origin
      // fetch would hit in prod.
      const token = await getAccessToken();
      if (!token) {
        return { type: 'ATTACH_VIDEO_MEDIA_RESULT', success: false, hasImage: false, hasAudio: false, error: 'not signed in' };
      }
      const base = await getApiBaseUrl();

      let imageBlob: Blob | null = null;
      if (msg.imageBase64) {
        try {
          imageBlob = base64ToBlob(msg.imageBase64, 'image/jpeg');
        } catch {
          imageBlob = null;
        }
      }

      let audioBlob: Blob | null = null;
      if (msg.audioBase64) {
        try {
          audioBlob = base64ToBlob(msg.audioBase64, msg.audioMime ?? 'audio/webm');
        } catch {
          audioBlob = null;
        }
      }

      if (!imageBlob && !audioBlob) {
        // Nothing capturable on this site (e.g. hard-DRM frame block + muted
        // captureStream). The card itself was already saved by MINE_CARD.
        return { type: 'ATTACH_VIDEO_MEDIA_RESULT', success: false, hasImage: false, hasAudio: false, error: 'no media capturable' };
      }

      try {
        const form = new FormData();
        if (imageBlob) form.append('image', imageBlob, 'frame.jpg');
        if (audioBlob) form.append('audio', audioBlob, 'clip.webm');
        form.append('subtitle_start_ms', String(msg.startMs));
        form.append('subtitle_end_ms', String(msg.endMs));
        if (msg.sourceUrl) form.append('video_source_url', msg.sourceUrl);
        if (msg.subtitleTranslation) form.append('subtitle_translation', msg.subtitleTranslation);

        const resp = await fetch(`${base}/v1/cards/${msg.cardId}/media`, {
          method: 'POST',
          headers: { Authorization: `Bearer ${token}` },
          body: form,
        });

        // Report what the SERVER actually persisted, not just what we sent —
        // the media service can reject a part (size/format) or be down, in
        // which case the API returns the failure and the URL is null. Trusting
        // local blobs here would tell the user "Mined! (+image & audio)" while
        // the card silently has no media.
        let body: { image_url?: string | null; audio_url?: string | null; error?: string } = {};
        try { body = await resp.json(); } catch {/* non-JSON error body */}

        const hasImage = !!body.image_url;
        const hasAudio = !!body.audio_url;
        return {
          type: 'ATTACH_VIDEO_MEDIA_RESULT',
          success: resp.ok && (hasImage || hasAudio),
          hasImage,
          hasAudio,
          error: resp.ok ? undefined : (body.error ?? `HTTP ${resp.status}`),
        };
      } catch (e: unknown) {
        return {
          type: 'ATTACH_VIDEO_MEDIA_RESULT',
          success: false,
          hasImage: false,
          hasAudio: false,
          error: e instanceof Error ? e.message : 'upload failed',
        };
      }
    }

    default:
      return { type: 'AUTH_STATE', isLoggedIn: false };
  }
}

async function persistLemmaMembership(
  lemma: string,
  target: 'known' | 'learning' | 'ignored',
): Promise<void> {
  const [knownArr, learningArr, ignoredArr] = await Promise.all([
    storageGet('knownLemmas'),
    storageGet('learningLemmas'),
    storageGet('ignoredLemmas'),
  ]);
  const known = new Set(knownArr ?? []);
  const learning = new Set(learningArr ?? []);
  const ignored = new Set(ignoredArr ?? []);

  known.delete(lemma);
  learning.delete(lemma);
  ignored.delete(lemma);
  if (target === 'known') {
    known.add(lemma);
  } else if (target === 'learning') {
    learning.add(lemma);
  } else {
    ignored.add(lemma);
  }

  await Promise.all([
    storageSet('knownLemmas', Array.from(known)),
    storageSet('learningLemmas', Array.from(learning)),
    storageSet('ignoredLemmas', Array.from(ignored)),
  ]);
}

function dataURLToBlob(dataUrl: string): Blob {
  const [header, data] = dataUrl.split(',');
  const mime = header.match(/:(.*?);/)?.[1] ?? 'image/png';
  const binary = atob(data);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return new Blob([bytes], { type: mime });
}

function base64ToBlob(b64: string, mime: string): Blob {
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return new Blob([bytes], { type: mime });
}

// Service workers have no FileReader, so encode via arrayBuffer + btoa. Chunked
// to keep the String.fromCharCode argument list within engine limits.
async function blobToBase64Worker(blob: Blob): Promise<string> {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let binary = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

/**
 * Crop a full-tab screenshot data URL to the video rectangle.
 *
 * `rect` is in CSS pixels (as reported by getBoundingClientRect in the content
 * script); the captured image is in device pixels, so we scale by `dpr`. If the
 * rect is unusable (zero-size, or the video is fully off-screen), we fall back
 * to returning the whole frame rather than failing the capture.
 *
 * Runs in the service worker, which provides OffscreenCanvas + createImageBitmap.
 */
async function cropDataUrl(
  dataUrl: string,
  rect: { x: number; y: number; width: number; height: number },
  dpr: number,
): Promise<Blob | null> {
  const fullBlob = dataURLToBlob(dataUrl);
  const bitmap = await createImageBitmap(fullBlob);

  const scale = dpr > 0 ? dpr : 1;
  let sx = Math.round(rect.x * scale);
  let sy = Math.round(rect.y * scale);
  let sw = Math.round(rect.width * scale);
  let sh = Math.round(rect.height * scale);

  // Clamp to the captured image bounds. captureVisibleTab only sees the
  // viewport, so a rect partially scrolled out of view must be trimmed.
  sx = Math.max(0, Math.min(sx, bitmap.width));
  sy = Math.max(0, Math.min(sy, bitmap.height));
  sw = Math.max(0, Math.min(sw, bitmap.width - sx));
  sh = Math.max(0, Math.min(sh, bitmap.height - sy));

  // Degenerate rect (video hidden/zero-size/fully scrolled off-screen). Return
  // null rather than a whole-viewport screenshot of unrelated page content —
  // the caller turns this into an honest "no frame" outcome. The content script
  // already skips capture for <16px videos; this is the worker-side backstop.
  if (sw < 16 || sh < 16) {
    bitmap.close();
    return null;
  }

  const canvas = new OffscreenCanvas(sw, sh);
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    bitmap.close();
    return null;
  }
  ctx.drawImage(bitmap, sx, sy, sw, sh, 0, 0, sw, sh);
  bitmap.close();
  return canvas.convertToBlob({ type: 'image/jpeg', quality: 0.85 });
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

// All offlineReviewQueue mutations are serialized through this promise chain.
// queueOfflineReviewEvent, syncOfflineQueue (called from the message handler,
// the 5-min alarm, AND the `online` event) otherwise do lockless get-then-set
// on the same key and would drop events when their read/write windows
// interleave. Each op appends to the chain and awaits its turn; a thrown op
// still resolves the chain so a later op isn't deadlocked.
let queueLock: Promise<void> = Promise.resolve();

function withQueueLock<T>(op: () => Promise<T>): Promise<T> {
  const run = queueLock.then(op, op);
  // Keep the chain alive regardless of whether `op` resolved or rejected.
  queueLock = run.then(
    () => undefined,
    () => undefined,
  );
  return run;
}

/**
 * Add a review event to the offline queue.
 */
async function queueOfflineReviewEvent(event: OfflineReviewEvent): Promise<void> {
  await withQueueLock(async () => {
    const queue = (await storageGet('offlineReviewQueue')) ?? [];
    queue.push(event);
    await storageSet('offlineReviewQueue', queue);
  });
}

/**
 * Attempt to flush all queued offline review events to the server.
 * Events that succeed are removed; failed events remain for next retry.
 */
async function syncOfflineQueue(): Promise<void> {
  const token = await getAccessToken();
  if (!token) return;

  await withQueueLock(async () => {
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
  });
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
