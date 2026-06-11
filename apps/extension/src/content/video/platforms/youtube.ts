import type { SubtitleOverlay } from '../SubtitleOverlay';
import { extractNativeCueText } from './nativeText';
import { hasTargetTextTrack, readAnyActiveTextTrackTiming, readTargetTextTrackCue } from './textTrackCue';

const NATIVE_SELECTOR = '.ytp-caption-window-container, .caption-window';
const VIDEO_SELECTOR = 'video.html5-main-video, video.video-stream';
const CAPTION_BUTTON_SELECTOR = [
  '.ytp-subtitles-button',
  'button[aria-keyshortcuts="c"]',
  'button[aria-label*="Subtitles" i]',
  'button[aria-label*="captions" i]',
  'button[title*="Subtitles" i]',
  'button[title*="captions" i]',
].join(', ');
const CAPTION_CLICK_THROTTLE_MS = 700;
const MAX_CONFIDENT_CAPTION_CLICKS = 3;

export class YouTubeHook {
  private observer: MutationObserver | null = null;
  private pollId: number | null = null;
  private lastText = '';
  private captionRequestKey = '';
  private captionClickAttempts = 0;
  private lastCaptionClickAt = 0;

  constructor(private overlay: SubtitleOverlay, private lang: string = '') {}

  mount(): void {
    this.overlay.hideNativeContainer(NATIVE_SELECTOR);

    this.observer = new MutationObserver(() => this.checkSubtitle());
    this.observer.observe(document.body, { childList: true, subtree: true, characterData: true });
    this.pollId = window.setInterval(() => this.checkSubtitle(), 300);

    this.checkSubtitle();
  }

  private checkSubtitle(): void {
    const video = document.querySelector<HTMLVideoElement>(VIDEO_SELECTOR);
    this.ensureNativeCaptionSource(video);

    const segments = document.querySelectorAll<HTMLElement>('.ytp-caption-segment');
    const domText = Array.from(segments)
      .map(s => s.textContent?.trim() ?? '')
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

  private ensureNativeCaptionSource(video: HTMLVideoElement | null): void {
    readTargetTextTrackCue(video, this.lang);
    if (!video) return;
    this.resetCaptionRequestState(video);
    if (hasTargetTextTrack(video, this.lang) || this.hasNativeCaptionText()) return;
    this.clickCaptionButtonIfNeeded();
  }

  private resetCaptionRequestState(video: HTMLVideoElement): void {
    const key = `${location.href}::${video.currentSrc || video.src || ''}`;
    if (key === this.captionRequestKey) return;
    this.captionRequestKey = key;
    this.captionClickAttempts = 0;
    this.lastCaptionClickAt = 0;
  }

  private hasNativeCaptionText(): boolean {
    return Array.from(document.querySelectorAll<HTMLElement>('.ytp-caption-segment'))
      .some((el) => Boolean(el.textContent?.trim()));
  }

  private clickCaptionButtonIfNeeded(): void {
    const button = document.querySelector<HTMLButtonElement>(CAPTION_BUTTON_SELECTOR);
    if (!button || button.disabled || !button.isConnected) return;

    const state = captionButtonState(button);
    if (state === 'on') return;
    if (state === 'unknown' && this.captionClickAttempts > 0) return;
    if (state === 'off' && this.captionClickAttempts >= MAX_CONFIDENT_CAPTION_CLICKS) return;

    const now = Date.now();
    if (now - this.lastCaptionClickAt < CAPTION_CLICK_THROTTLE_MS) return;

    this.captionClickAttempts += 1;
    this.lastCaptionClickAt = now;
    button.click();
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

function captionButtonState(button: HTMLButtonElement): 'on' | 'off' | 'unknown' {
  const pressed = button.getAttribute('aria-pressed');
  if (pressed === 'true') return 'on';
  if (pressed === 'false') return 'off';

  const label = [
    button.getAttribute('aria-label'),
    button.getAttribute('title'),
    button.getAttribute('data-title-no-tooltip'),
    button.textContent,
  ].filter(Boolean).join(' ').toLowerCase();

  if (/\b(turn off|disable|hide|off)\b/.test(label)) return 'on';
  if (/\b(turn on|enable|show|on)\b/.test(label)) return 'off';
  return 'unknown';
}

function defaultTiming(): { startMs: number; endMs: number } {
  const video = document.querySelector<HTMLVideoElement>('video.html5-main-video, video.video-stream');
  const t = video ? video.currentTime * 1000 : 0;
  return { startMs: Math.max(0, t - 2000), endMs: t + 2000 };
}
