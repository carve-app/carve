import type { SubtitleOverlay } from '../SubtitleOverlay';
import { extractNativeCueText } from './nativeText';

// Prime Video renders captions inside `.atvwebplayersdk-captions-overlay`
// with dialogue lines under `.f35bt6a` style classes — selector chosen for
// stability (data-testid was inconsistent across regions).
const NATIVE_SELECTOR = '.atvwebplayersdk-captions-overlay';
const TEXT_SELECTOR = '.atvwebplayersdk-captions-text, .f35bt6a';

export class AmazonPrimeHook {
  private observer: MutationObserver | null = null;
  private lastText = '';

  constructor(private overlay: SubtitleOverlay, private lang: string = '') {}

  mount(): void {
    this.overlay.hideNativeContainer(NATIVE_SELECTOR);
    this.observer = new MutationObserver(() => this.checkSubtitle());
    this.observer.observe(document.body, { childList: true, subtree: true, characterData: true });
    this.checkSubtitle();
  }

  private checkSubtitle(): void {
    const els = document.querySelectorAll<HTMLElement>(TEXT_SELECTOR);
    if (!els.length) return;
    const text = Array.from(els)
      .map(el => el.textContent?.trim() ?? '')
      .filter(Boolean)
      .join(' ');
    if (!text || text === this.lastText) return;
    this.lastText = text;
    const video = document.querySelector<HTMLVideoElement>('video');
    const { startMs, endMs } = this.getActiveCueTiming(video);
    const nativeText = extractNativeCueText(video, this.lang);
    this.overlay.onCue({ text, startMs, endMs, nativeText });
  }

  private getActiveCueTiming(video: HTMLVideoElement | null): { startMs: number; endMs: number } {
    if (!video) return defaultTiming();
    for (let i = 0; i < video.textTracks.length; i++) {
      const track = video.textTracks[i];
      if (track.mode !== 'showing') continue;
      if (track.activeCues && track.activeCues.length > 0) {
        const cue = track.activeCues[0] as VTTCue;
        return {
          startMs: Math.round(cue.startTime * 1000),
          endMs: Math.round(cue.endTime * 1000),
        };
      }
    }
    return defaultTiming();
  }

  unmount(): void {
    this.observer?.disconnect();
    this.overlay.showNativeContainer(NATIVE_SELECTOR);
  }
}

function defaultTiming(): { startMs: number; endMs: number } {
  const video = document.querySelector<HTMLVideoElement>('video');
  const t = video ? video.currentTime * 1000 : 0;
  return { startMs: Math.max(0, t - 2000), endMs: t + 2000 };
}
