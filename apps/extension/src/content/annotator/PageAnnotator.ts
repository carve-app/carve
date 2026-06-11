import { browser } from '../../shared/browser';
import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import type { Token } from '../../shared/types';
import { wasmTokenize } from '../../nlp/WasmTokenizer';

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

  /**
   * Stop observing and unwrap every token span this annotator added, restoring
   * the original text. Idempotent — safe to call when never started. Used by
   * the "enable on this site" toggle to disable cleanly without a page reload.
   */
  stop(): void {
    this.observer.disconnect();
    const touched = new Set<Node>();
    document.querySelectorAll('[data-carve="token"]').forEach((span) => {
      const parent = span.parentNode;
      // Replace the span with its plain text so the page reads normally again.
      span.replaceWith(document.createTextNode(span.textContent ?? ''));
      if (parent) touched.add(parent);
    });
    document.querySelectorAll('[data-carve="processed"]').forEach((el) => {
      el.removeAttribute('data-carve');
    });
    // Merge the adjacent text nodes left behind by unwrapping, so a later
    // re-enable sees clean, whole text rather than many fragments.
    touched.forEach((node) => (node as Element | Text).normalize?.());
  }

  private scriptMatcher(): (text: string) => boolean {
    switch (this.lang) {
      case 'ja':
        return (t) => /[぀-ヿ一-鿿]/.test(t);
      case 'zh-cn':
      case 'zh-tw':
      case 'zh':
        return (t) => /[一-鿿]/.test(t);
      case 'ko':
        return (t) => /[가-힣]/.test(t);
      case 'en':
        return (t) => /[A-Za-z]{2,}/.test(t) && t.trim().length >= 3;
      default:
        return (t) => /\S{2,}/.test(t);
    }
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
    const langHasScript = this.scriptMatcher();
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: (node) => {
        const parent = node.parentElement;
        if (!parent) return NodeFilter.FILTER_REJECT;
        if (parent.closest('[data-carve]')) return NodeFilter.FILTER_REJECT;
        if (SKIP_TAGS.has(parent.tagName)) return NodeFilter.FILTER_REJECT;
        const text = node.textContent ?? '';
        if (!langHasScript(text)) return NodeFilter.FILTER_REJECT;
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

  private async tokenize(text: string): Promise<{ tokens: Token[] } | null> {
    if (this.lang === 'ja') {
      try {
        const tokens = await wasmTokenize(text);
        return { tokens };
      } catch {
        return null;
      }
    }
    // Non-Japanese: fall back to background → NLP API path.
    // MV3 service workers can be terminated mid-request; retry up to 3 times.
    const msg = {
      type: 'TOKENIZE' as const,
      text,
      language: this.lang,
      knownLemmas: this.vocabCache.getKnownLemmas(),
      learningLemmas: this.vocabCache.getLearningLemmas(),
    };
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        return await browser.runtime.sendMessage(msg);
      } catch (e: unknown) {
        const errMsg = e instanceof Error ? e.message : '';
        const isChannelClosed = errMsg.includes('message channel closed') || errMsg.includes('Extension context');
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
