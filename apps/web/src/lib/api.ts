/**
 * Typed API client for the Carve web app.
 * All requests use the browser's fetch; no server-side usage.
 */

const API_BASE = 'http://localhost:8080';

function getToken(): string | null {
  if (typeof localStorage === 'undefined') return null;
  return localStorage.getItem('carve_access_token');
}

// ── Types ──────────────────────────────────────────────────────────────────────

export interface Card {
  id: string;
  front_text: string;   // lemma
  back_text: string;    // definition / translation
  sentence: string | null;
  fsrs_state: 'new' | 'learning' | 'review' | 'relearning';
  created_at: string;   // ISO 8601
}

export interface User {
  id: string;
  email: string;
}

export interface CardsResponse {
  cards: Card[];
  total: number;
}

export interface LoginResponse {
  access_token: string;
  user: User;
}

// ── Helpers ────────────────────────────────────────────────────────────────────

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
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
      message = body.detail ?? body.message ?? message;
    } catch {
      // ignore JSON parse error
    }
    throw new ApiError(res.status, message);
  }

  return res.json() as Promise<T>;
}

// ── Public API ─────────────────────────────────────────────────────────────────

export async function fetchCards(
  language = 'ja',
  limit = 50,
): Promise<CardsResponse> {
  const params = new URLSearchParams({ language, limit: String(limit) });
  return apiFetch<CardsResponse>(`/v1/cards?${params}`);
}

export async function login(
  email: string,
  password: string,
): Promise<LoginResponse> {
  return apiFetch<LoginResponse>('/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
}

export { ApiError };
