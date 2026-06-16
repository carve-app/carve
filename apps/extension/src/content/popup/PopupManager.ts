import { browser } from '../../shared/browser';
import type { VocabCache } from '../../nlp/VocabCache';
import type { DictEntry, FuriganaSpan } from '../../shared/types';

export interface PopupHoverCallbacks {
  onEnter?: () => void;
  onLeave?: () => void;
}

export class PopupManager {
  private popup: HTMLElement | null = null;
  private currentToken: HTMLElement | null = null;
  private hideTimer: number | null = null;
  private hoverCallbacks: PopupHoverCallbacks | null = null;

  constructor(
    private language: string,
    private vocabCache: VocabCache,
  ) {
    this.setupListeners();
  }

  setInteractiveHoverCallbacks(callbacks: PopupHoverCallbacks | null): void {
    this.hoverCallbacks = callbacks;
  }

  private setupListeners(): void {
    document.addEventListener('click', (e) => {
      const target = e.target as HTMLElement;
      if (
        target.getAttribute('data-carve') === 'token' &&
        target.getAttribute('data-content') === '1'
      ) {
        e.stopPropagation();
        this.showPopupForToken(target);
      } else if (!target.closest('#carve-popup')) {
        this.hidePopup();
      }
    });

    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') this.hidePopup();
    });
  }

  private async showPopupForToken(tokenEl: HTMLElement): Promise<void> {
    this.cancelScheduledHide();
    if (this.currentToken === tokenEl) return;
    this.currentToken = tokenEl;

    const lemma = tokenEl.getAttribute('data-lemma')!;
    const surface = tokenEl.textContent!;
    const readingHira = tokenEl.getAttribute('data-reading') ?? '';
    const status = (tokenEl.getAttribute('data-status') ?? 'unknown') as 'known' | 'learning' | 'unknown';

    // Show loading state immediately
    this.showLoadingPopup(tokenEl, surface, readingHira, status);

    const response = await browser.runtime.sendMessage({
      type: 'LOOKUP',
      surface: lemma,
      language: this.language,
    });

    if (this.currentToken !== tokenEl) return;

    const entry: DictEntry | null = response?.entry ?? null;
    this.showFullPopup(tokenEl, surface, lemma, readingHira, status, entry);
  }

  private showLoadingPopup(
    tokenEl: HTMLElement,
    surface: string,
    reading: string,
    _status: string,
  ): void {
    const popup = this.getOrCreatePopup();
    const readingHtml = shouldShowReading(this.language, reading)
      ? `<div class="carve-reading">${escapeHtml(reading)}</div>`
      : '';
    popup.innerHTML = `
      <div class="carve-word">${escapeHtml(surface)}</div>
      ${readingHtml}
      <div class="carve-defs"><span style="color:#6b7a99">Loading…</span></div>
    `;
    this.positionPopup(tokenEl);
  }

  private showFullPopup(
    tokenEl: HTMLElement,
    surface: string,
    lemma: string,
    reading: string,
    status: 'known' | 'learning' | 'unknown',
    entry: DictEntry | null,
  ): void {
    const popup = this.getOrCreatePopup();
    const isJapanese = this.language === 'ja';

    const furiganaHtml =
      isJapanese && entry?.furigana?.length
        ? buildFurigana(entry.furigana)
        : `<span>${escapeHtml(surface)}</span>`;

    const defsHtml =
      entry?.definitions
        ?.slice(0, 3)
        .map(
          (d) => `
        <div class="carve-def">
          <span class="carve-pos">${escapeHtml(d.pos)}</span>
          ${escapeHtml(d.definition)}
        </div>
      `,
        )
        .join('') ?? '<div class="carve-def" style="color:#6b7a99">Not found in dictionary</div>';

    const jlptHtml = entry?.jlpt_level
      ? `<span class="carve-jlpt">${escapeHtml(entry.jlpt_level)}</span>`
      : '';
    const freqHtml = frequencyBandHtml(entry?.frequency_rank ?? null);
    const morae = (entry?.reading ?? reading).length || 1;
    const pitchHtml = isJapanese && entry?.pitch_accent != null
      ? pitchSvg(entry.pitch_accent, morae)
      : '';

    const sentence = getSurroundingSentence(tokenEl);
    const audioReading = entry?.reading ?? reading;
    const readingHtml = popupMetaHtml(this.language, audioReading, jlptHtml, freqHtml, pitchHtml);

    popup.innerHTML = `
      <div>
        <div class="carve-furigana">${furiganaHtml}</div>
        ${readingHtml}
        <span class="carve-status ${escapeHtml(status)}">${escapeHtml(status)}</span>
        <img class="carve-word-image" alt="" style="display:none;max-width:120px;max-height:120px;border-radius:8px;margin-top:8px" />
        <div class="carve-section-label">Meaning</div>
        <div class="carve-defs">${defsHtml}</div>
        <div class="carve-ai" style="display:none;margin-top:8px;padding-top:6px;border-top:1px solid #2d3344">
          <div class="carve-section-label">In this sentence</div>
          <div class="carve-ai-body" style="font-size:12px;color:#b8c2d8;line-height:1.45"></div>
        </div>
        ${sentence ? `<div class="carve-sentence">${escapeHtml(sentence)}</div>` : ''}
        <div class="carve-actions">
          <button class="btn-mine" data-lemma="${escapeHtml(lemma)}" data-sentence="${escapeHtml(sentence ?? '')}">Mine</button>
          <button class="btn-known" data-lemma="${escapeHtml(lemma)}">I know this</button>
          <button class="btn-ignore" data-lemma="${escapeHtml(lemma)}">Ignore</button>
        </div>
      </div>
    `;

    // Lazy-load word audio. Show the ▶ button only once a URL resolves; clicking
    // plays it via the Audio API. Best-effort — silently stays hidden on failure.
    this.loadWordAudio(popup, tokenEl, lemma, audioReading);

    // Lazy-load the AI contextual explanation. The section stays hidden unless
    // the server returns text (e.g. when an API key is configured).
    this.loadExplanation(popup, tokenEl, lemma, sentence);

    // Lazily fetch a best-effort dictionary image. Only show the slot if a URL
    // comes back AND this popup is still showing the same token. The src is
    // assigned via the element property (never via innerHTML interpolation) so
    // a hostile URL can't break out of an attribute.
    void this.loadWordImage(tokenEl, lemma, popup);

    popup.querySelector('.btn-mine')?.addEventListener('click', (e) => {
      // The button is removed from the DOM when innerHTML is replaced below.
      // Stop the bubbled click before it reaches the document-level "click
      // outside #carve-popup" handler, which would otherwise see the orphaned
      // node, conclude the click was outside, and hide the popup.
      e.stopPropagation();
      const candidates = getCandidateSentences(tokenEl, tokenEl.textContent ?? '');
      this.showMineForm(popup, tokenEl, lemma, entry?.reading ?? reading, entry, sentence, candidates);
    });

    popup.querySelector('.btn-known')?.addEventListener('click', async () => {
      await browser.runtime.sendMessage({
        type: 'MARK_KNOWN_WORD',
        lemma,
        languageCode: this.language,
      });
      await this.vocabCache.markKnown(lemma);
      tokenEl.setAttribute('data-status', 'known');
      this.hidePopup();
    });

    popup.querySelector('.btn-ignore')?.addEventListener('click', async () => {
      await browser.runtime.sendMessage({
        type: 'IGNORE_WORD',
        lemma,
        languageCode: this.language,
      });
      await this.vocabCache.markIgnored(lemma);
      tokenEl.setAttribute('data-status', 'known');
      this.hidePopup();
    });

    this.positionPopup(tokenEl);
  }

  /**
   * Resolve a word-audio URL in the background and, if found, reveal the play
   * button. Guards against the popup having moved to a different token while
   * the request was in flight. Best-effort — failures leave the button hidden.
   */
  private loadWordAudio(
    popup: HTMLElement,
    tokenEl: HTMLElement,
    lemma: string,
    reading: string,
  ): void {
    if (!reading) return;
    browser.runtime.sendMessage({
      type: 'WORD_AUDIO',
      language: this.language,
      lemma,
      reading,
    })
      .then((res) => {
        const url = (res?.audioUrl as string | null) ?? null;
        if (!url || this.currentToken !== tokenEl) return;
        const btn = popup.querySelector<HTMLButtonElement>('.carve-audio-btn');
        if (!btn) return;
        const meta = btn.closest<HTMLElement>('.carve-reading');
        if (meta) meta.style.display = '';
        btn.style.display = 'inline-block';
        let audio: HTMLAudioElement | null = null;
        btn.addEventListener('click', (e) => {
          e.stopPropagation();
          if (!audio) audio = new Audio(url);
          audio.currentTime = 0;
          audio.play().catch(() => {/* autoplay/network — non-fatal */});
        });
      })
      .catch(() => {/* audio is optional */});
  }

  /**
   * Lazy-load an AI contextual explanation. If the server returns null (e.g. no
   * API key) the section stays hidden.
   */
  private loadExplanation(
    popup: HTMLElement,
    tokenEl: HTMLElement,
    lemma: string,
    sentence: string | null,
  ): void {
    if (!sentence) return;
    const section = popup.querySelector<HTMLElement>('.carve-ai');
    const body = popup.querySelector<HTMLElement>('.carve-ai-body');
    if (!section || !body) return;

    browser.runtime.sendMessage({
      type: 'EXPLAIN_WORD',
      word: lemma,
      sentence,
      language: this.language,
    })
      .then((res) => {
        if (this.currentToken !== tokenEl) return;
        const explanation = (res?.explanation as string | null) ?? null;
        if (!explanation) {
          section.style.display = 'none';
          return;
        }
        // textContent escapes by construction — never inject unescaped strings.
        body.textContent = explanation;
        body.style.fontStyle = 'normal';
        body.style.color = '#b8c2d8';
        section.style.display = 'block';
      })
      .catch(() => {
        section.style.display = 'none';
      });
  }

  /**
   * Fetch a best-effort dictionary image for `lemma` and, if one comes back,
   * reveal the popup's image slot. No-op (slot stays hidden) on null/error or
   * if the user has since moved to a different token.
   */
  private async loadWordImage(tokenEl: HTMLElement, lemma: string, popup: HTMLElement): Promise<void> {
    try {
      const result = await browser.runtime.sendMessage({
        type: 'WORD_IMAGE',
        word: lemma,
        language: this.language,
      });
      const url: string | null = result?.imageUrl ?? null;
      // Bail if the popup moved on to another token while we were fetching.
      if (this.currentToken !== tokenEl) return;
      if (!url) return;
      const img = popup.querySelector<HTMLImageElement>('.carve-word-image');
      if (!img) return;
      // Assign the URL via the property — not string-interpolated into HTML.
      img.src = url;
      img.style.display = 'block';
      // The image changes the popup height; re-anchor so it stays in view.
      img.addEventListener('load', () => {
        if (this.currentToken === tokenEl) this.positionPopup(tokenEl);
      }, { once: true });
    } catch {
      // Image is purely optional — ignore failures.
    }
  }

  private getOrCreatePopup(): HTMLElement {
    if (!this.popup) {
      this.popup = document.createElement('div');
      this.popup.id = 'carve-popup';
      this.popup.setAttribute('data-carve', 'ui');
      this.popup.addEventListener('mouseenter', () => {
        this.cancelScheduledHide();
        this.hoverCallbacks?.onEnter?.();
      });
      this.popup.addEventListener('mouseleave', () => {
        this.hoverCallbacks?.onLeave?.();
        this.scheduleHidePopup();
      });
      document.body.appendChild(this.popup);
    }
    return this.popup;
  }

  private positionPopup(tokenEl: HTMLElement): void {
    const popup = this.getOrCreatePopup();
    popup.style.display = 'block';
    // Force layout then read the rendered size.
    const measuredH = popup.getBoundingClientRect().height || 200;
    const rect = tokenEl.getBoundingClientRect();
    const viewportH = window.innerHeight;
    const margin = 8;

    // Choose anchor side, then clamp inside the viewport so a tall mine form
    // is never positioned below the viewport (the Save button has to stay
    // reachable without scrolling).
    let top: number;
    if (rect.top - margin >= measuredH) {
      top = rect.top - measuredH - margin;
    } else {
      top = rect.bottom + margin;
    }
    const maxTop = Math.max(margin, viewportH - measuredH - margin);
    top = Math.max(margin, Math.min(top, maxTop));

    let left = rect.left;
    const maxLeft = window.innerWidth - 360;
    left = Math.min(Math.max(left, 8), maxLeft);

    popup.style.top = `${top}px`;
    popup.style.left = `${left}px`;
  }

  private showMineForm(
    popup: HTMLElement,
    tokenEl: HTMLElement,
    lemma: string,
    reading: string,
    entry: DictEntry | null,
    sentence: string | null,
    candidates: string[] = [],
  ): void {
    const topDef = entry?.definitions?.[0]?.definition ?? '';
    const escapedLemma = escapeHtml(lemma);
    const escapedReading = escapeHtml(reading);
    const escapedDef = escapeHtml(topDef);
    const escapedSentence = escapeHtml(sentence ?? '');

    // Kick off translation in background — will fill in async
    if (sentence) {
      browser.runtime.sendMessage({ type: 'TRANSLATE', text: sentence, sourceLanguage: this.language })
        .then((result) => {
          const t = result?.translation as string | null;
          const el = popup.querySelector<HTMLInputElement>('#mine-translation');
          if (el && t) el.value = t;
        })
        .catch(() => {/* optional field — ignore */});
    }

    popup.innerHTML = `
      <div class="carve-mine-form">
        <div class="carve-mine-title">Add card</div>
        <div class="carve-mine-similar" id="mine-similar" style="display:none"></div>
        <label>Word</label>
        <input class="carve-mine-input" id="mine-lemma" value="${escapedLemma}" />
        <label>Reading</label>
        <input class="carve-mine-input" id="mine-reading" value="${escapedReading}" />
        <label>Definition</label>
        <input class="carve-mine-input" id="mine-def" value="${escapedDef}" />
        <label>Sentence <span class="carve-mine-badge" id="mine-sentence-badge" style="display:none"></span></label>
        <textarea class="carve-mine-input" id="mine-sentence" rows="2">${escapedSentence}</textarea>
        <label>Translation <span style="color:#4a5568">(auto-filling…)</span></label>
        <input class="carve-mine-input" id="mine-translation" placeholder="Fetching translation…" />
        <label>Notes <span style="color:#4a5568">(optional)</span></label>
        <input class="carve-mine-input" id="mine-notes" placeholder="Add a note…" />
        <div class="carve-mine-actions">
          <button class="btn-mine-save">Save card</button>
          <button class="btn-mine-cancel">Cancel</button>
        </div>
        <div class="carve-mine-status" id="mine-status"></div>
      </div>
    `;

    // Re-position now that the form content is much taller than the lookup
    // popup — otherwise the Save button can land below the viewport.
    this.positionPopup(tokenEl);

    // Check for near-duplicate cards. Re-runs whenever the sentence textarea
    // changes (e.g. after the picker swaps it in, or after user edits).
    const refreshSimilar = (sourceSentence: string) => {
      if (!sourceSentence || sourceSentence.length < 4) return;
      browser.runtime.sendMessage({
        type: 'FIND_SIMILAR_CARDS',
        languageCode: this.language,
        sentence: sourceSentence,
      })
        .then((res) => {
          const matches = (res?.matches ?? []) as Array<{ id: string; front_text: string; similarity: number }>;
          const el = popup.querySelector<HTMLElement>('#mine-similar');
          if (!el) return;
          if (matches.length === 0) {
            el.style.display = 'none';
            el.innerHTML = '';
            return;
          }
          const items = matches
            .map((m) => `<li>${escapeHtml(m.front_text)} <span class="carve-mine-sim">${Math.round(m.similarity * 100)}%</span></li>`)
            .join('');
          el.innerHTML = `<div class="carve-mine-warn">⚠ ${matches.length} similar card${matches.length === 1 ? '' : 's'} exist</div><ul>${items}</ul>`;
          el.style.display = 'block';
        })
        .catch(() => {/* non-fatal */});
    };
    if (sentence) refreshSimilar(sentence);
    popup.querySelector<HTMLTextAreaElement>('#mine-sentence')?.addEventListener('blur', (e) => {
      refreshSimilar((e.target as HTMLTextAreaElement).value.trim());
    });

    // Kick off i+1 sentence selection. If a materially better candidate exists,
    // swap it into the textarea and surface a small "picked better example"
    // badge with a one-click "use original" toggle.
    if (sentence && candidates.length > 1) {
      const all = candidates.includes(sentence) ? candidates : [sentence, ...candidates];
      browser.runtime.sendMessage({
        type: 'SELECT_SENTENCE',
        candidates: all,
        targetLemma: lemma,
        language: this.language,
        knownLemmas: this.vocabCache.getKnownLemmas(),
        learningLemmas: this.vocabCache.getLearningLemmas(),
      })
        .then((result) => {
          const best: string | null = result?.bestText ?? null;
          const pct: number | null = result?.bestComprehensionPct ?? null;
          const containsTarget: boolean = result?.bestContainsTarget ?? false;
          if (!best || best === sentence || !containsTarget) return;
          const ta = popup.querySelector<HTMLTextAreaElement>('#mine-sentence');
          const badge = popup.querySelector<HTMLElement>('#mine-sentence-badge');
          if (!ta || !badge) return;
          ta.value = best;
          const pctLabel = pct != null ? ` (${Math.round(pct)}%)` : '';
          badge.textContent = `✨ better example picked${pctLabel} · use original`;
          badge.style.display = 'inline';
          badge.style.cursor = 'pointer';
          badge.style.color = '#7ab8ff';
          badge.style.fontSize = '11px';
          badge.addEventListener('click', () => {
            ta.value = sentence;
            badge.style.display = 'none';
          }, { once: true });
        })
        .catch(() => {/* selector is best-effort */});
    }

    popup.querySelector('.btn-mine-cancel')?.addEventListener('click', () => {
      this.hidePopup();
    });

    popup.querySelector('.btn-mine-save')?.addEventListener('click', async () => {
      const btn = popup.querySelector<HTMLButtonElement>('.btn-mine-save')!;
      const statusEl = popup.querySelector<HTMLElement>('#mine-status')!;

      const mineLemma = (popup.querySelector<HTMLInputElement>('#mine-lemma')?.value ?? lemma).trim();
      const mineReading = (popup.querySelector<HTMLInputElement>('#mine-reading')?.value ?? reading).trim();
      const mineDef = (popup.querySelector<HTMLInputElement>('#mine-def')?.value ?? topDef).trim();
      const mineSentence = (popup.querySelector<HTMLTextAreaElement>('#mine-sentence')?.value ?? sentence ?? '').trim();
      const mineTranslation = (popup.querySelector<HTMLInputElement>('#mine-translation')?.value ?? '').trim();

      btn.disabled = true;
      btn.textContent = 'Saving…';

      const result = await browser.runtime.sendMessage({
        type: 'MINE_CARD',
        lemma: mineLemma,
        reading: mineReading,
        definition: mineDef,
        translation: mineTranslation,
        sentence: mineSentence,
        sourceUrl: window.location.href,
        languageCode: this.language,
      });

      const isVideoPage = /netflix\.com\/watch|youtube\.com\/watch|youtu\.be\//.test(window.location.href);

      if (result?.cardId) {
        await this.vocabCache.markLearning(mineLemma);
        tokenEl.setAttribute('data-status', 'learning');
        statusEl.textContent = '✓ Saved';
        statusEl.style.color = '#4caf50';

        if (!isVideoPage) {
          browser.runtime.sendMessage({
            type: 'ATTACH_PAGE_SCREENSHOT',
            cardId: result.cardId,
          }).catch(() => {/* non-fatal */});
        }

        setTimeout(() => this.hidePopup(), 900);
      } else {
        statusEl.textContent = result?.error ?? 'Failed to save';
        statusEl.style.color = '#ef5350';
        btn.disabled = false;
        btn.textContent = 'Save card';
      }
    });
  }

  hidePopup(): void {
    this.cancelScheduledHide();
    if (this.popup) {
      this.popup.style.display = 'none';
    }
    this.currentToken = null;
  }

  scheduleHidePopup(delayMs = 180): void {
    this.cancelScheduledHide();
    this.hideTimer = window.setTimeout(() => this.hidePopup(), delayMs);
  }

  cancelScheduledHide(): void {
    if (this.hideTimer == null) return;
    window.clearTimeout(this.hideTimer);
    this.hideTimer = null;
  }

  /**
   * Called by subtitle hooks to show the popup for a given token element.
   */
  async showForElement(tokenEl: HTMLElement): Promise<void> {
    await this.showPopupForToken(tokenEl);
  }
}

