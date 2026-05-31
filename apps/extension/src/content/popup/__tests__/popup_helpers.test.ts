import { describe, it, expect, beforeEach } from 'vitest';
import { escapeHtml, buildFurigana, pitchLabel, getSurroundingSentence } from '../PopupManager';

// ── escapeHtml ────────────────────────────────────────────────────────────────

describe('escapeHtml', () => {
  it('escapes ampersand', () => {
    expect(escapeHtml('a & b')).toBe('a &amp; b');
  });

  it('escapes less-than', () => {
    expect(escapeHtml('<script>')).toBe('&lt;script&gt;');
  });

  it('escapes double-quotes (attribute injection prevention)', () => {
    expect(escapeHtml('"hello"')).toBe('&quot;hello&quot;');
  });

  it('escapes XSS payload', () => {
    const xss = '<img src=x onerror="alert(1)">';
    const escaped = escapeHtml(xss);
    expect(escaped).not.toContain('<img');
    expect(escaped).not.toContain('"alert');
    expect(escaped).toContain('&lt;img');
  });

  it('passes through plain text unchanged', () => {
    expect(escapeHtml('日本語')).toBe('日本語');
    expect(escapeHtml('hello world')).toBe('hello world');
  });

  it('handles empty string', () => {
    expect(escapeHtml('')).toBe('');
  });

  it('escapes multiple entities in one string', () => {
    expect(escapeHtml('<a href="x">&</a>')).toBe('&lt;a href=&quot;x&quot;&gt;&amp;&lt;/a&gt;');
  });
});

// ── buildFurigana ─────────────────────────────────────────────────────────────

describe('buildFurigana', () => {
  it('returns plain text for span without reading', () => {
    expect(buildFurigana([{ text: '日本語', reading: '' }])).toBe('日本語');
  });

  it('wraps span with reading in ruby tags', () => {
    const html = buildFurigana([{ text: '日本', reading: 'にほん' }]);
    expect(html).toBe('<ruby>日本<rt>にほん</rt></ruby>');
  });

  it('handles mixed spans (some with reading, some without)', () => {
    const html = buildFurigana([
      { text: '東京', reading: 'とうきょう' },
      { text: 'の', reading: '' },
      { text: '駅', reading: 'えき' },
    ]);
    expect(html).toContain('<ruby>東京<rt>とうきょう</rt></ruby>');
    expect(html).toContain('の');
    expect(html).toContain('<ruby>駅<rt>えき</rt></ruby>');
  });

  it('escapes HTML entities in text and reading', () => {
    const html = buildFurigana([{ text: '<b>', reading: '"test"' }]);
    expect(html).toContain('&lt;b&gt;');
    expect(html).toContain('&quot;test&quot;');
  });

  it('returns empty string for empty spans array', () => {
    expect(buildFurigana([])).toBe('');
  });
});

// ── pitchLabel ────────────────────────────────────────────────────────────────

describe('pitchLabel', () => {
  it('marks heiban (0) with circled 0', () => {
    expect(pitchLabel('0')).toBe('[0⓪]');
  });

  it('marks atamadaka (1) with circled 1', () => {
    expect(pitchLabel('1')).toBe('[1①]');
  });

  it('returns plain bracket for nakadaka / odaka patterns', () => {
    expect(pitchLabel('2')).toBe('[2]');
    expect(pitchLabel('3')).toBe('[3]');
    expect(pitchLabel('5')).toBe('[5]');
  });

  it('returns non-numeric accent wrapped in brackets', () => {
    expect(pitchLabel('LHL')).toBe('[LHL]');
  });

  it('handles empty string as non-numeric', () => {
    expect(pitchLabel('')).toBe('[]');
  });
});

// ── getSurroundingSentence ────────────────────────────────────────────────────

describe('getSurroundingSentence', () => {
  function makeTokenEl(text: string, parentTag = 'p', parentText?: string): HTMLElement {
    const parent = document.createElement(parentTag);
    const span = document.createElement('span');
    span.textContent = text;
    if (parentText) {
      parent.textContent = parentText;
      // Replace the token text within parent; crude but works for test setup
      parent.innerHTML = '';
      const before = document.createTextNode(parentText.slice(0, parentText.indexOf(text)));
      const after = document.createTextNode(parentText.slice(parentText.indexOf(text) + text.length));
      parent.appendChild(before);
      parent.appendChild(span);
      parent.appendChild(after);
    } else {
      parent.appendChild(span);
    }
    document.body.appendChild(parent);
    return span;
  }

  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('returns text from parent <p>', () => {
    // Build parent with text + token span inline so span stays in DOM
    const parent = document.createElement('p');
    parent.appendChild(document.createTextNode('ご飯を'));
    const el = document.createElement('span');
    el.textContent = '食べ';
    parent.appendChild(el);
    parent.appendChild(document.createTextNode('る'));
    document.body.appendChild(parent);
    const result = getSurroundingSentence(el);
    expect(result).not.toBeNull();
    expect(result).toContain('食べ');
  });

  it('returns null when element has no block-level ancestor', () => {
    const span = document.createElement('span');
    document.body.appendChild(span);
    // no p/li/td/div/span[data-carve] ancestor
    const inner = document.createElement('span');
    span.appendChild(inner);
    // direct body child with no matching ancestor
    const orphan = document.createElement('span');
    document.body.appendChild(orphan);
    expect(getSurroundingSentence(orphan)).toBeNull();
  });

  it('truncates text longer than 200 chars and adds ellipsis', () => {
    const parent = document.createElement('p');
    const longText = 'あ'.repeat(300);
    const el = document.createElement('span');
    el.textContent = 'TARGET';
    parent.appendChild(document.createTextNode(longText.slice(0, 150)));
    parent.appendChild(el);
    parent.appendChild(document.createTextNode(longText.slice(150)));
    document.body.appendChild(parent);

    const result = getSurroundingSentence(el);
    expect(result).not.toBeNull();
    expect(result!.length).toBeLessThan(210); // 50+target+50 + possible ellipsis
    expect(result).toContain('…');
  });

  it('returns null when parent text is empty', () => {
    const parent = document.createElement('p');
    const el = document.createElement('span');
    parent.appendChild(el);
    document.body.appendChild(parent);
    const result = getSurroundingSentence(el);
    expect(result).toBeNull();
  });

  it('caps output at 200 chars for short-enough parents', () => {
    const parent = document.createElement('p');
    parent.textContent = 'あ'.repeat(190);
    const el = document.createElement('span');
    parent.appendChild(el);
    document.body.appendChild(parent);
    const result = getSurroundingSentence(el);
    expect(result!.length).toBeLessThanOrEqual(200);
  });
});
