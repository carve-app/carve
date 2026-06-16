import { browser } from '../../shared/browser';
import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import type { Token } from '../../shared/types';
import { getVideoElement, attachVideoMedia } from './VideoCapture';
import { VIDEO_SHORTCUT_EVENT, type VideoShortcutAction } from './shortcutEvents';

export interface ActiveCue {
  text: string;
  startMs: number;
  endMs: number;
  nativeText?: string;
}

const OVERLAY_ID = 'carve-sub-overlay';
const MAX_HISTORY = 30;
const CUE_DEFERRED_RENDER_DELAY_MS = 320;
const CUE_IDLE_FINALIZE_DELAY_MS = 1200;
const MAX_CHUNK_CHARS = 120;
const MAX_CUE_GAP_MS = 1600;
const MIN_INITIAL_CHUNK_WORDS = 4;
const MIN_INITIAL_CHUNK_CHARS = 16;
const MIN_CHUNK_ADVANCE_WORDS = 4;
const MIN_CHUNK_ADVANCE_CHARS = 28;
const SENTENCE_END_RE = /[.!?。！？…]["')\]]*$/u;

export class SubtitleOverlay {
  private container: HTMLElement;
  private cueHistory: ActiveCue[] = [];
  private historyIndex = -1;
  private showNative = true;
  private miningInProgress = false;
  private keyHandler: ((e: KeyboardEvent) => void) | null = null;
  private shortcutHandler: ((e: Event) => void) | null = null;
  private renderVersion = 0;
  private hoverPausedVideo: HTMLVideoElement | null = null;
  private hoverResumeTimer: number | null = null;
  private nativeHideStyles: Array<{ selector: string; element: HTMLStyleElement }> = [];
  private resizeObserver: ResizeObserver | null = null;
  private observedVideo: HTMLVideoElement | null = null;
  private readonly updatePositionBound = () => this.updatePosition();
  private pendingCue: ActiveCue | null = null;
  private pendingHistoryIndex = -1;
  private pendingRenderTimer: number | null = null;
  private pendingFinalizeTimer: number | null = null;

  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
  ) {
    this.container = this.buildContainer();
    document.body.appendChild(this.container);
    this.popupManager.setInteractiveHoverCallbacks({
      onEnter: () => {
        this.cancelHoverResume();
        this.pauseForHover();
      },
      onLeave: () => {
        this.scheduleResumeAfterHover();
      },
    });
    this.bindPositionUpdates();
    this.updatePosition();
    this.bindKeys();
  }

  private buildContainer(): HTMLElement {
    const el = document.createElement('div');
    el.id = OVERLAY_ID;
    el.innerHTML = `
      <div class="cso-lines">
        <div class="cso-target" id="cso-target"></div>
        <div class="cso-native" id="cso-native"></div>
      </div>
      <div class="cso-mine-status" id="cso-mine-status"></div>
    `;

    // Inject styles once
    if (!document.getElementById('carve-sub-overlay-styles')) {
      const style = document.createElement('style');
      style.id = 'carve-sub-overlay-styles';
      style.textContent = OVERLAY_STYLES;
      document.head.appendChild(style);
    }

    const lines = el.querySelector<HTMLElement>('.cso-lines');
    lines?.addEventListener('mouseenter', () => {
      this.popupManager.cancelScheduledHide();
      this.cancelHoverResume();
      this.pauseForHover();
    });
    lines?.addEventListener('mouseleave', () => {
      this.popupManager.scheduleHidePopup();
      this.scheduleResumeAfterHover();
    });

    return el;
  }

  /** Called by platform hooks when a new subtitle cue becomes active. */
  onCue(cue: ActiveCue): void {
    this.queueCue(cue);
  }

  private queueCue(cue: ActiveCue): void {
    const normalized = normalizeCue(cue);
    if (!normalized) return;

    this.updatePosition();

    if (!this.pendingCue) {
      this.startPendingCue(normalized);
      return;
    }

    const pending = this.pendingCue;
    const gapMs = normalized.startMs > 0 && pending.endMs > 0
      ? normalized.startMs - pending.endMs
      : 0;
    const mergedText = mergeSubtitleText(pending.text, normalized.text);
    const textWouldOverflow = mergedText.length > MAX_CHUNK_CHARS;

    if (isSentenceComplete(pending.text) || gapMs > MAX_CUE_GAP_MS || textWouldOverflow) {
      this.finalizePendingCue();
      this.startPendingCue(normalized);
      return;
    }

    this.pendingCue = {
      text: mergedText,
      startMs: pending.startMs,
      endMs: Math.max(pending.endMs, normalized.endMs),
      nativeText: mergeOptionalSubtitleText(pending.nativeText, normalized.nativeText),
    };
    this.publishOrSchedulePendingCue();
    this.finalizeOrSchedulePendingCue();
  }

  private startPendingCue(cue: ActiveCue): void {
    this.pendingCue = cue;
    this.pendingHistoryIndex = -1;
    this.publishOrSchedulePendingCue();
    this.finalizeOrSchedulePendingCue();
  }

  private publishOrSchedulePendingCue(): void {
    if (!this.pendingCue) return;
    if (shouldPublishPendingCue(this.pendingCue.text, this.renderedPendingText())) {
      this.publishPendingCue();
      return;
    }
    if (!this.renderedPendingText()) {
      this.schedulePendingRender();
    }
  }

  private schedulePendingRender(): void {
    this.clearPendingRender();
    this.pendingRenderTimer = window.setTimeout(() => {
      this.pendingRenderTimer = null;
      this.publishPendingCue(true);
    }, CUE_DEFERRED_RENDER_DELAY_MS);
  }

  private clearPendingRender(): void {
    if (this.pendingRenderTimer == null) return;
    window.clearTimeout(this.pendingRenderTimer);
    this.pendingRenderTimer = null;
  }

  private finalizeOrSchedulePendingCue(): void {
    if (!this.pendingCue) return;
    if (shouldFinalizeCue(this.pendingCue)) {
      this.finalizePendingCue();
      return;
    }
    this.schedulePendingFinalize();
  }

  private schedulePendingFinalize(): void {
    this.clearPendingFinalize();
    this.pendingFinalizeTimer = window.setTimeout(() => {
      this.pendingFinalizeTimer = null;
      this.finalizePendingCue();
    }, CUE_IDLE_FINALIZE_DELAY_MS);
  }

  private clearPendingFinalize(): void {
    if (this.pendingFinalizeTimer == null) return;
    window.clearTimeout(this.pendingFinalizeTimer);
    this.pendingFinalizeTimer = null;
  }

  private finalizePendingCue(): void {
    if (this.pendingCue && !this.isPendingCueRendered()) {
      this.publishPendingCue(true);
    }
    this.clearPendingRender();
    this.clearPendingFinalize();
    this.pendingCue = null;
    this.pendingHistoryIndex = -1;
  }

  private publishPendingCue(force = false): void {
    const cue = this.pendingCue;
    if (!cue) return;
    if (!force && !shouldPublishPendingCue(cue.text, this.renderedPendingText())) return;
    if (this.isPendingCueRendered()) return;

    this.clearPendingRender();

    if (this.pendingHistoryIndex >= 0 && this.cueHistory[this.pendingHistoryIndex]) {
      this.cueHistory[this.pendingHistoryIndex] = cue;
      this.historyIndex = this.pendingHistoryIndex;
      this.renderAt(this.historyIndex);
      return;
    }

    this.cueHistory.push(cue);
    if (this.cueHistory.length > MAX_HISTORY) this.cueHistory.shift();
    this.historyIndex = this.cueHistory.length - 1;
    this.pendingHistoryIndex = this.historyIndex;
    this.renderAt(this.historyIndex);
  }

  private renderedPendingText(): string | null {
    if (this.pendingHistoryIndex < 0) return null;
    return this.cueHistory[this.pendingHistoryIndex]?.text ?? null;
  }

  private isPendingCueRendered(): boolean {
    const cue = this.pendingCue;
    if (!cue) return false;
    return this.renderedPendingText() === cue.text;
  }

  private stepHistory(delta: number): void {
    const next = this.historyIndex + delta;
    if (next < 0 || next >= this.cueHistory.length) return;
    this.historyIndex = next;
    this.renderAt(this.historyIndex);
  }

  private async renderAt(idx: number): Promise<void> {
    const cue = this.cueHistory[idx];
    if (!cue) return;
    const version = ++this.renderVersion;

    const targetEl = document.getElementById('cso-target');
    const nativeEl = document.getElementById('cso-native');
    if (!targetEl || !nativeEl) return;

    targetEl.textContent = cue.text;

    // Tokenize via background
    const response = await browser.runtime.sendMessage({
      type: 'TOKENIZE',
      text: cue.text,
      language: this.lang,
      knownLemmas: this.vocabCache.getKnownLemmas(),
      learningLemmas: this.vocabCache.getLearningLemmas(),
    });

    if (version !== this.renderVersion) return;

    if (!response?.tokens?.length) {
      targetEl.textContent = cue.text;
    } else {
      // Reconstruct the line by walking the ORIGINAL cue text and emitting the
      // characters between tokens (spaces, punctuation) as plain text nodes.
      // Concatenating token surfaces alone drops inter-word spacing — fine for
      // Japanese/Chinese, but it mashes English/Latin words together
      // ("It's harder working" → "It'sharderworking"). Mirrors PageAnnotator.
      targetEl.innerHTML = '';
      const text = cue.text;
      let pos = 0;
      for (const tok of response.tokens as Token[]) {
        const idx = text.indexOf(tok.surface, pos);
        if (idx === -1) continue;
        if (idx > pos) {
          targetEl.appendChild(document.createTextNode(text.slice(pos, idx)));
        }

        const span = document.createElement('span');
        span.setAttribute('data-carve', 'token');
        span.setAttribute('data-lemma', tok.lemma);
        span.setAttribute('data-reading', tok.reading_hira);
        span.setAttribute('data-content', tok.is_content_word ? '1' : '0');
        span.setAttribute(
          'data-status',
          tok.is_content_word ? this.vocabCache.getStatus(tok.lemma) : 'function',
        );
        span.textContent = tok.surface;
        span.className = 'cso-token';
        if (tok.is_content_word) {
          span.classList.add(`cso-${this.vocabCache.getStatus(tok.lemma)}`);
          span.addEventListener('mouseenter', () => {
            this.popupManager.cancelScheduledHide();
            this.cancelHoverResume();
            this.pauseForHover();
            void this.popupManager.showForElement(span);
          });
          span.addEventListener('click', e => {
            e.stopPropagation();
            this.popupManager.cancelScheduledHide();
            this.popupManager.showForElement(span);
          });
        }
        targetEl.appendChild(span);
        pos = idx + tok.surface.length;
      }
      if (pos < text.length) {
        targetEl.appendChild(document.createTextNode(text.slice(pos)));
      }
    }

    // Native subtitle
    if (cue.nativeText && this.showNative) {
      nativeEl.textContent = cue.nativeText;
      nativeEl.style.display = '';
    } else {
      nativeEl.style.display = 'none';
    }
  }

  private async mineCurrentCue(): Promise<void> {
    this.finalizePendingCue();
    if (this.miningInProgress || this.historyIndex < 0) return;
    const cue = this.cueHistory[this.historyIndex];
    if (!cue) return;

    // Find first unknown content word
    const targetEl = document.getElementById('cso-target');
    let targetToken: HTMLElement | null = null;
    if (targetEl) {
      const tokens = targetEl.querySelectorAll<HTMLElement>('[data-carve="token"][data-content="1"]');
      for (const t of Array.from(tokens)) {
        const st = t.getAttribute('data-status');
        if (st === 'unknown') { targetToken = t; break; }
      }
      // fallback: first content word
      if (!targetToken) targetToken = targetEl.querySelector<HTMLElement>('[data-carve="token"][data-content="1"]');
    }

    // Front of the card. When tokenization is unavailable (NLP down → no token
    // spans) we fall back to the whole cue text rather than an arbitrary
    // mid-grapheme 10-char slice — a coherent, editable front beats a truncated
    // fragment. The user can trim it in the card editor.
    const targetLemma = targetToken?.getAttribute('data-lemma')?.trim();
    const lemma = targetLemma && targetLemma.length > 0 ? targetLemma : cue.text.trim().slice(0, 80);
    const reading = targetToken?.getAttribute('data-reading')?.trim() ?? '';

    this.setMineStatus('Mining…');
    this.miningInProgress = true;

    try {
      // i+1 sentence selection: prev + current + next cues are candidates.
      let pickedSentence = cue.text;
      try {
        const prev = this.cueHistory[this.historyIndex - 1]?.text;
        const next = this.cueHistory[this.historyIndex + 1]?.text;
        const candidates = [prev, cue.text, next].filter((t): t is string => !!t);
        if (candidates.length > 1) {
          const sel = await browser.runtime.sendMessage({
            type: 'SELECT_SENTENCE',
            candidates,
            targetLemma: lemma,
            language: this.lang,
            knownLemmas: this.vocabCache.getKnownLemmas(),
            learningLemmas: this.vocabCache.getLearningLemmas(),
          });
          if (sel?.bestText && sel.bestContainsTarget) {
            pickedSentence = sel.bestText;
          }
        }
      } catch {/* selector is best-effort; fall back to current cue */}

      // Real translation of the mined sentence. Prefer the on-screen native
      // subtitle (a genuine human translation, when the user runs dual subs);
      // otherwise ask the NLP service. The popup mining path already does this —
      // video mining used to skip it entirely, so video cards had no
      // translation at all. Best-effort: a missing translation never blocks the
      // card.
      let translation = this.getNativeCueText(cue);
      if (!translation) {
        try {
          const tr = await browser.runtime.sendMessage({
            type: 'TRANSLATE',
            text: pickedSentence,
            sourceLanguage: this.lang,
          });
          translation = (tr?.translation as string | null) ?? undefined;
        } catch {/* translation is optional */}
      }

      // Link the card back to the exact moment in the video (seconds).
      const sourceTimestamp = cue.startMs > 0 ? cue.startMs / 1000 : undefined;

      // Create card via background (to reuse existing API path). The backend is
      // idempotent on (lemma, language): re-mining the same word returns the
      // existing card id, so media still attaches instead of orphaning.
      const result = await browser.runtime.sendMessage({
        type: 'MINE_CARD',
        lemma,
        reading,
        translation,
        sentence: pickedSentence,
        sourceUrl: window.location.href,
        sourceTimestamp,
        languageCode: this.lang,
      });

      if (!result?.cardId) {
        this.setMineStatus(result?.error ?? 'Mine failed', true);
        return;
      }

      const cardId = result.cardId as string;

      // Update vocab cache
      await this.vocabCache.markLearning(lemma);
      if (targetToken) targetToken.setAttribute('data-status', 'learning');

      // Capture screenshot + EXACT-sentence audio and attach to the card.
      // attachVideoMedia seeks to the cue's true source timing, grabs the frame
      // there (DRM-safe, via the worker) and records the audio over the cue
      // window — so the card's audio matches the sentence even if the user
      // paused or scrolled back through history. hasImage/hasAudio reflect what
      // the SERVER persisted, so DRM/upload failures get an honest message.
      const video = getVideoElement();
      if (video) {
        this.setMineStatus('Mined! Capturing media…');
        const media = await attachVideoMedia(
          video,
          cardId,
          { startMs: cue.startMs, endMs: cue.endMs },
          { sourceUrl: window.location.href, subtitleTranslation: translation },
        );
        if (media.hasImage || media.hasAudio) {
          const parts = [media.hasImage ? 'image' : null, media.hasAudio ? 'audio' : null].filter(Boolean);
          this.setMineStatus(`Mined! (+${parts.join(' & ')})`);
        } else if (media.error) {
          // Card saved (with sentence + translation) but media failed. Tell the
          // user honestly rather than implying success.
          this.setMineStatus(`Mined! (media failed: ${media.error})`, true);
        } else {
          // This player blocks frame/audio capture (e.g. HDCP-protected DRM).
          this.setMineStatus('Mined! (media unavailable on this site)');
        }
      } else {
        this.setMineStatus('Mined!');
      }

      setTimeout(() => this.setMineStatus(''), 2500);
    } catch {
      this.setMineStatus('Mine failed', true);
    } finally {
      this.miningInProgress = false;
    }
  }

  /**
   * Returns the text of a second "showing" subtitle track whose language
   * differs from the learning target — i.e. the user's native-language
   * subtitle, a real human translation of the current line. Returns undefined
   * when no such track is active (the common single-subtitle case), in which
   * case the caller falls back to the NLP translation.
   */
  private getNativeCueText(cue: ActiveCue): string | undefined {
    if (cue.nativeText && cue.nativeText.trim()) return cue.nativeText.trim();
    const video = getVideoElement();
    if (!video || !video.textTracks) return undefined;
    for (let i = 0; i < video.textTracks.length; i++) {
      const track = video.textTracks[i];
      if (track.mode !== 'showing') continue;
      const lang = (track.language || '').slice(0, 2).toLowerCase();
      if (lang && lang === this.lang.slice(0, 2).toLowerCase()) continue; // same as target — skip
      const active = track.activeCues;
      if (!active || active.length === 0) continue;
      const text = Array.from(active)
        .map(c => (c as VTTCue).text ?? '')
        .join(' ')
        .replace(/<[^>]+>/g, '')
        .trim();
      if (text) return text;
    }
    return undefined;
  }

  private setMineStatus(msg: string, error = false): void {
    const el = document.getElementById('cso-mine-status');
    if (!el) return;
    el.textContent = msg;
    el.className = 'cso-mine-status' + (error ? ' cso-mine-error' : msg ? ' cso-mine-ok' : '');
  }

  private bindPositionUpdates(): void {
    window.addEventListener('resize', this.updatePositionBound, { passive: true });
    window.addEventListener('scroll', this.updatePositionBound, { capture: true, passive: true });
    document.addEventListener('fullscreenchange', this.updatePositionBound);
    if (typeof ResizeObserver !== 'undefined') {
      this.resizeObserver = new ResizeObserver(() => this.updatePosition());
    }
  }

  private updatePosition(): void {
    const video = getVideoElement();
    if (!video) {
      this.observedVideo = null;
      this.resizeObserver?.disconnect();
      this.applyViewportPosition();
      return;
    }

    if (this.observedVideo !== video) {
      this.resizeObserver?.disconnect();
      this.resizeObserver?.observe(video);
      this.observedVideo = video;
    }

    const rect = video.getBoundingClientRect();
    const viewportW = window.innerWidth || document.documentElement.clientWidth || 0;
    const viewportH = window.innerHeight || document.documentElement.clientHeight || 0;
    if (rect.width < 20 || rect.height < 20 || viewportW <= 0 || viewportH <= 0) {
      this.applyViewportPosition();
      return;
    }

    const margin = 8;
    const visibleLeft = Math.max(rect.left, margin);
    const visibleRight = Math.min(rect.right, viewportW - margin);
    const width = visibleRight - visibleLeft;
    if (width < 20) {
      this.applyViewportPosition();
      return;
    }
    const visibleTop = clamp(rect.top, 0, viewportH);
    const visibleBottom = clamp(rect.bottom, 0, viewportH);
    const visibleHeight = Math.max(0, visibleBottom - visibleTop);
    const offset = Math.round(clamp(visibleHeight * 0.12, 34, 86));
    const bottom = Math.max(margin, viewportH - visibleBottom + offset);

    this.container.style.left = `${visibleLeft}px`;
    this.container.style.width = `${width}px`;
    this.container.style.bottom = `${bottom}px`;
    this.container.style.transform = 'none';
  }

  private applyViewportPosition(): void {
    this.container.style.left = '50%';
    this.container.style.width = 'min(1180px, 92vw)';
    this.container.style.bottom = window.innerWidth <= 720 ? '58px' : '86px';
    this.container.style.transform = 'translateX(-50%)';
  }

  private pauseForHover(): void {
    const video = getVideoElement();
    if (!video || this.hoverPausedVideo === video) return;
    if (!video.paused) {
      video.pause();
      this.hoverPausedVideo = video;
    }
  }

  private resumeAfterHover(): void {
    this.cancelHoverResume();
    const video = this.hoverPausedVideo;
    this.hoverPausedVideo = null;
    if (!video || !video.paused) return;
    void video.play().catch(() => {
      // Browser autoplay policy or a streaming-player guard can reject resume;
      // the user can still press play manually.
    });
  }

  private scheduleResumeAfterHover(delayMs = 180): void {
    this.cancelHoverResume();
    this.hoverResumeTimer = window.setTimeout(() => {
      this.hoverResumeTimer = null;
      this.resumeAfterHover();
    }, delayMs);
  }

  private cancelHoverResume(): void {
    if (this.hoverResumeTimer == null) return;
    window.clearTimeout(this.hoverResumeTimer);
    this.hoverResumeTimer = null;
  }

  private bindKeys(): void {
    const runShortcut = (action: VideoShortcutAction) => {
      if (action === 'prev') this.stepHistory(-1);
      else if (action === 'next') this.stepHistory(+1);
      else this.mineCurrentCue();
    };

    this.shortcutHandler = (e: Event) => {
      const action = (e as CustomEvent<{ action?: VideoShortcutAction }>).detail?.action;
      if (action === 'prev' || action === 'next' || action === 'mine') {
        runShortcut(action);
      }
    };
    window.addEventListener(VIDEO_SHORTCUT_EVENT, this.shortcutHandler);

    this.keyHandler = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        e.stopImmediatePropagation();
        runShortcut('prev');
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        e.stopImmediatePropagation();
        runShortcut('next');
      } else if (e.key.toLowerCase() === 'm' && !e.ctrlKey && !e.metaKey) {
        e.preventDefault();
        e.stopImmediatePropagation();
        runShortcut('mine');
      }
    };
    document.addEventListener('keydown', this.keyHandler, true);
  }

  /** Hide the native subtitle container for a given selector. */
  hideNativeContainer(selector: string): void {
    document.querySelectorAll<HTMLElement>(selector).forEach((el) => {
      el.style.visibility = 'hidden';
    });
    if (this.nativeHideStyles.some((entry) => entry.selector === selector)) return;
    const style = document.createElement('style');
    style.textContent = `${selector} { visibility: hidden !important; }`;
    document.head.appendChild(style);
    this.nativeHideStyles.push({ selector, element: style });
  }

  showNativeContainer(selector: string): void {
    document.querySelectorAll<HTMLElement>(selector).forEach((el) => {
      el.style.visibility = '';
    });
    this.nativeHideStyles = this.nativeHideStyles.filter((entry) => {
      if (entry.selector !== selector) return true;
      entry.element.remove();
      return false;
    });
  }

  destroy(): void {
    this.popupManager.hidePopup();
    this.nativeHideStyles.forEach((entry) => entry.element.remove());
    this.nativeHideStyles = [];
    this.resumeAfterHover();
    this.popupManager.setInteractiveHoverCallbacks(null);
    this.clearPendingRender();
    this.clearPendingFinalize();
    this.resizeObserver?.disconnect();
    window.removeEventListener('resize', this.updatePositionBound);
    window.removeEventListener('scroll', this.updatePositionBound, true);
    document.removeEventListener('fullscreenchange', this.updatePositionBound);
    this.container.remove();
    if (this.shortcutHandler) window.removeEventListener(VIDEO_SHORTCUT_EVENT, this.shortcutHandler);
    if (this.keyHandler) document.removeEventListener('keydown', this.keyHandler, true);
    document.getElementById('carve-sub-overlay-styles')?.remove();
  }
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function normalizeCue(cue: ActiveCue): ActiveCue | null {
  const text = normalizeSubtitleText(cue.text);
  if (!text) return null;
  const nativeText = cue.nativeText ? normalizeSubtitleText(cue.nativeText) : undefined;
  return { ...cue, text, nativeText };
}

