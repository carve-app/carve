import type { SubtitleOverlay } from '../SubtitleOverlay';

const NATIVE_SELECTOR = '.ytp-caption-window-container';

export class YouTubeHook {
  private observer: MutationObserver | null = null;
  private lastText = '';

  constructor(private overlay: SubtitleOverlay) {}

  mount(): void {
    this.overlay.hideNativeContainer(NATIVE_SELECTOR);

    this.observer = new MutationObserver(() => this.checkSubtitle());
    this.observer.observe(document.body, { childList: true, subtree: true, characterData: true });

    this.checkSubtitle();
  }

  private checkSubtitle(): void {
    const segments = document.querySelectorAll<HTMLElement>('.ytp-caption-segment');
    if (!segments.length) return;

    const text = Array.from(segments)
      .map(s => s.textContent?.trim() ?? '')
      .filter(Boolean)
      .join(' ');

    if (!text || text === this.lastText) return;
    this.lastText = text;

    const { startMs, endMs } = this.getActiveCueTiming();
    this.overlay.onCue({ text, startMs, endMs });
  }

  private getActiveCueTiming(): { startMs: number; endMs: number } {
    const video = document.querySelector<HTMLVideoElement>('video.html5-main-video, video.video-stream');
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
  const video = document.querySelector<HTMLVideoElement>('video.html5-main-video, video.video-stream');
  const t = video ? video.currentTime * 1000 : 0;
  return { startMs: Math.max(0, t - 2000), endMs: t + 2000 };
}
