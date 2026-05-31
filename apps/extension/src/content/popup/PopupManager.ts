/// <reference types="chrome" />
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

    const response = await chrome.runtime.sendMessage({
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
    const pitchHtml = entry?.pitch_accent != null
      ? `<span class="carve-pitch" title="NHK pitch accent">${escapeHtml(pitchLabel(entry.pitch_accent))}</span>`
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
      await chrome.runtime.sendMessage({
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
      chrome.runtime.sendMessage({ type: 'TRANSLATE', text: sentence, sourceLanguage: 'ja' })
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

      const result = await chrome.runtime.sendMessage({
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
          chrome.runtime.sendMessage({
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

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function buildFurigana(spans: FuriganaSpan[]): string {
  return spans
    .map((s) => {
      if (!s.reading) return escapeHtml(s.text);
      return `<ruby>${escapeHtml(s.text)}<rt>${escapeHtml(s.reading)}</rt></ruby>`;
    })
    .join('');
}

function pitchLabel(accent: string): string {
  const n = parseInt(accent, 10);
  if (isNaN(n)) return `[${accent}]`;
  if (n === 0) return `[${accent}⓪]`;   // heiban
  if (n === 1) return `[${accent}①]`;   // atamadaka
  return `[${accent}]`;                  // nakadaka / odaka
}

function getSurroundingSentence(el: HTMLElement): string | null {
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
