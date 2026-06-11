/**
 * Tests for PageAnnotator.stop() — the teardown that lets "enable on this site"
 * turn OFF cleanly without a page reload: token spans are unwrapped back to
 * plain text, processed-markers cleared, and the MutationObserver disconnected
 * so no further DOM mutations get annotated.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('../../../shared/browser', () => ({ browser: { storage: { local: { get: vi.fn() } } } }));
vi.mock('../../../nlp/WasmTokenizer', () => ({ wasmTokenize: vi.fn().mockResolvedValue([]) }));

// requestIdleCallback is absent in jsdom — make it a no-op so start() doesn't
// schedule real annotation work during the test.
(globalThis as any).requestIdleCallback = () => 0;

import { PageAnnotator } from '../PageAnnotator';

const vocabCache = { getStatus: () => 'unknown' as const };
const popupManager = {};

function makeAnnotator(): PageAnnotator {
  return new PageAnnotator('en', vocabCache as any, popupManager as any);
}

// Build a paragraph that looks like annotator output: text + token spans + text.
function seedAnnotated(): void {
  const p = document.createElement('p');
  p.setAttribute('data-carve', 'processed');
  p.appendChild(document.createTextNode('I '));
  const t1 = document.createElement('span');
  t1.setAttribute('data-carve', 'token');
  t1.textContent = 'study';
  p.appendChild(t1);
  p.appendChild(document.createTextNode(' Japanese.'));
  document.body.appendChild(p);
}

describe('PageAnnotator.stop()', () => {
  beforeEach(() => { document.body.innerHTML = ''; });

  it('unwraps token spans back to plain text and restores the sentence', () => {
    seedAnnotated();
    expect(document.querySelectorAll('[data-carve="token"]').length).toBe(1);

    makeAnnotator().stop();

    expect(document.querySelectorAll('[data-carve="token"]').length).toBe(0);
    expect(document.querySelector('[data-carve="processed"]')).toBeNull();
    // Text is intact and contiguous (normalized) after unwrapping.
    expect(document.querySelector('p')?.textContent).toBe('I study Japanese.');
    expect(document.querySelector('p')?.childNodes.length).toBe(1);
  });

  it('disconnects the observer so later DOM changes are not annotated', async () => {
    const annotator = makeAnnotator();
    annotator.start();
    const spy = vi.spyOn(annotator as any, 'scheduleAnnotation');
    annotator.stop();

    // Mutate the DOM after stop(); the disconnected observer must not react.
    document.body.appendChild(document.createElement('div'));
    await new Promise((r) => setTimeout(r, 10));
    expect(spy).not.toHaveBeenCalled();
  });

  it('is idempotent / safe when nothing was annotated', () => {
    expect(() => makeAnnotator().stop()).not.toThrow();
  });

  it('does not collect text from Carve UI roots', () => {
    const popup = document.createElement('div');
    popup.id = 'carve-popup';
    popup.setAttribute('data-carve', 'ui');
    popup.textContent = 'conversation';
    document.body.appendChild(popup);

    const nodes = (makeAnnotator() as any).collectTextNodes(popup) as Text[];
    expect(nodes).toEqual([]);
  });
});
