/// <reference types="chrome" />
import { VocabCache } from '../nlp/VocabCache';
import { PageAnnotator } from './annotator/PageAnnotator';
import { PopupManager } from './popup/PopupManager';
import { ImmersionTracker } from './tracker/ImmersionTracker';
import { SubtitleHook } from './video/SubtitleHook';
import { injectStyles } from './annotator/styles';

async function init(): Promise<void> {
  const lang = detectLanguage();
  if (!lang) return;

  injectStyles();

  const vocabCache = new VocabCache();
  await vocabCache.load();

  const popupManager = new PopupManager(vocabCache);
  new ImmersionTracker(lang);

  const subtitleHook = new SubtitleHook(lang, vocabCache, popupManager);
  subtitleHook.mount();

  const annotator = new PageAnnotator(lang, vocabCache, popupManager);
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

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => { init(); });
} else {
  init();
}
