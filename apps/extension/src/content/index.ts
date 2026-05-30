/// <reference types="chrome" />
import { VocabCache } from '../nlp/VocabCache';
import { PageAnnotator } from './annotator/PageAnnotator';
import { PopupManager } from './popup/PopupManager';
import { ImmersionTracker } from './tracker/ImmersionTracker';
import { SubtitleHook } from './video/SubtitleHook';
import { injectStyles } from './annotator/styles';

let annotator: PageAnnotator | null = null;
let overlayVisible = false;

async function isSiteDisabled(): Promise<boolean> {
  const result = await chrome.storage.local.get('disabledDomains');
  const disabled: string[] = result['disabledDomains'] ?? [];
  return disabled.includes(location.hostname);
}

function teardown(): void {
  // Remove all token spans, restoring original text nodes
  document.querySelectorAll('[data-carve="processed"]').forEach(el => {
    el.removeAttribute('data-carve');
  });
  document.querySelectorAll('[data-carve="token"]').forEach(span => {
    span.replaceWith(span.textContent ?? '');
  });
  document.getElementById('carve-popup')?.remove();
  document.getElementById('carve-styles')?.remove();
  annotator = null;
}

async function init(): Promise<void> {
  if (await isSiteDisabled()) return;

  const lang = detectLanguage();
  if (!lang) return;

  injectStyles();

  const vocabCache = new VocabCache();
  await vocabCache.load();

  const popupManager = new PopupManager(vocabCache);
  new ImmersionTracker(lang);

  const subtitleHook = new SubtitleHook(lang, vocabCache, popupManager);
  subtitleHook.mount();

  annotator = new PageAnnotator(lang, vocabCache, popupManager);
  annotator.start();
}

function detectLanguage(): string | null {
  const htmlLang = document.documentElement.lang?.slice(0, 2).toLowerCase();
  if (htmlLang === 'ja') return 'ja';

  const text = document.body?.textContent?.slice(0, 2000) ?? '';
  const cjkCount = (text.match(/[぀-ヿ一-鿿]/g) ?? []).length;
  if (cjkCount > 50) return 'ja';

  return null;
}

// Compute comprehension from annotated token spans on the current page.
function computePageComprehension(): { pct: number | null; total: number } {
  const contentTokens = document.querySelectorAll('[data-carve="token"][data-content="1"]');
  const total = contentTokens.length;
  if (total === 0) return { pct: null, total: 0 };
  let known = 0;
  contentTokens.forEach(span => {
    const status = span.getAttribute('data-status');
    if (status === 'known' || status === 'learning') known++;
  });
  return { pct: Math.round((known / total) * 100), total };
}

function showOverlay(): void {
  let el = document.getElementById('carve-overlay');
  if (!el) {
    el = document.createElement('div');
    el.id = 'carve-overlay';
    document.body.appendChild(el);
  }
  const { pct, total } = computePageComprehension();
  const color = pct == null ? '#9ba8c0'
    : pct >= 95 ? '#4caf50'
    : pct >= 80 ? '#ffa726'
    : '#ef5350';
  el.innerHTML = `
    <span class="overlay-close" id="carve-overlay-close">✕</span>
    <div class="overlay-label">Comprehension</div>
    <div class="overlay-pct" style="color:${color}">${pct != null ? pct + '%' : '—'}</div>
    <div class="overlay-sub">${total} content words</div>
  `;
  el.querySelector('#carve-overlay-close')?.addEventListener('click', () => {
    hideOverlay();
    chrome.storage.local.set({ overlayEnabled: false });
  });
  overlayVisible = true;
}

function hideOverlay(): void {
  document.getElementById('carve-overlay')?.remove();
  overlayVisible = false;
}

function toggleOverlay(): void {
  if (overlayVisible) {
    hideOverlay();
  } else {
    showOverlay();
  }
}

// Listen for messages from the popup or background script.
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg.type === 'SET_SITE_ENABLED') {
    if (msg.enabled) { init(); } else { teardown(); }
    sendResponse({});
    return;
  }
  if (msg.type === 'GET_COMPREHENSION') {
    const result = computePageComprehension();
    if (overlayVisible) showOverlay();
    sendResponse(result);
    return;
  }
  if (msg.type === 'SET_OVERLAY') {
    if (msg.enabled) { showOverlay(); } else { hideOverlay(); }
    sendResponse({});
    return;
  }
});

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => { init(); });
} else {
  init();
}
