/// <reference types="chrome" />
import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import type { Token } from '../../shared/types';

const SKIP_TAGS = new Set([
  'SCRIPT', 'STYLE', 'NOSCRIPT', 'CODE', 'PRE',
  'TEXTAREA', 'INPUT', 'BUTTON', 'SELECT', 'OPTION',
  'IFRAME', 'OBJECT', 'EMBED',
  'RT', 'RP', 'RB',
]);

export class PageAnnotator {
  private observer: MutationObserver;

  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
  ) {
    this.observer = new MutationObserver((mutations) => {
      for (const m of mutations) {
        for (const node of Array.from(m.addedNodes)) {
          if (node.nodeType === Node.ELEMENT_NODE) {
            this.scheduleAnnotation(node as Element);
          }
        }
      }
    });
  }

  start(): void {
    this.scheduleAnnotation(document.body);
    this.observer.observe(document.body, { childList: true, subtree: true });
  }

  private scheduleAnnotation(root: Element): void {
    requestIdleCallback(() => this.annotateElement(root), { timeout: 2000 });
  }

  private async annotateElement(root: Element): Promise<void> {
    const textNodes = this.collectTextNodes(root);
    for (const node of textNodes) {
      await this.processNode(node);
    }
  }

  private collectTextNodes(root: Element): Text[] {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: (node) => {
        const parent = node.parentElement;
        if (!parent) return NodeFilter.FILTER_REJECT;
        if (parent.closest('[data-carve]')) return NodeFilter.FILTER_REJECT;
        if (SKIP_TAGS.has(parent.tagName)) return NodeFilter.FILTER_REJECT;
        const text = node.textContent ?? '';
        if (!/[぀-ヿ一-鿿]/.test(text)) return NodeFilter.FILTER_REJECT;
        return NodeFilter.FILTER_ACCEPT;
      },
    });

    const nodes: Text[] = [];
    let node: Node | null;
    while ((node = walker.nextNode())) {
      nodes.push(node as Text);
    }
    return nodes;
  }

  // MV3 service workers can be terminated mid-request. Retry up to 3 times
  // on "message channel closed" — each retry wakes a fresh SW instance.
  private async tokenize(text: string): Promise<{ tokens: Token[] } | null> {
    const msg = {
      type: 'TOKENIZE' as const,
      text,
      language: this.lang,
      knownLemmas: this.vocabCache.getKnownLemmas(),
      learningLemmas: this.vocabCache.getLearningLemmas(),
    };
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        return await chrome.runtime.sendMessage(msg);
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : '';
        const isChannelClosed = msg.includes('message channel closed') || msg.includes('Extension context');
        if (!isChannelClosed || attempt === 2) return null;
        await new Promise(r => setTimeout(r, 100 * (attempt + 1)));
      }
    }
    return null;
  }

  private async processNode(node: Text): Promise<void> {
    if (!node.parentNode) return;
    const text = node.textContent!;

    const response = await this.tokenize(text);
    if (!response?.tokens?.length) return;
    if (!node.parentNode) return;  // detached during await

    const fragment = this.buildFragment(text, response.tokens);
    const parent = node.parentElement!;
    parent.setAttribute('data-carve', 'processed');
    node.parentNode.replaceChild(fragment, node);
  }

  // Reconstruct a fragment from tokens, preserving any characters not covered
  // by tokens (leading/trailing punctuation, whitespace) as plain text nodes.
  // This avoids dropping characters that the tokenizer skips (e.g. 。).
  private buildFragment(text: string, tokens: Token[]): DocumentFragment {
    const fragment = document.createDocumentFragment();
    let pos = 0;

    for (const tok of tokens) {
      const idx = text.indexOf(tok.surface, pos);
      if (idx === -1) continue;

      if (idx > pos) {
        fragment.appendChild(document.createTextNode(text.slice(pos, idx)));
      }

      const span = document.createElement('span');
      span.setAttribute('data-carve', 'token');
      span.setAttribute('data-lemma', tok.lemma);
      span.setAttribute('data-reading', tok.reading_hira);
      span.setAttribute('data-pos', tok.pos);
      span.setAttribute('data-content', tok.is_content_word ? '1' : '0');

      const status = this.vocabCache.getStatus(tok.lemma);
      span.setAttribute('data-status', tok.is_content_word ? status : 'function');

      if (tok.frequency_rank !== null) {
        span.setAttribute('data-rank', String(tok.frequency_rank));
      }
      if (tok.is_content_word) {
        const band = tok.frequency_rank == null ? 'red'
          : tok.frequency_rank <= 2000 ? 'green'
          : tok.frequency_rank <= 5000 ? 'yellow'
          : 'red';
        span.setAttribute('data-band', band);
      }

      span.textContent = tok.surface;
      fragment.appendChild(span);
      pos = idx + tok.surface.length;
    }

    if (pos < text.length) {
      fragment.appendChild(document.createTextNode(text.slice(pos)));
    }

    return fragment;
  }
}
