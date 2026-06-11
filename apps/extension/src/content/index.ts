import { browser } from '../shared/browser';
import { VocabCache } from '../nlp/VocabCache';
import { PageAnnotator } from './annotator/PageAnnotator';
import { PopupManager } from './popup/PopupManager';
import { ImmersionTracker } from './tracker/ImmersionTracker';
import { SubtitleHook } from './video/SubtitleHook';
import { injectStyles } from './annotator/styles';

// All active subsystems are held so the "enable on this site" toggle can tear
// them down (and rebuild them) cleanly, without a page reload. `active` guards
// init/teardown so repeated toggles never double-mount or leave leaks.
let annotator: PageAnnotator | null = null;
let subtitleHook: SubtitleHook | null = null;
let immersionTracker: ImmersionTracker | null = null;
let active = false;
let overlayVisible = false;

async function isSiteDisabled(): Promise<boolean> {
  const result = await browser.storage.local.get('disabledDomains');
  const disabled: string[] = result['disabledDomains'] ?? [];
  return disabled.includes(location.hostname);
}

function teardown(): void {
  if (!active) return;
  active = false;

  annotator?.stop();              // disconnect observer + unwrap token spans
  annotator = null;
  subtitleHook?.destroy();        // remove the video subtitle overlay
  subtitleHook = null;
  immersionTracker?.destroy();    // clear interval + remove listeners + flush
  immersionTracker = null;

  document.getElementById('carve-popup')?.remove();
  document.getElementById('carve-overlay')?.remove();
  document.getElementById('carve-styles')?.remove();
  overlayVisible = false;
}

async function init(): Promise<void> {
  if (active) return;             // already running on this page
  if (await isSiteDisabled()) return;

  const lang = await detectLanguage();
  if (!lang) return;

  active = true;
  injectStyles();

  const vocabCache = new VocabCache();
  await vocabCache.load();

  // Re-check: a disable toggle could have raced the async load above.
  if (!active) return;

  const popupManager = new PopupManager(vocabCache);
  immersionTracker = new ImmersionTracker(lang);

  subtitleHook = new SubtitleHook(lang, vocabCache, popupManager);
  subtitleHook.mount();

  annotator = new PageAnnotator(lang, vocabCache, popupManager);
  annotator.start();

  // Restore the comprehension overlay if the user had it on.
  const ov = await browser.storage.local.get('overlayEnabled');
  if (ov['overlayEnabled']) showOverlay();
}

async function detectLanguage(): Promise<string | null> {
  const htmlLang = document.documentElement.lang?.slice(0, 5).toLowerCase();
  const text = document.body?.textContent?.slice(0, 2000) ?? '';
  const kanaCount = (text.match(/[぀-ヿ]/g) ?? []).length;
  const cjkCount = (text.match(/[一-鿿]/g) ?? []).length;
  const hangulCount = (text.match(/[가-힣]/g) ?? []).length;

  // 1. Strong script-based signals override any user preference.
  if (htmlLang?.startsWith('ja') || kanaCount > 20) return 'ja';
  if (htmlLang?.startsWith('ko') || hangulCount > 30) return 'ko';
  if (htmlLang?.startsWith('zh') || (cjkCount > 50 && kanaCount === 0 && hangulCount === 0)) return 'zh-cn';

  // 2. For English (and other Latin-script pages) require explicit opt-in:
  //    the user must have selected English as their target in the popup or
  //    enabled "annotate this site" via per-domain settings.
  const result = await browser.storage.local.get(['targetLanguage', 'annotateLatinSites']);
  const target = result['targetLanguage'] as string | undefined;
  const annotateLatin = result['annotateLatinSites'] as boolean | undefined;

  if (target === 'en' && (annotateLatin || htmlLang?.startsWith('en'))) {
    return 'en';
  }

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
    browser.storage.local.set({ overlayEnabled: false });
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
browser.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg.type === 'SET_SITE_ENABLED') {
    // Apply immediately, both ways, without a page reload.
    if (msg.enabled) {
      init().then(() => sendResponse({ ok: true })).catch(() => sendResponse({ ok: false }));
      return true; // async response
    }
    teardown();
    sendResponse({ ok: true });
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

// ── SPA navigation ──────────────────────────────────────────────────────────
// YouTube/Netflix/Disney+/etc. are single-page apps: navigating from the
// homepage to a /watch URL happens client-side with NO new page load, so a
// content script that only runs init() once (at document_idle on the homepage,
// where there's no video and no target language) would never mount the overlay
// when the user actually opens a video. We watch for URL changes and re-run
// init() — the flagship mining/annotation features depend on this.
let lastUrl = location.href;

function onNavigation(): void {
  if (location.href === lastUrl) return;
  lastUrl = location.href;
  // Rebuild for the new page: tear down the previous page's state, then
  // re-init (which re-detects language and re-mounts the platform hook). init()
  // is a no-op if the new page has no target language / is disabled.
  teardown();
  // Defer so the SPA has swapped in the new DOM (video element, captions)
  // before we detect language and mount hooks.
  setTimeout(() => { init(); }, 300);
}

function watchSpaNavigation(): void {
  // 1. history API (covers pushState/replaceState used by SPA routers).
  const wrap = (key: 'pushState' | 'replaceState') => {
    const orig = history[key];
    history[key] = function (this: History, ...args: Parameters<History['pushState']>) {
      const ret = orig.apply(this, args);
      onNavigation();
      return ret;
    } as History[typeof key];
  };
  wrap('pushState');
  wrap('replaceState');
  // 2. back/forward.
  window.addEventListener('popstate', onNavigation);
  // 3. YouTube's own SPA navigation event (fires after the watch page is ready).
  window.addEventListener('yt-navigate-finish', onNavigation);
  // 4. Safety net: a low-frequency poll catches any router that bypasses the
  //    above (some sites mutate location without the history API).
  setInterval(() => { if (location.href !== lastUrl) onNavigation(); }, 1000);
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => { init(); });
} else {
  init();
}
watchSpaNavigation();
