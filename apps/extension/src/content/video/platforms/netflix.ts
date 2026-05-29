/// <reference types="chrome" />
import type { VocabCache } from '../../../nlp/VocabCache';
import type { PopupManager } from '../../popup/PopupManager';
import type { Token } from '../../../shared/types';

export class NetflixHook {
  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
  ) {}

  mount(): void {
    const observer = new MutationObserver(() => {
      const container = document.querySelector('.player-timedtext');
      if (container) {
        this.processSubtitles(container as Element);
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  private async processSubtitles(container: Element): Promise<void> {
    const text = container.textContent?.trim() ?? '';
    if (!text) return;
    // Only re-process if the text changed (reset data-carve-text cache)
    if (container.getAttribute('data-carve-text') === text) return;
    container.setAttribute('data-carve-text', text);

    const response = await chrome.runtime.sendMessage({
      type: 'TOKENIZE',
      text,
      language: this.lang,
      knownLemmas: this.vocabCache.getKnownLemmas(),
      learningLemmas: this.vocabCache.getLearningLemmas(),
    });

    if (!response?.tokens) return;

    container.setAttribute('data-carve', 'done');
    container.innerHTML = '';

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
      if (tok.frequency_rank !== null) {
        span.setAttribute('data-rank', String(tok.frequency_rank));
      }
      span.textContent = tok.surface;

      if (tok.is_content_word) {
        span.addEventListener('mouseover', () => {
          this.popupManager.showForElement(span);
        });
      }

      container.appendChild(span);
    }
  }
}
