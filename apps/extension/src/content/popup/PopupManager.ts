import { browser } from '../../shared/browser';
import type { VocabCache } from '../../nlp/VocabCache';
import type { DictEntry, FuriganaSpan } from '../../shared/types';

export class PopupManager {
  private popup: HTMLElement | null = null;
  private currentToken: HTMLElement | null = null;

  constructor(private vocabCache: VocabCache) {
    this.setupListeners();
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
      language: 'ja',
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
    popup.innerHTML = `
      <div class="carve-word">${escapeHtml(surface)}</div>
      <div class="carve-reading">${escapeHtml(reading)}</div>
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

    const furiganaHtml =
      entry?.furigana?.length
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
    const freqHtml = entry?.frequency_rank
      ? `<span style="font-size:11px;color:#6b7a99"> #${entry.frequency_rank}</span>`
      : '';
    const morae = (entry?.reading ?? reading).length || 1;
    const pitchHtml = entry?.pitch_accent != null
      ? pitchSvg(entry.pitch_accent, morae)
      : '';

    const sentence = getSurroundingSentence(tokenEl);

    popup.innerHTML = `
      <div>
        <div class="carve-furigana">${furiganaHtml}</div>
        <div class="carve-reading">${escapeHtml(entry?.reading ?? reading)}${jlptHtml}${freqHtml}${pitchHtml}</div>
        <span class="carve-status ${escapeHtml(status)}">${escapeHtml(status)}</span>
        <div class="carve-defs">${defsHtml}</div>
        ${sentence ? `<div class="carve-sentence">${escapeHtml(sentence)}</div>` : ''}
        <div class="carve-actions">
          <button class="btn-mine" data-lemma="${escapeHtml(lemma)}" data-sentence="${escapeHtml(sentence ?? '')}">Mine</button>
          <button class="btn-ignore" data-lemma="${escapeHtml(lemma)}">Ignore</button>
        </div>
      </div>
    `;

    popup.querySelector('.btn-mine')?.addEventListener('click', () => {
      this.showMineForm(popup, tokenEl, lemma, entry?.reading ?? reading, entry, sentence);
    });

    popup.querySelector('.btn-ignore')?.addEventListener('click', async () => {
      await browser.runtime.sendMessage({
        type: 'IGNORE_WORD',
        lemma,
        languageCode: 'ja',
      });
      await this.vocabCache.markKnown(lemma);
      tokenEl.setAttribute('data-status', 'known');
      this.hidePopup();
    });

    this.positionPopup(tokenEl);
  }

  private getOrCreatePopup(): HTMLElement {
    if (!this.popup) {
      this.popup = document.createElement('div');
      this.popup.id = 'carve-popup';
      document.body.appendChild(this.popup);
    }
    return this.popup;
  }

  private positionPopup(tokenEl: HTMLElement): void {
    const popup = this.getOrCreatePopup();
    const rect = tokenEl.getBoundingClientRect();
    const popupH = 200;

    // popup is position:fixed, so getBoundingClientRect() coords are already in
    // viewport space — do not add window.scroll* offsets
    let top: number;
    if (rect.top > popupH + 8) {
      top = rect.top - popupH - 8;
    } else {
      top = rect.bottom + 8;
    }

    let left = rect.left;
    const maxLeft = window.innerWidth - 360;
    left = Math.min(Math.max(left, 8), maxLeft);

    popup.style.top = `${top}px`;
    popup.style.left = `${left}px`;
    popup.style.display = 'block';
  }

  private showMineForm(
    popup: HTMLElement,
    tokenEl: HTMLElement,
    lemma: string,
    reading: string,
    entry: DictEntry | null,
    sentence: string | null,
  ): void {
    const topDef = entry?.definitions?.[0]?.definition ?? '';
    const escapedLemma = escapeHtml(lemma);
    const escapedReading = escapeHtml(reading);
    const escapedDef = escapeHtml(topDef);
    const escapedSentence = escapeHtml(sentence ?? '');

    // Kick off translation in background — will fill in async
    if (sentence) {
      browser.runtime.sendMessage({ type: 'TRANSLATE', text: sentence, sourceLanguage: 'ja' })
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
        <label>Word</label>
        <input class="carve-mine-input" id="mine-lemma" value="${escapedLemma}" />
        <label>Reading</label>
        <input class="carve-mine-input" id="mine-reading" value="${escapedReading}" />
        <label>Definition</label>
        <input class="carve-mine-input" id="mine-def" value="${escapedDef}" />
        <label>Sentence</label>
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
        languageCode: 'ja',
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
    if (this.popup) {
      this.popup.style.display = 'none';
    }
    this.currentToken = null;
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

  return `<svg class="carve-pitch-svg" width="${totalW}" height="${H}"
    viewBox="0 0 ${totalW} ${H}" aria-label="Pitch accent ${accent}"
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
