/// <reference types="chrome" />
import { nlpTokenize, nlpLookup, createCard, logImmersion, getDueCount } from '../shared/api';
import { getAccessToken, storageGet, storageSet } from '../shared/storage';
import type { Message, MessageResponse } from '../shared/messages';

// Handle messages from content scripts
chrome.runtime.onMessage.addListener((message: Message, _sender, sendResponse) => {
  handleMessage(message)
    .then(sendResponse)
    .catch((err: Error) => {
      sendResponse({ error: err.message });
    });
  return true; // keep message channel open for async response
});

// Create alarm only on install/update — not every service worker wake-up
chrome.runtime.onInstalled.addListener(async () => {
  chrome.alarms.create('refresh_due_count', { periodInMinutes: 30 });
  await updateBadge();
});

chrome.alarms.onAlarm.addListener(async (alarm) => {
  if (alarm.name === 'refresh_due_count') {
    await updateBadge();
  }
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

    default:
      return { type: 'AUTH_STATE', isLoggedIn: false };
  }
}

async function updateBadge(): Promise<number> {
  try {
    const count = await getDueCount('ja');
    await storageSet('dueCount', count);
    chrome.action.setBadgeText({ text: count > 0 ? String(count) : '' });
    chrome.action.setBadgeBackgroundColor({ color: '#4CAF50' });
    return count;
  } catch {
    return 0;
  }
}
