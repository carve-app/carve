import type { Token, DictEntry } from './types';
import type { CachedReviewCard } from './storage';

export type Message =
  | { type: 'TOKENIZE'; text: string; language: string; knownLemmas: string[]; learningLemmas: string[] }
  | { type: 'LOOKUP'; surface: string; language: string }
  | { type: 'MINE_CARD'; lemma: string; sentence: string; sourceUrl: string; languageCode: string }
  | { type: 'IGNORE_WORD'; lemma: string; languageCode: string }
  | { type: 'LOG_IMMERSION'; languageCode: string; sessionType: string; durationSec: number; startedAt: string; url: string }
  | { type: 'GET_AUTH_STATE' }
  | { type: 'GET_DUE_COUNT' }
  | { type: 'GET_CACHED_REVIEW_CARDS' }
  | { type: 'QUEUE_REVIEW_EVENT'; cardId: string; rating: 1|2|3|4; timeTakenMs: number }
  | { type: 'SET_SITE_ENABLED'; enabled: boolean };

export type MessageResponse =
  | { type: 'TOKENIZE_RESULT'; tokens: Token[]; comprehension_pct: number | null }
  | { type: 'LOOKUP_RESULT'; entry: DictEntry | null }
  | { type: 'MINE_CARD_RESULT'; success: boolean; cardId?: string; error?: string }
  | { type: 'IGNORE_WORD_RESULT'; success: boolean }
  | { type: 'LOG_IMMERSION_RESULT'; success: boolean }
  | { type: 'AUTH_STATE'; isLoggedIn: boolean; userId?: string }
  | { type: 'DUE_COUNT'; count: number }
  | { type: 'CACHED_REVIEW_CARDS'; cards: CachedReviewCard[] };
