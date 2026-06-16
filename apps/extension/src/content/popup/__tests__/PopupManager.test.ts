import { describe, it, expect, beforeEach, vi } from 'vitest';

const mockSendMessage = vi.hoisted(() => vi.fn());

vi.mock('../../../shared/browser', () => ({
  browser: {
    runtime: { sendMessage: mockSendMessage },
  },
}));

import { PopupManager } from '../PopupManager';

const vocabCache = {
  markKnown: vi.fn().mockResolvedValue(undefined),
  markIgnored: vi.fn().mockResolvedValue(undefined),
  markLearning: vi.fn().mockResolvedValue(undefined),
  getKnownLemmas: () => [] as string[],
  getLearningLemmas: () => [] as string[],
};

function makeToken(): HTMLElement {
  const parent = document.createElement('div');
  parent.textContent = 'I had a conversation recently.';
  const token = document.createElement('span');
  token.textContent = 'conversation';
  token.setAttribute('data-carve', 'token');
  token.setAttribute('data-content', '1');
  token.setAttribute('data-lemma', 'conversation');
  token.setAttribute('data-reading', 'conversation');
  token.setAttribute('data-status', 'unknown');
  parent.textContent = 'I had a ';
  parent.appendChild(token);
  parent.appendChild(document.createTextNode(' recently.'));
  document.body.appendChild(parent);
  return token;
}

describe('PopupManager', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    mockSendMessage.mockReset();
    vocabCache.markKnown.mockClear();
    vocabCache.markIgnored.mockClear();
    mockSendMessage.mockImplementation((msg: { type: string }) => {
      if (msg.type === 'LOOKUP') {
        return Promise.resolve({
          entry: {
            lemma: 'conversation',
            reading: null,
            frequency_rank: 1280,
            jlpt_level: null,
            pitch_accent: null,
            definitions: [{ definition: 'an informal talk between people', pos: 'noun', confidence: 1 }],
            furigana: [{ text: 'conversation', reading: '' }],
            found: true,
          },
        });
      }
      if (msg.type === 'EXPLAIN_WORD') {
        return Promise.resolve({ explanation: 'Here it means a talk between people.' });
      }
      if (msg.type === 'WORD_AUDIO') return Promise.resolve({ audioUrl: null });
      if (msg.type === 'WORD_IMAGE') return Promise.resolve({ imageUrl: null });
      if (msg.type === 'MARK_KNOWN_WORD') return Promise.resolve({ success: true });
      if (msg.type === 'IGNORE_WORD') return Promise.resolve({ success: true });
      return Promise.resolve({});
    });
  });

  it('renders English lookup content without orthographic reading chrome', async () => {
    const token = makeToken();
    const manager = new PopupManager('en', vocabCache as any);

    await manager.showForElement(token);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(mockSendMessage).toHaveBeenCalledWith(expect.objectContaining({
      type: 'LOOKUP',
      language: 'en',
      surface: 'conversation',
    }));
    expect(mockSendMessage).toHaveBeenCalledWith(expect.objectContaining({
      type: 'EXPLAIN_WORD',
      language: 'en',
      word: 'conversation',
    }));

    const popup = document.getElementById('carve-popup')!;
    expect(popup.getAttribute('data-carve')).toBe('ui');
    expect(popup.querySelector('ruby')).toBeNull();
    expect(popup.querySelector('.carve-furigana')?.textContent).toBe('conversation');
    expect(popup.querySelector('.carve-reading')?.textContent).not.toContain('conversation');
    expect(popup.textContent).toContain('Meaning');
    expect(popup.textContent).toContain('an informal talk between people');
    expect(popup.textContent).toContain('Here it means a talk between people.');
    expect(popup.textContent).not.toContain('explaining');
    expect(popup.textContent).toContain('I know this');
    expect(popup.textContent).toContain('Ignore');
  });

  it('marks an English word known from the known action', async () => {
    const token = makeToken();
    const manager = new PopupManager('en', vocabCache as any);

    await manager.showForElement(token);
    const button = document.querySelector<HTMLButtonElement>('.btn-known')!;
    button.click();
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(mockSendMessage).toHaveBeenCalledWith(expect.objectContaining({
      type: 'MARK_KNOWN_WORD',
      lemma: 'conversation',
      languageCode: 'en',
    }));
    expect(vocabCache.markKnown).toHaveBeenCalledWith('conversation');
    expect(vocabCache.markIgnored).not.toHaveBeenCalled();
    expect(token.getAttribute('data-status')).toBe('known');
  });

  it('marks an English token ignored from the ignore action', async () => {
    const token = makeToken();
    const manager = new PopupManager('en', vocabCache as any);

    await manager.showForElement(token);
    const button = document.querySelector<HTMLButtonElement>('.btn-ignore')!;
    button.click();
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(mockSendMessage).toHaveBeenCalledWith(expect.objectContaining({
      type: 'IGNORE_WORD',
      lemma: 'conversation',
      languageCode: 'en',
    }));
    expect(vocabCache.markIgnored).toHaveBeenCalledWith('conversation');
    expect(vocabCache.markKnown).not.toHaveBeenCalled();
    expect(token.getAttribute('data-status')).toBe('known');
  });

  it('keeps the popup open when the cursor moves from token to popup', async () => {
    vi.useFakeTimers();
    const token = makeToken();
    const manager = new PopupManager('en', vocabCache as any);
    const onEnter = vi.fn();
    const onLeave = vi.fn();
    manager.setInteractiveHoverCallbacks({ onEnter, onLeave });

    await manager.showForElement(token);

    const popup = document.getElementById('carve-popup')!;
    expect(popup.style.display).toBe('block');

    manager.scheduleHidePopup();
    vi.advanceTimersByTime(120);
    popup.dispatchEvent(new MouseEvent('mouseenter'));
    expect(onEnter).toHaveBeenCalled();

    vi.advanceTimersByTime(1000);
    expect(popup.style.display).toBe('block');

    popup.dispatchEvent(new MouseEvent('mouseleave'));
    expect(onLeave).toHaveBeenCalled();
    vi.advanceTimersByTime(180);
    expect(popup.style.display).toBe('none');
    vi.useRealTimers();
  });
});
