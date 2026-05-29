/// <reference types="chrome" />
import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import type { Token } from '../../shared/types';

const SKIP_TAGS = new Set([
  'SCRIPT', 'STYLE', 'NOSCRIPT', 'CODE', 'PRE',
  'TEXTAREA', 'INPUT', 'BUTTON', 'SELECT', 'OPTION',
  'IFRAME', 'OBJECT', 'EMBED',
]);
const MAX_BATCH_CHARS = 3000;

// Separator: word-joiner character unlikely to appear in Japanese text
const SEPARATOR = '\n⁠\n';

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
    if (textNodes.length === 0) return;

    let batch: Array<{ node: Text; text: string }> = [];
    let batchChars = 0;

    for (const node of textNodes) {
      const text = node.textContent!;
      batch.push({ node, text });
      batchChars += text.length;
      if (batchChars >= MAX_BATCH_CHARS) {
        await this.processBatch(batch);
        batch = [];
        batchChars = 0;
      }
    }
    if (batch.length > 0) {
      await this.processBatch(batch);
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

  private async processBatch(batch: Array<{ node: Text; text: string }>): Promise<void> {
    const combinedText = batch.map((b) => b.text).join(SEPARATOR);

    const response = await chrome.runtime.sendMessage({
      type: 'TOKENIZE',
      text: combinedText,
      language: this.lang,
      knownLemmas: this.vocabCache.getKnownLemmas(),
      learningLemmas: this.vocabCache.getLearningLemmas(),
    });

    if (!response?.tokens) return;

    const allTokens: Token[] = response.tokens;
    let tokenIdx = 0;

    for (const { node, text } of batch) {
      if (!node.parentNode) continue;

      // Collect tokens for this segment (stop at separator token)
      const segTokens: Token[] = [];
      let chars = 0;
      while (tokenIdx < allTokens.length && chars < text.length) {
        const tok = allTokens[tokenIdx];
        // Separator token — skip it and move to next segment
        if (tok.surface.includes('⁠')) {
          tokenIdx++;
          break;
        }
        segTokens.push(tok);
        chars += tok.surface.length;
        tokenIdx++;
      }

      if (segTokens.length === 0) continue;

      const fragment = document.createDocumentFragment();
      for (const tok of segTokens) {
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

        span.textContent = tok.surface;
        fragment.appendChild(span);
      }

      const parent = node.parentElement!;
      parent.setAttribute('data-carve', 'processed');
      node.parentNode.replaceChild(fragment, node);
    }
  }
}