export function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function shouldShowReading(language: string, reading: string): boolean {
  return Boolean(reading) && !isOrthographicReadingLanguage(language);
}

function isOrthographicReadingLanguage(language: string): boolean {
  return language === 'en' || language === 'de' || language === 'es' || language === 'fr' ||
    language === 'it' || language === 'pt' || language === 'vi';
}

function audioButtonHtml(): string {
  return '<button class="carve-audio-btn" title="Play audio" aria-label="Play word audio" style="display:none;flex:0 0 auto;width:22px;height:22px;padding:0;margin-left:6px;border:none;border-radius:50%;background:#37404e;color:#cdd6e8;font-size:11px;line-height:22px;cursor:pointer;vertical-align:middle">▶</button>';
}

function popupMetaHtml(
  language: string,
  reading: string,
  jlptHtml: string,
  freqHtml: string,
  pitchHtml: string,
): string {
  const button = audioButtonHtml();
  if (shouldShowReading(language, reading)) {
    return `<div class="carve-reading">${escapeHtml(reading)}${jlptHtml}${freqHtml}${pitchHtml}${button}</div>`;
  }

  const metadata = `${jlptHtml}${freqHtml}${pitchHtml}`;
  const hidden = metadata ? '' : ' style="display:none"';
  return `<div class="carve-reading carve-reading-meta"${hidden}>${metadata}${button}</div>`;
}

