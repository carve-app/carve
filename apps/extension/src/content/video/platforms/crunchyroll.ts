import type { SubtitleOverlay } from '../SubtitleOverlay';
import { extractNativeCueText } from './nativeText';
import { readAnyActiveTextTrackTiming, readTargetTextTrackCue } from './textTrackCue';

// Crunchyroll's Beta player renders subtitles inside `.libassjs-subs` (their
// libass.js port) — each cue appears as a child `div`.
const NATIVE_SELECTOR = '.libassjs-subs, .vjs-text-track-display';
const TEXT_SELECTOR = '.libassjs-subs > div, .vjs-text-track-cue';

export class CrunchyrollHook {
  private observer: MutationObserver | null = null;
  private pollId: number | null = null;
  private lastText = '';

  constructor(private overlay: SubtitleOverlay, private lang: string = '') {}

  mount(): void {
    this.overlay.hideNativeContainer(NATIVE_SELECTOR);
    this.observer = new MutationObserver(() => this.checkSubtitle());
    this.observer.observe(document.body, { childList: true, subtree: true, characterData: true });
    this.pollId = window.setInterval(() => this.checkSubtitle(), 300);
    this.checkSubtitle();
  }

  private checkSubtitle(): void {
    const video = document.querySelector<HTMLVideoElement>('video');
    const els = document.querySelectorAll<HTMLElement>(TEXT_SELECTOR);
    const domText = Array.from(els)
      .map(el => el.textContent?.trim() ?? '')
      .filter(Boolean)
      .join(' ');
    const trackCue = readTargetTextTrackCue(video, this.lang);
    const text = domText || trackCue?.text || '';
    if (!text || text === this.lastText) return;
    this.lastText = text;
    const { startMs, endMs } = domText ? this.getActiveCueTiming(video) : trackCue ?? defaultTiming();
    const nativeText = extractNativeCueText(video, this.lang);
    this.overlay.onCue({ text, startMs, endMs, nativeText });
  }

  private getActiveCueTiming(video: HTMLVideoElement | null): { startMs: number; endMs: number } {
    if (!video) return defaultTiming();
    const timing = readAnyActiveTextTrackTiming(video);
    if (timing) return timing;
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
    if (this.pollId != null) window.clearInterval(this.pollId);
    this.overlay.showNativeContainer(NATIVE_SELECTOR);
  }
}

function defaultTiming(): { startMs: number; endMs: number } {
  const video = document.querySelector<HTMLVideoElement>('video');
  const t = video ? video.currentTime * 1000 : 0;
  return { startMs: Math.max(0, t - 2000), endMs: t + 2000 };
}
