import { getAccessToken, getApiBaseUrl } from './storage';

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

async function apiFetch(path: string, options: RequestInit = {}): Promise<Response> {
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
  sentence?: string;
  source_url?: string;
}): Promise<{ id: string; lemma: string }> {
  const response = await apiFetch('/v1/cards', {
    method: 'POST',
    body: JSON.stringify(params),
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
