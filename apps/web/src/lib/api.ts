/**
 * Typed API client for the Carve web app.
 */

const API_BASE = 'http://localhost:8080';

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
  source_url: string | null;
  audio_url: string | null;
  image_url: string | null;
  fsrs_state: FsrsState;
  stability: number | null;
  difficulty: number | null;
  due: string | null;
  reps: number;
  lapses: number;
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

async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
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

// ── Auth ─────────────────────────────────────────────────────────────────────

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

// ── Cards ─────────────────────────────────────────────────────────────────────

export async function fetchCards(language = 'ja', limit = 50): Promise<CardsResponse> {
  const params = new URLSearchParams({ language, limit: String(limit) });
  return apiFetch<CardsResponse>(`/v1/cards?${params}`);
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

// ── Export ────────────────────────────────────────────────────────────────────

export function getExportUrl(): string {
  const token = getToken();
  return `${API_BASE}/v1/export${token ? `?token=${encodeURIComponent(token)}` : ''}`;
}

export async function triggerExport(): Promise<void> {
  const token = getToken();
  const headers: Record<string, string> = {};
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}/v1/export`, { headers });
  if (!res.ok) throw new ApiError(res.status, 'Export failed');

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `carve-export-${new Date().toISOString().slice(0, 10)}.json`;
  a.click();
  URL.revokeObjectURL(url);
}
