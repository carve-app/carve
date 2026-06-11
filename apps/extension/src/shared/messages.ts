import type { Token, DictEntry } from './types';
import type { CachedReviewCard } from './storage';

export type Message =
  | { type: 'TOKENIZE'; text: string; language: string; knownLemmas: string[]; learningLemmas: string[] }
  | { type: 'LOOKUP'; surface: string; language: string }
  | { type: 'MINE_CARD'; lemma: string; reading: string; definition?: string; translation?: string; sentence: string; sourceUrl: string; sourceTimestamp?: number; languageCode: string }
  | { type: 'IGNORE_WORD'; lemma: string; languageCode: string }
  | { type: 'LOG_IMMERSION'; languageCode: string; sessionType: string; durationSec: number; startedAt: string; url: string }
  | { type: 'GET_AUTH_STATE' }
  | { type: 'GET_DUE_COUNT' }
  | { type: 'GET_CACHED_REVIEW_CARDS' }
  | { type: 'QUEUE_REVIEW_EVENT'; cardId: string; rating: 1|2|3|4; timeTakenMs: number }
  | { type: 'SET_SITE_ENABLED'; enabled: boolean }
  | { type: 'GET_COMPREHENSION' }
  | { type: 'SET_OVERLAY'; enabled: boolean }
  | { type: 'ATTACH_PAGE_SCREENSHOT'; cardId: string }
  | {
      // Capture a single video frame at mine-time. Runs in the background
      // worker so it can use chrome.tabs.captureVisibleTab (which, unlike a
      // content-script canvas drawImage on the <video>, is not blocked by
      // EME/DRM — this is how we capture Netflix/Disney+/Prime frames). The
      // content script supplies the on-screen video rectangle (CSS px) so the
      // background can crop the tab screenshot to just the video. The cropped
      // frame is returned as base64 so the page can pair it with the
      // concurrently-recorded audio for one upload.
      type: 'CAPTURE_VIDEO_FRAME';
      rect: { x: number; y: number; width: number; height: number };
      dpr: number;
    }
  | {
      // Upload an already-captured frame + audio for a mined video card. Runs
      // in the worker so the upload uses its host permission (no content-script
      // CORS wall). Media is passed as base64 since Blobs don't survive
      // structured-clone across the messaging boundary reliably in MV3.
      type: 'ATTACH_VIDEO_MEDIA';
      cardId: string;
      imageBase64: string | null;
      audioBase64: string | null;
      audioMime: string | null;
      startMs: number;
      endMs: number;
      sourceUrl?: string;
      subtitleTranslation?: string;
    }
  | { type: 'TRANSLATE'; text: string; sourceLanguage: string }
  | { type: 'SELECT_SENTENCE'; candidates: string[]; targetLemma: string; language: string; knownLemmas: string[]; learningLemmas: string[] }
  | { type: 'FIND_SIMILAR_CARDS'; languageCode: string; sentence: string }
  | { type: 'EXPLAIN_WORD'; word: string; sentence: string; language: string }
  | { type: 'WORD_AUDIO'; language: string; lemma: string; reading: string };

export type MessageResponse =
  | { type: 'TOKENIZE_RESULT'; tokens: Token[]; comprehension_pct: number | null }
  | { type: 'LOOKUP_RESULT'; entry: DictEntry | null }
  | { type: 'MINE_CARD_RESULT'; success: boolean; cardId?: string; error?: string }
  | { type: 'IGNORE_WORD_RESULT'; success: boolean }
  | { type: 'LOG_IMMERSION_RESULT'; success: boolean }
  | { type: 'AUTH_STATE'; isLoggedIn: boolean; userId?: string }
  | { type: 'DUE_COUNT'; count: number }
  | { type: 'CACHED_REVIEW_CARDS'; cards: CachedReviewCard[] }
  | { type: 'COMPREHENSION_RESULT'; pct: number | null; total: number }
  | { type: 'ATTACH_SCREENSHOT_RESULT'; success: boolean }
  | { type: 'CAPTURE_VIDEO_FRAME_RESULT'; imageBase64: string | null }
  | { type: 'ATTACH_VIDEO_MEDIA_RESULT'; success: boolean; hasImage: boolean; hasAudio: boolean; error?: string }
  | { type: 'TRANSLATE_RESULT'; translation: string | null }
  | { type: 'SELECT_SENTENCE_RESULT'; bestText: string | null; bestComprehensionPct: number | null; bestContainsTarget: boolean }
  | { type: 'FIND_SIMILAR_CARDS_RESULT'; matches: { id: string; front_text: string; sentence: string; similarity: number }[] }
  | { type: 'EXPLAIN_WORD_RESULT'; explanation: string | null }
  | { type: 'WORD_AUDIO_RESULT'; audioUrl: string | null };