/**
 * Migaku-style frequency band. Maps a frequency rank (lower = more common)
 * to a colored pill so learners can gauge a word's usefulness at a glance.
 *
 * Thresholds (rank is "Nth most frequent word"):
 *   ≤ 1500  → common (green)  — high-value, learn early
 *   ≤ 6000  → mid    (yellow) — worth knowing
 *   > 6000  → rare   (red)    — niche / advanced
 *   null    → unknown (gray)  — not in the frequency list
 *
 * The raw rank is kept inside the pill for users who want the exact number.
 */
const FREQ_COMMON_MAX = 1500;
const FREQ_MID_MAX = 6000;

export function frequencyBandHtml(rank: number | null): string {
  let label: string;
  let color: string;
  let text: string;

  if (rank == null) {
    return '';
  } else if (rank <= FREQ_COMMON_MAX) {
    label = 'common';
    color = '#4caf50';
    text = `common #${rank}`;
  } else if (rank <= FREQ_MID_MAX) {
    label = 'mid';
    color = '#ffa726';
    text = `mid #${rank}`;
  } else {
    label = 'rare';
    color = '#ef5350';
    text = `rare #${rank}`;
  }

  return `<span class="carve-freq-band carve-freq-${label}" title="Frequency rank ${rank}"
    style="display:inline-block;margin-left:6px;padding:1px 6px;border-radius:8px;font-size:11px;font-weight:600;color:#fff;background:${color}">${escapeHtml(text)}</span>`;
}

