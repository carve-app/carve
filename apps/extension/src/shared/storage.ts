import { browser } from './browser';

export interface OfflineReviewEvent {
  card_id: string;
  rating: 1 | 2 | 3 | 4;
  time_taken_ms: number;
  reviewed_at: string; // ISO 8601
}

export interface CachedReviewCard {
  id: string;
  front_text: string;
  back_text: string | null;
  sentence: string | null;
  source_url: string | null;
  fsrs_state: string;
  stability: number | null;
  difficulty: number | null;
  reps: number;
  lapses: number;
}

export interface StorageData {
  accessToken?: string;        // short-lived bearer JWT
  refreshToken?: string;       // long-lived; exchanged for a new access token on 401
  userId?: string;
  apiBaseUrl?: string;         // defaults to 'http://localhost:8080'
  nlpBaseUrl?: string;         // not used directly - goes through API
  enabledDomains?: string[];   // null = all domains
  disabledDomains?: string[];  // domains where annotation is suppressed
  knownLemmas?: string[];
  learningLemmas?: string[];
  ignoredLemmas?: string[];
  dueCount?: number;
  lastDueCountFetch?: number;  // timestamp
  offlineReviewQueue?: OfflineReviewEvent[];  // events pending sync
  cachedReviewCards?: CachedReviewCard[];     // cards for offline review
  cachedReviewAt?: number;                    // timestamp of last cache
}

export async function storageGet<K extends keyof StorageData>(
  key: K,
): Promise<StorageData[K] | undefined> {
  const result = await browser.storage.local.get(key);
  return result[key] as StorageData[K] | undefined;
}

export async function storageSet<K extends keyof StorageData>(
  key: K,
  value: StorageData[K],
): Promise<void> {
  await browser.storage.local.set({ [key]: value });
}

export async function getAccessToken(): Promise<string | null> {
  const result = await browser.storage.local.get('accessToken');
  return (result['accessToken'] as string | undefined) ?? null;
}

export async function setAccessToken(token: string): Promise<void> {
  await browser.storage.local.set({ accessToken: token });
}

export async function getRefreshToken(): Promise<string | null> {
  const result = await browser.storage.local.get('refreshToken');
  return (result['refreshToken'] as string | undefined) ?? null;
}

export async function setRefreshToken(token: string): Promise<void> {
  await browser.storage.local.set({ refreshToken: token });
}

export async function clearSession(): Promise<void> {
  await browser.storage.local.remove(['accessToken', 'refreshToken']);
}

export async function getApiBaseUrl(): Promise<string> {
  const url = await storageGet('apiBaseUrl');
  return url ?? 'http://localhost:8080';
}
