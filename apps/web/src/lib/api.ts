/**
 * Typed API client for the Carve web app.
 */

const API_BASE: string = import.meta.env.VITE_API_BASE ?? 'http://localhost:8080';

function getToken(): string | null {
  if (typeof localStorage === 'undefined') return null;
  return localStorage.getItem('carve_access_token');
}

// ── Error type ────────────────────────────────────────────────────────────────

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

// ── Types ─────────────────────────────────────────────────────────────────────

export type FsrsState = 'new' | 'learning' | 'review' | 'relearning';

export interface Card {
  id: string;
  front_text: string;
  back_text: string | null;
  sentence: string | null;
  subtitle_translation: string | null;
  source_url: string | null;
  audio_url: string | null;
  sentence_audio_url: string | null;
  image_url: string | null;
  fsrs_state: FsrsState;
  stability: number | null;
  difficulty: number | null;
  due: string | null;
  reps: number;
  lapses: number;
  suspended: boolean;
  is_leech: boolean;
  tags: string[];
  created_at: string;
}

export interface User {
  id: string;
  email: string;
  display_name: string;
}

export interface CardsResponse {
  cards: Card[];
  total: number;
}

export interface LoginResponse {
  access_token: string;
  user: User;
}

export interface SessionResponse {
  cards: Card[];
  total: number;
  language_code: string;
}

export interface ReviewEventResponse {
  state: FsrsState;
  stability: number;
  difficulty: number;
  due: string;
  reps: number;
  lapses: number;
  is_leech: boolean;
}

export interface IntervalsResponse {
  again: string;
  hard: string;
  good: string;
  easy: string;
}

export interface ForecastDay {
  date: string;
  count: number;
}

export interface ForecastResponse {
  forecast: ForecastDay[];
  language_code: string;
}

export interface Deck {
  id: string;
  name: string;
  description: string | null;
  is_public: boolean;
  is_official: boolean;
  tags: string[];
  card_count: number;
  download_count: number;
  avg_rating: number | null;
  is_subscribed: boolean;
  created_at: string;
}

export interface DecksResponse {
  decks: Deck[];
}

export interface FsrsSettings {
  language_code: string;
  weights: number[];
  target_retention: number;
  leech_threshold: number;
  daily_new_limit: number;
  is_customized: boolean;
}

export interface WorkloadPreview {
  target_retention: number;
  avg_interval_days: number;
  total_review_cards: number;
}

export interface Notification {
  id: string;
  type: string;
  payload: Record<string, unknown>;
  created_at: string;
}

// ── Fetch helper ─────────────────────────────────────────────────────────────

export async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> | undefined),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, { ...options, headers });

  if (!res.ok) {
    // Expired/invalid session: clear token and redirect
    if (res.status === 401 && typeof window !== 'undefined' && localStorage.getItem('carve_access_token')) {
      localStorage.removeItem('carve_access_token');
      window.location.href = '/login';
      throw new ApiError(401, 'Session expired');
    }

    let message = `HTTP ${res.status}`;
    try {
      const body = await res.json();
      message = body.error ?? body.detail ?? body.message ?? message;
    } catch {
      // ignore
    }
    throw new ApiError(res.status, message);
  }

  return res.json() as Promise<T>;
}

/**
 * Upload a multipart form. The browser sets the Content-Type with the boundary,
 * so we omit it here and only pass the Authorization header.
 */
export async function postMultipart<T>(path: string, form: FormData): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {};
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    body: form,
    headers,
  });

  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    try {
      const body = await res.json();
      message = body.error ?? body.detail ?? body.message ?? message;
    } catch { /* ignore */ }
    throw new ApiError(res.status, message);
  }

  return res.json() as Promise<T>;
}

// ── Auth ─────────────────────────────────────────────────────────────────────

export async function fetchUser(): Promise<User> {
  return apiFetch<User>('/v1/users/me');
}

