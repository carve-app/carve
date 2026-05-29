/// <reference types="chrome" />
import type { VocabCache } from '../../../nlp/VocabCache';
import type { PopupManager } from '../../popup/PopupManager';
import type { Token } from '../../../shared/types';

export class YouTubeHook {
  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
  ) {}

  mount(): void {
    const observer = new MutationObserver((mutations) => {
      for (const m of mutations) {
        for (const node of Array.from(m.addedNodes)) {
          if (node.nodeType !== Node.ELEMENT_NODE) continue;
          const el = node as Element;
          // Process new caption segments
          const segments = el.classList.contains('ytp-caption-segment')
            ? [el]
            : Array.from(el.querySelectorAll('.ytp-caption-segment'));
          for (const seg of segments) {
            this.processSegment(seg);
          }
        }
      }
    });

    observer.observe(document.body, { childList: true, subtree: true });

    // Process any already-present segments
    document.querySelectorAll('.ytp-caption-segment').forEach((seg) => {
      this.processSegment(seg);
    });
  }

  private async processSegment(segment: Element): Promise<void> {
    const text = segment.textContent?.trim() ?? '';
    if (!text) return;
    if (segment.getAttribute('data-carve-text') === text) return;
    segment.setAttribute('data-carve-text', text);

    const response = await chrome.runtime.sendMessage({
      type: 'TOKENIZE',
      text,
      language: this.lang,
      knownLemmas: this.vocabCache.getKnownLemmas(),
      learningLemmas: this.vocabCache.getLearningLemmas(),
    });

    if (!response?.tokens) return;

    segment.setAttribute('data-carve', 'done');
    segment.innerHTML = '';

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

      segment.appendChild(span);
    }
  }
}