export function buildFurigana(spans: FuriganaSpan[]): string {
  return spans
    .map((s) => {
      if (!s.reading) return escapeHtml(s.text);
      return `<ruby>${escapeHtml(s.text)}<rt>${escapeHtml(s.reading)}</rt></ruby>`;
    })
    .join('');
}

export function pitchLabel(accent: string): string {
  const n = parseInt(accent, 10);
  if (isNaN(n)) return `[${accent}]`;
  if (n === 0) return `[${accent}⓪]`;
  if (n === 1) return `[${accent}①]`;
  return `[${accent}]`;
}

/**
 * Renders an inline SVG pitch contour over the mora of a word.
 *
 * Pitch accent encoding (NHK):
 *   0 = heiban   (L H H H…)  — rises on mora 2, stays high, no drop
 *   1 = atamadaka (H L L L…) — high only on mora 1, drops to low
 *   N = nakadaka/odaka        — rises on mora 2, drops AFTER mora N
 *
 * The contour is drawn as a polyline: each mora gets a dot; the line
 * connects them at high (y=4) or low (y=16) levels.
 */
export function pitchSvg(accent: string, morae: number): string {
  const n = parseInt(accent, 10);
  if (isNaN(n) || morae < 1) return '';

  const W = 10; // px per mora
  const H = 22; // total SVG height
  const hi = 3;
  const lo = 16;

  // Build per-mora pitch level: true = high, false = low
  const levels: boolean[] = [];
  for (let i = 0; i < morae; i++) {
    if (n === 0) {
      // heiban: mora 1 is low, all others are high
      levels.push(i > 0);
    } else if (n === 1) {
      // atamadaka: mora 1 is high, rest are low
      levels.push(i === 0);
    } else {
      // nakadaka / odaka: mora 1 low, mora 2…N high, rest low
      levels.push(i > 0 && i < n);
    }
  }

  const totalW = W * morae + 6;
  const points = levels
    .map((high, i) => `${i * W + 3},${high ? hi : lo}`)
    .join(' ');

  // Color by accent type for quick visual recognition
  const colors: Record<number, string> = { 0: '#4caf50', 1: '#e57373' };
  const color = n < 2 ? (colors[n] ?? '#64b5f6') : '#64b5f6';

  // Use the validated integer `n` — never the raw `accent` string — so this
  // SVG (assigned via innerHTML) can't be an injection sink if pitch_accent
  // ever carries untrusted data.
  return `<svg class="carve-pitch-svg" width="${totalW}" height="${H}"
    viewBox="0 0 ${totalW} ${H}" aria-label="Pitch accent ${n}"
    style="vertical-align:middle;margin-left:4px">
    <polyline points="${points}" fill="none" stroke="${color}" stroke-width="1.5" stroke-linejoin="round"/>
    ${levels.map((high, i) =>
      `<circle cx="${i * W + 3}" cy="${high ? hi : lo}" r="2" fill="${color}"/>`
    ).join('')}
  </svg>`;
}

