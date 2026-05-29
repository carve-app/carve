/// <reference types="chrome" />
import { VocabCache } from '../nlp/VocabCache';
import { PageAnnotator } from './annotator/PageAnnotator';
import { PopupManager } from './popup/PopupManager';
import { ImmersionTracker } from './tracker/ImmersionTracker';
import { SubtitleHook } from './video/SubtitleHook';
import { injectStyles } from './annotator/styles';

let annotator: PageAnnotator | null = null;

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

// Listen for toggle from the popup
chrome.runtime.onMessage.addListener((msg) => {
  if (msg.type !== 'SET_SITE_ENABLED') return;
  if (msg.enabled) {
    init();
  } else {
    teardown();
  }
});

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => { init(); });
} else {
  init();
}
