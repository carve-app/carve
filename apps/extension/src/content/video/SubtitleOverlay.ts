import { browser } from '../../shared/browser';
import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import type { Token } from '../../shared/types';
import { getVideoElement, attachVideoMedia } from './VideoCapture';

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
          span.addEventListener('click', e => {
            e.stopPropagation();
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
  bottom: 120px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 2147483647;
  background: linear-gradient(180deg, rgba(20,22,28,0.94), rgba(13,15,20,0.94));
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 14px;
  padding: 10px 18px 14px;
  max-width: min(900px, 80vw);
  min-width: 360px;
  width: max-content;
  box-sizing: border-box;
  font-family: 'Noto Sans JP', 'Hiragino Sans', -apple-system, BlinkMacSystemFont, system-ui, sans-serif;
  -webkit-font-smoothing: antialiased;
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  pointer-events: auto;
  user-select: none;
  box-shadow: 0 8px 32px rgba(0,0,0,0.55), inset 0 1px 0 rgba(255,255,255,0.04);
}

.cso-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 8px;
  opacity: 0.55;
  transition: opacity 0.15s ease;
}
#carve-sub-overlay:hover .cso-bar { opacity: 1; }

.cso-toggles {
  display: flex;
  gap: 0.4rem;
}

.cso-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 26px;
  background: rgba(255,255,255,0.06);
  border: 1px solid rgba(255,255,255,0.10);
  color: #aeb9cf;
  border-radius: 7px;
  padding: 0 0.55rem;
  font-size: 0.72rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.12s, color 0.12s, border-color 0.12s;
  line-height: 1;
}
.cso-btn:hover { background: rgba(255,255,255,0.14); color: #fff; }
.cso-btn:disabled { opacity: 0.25; cursor: default; }
.cso-btn.cso-active { background: rgba(76,175,80,0.22); border-color: rgba(76,175,80,0.7); color: #6ddf72; }

.cso-target {
  font-size: 1.7rem;
  line-height: 1.5;
  color: #f1f3f8;
  text-align: center;
  min-height: 2rem;
  letter-spacing: 0.01em;
  font-weight: 500;
  overflow-wrap: break-word;
  word-break: normal;
  text-shadow: 0 1px 3px rgba(0,0,0,0.4);
}

.cso-hint { color: #5a6478; font-size: 0.95rem; font-weight: 400; }

.cso-token {
  cursor: default;
  border-radius: 3px;
  transition: background 0.1s;
}
.cso-token.cso-unknown { color: #ff9b9b; cursor: pointer; }
.cso-token.cso-learning { color: #ffc266; cursor: pointer; }
.cso-token.cso-known { color: #f1f3f8; cursor: pointer; }
.cso-token.cso-unknown:hover,
.cso-token.cso-learning:hover,
.cso-token.cso-known:hover { background: rgba(255,255,255,0.12); }

.cso-native {
  font-size: 1rem;
  color: #93a0bb;
  text-align: center;
  margin-top: 6px;
  line-height: 1.4;
}

.cso-mine-status {
  font-size: 0.78rem;
  text-align: center;
  min-height: 1rem;
  margin-top: 6px;
  transition: color 0.2s;
  font-weight: 500;
}
.cso-mine-ok { color: #6ddf72; }
.cso-mine-error { color: #ff6b6b; }
`;
