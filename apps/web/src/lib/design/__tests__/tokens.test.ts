import { describe, it, expect } from 'vitest';
import { color, space, radius, type, motion, cssVariables } from '../tokens';

describe('design tokens', () => {
  it('color palette stays in sync with brand', () => {
    expect(color.green).toBe('#4caf50');
    expect(color.bg).toBe('#13151a');
    expect(color.ratingAgain).toBe('#e57373');
    expect(color.ratingEasy).toBe('#42a5f5');
  });

  it('space scale uses 4px increments', () => {
    // 0/1/2/3/4 → 0, 4, 8, 12, 16 px
    expect(space[0]).toBe('0');
    expect(space[1]).toBe('0.25rem');
    expect(space[2]).toBe('0.5rem');
    expect(space[4]).toBe('1rem');
  });

  it('radius scale includes "full"', () => {
    expect(radius.full).toBe('999px');
  });

  it('typography includes CJK font stack', () => {
    expect(type.familyCJK).toContain('Noto Sans JP');
    expect(type.familyCJK).toContain('Noto Sans SC');
    expect(type.familyCJK).toContain('Noto Sans KR');
  });

  it('motion easings are valid CSS', () => {
    expect(motion.easing).toMatch(/^cubic-bezier\(/);
  });

  describe('cssVariables()', () => {
    it('emits :root with every color', () => {
      const out = cssVariables();
      expect(out.startsWith(':root')).toBe(true);
      for (const [k, v] of Object.entries(color)) {
        expect(out).toContain(`--c-${k}: ${v};`);
      }
    });

    it('emits space tokens too', () => {
      const out = cssVariables();
      expect(out).toContain('--s-4: 1rem;');
      expect(out).toContain('--s-6: 1.5rem;');
    });

    it('produces no duplicate declarations', () => {
      const out = cssVariables();
      const decls = out.match(/--[a-zA-Z0-9-]+:/g) ?? [];
      const set = new Set(decls);
      expect(decls.length).toBe(set.size);
    });
  });
});
