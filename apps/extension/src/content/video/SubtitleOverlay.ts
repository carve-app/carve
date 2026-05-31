import { browser } from '../../shared/browser';
import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import type { Token } from '../../shared/types';
import { getVideoElement, captureVideoMedia, uploadCardMedia } from './VideoCapture';

export interface ActiveCue {
  text: string;
  startMs: number;
  endMs: number;
  nativeText?: string;
}

const OVERLAY_ID = 'carve-sub-overlay';
const MAX_HISTORY = 30;

export class SubtitleOverlay {
  private container: HTMLElement;
  private cueHistory: ActiveCue[] = [];
  private historyIndex = -1;
  private pauseOnSub = false;
  private showNative = true;
  private miningInProgress = false;
  private keyHandler: ((e: KeyboardEvent) => void) | null = null;

  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
  ) {
    this.container = this.buildContainer();
    document.body.appendChild(this.container);
    this.bindKeys();
  }

  private buildContainer(): HTMLElement {
    const el = document.createElement('div');
    el.id = OVERLAY_ID;
    el.innerHTML = `
      <div class="cso-bar">
        <button class="cso-btn cso-prev" title="Previous subtitle (←)">◀</button>
        <div class="cso-toggles">
          <button class="cso-btn cso-pause" title="Pause on subtitle">⏸</button>
          <button class="cso-btn cso-native-toggle" title="Toggle native subtitle">CC</button>
        </div>
        <button class="cso-btn cso-next" title="Next subtitle (→)">▶</button>
      </div>
      <div class="cso-target" id="cso-target"><span class="cso-hint">Subtitles will appear here</span></div>
      <div class="cso-native" id="cso-native"></div>
      <div class="cso-mine-status" id="cso-mine-status"></div>
    `;

    // Inject styles once
    if (!document.getElementById('carve-sub-overlay-styles')) {
      const style = document.createElement('style');
      style.id = 'carve-sub-overlay-styles';
      style.textContent = OVERLAY_STYLES;
      document.head.appendChild(style);
    }

    el.querySelector('.cso-prev')?.addEventListener('click', () => this.stepHistory(-1));
    el.querySelector('.cso-next')?.addEventListener('click', () => this.stepHistory(+1));
    el.querySelector('.cso-pause')?.addEventListener('click', () => this.togglePause());
    el.querySelector('.cso-native-toggle')?.addEventListener('click', () => this.toggleNative());

    return el;
  }

  /** Called by platform hooks when a new subtitle cue becomes active. */
  onCue(cue: ActiveCue): void {
    this.cueHistory.push(cue);
    if (this.cueHistory.length > MAX_HISTORY) this.cueHistory.shift();
    this.historyIndex = this.cueHistory.length - 1;

    if (this.pauseOnSub) {
      getVideoElement()?.pause();
    }

    this.renderAt(this.historyIndex);
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

    const targetEl = document.getElementById('cso-target');
    const nativeEl = document.getElementById('cso-native');
    if (!targetEl || !nativeEl) return;

    targetEl.innerHTML = '<span class="cso-hint">Tokenizing…</span>';

    // Tokenize via background
    const response = await browser.runtime.sendMessage({
      type: 'TOKENIZE',
      text: cue.text,
      language: this.lang,
      knownLemmas: this.vocabCache.getKnownLemmas(),
      learningLemmas: this.vocabCache.getLearningLemmas(),
    });

    if (!response?.tokens) {
      targetEl.textContent = cue.text;
    } else {
      targetEl.innerHTML = '';
      for (const tok of response.tokens as Token[]) {
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
          span.addEventListener('click', e => {
            e.stopPropagation();
            this.popupManager.showForElement(span);
          });
        }
        targetEl.appendChild(span);
      }
    }

    // Native subtitle
    if (cue.nativeText && this.showNative) {
      nativeEl.textContent = cue.nativeText;
      nativeEl.style.display = '';
    } else {
      nativeEl.style.display = 'none';
    }

    // Prev/next button state
    this.container.querySelector<HTMLButtonElement>('.cso-prev')!.disabled = idx === 0;
    this.container.querySelector<HTMLButtonElement>('.cso-next')!.disabled = idx === this.cueHistory.length - 1;
  }

  private togglePause(): void {
    this.pauseOnSub = !this.pauseOnSub;
    const btn = this.container.querySelector('.cso-pause');
    if (btn) btn.classList.toggle('cso-active', this.pauseOnSub);
  }

  private toggleNative(): void {
    this.showNative = !this.showNative;
    const btn = this.container.querySelector('.cso-native-toggle');
    if (btn) btn.classList.toggle('cso-active', this.showNative);
    const nativeEl = document.getElementById('cso-native');
    if (nativeEl) nativeEl.style.display = this.showNative ? '' : 'none';
  }

  private async mineCurrentCue(): Promise<void> {
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

    const lemma = targetToken?.getAttribute('data-lemma') ?? cue.text.slice(0, 10);
    const reading = targetToken?.getAttribute('data-reading') ?? '';

    this.setMineStatus('Mining…');
    this.miningInProgress = true;

    try {
      // Create card via background (to reuse existing API path)
      const result = await browser.runtime.sendMessage({
        type: 'MINE_CARD',
        lemma,
        reading,
        sentence: cue.text,
        sourceUrl: window.location.href,
        languageCode: this.lang,
      });

      if (!result?.cardId) {
        this.setMineStatus(result?.error ?? 'Already mined', true);
        return;
      }

      const cardId = result.cardId as string;

      // Update vocab cache
      await this.vocabCache.markLearning(lemma);
      if (targetToken) targetToken.setAttribute('data-status', 'learning');

      this.setMineStatus('Mined! Capturing…');

      // Capture + upload media asynchronously
      const video = getVideoElement();
      if (video) {
        const cueDurationMs = cue.endMs - cue.startMs || 4000;
        captureVideoMedia(video, Math.min(cueDurationMs, 8000)).then(capture =>
          uploadCardMedia(cardId, capture, {
            sourceUrl: window.location.href,
            subtitleTranslation: cue.nativeText,
          }),
        );
      }

      setTimeout(() => this.setMineStatus(''), 2500);
    } catch {
      this.setMineStatus('Mine failed', true);
    } finally {
      this.miningInProgress = false;
    }
  }

  private setMineStatus(msg: string, error = false): void {
    const el = document.getElementById('cso-mine-status');
    if (!el) return;
    el.textContent = msg;
    el.className = 'cso-mine-status' + (error ? ' cso-mine-error' : msg ? ' cso-mine-ok' : '');
  }

  private bindKeys(): void {
    this.keyHandler = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.key === 'ArrowLeft') { e.preventDefault(); this.stepHistory(-1); }
      else if (e.key === 'ArrowRight') { e.preventDefault(); this.stepHistory(+1); }
      else if (e.key.toLowerCase() === 'm' && !e.ctrlKey && !e.metaKey) {
        e.preventDefault();
        this.mineCurrentCue();
      }
    };
    document.addEventListener('keydown', this.keyHandler);
  }

  /** Hide the native subtitle container for a given selector. */
  hideNativeContainer(selector: string): void {
    const el = document.querySelector<HTMLElement>(selector);
    if (el) el.style.visibility = 'hidden';
  }

  showNativeContainer(selector: string): void {
    const el = document.querySelector<HTMLElement>(selector);
    if (el) el.style.visibility = '';
  }

  destroy(): void {
    this.container.remove();
    if (this.keyHandler) document.removeEventListener('keydown', this.keyHandler);
    document.getElementById('carve-sub-overlay-styles')?.remove();
  }
}