function normalizeSubtitleText(text: string): string {
  return text.replace(/\s+/g, ' ').trim();
}

function isSentenceComplete(text: string): boolean {
  return SENTENCE_END_RE.test(text.trim());
}

function shouldFinalizeCue(cue: ActiveCue): boolean {
  return isSentenceComplete(cue.text) || cue.text.length >= MAX_CHUNK_CHARS;
}

function shouldPublishPendingCue(text: string, renderedText: string | null): boolean {
  if (isSentenceComplete(text) || text.length >= MAX_CHUNK_CHARS) return true;

  if (!renderedText) {
    return wordCount(text) >= MIN_INITIAL_CHUNK_WORDS
      || compactCharCount(text) >= MIN_INITIAL_CHUNK_CHARS;
  }

  if (text === renderedText) return false;
  if (!text.startsWith(renderedText)) return true;

  return wordCount(text) - wordCount(renderedText) >= MIN_CHUNK_ADVANCE_WORDS
    || compactCharCount(text) - compactCharCount(renderedText) >= MIN_CHUNK_ADVANCE_CHARS;
}

function wordCount(text: string): number {
  return text.trim().split(/\s+/u).filter(Boolean).length;
}

function compactCharCount(text: string): number {
  return Array.from(text.replace(/\s+/gu, '')).length;
}

