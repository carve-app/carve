import { getAccessToken, getApiBaseUrl, getRefreshToken, setAccessToken, setRefreshToken, clearSession } from './storage';

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

// Exchange the stored refresh token for a fresh access token. The extension
// can't rely on the API's SameSite refresh cookie (its service-worker fetches
// are cross-site), so it sends the refresh token in the body and persists the
// rotated pair. Single-flight so concurrent 401s don't stampede /refresh.
let refreshInFlight: Promise<boolean> | null = null;

async function refreshAccessToken(): Promise<boolean> {
  // Coalesce concurrent refreshes into one in-flight call. Once it settles the
  // shared promise is cleared so a *later* expiry triggers a fresh refresh
  // rather than reusing the stale resolved result.
  if (refreshInFlight) return refreshInFlight;
  const run = (async () => {
    try {
      const refresh = await getRefreshToken();
      if (!refresh) return false;
      const baseUrl = await getApiBaseUrl();
      const res = await fetch(`${baseUrl}/v1/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refresh }),
      });
      if (!res.ok) {
        // Refresh token is invalid/expired/revoked — the session is over.
        await clearSession();
        return false;
      }
      const data = await res.json() as { access_token?: string; refresh_token?: string };
      if (!data.access_token) return false;
      await setAccessToken(data.access_token);
      if (data.refresh_token) await setRefreshToken(data.refresh_token); // rotation
      return true;
    } catch {
      return false;
    }
  })();
  refreshInFlight = run;
  try {
    return await run;
  } finally {
    refreshInFlight = null;
  }
}

async function apiFetch(path: string, options: RequestInit = {}, _retried = false): Promise<Response> {
  const baseUrl = await getApiBaseUrl();
  const token = await getAccessToken();

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> | undefined ?? {}),
  };

  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const response = await fetch(`${baseUrl}${path}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    // Access token expired → refresh once and retry transparently, so mining/
    // lookups during a long immersion session never silently fail on expiry.
    if (response.status === 401 && !_retried && token) {
      if (await refreshAccessToken()) {
        return apiFetch(path, options, true);
      }
    }
    const body = await response.text().catch(() => '');
    throw new ApiError(response.status, `HTTP ${response.status}: ${body}`);
  }

  return response;
}

/**
 * Tokenize text via the NLP API.
 */
export async function nlpTokenize(params: {
  text: string;
  language: string;
  knownLemmas?: string[];
  learningLemmas?: string[];
}): Promise<{ tokens: any[]; comprehension_pct: number | null }> {
  const response = await apiFetch('/v1/nlp/tokenize', {
    method: 'POST',
    body: JSON.stringify({
      text: params.text,
      language: params.language,
      known_lemmas: params.knownLemmas ?? [],
      learning_lemmas: params.learningLemmas ?? [],
    }),
  });
  return response.json();
}

/**
 * Lookup a word via the NLP API.
 */
export async function nlpLookup(surface: string, language: string): Promise<any> {
  const response = await apiFetch('/v1/nlp/lookup', {
    method: 'POST',
    body: JSON.stringify({ surface, language }),
  });
  return response.json();
}

/**
 * Create a flashcard.
 */
export async function createCard(params: {
  language_code: string;
  lemma: string;
  reading?: string;
  definition?: string;
  translation?: string;
  sentence?: string;
  source_url?: string;
  source_timestamp?: number;
}): Promise<{ id: string; lemma: string }> {
  const response = await apiFetch('/v1/cards', {
    method: 'POST',
    body: JSON.stringify({
      language_code: params.language_code,
      lemma: params.lemma,
      reading: params.reading,
      back_text: params.definition,
      subtitle_translation: params.translation,
      sentence: params.sentence,
      source_url: params.source_url,
      source_timestamp: params.source_timestamp,
    }),
  });
  return response.json();
}

/**
 * Mark lemmas as known without creating cards.
 */
export async function markKnownWords(params: {
  language: string;
  lemmas: string[];
}): Promise<{ marked: number }> {
  const response = await apiFetch('/v1/onboarding/known-words', {
    method: 'POST',
    body: JSON.stringify({
      language: params.language,
      lemmas: params.lemmas,
    }),
  });
  return response.json();
}

/**
 * Get cards with optional filters.
 */
export async function getCards(
  language?: string,
  limit?: number,
  offset?: number,
): Promise<{ cards: any[]; total: number }> {
  const params = new URLSearchParams();
  if (language) params.set('language', language);
  if (limit !== undefined) params.set('limit', String(limit));
  if (offset !== undefined) params.set('offset', String(offset));
  const query = params.toString() ? `?${params.toString()}` : '';
  const response = await apiFetch(`/v1/cards${query}`);
  return response.json();
}

/**
 * Log an immersion session.
 */
export async function logImmersion(params: {
  language_code: string;
  session_type: string;
  duration_sec: number;
  started_at: string;
  url: string;
}): Promise<void> {
  await apiFetch('/v1/immersion', {
    method: 'POST',
    body: JSON.stringify(params),
  });
}

/**
 * Find existing cards with a near-duplicate sentence.
 */
export interface SimilarCard {
  id: string;
  front_text: string;
  sentence: string;
  similarity: number;
}

export async function findSimilarCards(params: {
  languageCode: string;
  sentence: string;
  threshold?: number;
  limit?: number;
}): Promise<{ matches: SimilarCard[] }> {
  const response = await apiFetch('/v1/cards/find-similar', {
    method: 'POST',
    body: JSON.stringify({
      language_code: params.languageCode,
      sentence: params.sentence,
      threshold: params.threshold,
      limit: params.limit,
    }),
  });
  return response.json();
}

/**
 * Pick the best i+1 candidate sentence for mining a word.
 */
export interface SentenceCandidate {
  index: number;
  text: string;
  comprehension_pct: number;
  content_word_count: number;
  unknown_count: number;
  contains_target: boolean;
  fit_score: number;
}

export async function selectMiningSentence(params: {
  candidates: string[];
  targetLemma: string;
  language: string;
  knownLemmas?: string[];
  learningLemmas?: string[];
}): Promise<{ best: SentenceCandidate | null; ranked: SentenceCandidate[] }> {
  const response = await apiFetch('/v1/nlp/select-sentence', {
    method: 'POST',
    body: JSON.stringify({
      candidates: params.candidates,
      target_lemma: params.targetLemma,
      language: params.language,
      known_lemmas: params.knownLemmas ?? [],
      learning_lemmas: params.learningLemmas ?? [],
    }),
  });
  return response.json();
}

/**
 * Translate text via the NLP API.
 */
export async function translateText(
  text: string,
  sourceLanguage = 'ja',
  targetLanguage = 'en',
): Promise<{ translation: string | null }> {
  const response = await apiFetch('/v1/nlp/translate', {
    method: 'POST',
    body: JSON.stringify({ text, source_language: sourceLanguage, target_language: targetLanguage }),
  });
  return response.json();
}

/**
 * Get an AI contextual explanation of how a word is used in a sentence.
 * Returns { explanation: null } when the server has no API key configured.
 */
export async function explainWord(params: {
  word: string;
  sentence: string;
  language: string;
  nativeLanguage?: string;
}): Promise<{ explanation: string | null }> {
  const response = await apiFetch('/v1/nlp/explain', {
    method: 'POST',
    body: JSON.stringify({
      word: params.word,
      sentence: params.sentence,
      language: params.language,
      native_language: params.nativeLanguage,
    }),
  });
  return response.json();
}

/**
 * Resolve a word-audio URL for the given lemma + reading.
 * Returns { audio_url: null } when no audio is available.
 */
export async function getWordAudio(params: {
  language: string;
  lemma: string;
  reading: string;
}): Promise<{ audio_url: string | null }> {
  const query = new URLSearchParams({
    language: params.language,
    lemma: params.lemma,
    reading: params.reading,
  });
  const response = await apiFetch(`/v1/nlp/word-audio?${query.toString()}`);
  return response.json();
}

/**
 * Fetch a best-effort dictionary image for a word (keyless Wikipedia source).
 * Returns { image_url: null } when no relevant image is found.
 */
export async function getWordImage(params: {
  word: string;
  language: string;
}): Promise<{ image_url: string | null }> {
  const search = new URLSearchParams({ word: params.word, language: params.language });
  const response = await apiFetch(`/v1/nlp/word-image?${search.toString()}`);
  return response.json();
}

/**
 * Get the number of cards due for review.
 */
export async function getDueCount(language?: string): Promise<number> {
  const params = new URLSearchParams();
  if (language) params.set('language', language);
  const response = await apiFetch(`/v1/review/due-count?${params.toString()}`);
  const data: { due_count: number } = await response.json();
  return data.due_count ?? 0;
}

/**
 * Fetch a review session (cards due for review).
 */
export async function getReviewSession(language = 'ja', limit = 20): Promise<{ cards: any[] }> {
  const params = new URLSearchParams({ language, limit: String(limit) });
  const response = await apiFetch(`/v1/review/session?${params.toString()}`);
  return response.json();
}

/**
 * Submit a review event to the server.
 */
export async function submitReviewEvent(params: {
  card_id: string;
  rating: 1 | 2 | 3 | 4;
  time_taken_ms?: number;
  reviewed_at?: string;
}): Promise<any> {
  const response = await apiFetch('/v1/review/events', {
    method: 'POST',
    body: JSON.stringify({
      card_id: params.card_id,
      rating: params.rating,
      time_taken_ms: params.time_taken_ms,
      reviewed_at: params.reviewed_at ?? new Date().toISOString(),
    }),
  });
  return response.json();
}