export async function fetchDueCount(language = 'ja'): Promise<{ due_count: number }> {
  return apiFetch<{ due_count: number }>(`/v1/review/due-count?language=${language}`);
}

export async function login(email: string, password: string): Promise<LoginResponse> {
  return apiFetch<LoginResponse>('/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
}

export async function register(email: string, password: string, displayName: string): Promise<LoginResponse> {
  return apiFetch<LoginResponse>('/v1/auth/register', {
    method: 'POST',
    body: JSON.stringify({ email, password, display_name: displayName }),
  });
}

export async function deleteAccount(): Promise<void> {
  await apiFetch('/v1/users/me', { method: 'DELETE' });
}

export async function markKnownWords(language: string, lemmas: string[]): Promise<{ marked: number }> {
  return apiFetch('/v1/onboarding/known-words', {
    method: 'POST',
    body: JSON.stringify({ language, lemmas }),
  });
}

export async function subscribeStarterDeck(language: string): Promise<{ deck_id?: string; status: string }> {
  return apiFetch('/v1/onboarding/starter-deck', {
    method: 'POST',
    body: JSON.stringify({ language }),
  });
}

// ── Cards ─────────────────────────────────────────────────────────────────────

export interface CardDetail {
  id: string;
  language_code: string;
  front_text: string;
  back_text: string | null;
  sentence: string | null;
  subtitle_translation: string | null;
  source_url: string | null;
  video_source_url: string | null;
  audio_url: string | null;
  image_url: string | null;
  subtitle_start_ms: number | null;
  subtitle_end_ms: number | null;
  fsrs_state: FsrsState;
  fsrs_due: string | null;
  stability: number | null;
  difficulty: number | null;
  reps: number;
  lapses: number;
  suspended: boolean;
  is_leech: boolean;
  notes: string | null;
  tags: string[];
  created_at: string;
}

export async function fetchCard(id: string): Promise<CardDetail> {
  return apiFetch<CardDetail>(`/v1/cards/${id}`);
}

export interface CardListParams {
  language?: string;
  limit?: number;
  offset?: number;
  search?: string;
  state?: FsrsState | '';
  suspended?: boolean | null;
  is_leech?: boolean | null;
  sort?: 'created' | 'due' | 'lapses' | 'alpha' | 'last_reviewed';
}

export async function fetchCards(language = 'ja', limit = 50): Promise<CardsResponse> {
  const params = new URLSearchParams({ language, limit: String(limit) });
  return apiFetch<CardsResponse>(`/v1/cards?${params}`);
}

export async function fetchCardsFiltered(p: CardListParams): Promise<CardsResponse> {
  const params = new URLSearchParams();
  if (p.language) params.set('language', p.language);
  if (p.limit != null) params.set('limit', String(p.limit));
  if (p.offset != null) params.set('offset', String(p.offset));
  if (p.search) params.set('search', p.search);
  if (p.state) params.set('state', p.state);
  if (p.suspended != null) params.set('suspended', String(p.suspended));
  if (p.is_leech != null) params.set('is_leech', String(p.is_leech));
  if (p.sort) params.set('sort', p.sort);
  return apiFetch<CardsResponse>(`/v1/cards?${params}`);
}

export async function updateCard(id: string, fields: {
  front_text?: string;
  front_reading?: string;
  back_text?: string;
  sentence?: string;
  subtitle_translation?: string;
  notes?: string;
  tags?: string[];
  deck_id?: string;
}): Promise<void> {
  await apiFetch(`/v1/cards/${id}`, { method: 'PATCH', body: JSON.stringify(fields) });
}

export async function suspendCard(id: string): Promise<void> {
  await apiFetch(`/v1/cards/${id}/suspend`, { method: 'POST' });
}

export async function unsuspendCard(id: string): Promise<void> {
  await apiFetch(`/v1/cards/${id}/unsuspend`, { method: 'POST' });
}

export async function buryCard(id: string): Promise<void> {
  await apiFetch(`/v1/cards/${id}/bury`, { method: 'POST' });
}

export async function bulkCards(
  action: 'suspend' | 'unsuspend' | 'bury' | 'delete',
  ids: string[],
): Promise<{ affected: number }> {
  return apiFetch('/v1/cards/bulk', {
    method: 'POST',
    body: JSON.stringify({ action, ids }),
  });
}

// ── Review ────────────────────────────────────────────────────────────────────

export async function fetchReviewSession(language = 'ja', limit = 20): Promise<SessionResponse> {
  const params = new URLSearchParams({ language, limit: String(limit) });
  return apiFetch<SessionResponse>(`/v1/review/session?${params}`);
}

export async function submitReviewEvent(
  cardId: string,
  rating: 1 | 2 | 3 | 4,
  timeTakenMs?: number,
): Promise<ReviewEventResponse> {
  return apiFetch<ReviewEventResponse>('/v1/review/events', {
    method: 'POST',
    body: JSON.stringify({
      card_id: cardId,
      rating,
      time_taken_ms: timeTakenMs,
      reviewed_at: new Date().toISOString(),
    }),
  });
}

export async function fetchIntervals(cardId: string): Promise<IntervalsResponse> {
  return apiFetch<IntervalsResponse>(`/v1/review/intervals?card_id=${encodeURIComponent(cardId)}`);
}

export interface UndoResponse {
  card_id: string;
  prior_state: string;
  undone_at: string;
}

export async function undoReview(): Promise<UndoResponse> {
  return apiFetch<UndoResponse>('/v1/review/undo', { method: 'POST' });
}

export async function fetchForecast(language = 'ja', days = 14): Promise<ForecastResponse> {
  const params = new URLSearchParams({ language, days: String(days) });
  return apiFetch<ForecastResponse>(`/v1/review/forecast?${params}`);
}

export async function fetchNotifications(): Promise<{ notifications: Notification[] }> {
  return apiFetch(`/v1/review/notifications`);
}

export async function markNotificationRead(id: string): Promise<void> {
  await apiFetch(`/v1/review/notifications/${id}/read`, { method: 'POST' });
}

// ── Decks ─────────────────────────────────────────────────────────────────────

export async function fetchDecks(language = 'ja', mine = false): Promise<DecksResponse> {
  const params = new URLSearchParams({ language, mine: String(mine) });
  return apiFetch<DecksResponse>(`/v1/decks?${params}`);
}

export async function subscribeDeck(deckId: string): Promise<{ subscribed: boolean }> {
  return apiFetch(`/v1/decks/${deckId}/subscribe`, { method: 'POST' });
}

export async function unsubscribeDeck(deckId: string): Promise<void> {
  await apiFetch(`/v1/decks/${deckId}/subscribe`, { method: 'DELETE' });
}

export async function rateDeck(deckId: string, rating: 1 | 2 | 3 | 4 | 5): Promise<void> {
  await apiFetch(`/v1/decks/${deckId}/rate`, {
    method: 'POST',
    body: JSON.stringify({ rating }),
  });
}

export async function createDeck(params: {
  name: string;
  description?: string;
  language_code?: string;
  tags?: string[];
  is_public?: boolean;
}): Promise<{ id: string; name: string }> {
  return apiFetch('/v1/decks', { method: 'POST', body: JSON.stringify(params) });
}

export async function updateDeck(deckId: string, params: {
  name?: string;
  description?: string;
  is_public?: boolean;
  tags?: string[];
}): Promise<void> {
  await apiFetch(`/v1/decks/${deckId}`, { method: 'PATCH', body: JSON.stringify(params) });
}

export async function deleteDeckById(deckId: string): Promise<void> {
  await apiFetch(`/v1/decks/${deckId}`, { method: 'DELETE' });
}

// ── Settings ──────────────────────────────────────────────────────────────────

export async function fetchFsrsSettings(language = 'ja'): Promise<FsrsSettings> {
  return apiFetch<FsrsSettings>(`/v1/settings/fsrs?language=${language}`);
}

export async function saveFsrsSettings(settings: Partial<FsrsSettings> & { language_code: string }): Promise<void> {
  await apiFetch('/v1/settings/fsrs', {
    method: 'PUT',
    body: JSON.stringify(settings),
  });
}

export async function fetchWorkloadPreview(language = 'ja', targetRetention: number): Promise<WorkloadPreview> {
  const params = new URLSearchParams({ language, target_retention: String(targetRetention) });
  return apiFetch<WorkloadPreview>(`/v1/settings/workload-preview?${params}`);
}

// ── Library reader ────────────────────────────────────────────────────────────

export interface ReaderToken {
  surface: string;
  lemma: string;
  reading_hira: string;
  pos: string;
  is_content_word: boolean;
  user_status: 'known' | 'learning' | 'unknown';
  frequency_rank: number | null;
}

export interface ReaderUnknownWord {
  lemma: string;
  reading: string;
  frequency_rank: number | null;
}

export interface ReaderResponse {
  id: string;
  title: string;
  url: string | null;
  language: string;
  tokens: ReaderToken[];
  comprehension_pct: number | null;
  unknown_words: ReaderUnknownWord[];
}

export async function fetchLibraryReader(id: string): Promise<ReaderResponse> {
  return apiFetch<ReaderResponse>(`/v1/library/${id}/reader`);
}

export async function importLibraryFile(
  file: File,
  language: string,
): Promise<{ id: string; title: string; language: string }> {
  const token = getToken();
  const form = new FormData();
  form.append('file', file);
  form.append('language', language);

  const headers: Record<string, string> = {};
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}/v1/library/import`, {
    method: 'POST',
    headers,
    body: form,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? `HTTP ${res.status}`);
  }
  return res.json();
}

export interface ImportResult {
  imported: number;
  skipped: number;
  language: string;
  type?: string;
}

export async function importAnki(file: File, language: string): Promise<ImportResult> {
  return _importFile('/v1/import/anki', file, language);
}

export async function importMigakuCSV(file: File, language: string): Promise<ImportResult> {
  return _importFile('/v1/import/migaku-csv', file, language);
}

export async function importYomitan(file: File, language: string): Promise<ImportResult> {
  return _importFile('/v1/import/yomitan', file, language);
}

export async function importJPDBCSV(file: File, language: string): Promise<ImportResult> {
  return _importFile('/v1/import/jpdb-csv', file, language);
}

async function _importFile(path: string, file: File, language: string): Promise<ImportResult> {
  const token = getToken();
  const form = new FormData();
  form.append('file', file);
  form.append('language', language);

  const headers: Record<string, string> = {};
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}${path}`, { method: 'POST', headers, body: form });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? `HTTP ${res.status}`);
  }
  return res.json();
}

// ── Export ────────────────────────────────────────────────────────────────────

export async function triggerExport(): Promise<void> {
  const token = getToken();
  const headers: Record<string, string> = {};
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}/v1/export`, { headers });
  if (!res.ok) {
    // Mirror apiFetch's auth handling so an expired session logs out instead
    // of leaving the user on a broken page, and surface the real error.
    if (res.status === 401 && typeof window !== 'undefined' && localStorage.getItem('carve_access_token')) {
      localStorage.removeItem('carve_access_token');
      window.location.href = '/login';
      throw new ApiError(401, 'Session expired');
    }
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? body.detail ?? body.message ?? `HTTP ${res.status}`);
  }

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `carve-export-${new Date().toISOString().slice(0, 10)}.json`;
  // Append to the DOM (Firefox requires it), click, then defer revocation so
  // the browser has begun reading the blob before the object URL is released.
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