function mergeOptionalSubtitleText(prev: string | undefined, next: string | undefined): string | undefined {
  if (!prev) return next;
  if (!next) return prev;
  return mergeSubtitleText(prev, next);
}

function mergeSubtitleText(prev: string, next: string): string {
  if (!prev) return next;
  if (!next || next === prev) return prev;
  if (next.startsWith(prev)) return next;
  if (prev.startsWith(next)) return prev;
  if (next.includes(prev)) return next;
  if (prev.includes(next)) return prev;
  return `${prev} ${next}`.replace(/\s+/g, ' ').trim();
}

const OVERLAY_STYLES = `
#carve-sub-overlay {
  position: fixed;
  bottom: 86px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 2147483647;
  width: min(1180px, 92vw);
  padding: 0 18px;
  box-sizing: border-box;
  font-family: 'Noto Sans JP', 'Hiragino Sans', -apple-system, BlinkMacSystemFont, system-ui, sans-serif;
  -webkit-font-smoothing: antialiased;
  pointer-events: none;
  user-select: none;
  text-align: center;
}

.cso-lines { display: inline-block; pointer-events: auto; max-width: min(100%, 1180px); }

.cso-target {
  font-size: 34px;
  line-height: 1.25;
  color: #fff;
  text-align: center;
  min-height: 42px;
  letter-spacing: 0;
  font-weight: 700;
  overflow-wrap: break-word;
  word-break: normal;
  text-shadow:
    0 2px 2px #000,
    0 -2px 2px #000,
    2px 0 2px #000,
    -2px 0 2px #000,
    0 4px 10px rgba(0,0,0,0.95);
}

.cso-token {
  cursor: default;
  border-radius: 3px;
  padding: 0 1px;
  transition: background 0.08s, color 0.08s;
}
.cso-token.cso-unknown { color: #ff9f9f; cursor: pointer; }
.cso-token.cso-learning { color: #ffc46b; cursor: pointer; }
.cso-token.cso-known { color: #fff; cursor: pointer; }
.cso-token.cso-unknown:hover,
.cso-token.cso-learning:hover,
.cso-token.cso-known:hover { background: rgba(0,0,0,0.52); }

.cso-native {
  font-size: 22px;
  color: #fff;
  text-align: center;
  margin-top: 8px;
  line-height: 1.25;
  font-weight: 650;
  text-shadow:
    0 2px 2px #000,
    0 -2px 2px #000,
    2px 0 2px #000,
    -2px 0 2px #000,
    0 4px 10px rgba(0,0,0,0.95);
}

.cso-mine-status {
  font-size: 13px;
  text-align: center;
  min-height: 18px;
  margin-top: 8px;
  transition: color 0.2s;
  font-weight: 650;
  text-shadow: 0 2px 6px #000;
}
.cso-mine-ok { color: #6ddf72; }
.cso-mine-error { color: #ff6b6b; }

@media (max-width: 720px) {
  #carve-sub-overlay { bottom: 58px; width: 96vw; padding: 0 10px; }
  .cso-target { font-size: 24px; min-height: 30px; }
  .cso-native { font-size: 18px; }
}
`;