/** Returns furigana HTML respecting the current furigana mode setting. */
export type FuriganaMode = 'always' | 'unknown-only' | 'kanji-only' | 'off';

export function buildFuriganaWithMode(
  spans: Array<{ text: string; reading?: string }>,
  mode: FuriganaMode,
  wordStatus: 'unknown' | 'learning' | 'known',
): string {
  if (mode === 'off') return spans.map(s => escapeHtml(s.text)).join('');

  return spans
    .map((s) => {
      if (!s.reading) return escapeHtml(s.text);
      const isKanji = /[一-龯]/.test(s.text);
      const showRuby =
        mode === 'always' ||
        (mode === 'unknown-only' && wordStatus === 'unknown') ||
        (mode === 'kanji-only' && isKanji);
      if (!showRuby) return escapeHtml(s.text);
      return `<ruby>${escapeHtml(s.text)}<rt>${escapeHtml(s.reading)}</rt></ruby>`;
    })
    .join('');
}

export function getSurroundingSentence(el: HTMLElement): string | null {
  const parent = el.closest('p, li, td, div, span[data-carve="processed"]');
  if (!parent) return null;
  const text = parent.textContent?.trim() ?? '';
  if (text.length > 200) {
    const elText = el.textContent ?? '';
    const idx = text.indexOf(elText);
    if (idx >= 0) {
      const start = Math.max(0, idx - 50);
      const end = Math.min(text.length, idx + elText.length + 50);
      return (start > 0 ? '…' : '') + text.slice(start, end) + (end < text.length ? '…' : '');
    }
  }
  return text.slice(0, 200) || null;
}

