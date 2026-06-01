/**
 * Property-based tests for popup helpers.
 *
 * These functions process content from arbitrary web pages (anything the
 * user clicks on). Property-based tests catch HTML-injection, length
 * blowups, and crash inputs that example tests don't enumerate.
 */

import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import {
  escapeHtml,
  buildFurigana,
  pitchLabel,
  pitchSvg,
  buildFuriganaWithMode,
} from '../PopupManager';

describe('escapeHtml — property', () => {
  it('never emits raw <, >, or " in output', () => {
    fc.assert(
      fc.property(fc.string(), (s) => {
        const out = escapeHtml(s);
        expect(/[<>"]/.test(out)).toBe(false);
      }),
    );
  });

  it('expansion ratio stays bounded', () => {
    fc.assert(
      fc.property(fc.string(), (s) => {
        // Each metacharacter expands to at most 6 chars (&quot;).
        expect(escapeHtml(s).length).toBeGreaterThanOrEqual(s.length);
        expect(escapeHtml(s).length).toBeLessThanOrEqual(s.length * 6);
      }),
    );
  });

  it('& always gets escaped first (no double-escape bug)', () => {
    fc.assert(
      fc.property(fc.string(), (s) => {
        const out = escapeHtml(s);
        // Every & in output must be the start of a known entity.
        const ampIdx = [...out.matchAll(/&/g)].map((m) => m.index!);
        for (const i of ampIdx) {
          const tail = out.slice(i, i + 6);
          expect(/^&(amp|lt|gt|quot);/.test(tail)).toBe(true);
        }
      }),
    );
  });
});

describe('buildFurigana — property', () => {
  it('returns a string for any input array', () => {
    fc.assert(
      fc.property(
        fc.array(fc.record({ text: fc.string(), reading: fc.string() })),
        (spans) => {
          const out = buildFurigana(spans);
          expect(typeof out).toBe('string');
        },
      ),
    );
  });

  it('never injects raw HTML from text or reading fields', () => {
    fc.assert(
      fc.property(
        fc.array(fc.record({ text: fc.string(), reading: fc.string() }), { minLength: 0, maxLength: 8 }),
        (spans) => {
          const out = buildFurigana(spans);
          // The only allowed tags are <ruby>, <rt>, </ruby>, </rt>.
          const tags = out.match(/<[^>]*>/g) ?? [];
          for (const t of tags) {
            expect(['<ruby>', '</ruby>', '<rt>', '</rt>'].includes(t)).toBe(true);
          }
        },
      ),
    );
  });
});

describe('pitchLabel — property', () => {
  it('returns a string for any input', () => {
    fc.assert(
      fc.property(fc.string(), (accent) => {
        expect(typeof pitchLabel(accent)).toBe('string');
      }),
    );
  });
});

describe('pitchSvg — property', () => {
  it('returns valid SVG-like markup for any input', () => {
    fc.assert(
      fc.property(fc.string(), fc.integer({ min: 0, max: 20 }), (accent, morae) => {
        const out = pitchSvg(accent, morae);
        expect(typeof out).toBe('string');
        // Either an empty string (no pitch info) or contains an <svg ...>
        if (out.length > 0) {
          expect(out.includes('<svg')).toBe(true);
        }
      }),
    );
  });
});

describe('buildFuriganaWithMode — property', () => {
  it('returns a string for every mode and input combination', () => {
    fc.assert(
      fc.property(
        fc.array(fc.record({ text: fc.string(), reading: fc.string() })),
        fc.constantFrom('always', 'unknown-only', 'kanji-only', 'off'),
        fc.constantFrom('known', 'learning', 'unknown'),
        (spans, mode, status) => {
          const out = buildFuriganaWithMode(spans, mode as any, status as any);
          expect(typeof out).toBe('string');
        },
      ),
    );
  });

  it('produces empty <rt> content when mode === "off"', () => {
    fc.assert(
      fc.property(
        fc.array(fc.record({ text: fc.string(), reading: fc.string() }), { minLength: 1, maxLength: 8 }),
        (spans) => {
          const out = buildFuriganaWithMode(spans, 'off', 'unknown');
          expect(out.includes('<rt>')).toBe(false);
        },
      ),
    );
  });
});
