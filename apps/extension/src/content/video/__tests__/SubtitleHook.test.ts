import { describe, it, expect, beforeEach, vi } from 'vitest';

import { SubtitleHook } from '../SubtitleHook';

const vocabCache = {
  getKnownLemmas: () => [] as string[],
  getLearningLemmas: () => [] as string[],
  getStatus: () => 'unknown' as const,
  markLearning: vi.fn().mockResolvedValue(undefined),
};

const popupManager = {
  hidePopup: vi.fn(),
  showForElement: vi.fn().mockResolvedValue(undefined),
  scheduleHidePopup: vi.fn(),
  cancelScheduledHide: vi.fn(),
  setInteractiveHoverCallbacks: vi.fn(),
};

describe('SubtitleHook lifecycle', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    document.head.innerHTML = '';
    popupManager.setInteractiveHoverCallbacks.mockClear();
  });

  it('unmounts the platform hook when destroyed', () => {
    const mount = vi.fn();
    const unmount = vi.fn();
    class FakePlatformHook {
      mount = mount;
      unmount = unmount;
    }

    const hook = new SubtitleHook('en', vocabCache as any, popupManager as any, [
      { match: /.*/, ctor: FakePlatformHook as any },
    ]);

    hook.mount();
    expect(mount).toHaveBeenCalledTimes(1);
    expect(document.getElementById('carve-sub-overlay')).not.toBeNull();

    hook.destroy();
    expect(unmount).toHaveBeenCalledTimes(1);
    expect(document.getElementById('carve-sub-overlay')).toBeNull();
    expect(popupManager.setInteractiveHoverCallbacks).toHaveBeenLastCalledWith(null);
  });
});
