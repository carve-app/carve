import { describe, it, expect, beforeEach } from 'vitest';
import {
  escapeHtml,
  buildFurigana,
  pitchLabel,
  pitchSvg,
  buildFuriganaWithMode,
  getSurroundingSentence,
  getCandidateSentences,
} from '../PopupManager';

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

// ── getCandidateSentences ─────────────────────────────────────────────────────

describe('getCandidateSentences', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('splits paragraph into multiple sentences on CJK terminators', () => {
    const parent = document.createElement('p');
    parent.textContent = '今日は天気がいいです。私は寿司を食べる。明日も晴れる。';
    const el = document.createElement('span');
    el.textContent = '食べる';
    parent.appendChild(el);
    document.body.appendChild(parent);

    const cands = getCandidateSentences(el, '食べる');
    expect(cands.length).toBeGreaterThanOrEqual(2);
    // The sentence containing the target should be first (priority sort).
    expect(cands[0]).toContain('食べる');
  });

  it('splits on ASCII terminators', () => {
    const parent = document.createElement('p');
    parent.textContent = 'I had lunch. I ate sushi yesterday. Tomorrow will be sunny.';
    const el = document.createElement('span');
    el.textContent = 'ate';
    parent.appendChild(el);
    document.body.appendChild(parent);

    const cands = getCandidateSentences(el, 'ate');
    expect(cands.length).toBeGreaterThanOrEqual(2);
    expect(cands[0]).toContain('ate');
  });

  it('returns empty when no block ancestor', () => {
    const orphan = document.createElement('span');
    document.body.appendChild(orphan);
    expect(getCandidateSentences(orphan, 'x')).toEqual([]);
  });

  it('respects max cap', () => {
    const parent = document.createElement('p');
    parent.textContent = 'One. Two. Three. Four. Five. Six. Seven.';
    const el = document.createElement('span');
    el.textContent = 'Four';
    parent.appendChild(el);
    document.body.appendChild(parent);

    const cands = getCandidateSentences(el, 'Four', 3);
    expect(cands.length).toBeLessThanOrEqual(3);
  });

  it('deduplicates identical sentences', () => {
    const parent = document.createElement('p');
    parent.textContent = '私は食べる。私は食べる。違う文です。';
    const el = document.createElement('span');
    el.textContent = '食べる';
    parent.appendChild(el);
    document.body.appendChild(parent);

    const cands = getCandidateSentences(el, '食べる');
    const eatCount = cands.filter((c) => c === '私は食べる。').length;
    expect(eatCount).toBe(1);
  });
});

// ── pitchSvg ──────────────────────────────────────────────────────────────────

describe('pitchSvg', () => {
  it('returns empty string for non-numeric accent', () => {
    expect(pitchSvg('LHL', 3)).toBe('');
  });

  it('returns empty string for 0 morae', () => {
    expect(pitchSvg('0', 0)).toBe('');
  });

  it('returns SVG string for valid accent', () => {
    const svg = pitchSvg('0', 3);
    expect(svg).toContain('<svg');
    expect(svg).toContain('</svg>');
  });

  it('includes polyline element', () => {
    expect(pitchSvg('2', 4)).toContain('<polyline');
  });

  it('heiban (0): mora 1 is low, mora 2+ are high', () => {
    // For accent=0, morae=3: levels = [false, true, true]
    // Points: (3, 16), (13, 3), (23, 3)
    const svg = pitchSvg('0', 3);
    expect(svg).toContain('aria-label="Pitch accent 0"');
    // mora 1 low → y=16
    expect(svg).toContain('3,16');
    // mora 2 high → y=3
    expect(svg).toContain('13,3');
  });

  it('atamadaka (1): mora 1 is high, rest are low', () => {
    const svg = pitchSvg('1', 3);
    expect(svg).toContain('aria-label="Pitch accent 1"');
    // mora 1 high → y=3
    expect(svg).toContain('3,3');
    // mora 2 low → y=16
    expect(svg).toContain('13,16');
  });

  it('nakadaka (2): low, high, low for morae=3', () => {
    const svg = pitchSvg('2', 3);
    // mora 1 low (i=0, i>0 is false)
    expect(svg).toContain('3,16');
    // mora 2 high (i=1, 1>0 && 1<2 = true)
    expect(svg).toContain('13,3');
    // mora 3 low (i=2, 2<2 is false)
    expect(svg).toContain('23,16');
  });

  it('uses different colors for heiban vs atamadaka', () => {
    const heiban = pitchSvg('0', 2);
    const atama = pitchSvg('1', 2);
    // both are valid SVG but should differ
    expect(heiban).not.toBe(atama);
  });

  it('generates one circle per mora', () => {
    const svg = pitchSvg('0', 4);
    const circleCount = (svg.match(/<circle/g) ?? []).length;
    expect(circleCount).toBe(4);
  });
});

// ── buildFuriganaWithMode ─────────────────────────────────────────────────────

describe('buildFuriganaWithMode', () => {
  const spans = [
    { text: '東京', reading: 'とうきょう' },
    { text: 'の', reading: '' },
  ];

  it('mode=always shows ruby for all spans with readings', () => {
    const html = buildFuriganaWithMode(spans, 'always', 'unknown');
    expect(html).toContain('<ruby>東京<rt>とうきょう</rt></ruby>');
  });

  it('mode=off shows plain text only, no ruby', () => {
    const html = buildFuriganaWithMode(spans, 'off', 'unknown');
    expect(html).not.toContain('<ruby');
    expect(html).toContain('東京');
  });

  it('mode=unknown-only shows ruby only for unknown words', () => {
    const htmlUnknown = buildFuriganaWithMode(spans, 'unknown-only', 'unknown');
    expect(htmlUnknown).toContain('<ruby>東京<rt>とうきょう</rt></ruby>');

    const htmlKnown = buildFuriganaWithMode(spans, 'unknown-only', 'known');
    expect(htmlKnown).not.toContain('<ruby');
  });

  it('mode=kanji-only shows ruby only for kanji-containing spans', () => {
    const html = buildFuriganaWithMode(spans, 'kanji-only', 'known');
    expect(html).toContain('<ruby>東京<rt>とうきょう</rt></ruby>');
  });

  it('mode=kanji-only does not show ruby for hiragana/katakana', () => {
    const kanaSpans = [{ text: 'ねこ', reading: 'ねこ' }];
    const html = buildFuriganaWithMode(kanaSpans, 'kanji-only', 'unknown');
    expect(html).not.toContain('<ruby');
  });

  it('always escapes HTML entities in text and reading', () => {
    const dangerSpans = [{ text: '<b>', reading: '"xss"' }];
    const html = buildFuriganaWithMode(dangerSpans, 'always', 'unknown');
    expect(html).toContain('&lt;b&gt;');
    expect(html).toContain('&quot;xss&quot;');
    expect(html).not.toContain('<b>');
  });

  it('span without reading renders as plain text in any mode', () => {
    const noReading = [{ text: 'の' }];
    for (const mode of ['always', 'off', 'unknown-only', 'kanji-only'] as const) {
      const html = buildFuriganaWithMode(noReading, mode, 'unknown');
      expect(html).toBe('の');
    }
  });
});