const OVERLAY_STYLES = `
#carve-sub-overlay {
  position: fixed;
  bottom: 130px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 2147483647;
  background: rgba(15, 17, 22, 0.88);
  border: 1px solid rgba(255,255,255,0.10);
  border-radius: 10px;
  padding: 0.5rem 0.75rem 0.6rem;
  max-width: 720px;
  min-width: 280px;
  width: max-content;
  font-family: 'Noto Sans JP', 'Hiragino Sans', system-ui, sans-serif;
  backdrop-filter: blur(6px);
  pointer-events: auto;
  user-select: none;
  box-shadow: 0 4px 24px rgba(0,0,0,0.5);
}

.cso-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-bottom: 0.4rem;
}

.cso-toggles {
  display: flex;
  gap: 0.35rem;
}

.cso-btn {
  background: rgba(255,255,255,0.08);
  border: 1px solid rgba(255,255,255,0.12);
  color: #9ba8c0;
  border-radius: 5px;
  padding: 0.15rem 0.5rem;
  font-size: 0.7rem;
  cursor: pointer;
  transition: background 0.1s, color 0.1s;
  line-height: 1.5;
}
.cso-btn:hover { background: rgba(255,255,255,0.14); color: #e8eaf0; }
.cso-btn:disabled { opacity: 0.3; cursor: default; }
.cso-btn.cso-active { background: rgba(76,175,80,0.25); border-color: #4caf50; color: #4caf50; }

.cso-target {
  font-size: 1.25rem;
  line-height: 1.6;
  color: #e8eaf0;
  text-align: center;
  min-height: 1.8rem;
  letter-spacing: 0.02em;
  word-break: break-all;
}

.cso-hint { color: #4a5568; font-size: 0.85rem; }

.cso-token { cursor: default; }
.cso-token.cso-unknown { color: #ef9a9a; cursor: pointer; }
.cso-token.cso-learning { color: #ffa726; cursor: pointer; }
.cso-token.cso-known { color: #e8eaf0; cursor: pointer; }

.cso-native {
  font-size: 0.82rem;
  color: #7a8aa6;
  text-align: center;
  margin-top: 0.25rem;
}

.cso-mine-status {
  font-size: 0.72rem;
  text-align: center;
  min-height: 1rem;
  margin-top: 0.15rem;
  transition: color 0.2s;
}
.cso-mine-ok { color: #4caf50; }
.cso-mine-error { color: #ef5350; }
`;
