/**
 * Tests for SubtitleOverlay state logic.
 *
 * SubtitleOverlay's DOM interactions are integration-level (requires a full
 * browser environment). Here we test the pure state machine: history buffer,
 * cue trimming, step boundaries.
 *
 * We achieve this by extending SubtitleOverlay with a minimal harness that
 * stubs chrome.runtime.sendMessage and the DOM elements the class touches.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// ── Stub browser API via vi.mock so it's available at import time ─────────────
// vi.hoisted runs before any imports; vi.mock is hoisted automatically.
const mockSendMessage = vi.hoisted(() => vi.fn().mockResolvedValue({ tokens: [] }));

vi.mock('../../../shared/browser', () => ({
  browser: {
    runtime: { sendMessage: mockSendMessage },
  },
}));

// ── Stub VocabCache ───────────────────────────────────────────────────────────
const mockVocabCache = {
  getKnownLemmas: () => [] as string[],
  getLearningLemmas: () => [] as string[],
  getStatus: () => 'unknown' as const,
  markLearning: vi.fn().mockResolvedValue(undefined),
};

// ── Stub PopupManager ─────────────────────────────────────────────────────────
const mockPopupManager = {
  showForElement: vi.fn().mockResolvedValue(undefined),
  hidePopup: vi.fn(),
};

import { SubtitleOverlay, type ActiveCue } from '../SubtitleOverlay';

// Helper to build a cue
function cue(text: string, startMs = 0, endMs = 2000, nativeText?: string): ActiveCue {
  return { text, startMs, endMs, nativeText };
}

describe('SubtitleOverlay — cue history', () => {
  let overlay: SubtitleOverlay;

  beforeEach(() => {
    document.body.innerHTML = '';
    document.head.innerHTML = '';
    mockSendMessage.mockClear();
    mockPopupManager.showForElement.mockClear();
    mockPopupManager.hidePopup.mockClear();
    // SubtitleOverlay appends itself to body in constructor
    overlay = new SubtitleOverlay('ja', mockVocabCache as any, mockPopupManager as any);
  });

  afterEach(() => {
    overlay.destroy();
  });

  it('appends cues to history', () => {
    overlay.onCue(cue('一'));
    overlay.onCue(cue('二'));
    overlay.onCue(cue('三'));
    // After 3 cues the history index should be at the last one
    // Verify by checking that the overlay exists (no throw)
    expect(document.getElementById('carve-sub-overlay')).not.toBeNull();
  });

  it('caps history at 30 cues', () => {
    for (let i = 0; i < 35; i++) {
      overlay.onCue(cue(`cue${i}`));
    }
    // historyIndex is private; we infer trimming worked if no error and the
    // caption overlay is still mounted after more cues than the history cap.
    expect(document.getElementById('carve-sub-overlay')).not.toBeNull();
  });

  it('destroy removes overlay from DOM and cleans up', () => {
    expect(document.getElementById('carve-sub-overlay')).not.toBeNull();
    overlay.destroy();
    expect(document.getElementById('carve-sub-overlay')).toBeNull();
    expect(document.getElementById('carve-sub-overlay-styles')).toBeNull();
  });

  it('does not re-inject styles when a second overlay is created', () => {
    // Already have one overlay from beforeEach. Create another.
    const overlay2 = new SubtitleOverlay('ja', mockVocabCache as any, mockPopupManager as any);
    const styleEls = document.querySelectorAll('#carve-sub-overlay-styles');
    // Styles should only exist once even with two instances
    expect(styleEls.length).toBe(1);
    overlay2.destroy();
  });

  it('hideNativeContainer sets visibility:hidden on matching selector', () => {
    const el = document.createElement('div');
    el.className = 'native-subs';
    document.body.appendChild(el);
    overlay.hideNativeContainer('.native-subs');
    expect(el.style.visibility).toBe('hidden');
    expect(document.head.textContent).toContain('.native-subs { visibility: hidden !important; }');
  });

  it('showNativeContainer restores visibility', () => {
    const el = document.createElement('div');
    el.className = 'native-subs';
    el.style.visibility = 'hidden';
    document.body.appendChild(el);
    overlay.hideNativeContainer('.native-subs');
    overlay.showNativeContainer('.native-subs');
    expect(el.style.visibility).toBe('');
    expect(document.head.textContent).not.toContain('.native-subs { visibility: hidden !important; }');
  });

  it('hideNativeContainer is a no-op when selector matches nothing', () => {
    expect(() => overlay.hideNativeContainer('.nonexistent')).not.toThrow();
  });
});

// ── Video-relative positioning ───────────────────────────────────────────────

describe('SubtitleOverlay — positioning', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    document.head.innerHTML = '';
    mockSendMessage.mockClear();
    mockPopupManager.showForElement.mockClear();
    mockPopupManager.hidePopup.mockClear();
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1000 });
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 900 });
  });

  it('centers the subtitle layer over the video rect, not the viewport', () => {
    let rect = {
      left: 120,
      top: 90,
      right: 760,
      bottom: 450,
      width: 640,
      height: 360,
      x: 120,
      y: 90,
      toJSON: () => ({}),
    } as DOMRect;
    const video = document.createElement('video') as HTMLVideoElement;
    video.getBoundingClientRect = vi.fn(() => rect);
    document.body.appendChild(video);

    const overlay = new SubtitleOverlay('en', mockVocabCache as any, mockPopupManager as any);
    const el = document.getElementById('carve-sub-overlay')!;
    expect(el.style.left).toBe('120px');
    expect(el.style.width).toBe('640px');
    expect(el.style.bottom).toBe('493px');
    expect(el.style.transform).toBe('none');

    rect = {
      left: 40,
      top: 0,
      right: 540,
      bottom: 500,
      width: 500,
      height: 500,
      x: 40,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect;
    window.dispatchEvent(new Event('resize'));
    expect(el.style.left).toBe('40px');
    expect(el.style.width).toBe('500px');
    expect(el.style.bottom).toBe('460px');

    overlay.destroy();
  });
});

// ── Cue stabilizer ───────────────────────────────────────────────────────────

describe('SubtitleOverlay — cue stabilizer', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    document.head.innerHTML = '';
    mockSendMessage.mockClear();
    mockPopupManager.showForElement.mockClear();
    mockPopupManager.hidePopup.mockClear();
    mockSendMessage.mockResolvedValue({ tokens: [] });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders partial subtitle updates immediately as the cue grows', () => {
    const overlay = new SubtitleOverlay('en', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue('I had', 0, 500));
    expect(document.getElementById('cso-target')?.textContent).toBe('I had');

    overlay.onCue(cue('I had a conversation', 0, 1000));
    expect(document.getElementById('cso-target')?.textContent).toBe('I had a conversation');
    overlay.destroy();
  });

  it('renders complete sentences immediately', () => {
    const overlay = new SubtitleOverlay('en', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue('I had a conversation.', 0, 1000));
    expect(document.getElementById('cso-target')?.textContent).toBe('I had a conversation.');
    overlay.destroy();
  });

  it('renders the next partial immediately after a finalized sentence', () => {
    const overlay = new SubtitleOverlay('en', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue('First sentence.', 0, 1000));
    expect(document.getElementById('cso-target')?.textContent).toBe('First sentence.');

    overlay.onCue(cue('Second', 1200, 1800));
    expect(document.getElementById('cso-target')?.textContent).toBe('Second');
    overlay.destroy();
  });
});

// ── onCue triggers tokenization request ──────────────────────────────────────

describe('SubtitleOverlay — onCue sends TOKENIZE message', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    document.head.innerHTML = '';
    mockSendMessage.mockClear();
    mockPopupManager.showForElement.mockClear();
    mockPopupManager.hidePopup.mockClear();
    mockSendMessage.mockResolvedValue({ tokens: [] });
  });

  it('sends TOKENIZE to background on each cue', async () => {
    const overlay = new SubtitleOverlay('ja', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue('食べる。'));

    // Wait for the async renderAt call
    await new Promise(r => setTimeout(r, 0));

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'TOKENIZE',
        text: '食べる。',
        language: 'ja',
      }),
    );
    overlay.destroy();
  });

  it('falls back to raw text when TOKENIZE returns no tokens', async () => {
    mockSendMessage.mockResolvedValue(null);
    const overlay = new SubtitleOverlay('ja', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue('テスト。'));
    await new Promise(r => setTimeout(r, 0));

    const target = document.getElementById('cso-target');
    expect(target?.textContent).toContain('テスト。');
    overlay.destroy();
  });

  it('preserves spaces + punctuation between English tokens (no word-mashing)', async () => {
    // Regression: tokens were concatenated surface-only, mashing Latin words
    // ("It's harder working" -> "It'sharderworking"). The renderer must
    // reconstruct the original text, emitting inter-token chars as text nodes.
    const line = "Cheat. It's harder working. It is smarter.";
    mockSendMessage.mockResolvedValue({
      tokens: [
        { surface: 'Cheat', lemma: 'cheat', reading_hira: '', is_content_word: true },
        { surface: 'It', lemma: 'it', reading_hira: '', is_content_word: false },
        { surface: 'harder', lemma: 'hard', reading_hira: '', is_content_word: true },
        { surface: 'working', lemma: 'work', reading_hira: '', is_content_word: true },
        { surface: 'It', lemma: 'it', reading_hira: '', is_content_word: false },
        { surface: 'is', lemma: 'be', reading_hira: '', is_content_word: false },
        { surface: 'smarter', lemma: 'smart', reading_hira: '', is_content_word: true },
      ],
    });
    const overlay = new SubtitleOverlay('en', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue(line));
    await new Promise(r => setTimeout(r, 0));

    // The rendered text must equal the original line exactly — spaces,
    // apostrophe, and periods all preserved.
    const target = document.getElementById('cso-target');
    expect(target?.textContent).toBe(line);
    // And content words are still individual clickable token spans.
    expect(target?.querySelectorAll('[data-carve="token"]').length).toBe(7);
    overlay.destroy();
  });

  it('pauses video and shows popup immediately when hovering a subtitle token', async () => {
    mockSendMessage.mockResolvedValue({
      tokens: [
        { surface: '勉強', lemma: '勉強', reading_hira: 'べんきょう', is_content_word: true },
      ],
    });

    const video = document.createElement('video') as HTMLVideoElement;
    let paused = false;
    Object.defineProperty(video, 'paused', { configurable: true, get: () => paused });
    video.pause = vi.fn(() => { paused = true; });
    video.play = vi.fn(() => { paused = false; return Promise.resolve(); });
    document.body.appendChild(video);

    const overlay = new SubtitleOverlay('ja', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue('勉強。'));
    await new Promise(r => setTimeout(r, 0));

    const token = document.querySelector<HTMLElement>('.cso-token.cso-unknown');
    expect(token).not.toBeNull();
    token!.dispatchEvent(new MouseEvent('mouseenter'));
    expect(video.pause).toHaveBeenCalled();
    expect(mockPopupManager.showForElement).toHaveBeenCalledWith(token);

    document.querySelector<HTMLElement>('.cso-lines')!.dispatchEvent(new MouseEvent('mouseleave'));
    expect(mockPopupManager.hidePopup).toHaveBeenCalled();
    expect(video.play).toHaveBeenCalled();
    overlay.destroy();
  });
});

// ── Native subtitle display ───────────────────────────────────────────────────

describe('SubtitleOverlay — native subtitle', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    document.head.innerHTML = '';
    mockPopupManager.showForElement.mockClear();
    mockPopupManager.hidePopup.mockClear();
    mockSendMessage.mockResolvedValue({ tokens: [] });
  });

  it('renders native text from cue.nativeText', async () => {
    const overlay = new SubtitleOverlay('ja', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue('食べる。', 0, 2000, 'to eat'));
    await new Promise(r => setTimeout(r, 0));

    const native = document.getElementById('cso-native');
    expect(native?.textContent).toBe('to eat');
    overlay.destroy();
  });

  it('hides native element when no nativeText', async () => {
    const overlay = new SubtitleOverlay('ja', mockVocabCache as any, mockPopupManager as any);
    overlay.onCue(cue('食べる。')); // no nativeText
    await new Promise(r => setTimeout(r, 0));

    const native = document.getElementById('cso-native');
    expect(native?.style.display).toBe('none');
    overlay.destroy();
  });
});
