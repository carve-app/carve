/// <reference types="chrome" />

export interface StorageData {
  accessToken?: string;        // in session storage
  userId?: string;
  apiBaseUrl?: string;         // defaults to 'http://localhost:8080'
  nlpBaseUrl?: string;         // not used directly - goes through API
  enabledDomains?: string[];   // null = all domains
  knownLemmas?: string[];
  learningLemmas?: string[];
  ignoredLemmas?: string[];
  dueCount?: number;
  lastDueCountFetch?: number;  // timestamp
}

/**
 * Get a value from chrome.storage.local.
 */
export async function storageGet<K extends keyof StorageData>(
  key: K,
): Promise<StorageData[K] | undefined> {
  const result = await chrome.storage.local.get(key);
  return result[key] as StorageData[K] | undefined;
}

/**
 * Set a value in chrome.storage.local.
 */
export async function storageSet<K extends keyof StorageData>(
  key: K,
  value: StorageData[K],
): Promise<void> {
  await chrome.storage.local.set({ [key]: value });
}

/**
 * Get access token from chrome.storage.session (clears on browser close).
 */
export async function getAccessToken(): Promise<string | null> {
  const result = await chrome.storage.session.get('accessToken');
  return (result['accessToken'] as string | undefined) ?? null;
}

/**
 * Set access token in chrome.storage.session.
 */
export async function setAccessToken(token: string): Promise<void> {
  await chrome.storage.session.set({ accessToken: token });
}

/**
 * Clear session storage.
 */
export async function clearSession(): Promise<void> {
  await chrome.storage.session.clear();
}

/**
 * Get API base URL (default: http://localhost:8080).
 */
export async function getApiBaseUrl(): Promise<string> {
  const url = await storageGet('apiBaseUrl');
  return url ?? 'http://localhost:8080';
}