/**
 * Return up to `max` candidate sentences from the paragraph containing `el`,
 * preferring ones that contain `targetSurface`. Used to give the i+1 selector
 * something to choose from instead of just the clicked sentence.
 */
export function getCandidateSentences(
  el: HTMLElement,
  targetSurface: string,
  max = 5,
): string[] {
  const parent = el.closest('p, li, td, div, blockquote, section, article');
  if (!parent) return [];
  const text = (parent.textContent ?? '').replace(/\s+/g, ' ').trim();
  if (!text) return [];

  // Split on terminal punctuation, keeping the terminator with the sentence.
  // CJK terminators (。！？) don't require trailing whitespace; ASCII (.!?) do
  // because otherwise we'd split inside e.g. "U.S.A.".
  const pieces = text
    .split(/(?<=[。！？])|(?<=[.!?])\s+/u)
    .map((s) => s.trim())
    .filter((s) => s.length >= 2 && s.length <= 240);

  const seen = new Set<string>();
  const ordered: string[] = [];
  for (const p of pieces) {
    if (seen.has(p)) continue;
    seen.add(p);
    ordered.push(p);
  }

  // Sort: sentences containing the target surface first, then by position.
  const withIdx = ordered.map((s, i) => ({ s, i, hit: targetSurface ? s.includes(targetSurface) : false }));
  withIdx.sort((a, b) => (Number(b.hit) - Number(a.hit)) || (a.i - b.i));
  return withIdx.slice(0, max).map((x) => x.s);
}
