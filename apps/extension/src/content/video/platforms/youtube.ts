import type { ActiveCue, SubtitleOverlay } from '../SubtitleOverlay';
import { extractNativeCueText } from './nativeText';
import { readAnyActiveTextTrackTiming, readTargetTextTrackCue } from './textTrackCue';

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
const TIMED_TEXT_LOOKAHEAD_MS = 900;
const TIMED_TEXT_HOLD_MS = 250;
const ANDROID_VR_CLIENT_NAME = '28';
const ANDROID_VR_CLIENT_VERSION = '1.65.10';
const ANDROID_VR_CONTEXT = {
  client: {
    clientName: 'ANDROID_VR',
    clientVersion: ANDROID_VR_CLIENT_VERSION,
    deviceMake: 'Oculus',
    deviceModel: 'Quest 3',
    androidSdkVersion: 32,
    userAgent: 'com.google.android.apps.youtube.vr.oculus/1.65.10 (Linux; U; Android 12L; eureka-user Build/SQ3A.220605.009.A1) gzip',
    osName: 'Android',
    osVersion: '12L',
    hl: 'en',
  },
};

interface YouTubeCaptionTrack {
  baseUrl?: string;
  languageCode?: string;
  kind?: string;
  isTranslatable?: boolean;
  name?: { simpleText?: string; runs?: Array<{ text?: string }> };
}

interface YouTubePlayerResponse {
  videoDetails?: { videoId?: string };
  captions?: {
    playerCaptionsTracklistRenderer?: {
      captionTracks?: YouTubeCaptionTrack[];
    };
  };
}

interface SelectedCaptionTrack {
  track: YouTubeCaptionTrack;
  translateTo?: string;
}

export class YouTubeHook {
  private observer: MutationObserver | null = null;
  private pollId: number | null = null;
  private lastCueKey = '';
  private captionRequestKey = '';
  private captionClickAttempts = 0;
  private lastCaptionClickAt = 0;
  private transcriptCues: ActiveCue[] = [];
  private transcriptKey = '';
  private transcriptLoadKey = '';
  private transcriptLoadInFlight = false;

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
    this.ensureTimedTextTranscript(video);

    const segments = document.querySelectorAll<HTMLElement>('.ytp-caption-segment');
    const domText = Array.from(segments)
      .map(s => s.textContent?.trim() ?? '')
      .filter(Boolean)
      .join(' ');
    const trackCue = readTargetTextTrackCue(video, this.lang);
    const text = domText || trackCue?.text || '';

    if (!text) {
      this.emitTimedTextCue(video);
      return;
    }

    const { startMs, endMs } = domText ? this.getActiveCueTiming(video) : trackCue ?? defaultTiming();
    const nativeText = extractNativeCueText(video, this.lang);
    this.emitCue({ text, startMs, endMs, nativeText });
  }

  private ensureNativeCaptionSource(video: HTMLVideoElement | null): void {
    const activeTargetCue = readTargetTextTrackCue(video, this.lang);
    if (!video) return;
    this.resetCaptionRequestState(video);
    if (activeTargetCue?.text || this.hasNativeCaptionText()) return;
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

  private ensureTimedTextTranscript(video: HTMLVideoElement | null): void {
    if (!video) return;

    const response = readYouTubePlayerResponse();
    const videoId = response?.videoDetails?.videoId || currentVideoId();
    const key = `${videoId || location.href}::${this.lang || ''}`;

    if (!response && !videoId) {
      if (this.transcriptKey) {
        this.transcriptKey = '';
        this.transcriptCues = [];
        this.transcriptLoadKey = '';
      }
      return;
    }

    if (key !== this.transcriptKey) {
      this.transcriptKey = key;
      this.transcriptCues = [];
      this.lastCueKey = '';
    }

    if (this.transcriptLoadInFlight || this.transcriptLoadKey === key) return;
    this.transcriptLoadInFlight = true;
    this.transcriptLoadKey = key;

    void loadTranscriptCues(response, this.lang, videoId)
      .then((cues) => {
        if (this.transcriptKey !== key) return;
        this.transcriptCues = cues;
        this.checkSubtitle();
      })
      .catch(() => {
        if (this.transcriptKey === key) this.transcriptCues = [];
      })
      .finally(() => {
        this.transcriptLoadInFlight = false;
      });
  }

  private emitTimedTextCue(video: HTMLVideoElement | null): void {
    if (!video || this.transcriptCues.length === 0) return;
    const currentMs = video.currentTime * 1000;
    const cue = findTimedTextCue(this.transcriptCues, currentMs);
    if (!cue) return;
    this.emitCue(cue);
  }

  private emitCue(cue: ActiveCue): void {
    const key = `${Math.round(cue.startMs)}:${Math.round(cue.endMs)}:${cue.text}`;
    if (key === this.lastCueKey) return;
    this.lastCueKey = key;
    this.overlay.onCue(cue);
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

function findTimedTextCue(cues: ActiveCue[], currentMs: number): ActiveCue | null {
  const readUntilMs = currentMs + TIMED_TEXT_LOOKAHEAD_MS;
  const holdAfterMs = currentMs - TIMED_TEXT_HOLD_MS;
  for (const cue of cues) {
    if (cue.endMs < holdAfterMs) continue;
    if (cue.startMs > readUntilMs) break;
    if (cue.startMs <= readUntilMs && cue.endMs >= holdAfterMs) return cue;
  }
  return null;
}

function selectCaptionTracks(response: YouTubePlayerResponse | null, targetLang: string): SelectedCaptionTrack[] {
  const tracks = response?.captions?.playerCaptionsTracklistRenderer?.captionTracks ?? [];
  const withUrl = tracks.filter((track) => Boolean(track.baseUrl));
  if (withUrl.length === 0) return [];

  const target = primarySubtag(targetLang);
  const selected: SelectedCaptionTrack[] = [];
  if (target) {
    selected.push(
      ...withUrl
        .filter((track) => primarySubtag(track.languageCode) === target)
        .map((track) => ({ track })),
    );

    selected.push(
      ...withUrl
        .filter((track) => primarySubtag(track.languageCode) !== target && track.isTranslatable)
        .map((track) => ({ track, translateTo: target })),
    );
  }

  selected.push(...withUrl.map((track) => ({ track })));
  return dedupeSelectedCaptionTracks(selected);
}

function dedupeSelectedCaptionTracks(selected: SelectedCaptionTrack[]): SelectedCaptionTrack[] {
  const seen = new Set<string>();
  return selected.filter((item) => {
    const key = `${item.track.baseUrl ?? ''}::${item.translateTo ?? ''}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function buildTimedTextUrl(selected: SelectedCaptionTrack): string {
  if (!selected.track.baseUrl) return '';
  try {
    const url = new URL(selected.track.baseUrl, location.href);
    url.searchParams.set('fmt', 'json3');
    if (selected.translateTo) url.searchParams.set('tlang', selected.translateTo);
    return url.toString();
  } catch {
    return '';
  }
}

async function loadTranscriptCues(
  pageResponse: YouTubePlayerResponse | null,
  targetLang: string,
  videoId: string,
): Promise<ActiveCue[]> {
  const pageCues = await loadFirstUsableCaptionTrack(pageResponse, targetLang);
  if (pageCues.length > 0 || !videoId) return pageCues;

  const fallbackResponse = await fetchAndroidPlayerResponse(videoId);
  return loadFirstUsableCaptionTrack(fallbackResponse, targetLang);
}

async function loadFirstUsableCaptionTrack(
  response: YouTubePlayerResponse | null,
  targetLang: string,
): Promise<ActiveCue[]> {
  for (const selected of selectCaptionTracks(response, targetLang)) {
    const url = buildTimedTextUrl(selected);
    if (!url) continue;
    const cues = await loadTimedTextCues(url);
    if (cues.length > 0) return cues;
  }
  return [];
}

async function fetchAndroidPlayerResponse(videoId: string): Promise<YouTubePlayerResponse | null> {
  try {
    const response = await fetch('https://www.youtube.com/youtubei/v1/player?prettyPrint=false', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        'X-YouTube-Client-Name': ANDROID_VR_CLIENT_NAME,
        'X-YouTube-Client-Version': ANDROID_VR_CLIENT_VERSION,
      },
      body: JSON.stringify({
        context: ANDROID_VR_CONTEXT,
        videoId,
      }),
    });
    if (!response.ok) return null;
    return await response.json() as YouTubePlayerResponse;
  } catch {
    return null;
  }
}

async function loadTimedTextCues(url: string): Promise<ActiveCue[]> {
  const response = await fetch(url, { credentials: 'include' });
  if (!response.ok) return [];
  const text = await response.text();

  try {
    return parseJson3TimedText(JSON.parse(text));
  } catch {
    return parseXmlTimedText(text);
  }
}

function parseJson3TimedText(payload: { events?: Array<{ tStartMs?: number; dDurationMs?: number; segs?: Array<{ utf8?: string }> }> }): ActiveCue[] {
  const cues = (payload.events ?? [])
    .map((event) => {
      const startMs = Number(event.tStartMs ?? 0);
      const durationMs = Number(event.dDurationMs ?? 0);
      const text = cleanTimedText(
        (event.segs ?? [])
          .map((segment) => segment.utf8 ?? '')
          .join(''),
      );
      if (!text) return null;
      return {
        text,
        startMs,
        endMs: durationMs > 0 ? startMs + durationMs : startMs + 2000,
      };
    })
    .filter((cue): cue is ActiveCue => cue != null);

  return normalizeTimedTextCueEnds(cues);
}

function parseXmlTimedText(xml: string): ActiveCue[] {
  const doc = new DOMParser().parseFromString(xml, 'text/xml');
  const cues = Array.from(doc.querySelectorAll('text'))
    .map((node) => {
      const startSeconds = Number(node.getAttribute('start') ?? 0);
      const durationSeconds = Number(node.getAttribute('dur') ?? 0);
      const startMs = Math.round(startSeconds * 1000);
      const text = cleanTimedText(node.textContent ?? '');
      if (!text) return null;
      return {
        text,
        startMs,
        endMs: durationSeconds > 0 ? startMs + Math.round(durationSeconds * 1000) : startMs + 2000,
      };
    })
    .filter((cue): cue is ActiveCue => cue != null);

  return normalizeTimedTextCueEnds(cues);
}

function normalizeTimedTextCueEnds(cues: ActiveCue[]): ActiveCue[] {
  const sorted = [...cues].sort((a, b) => a.startMs - b.startMs);
  return sorted.map((cue, index) => {
    const next = sorted[index + 1];
    if (!next || cue.endMs <= cue.startMs || cue.endMs <= next.startMs) return cue;
    return { ...cue, endMs: next.startMs };
  });
}

function cleanTimedText(text: string): string {
  return text
    .replace(/\s+/g, ' ')
    .trim();
}

function readYouTubePlayerResponse(): YouTubePlayerResponse | null {
  const win = window as unknown as {
    ytInitialPlayerResponse?: YouTubePlayerResponse;
    ytplayer?: { config?: { args?: { player_response?: string } } };
  };

  if (win.ytInitialPlayerResponse) return win.ytInitialPlayerResponse;

  const playerResponse = win.ytplayer?.config?.args?.player_response;
  if (playerResponse) {
    try {
      return JSON.parse(playerResponse) as YouTubePlayerResponse;
    } catch {
      // Fall through to script-tag extraction below.
    }
  }

  for (const script of Array.from(document.scripts)) {
    const text = script.textContent ?? '';
    if (!text.includes('ytInitialPlayerResponse')) continue;
    const json = extractAssignedJson(text, 'ytInitialPlayerResponse');
    if (!json) continue;
    try {
      return JSON.parse(json) as YouTubePlayerResponse;
    } catch {
      // Try the next script tag; YouTube occasionally ships helper snippets
      // containing the same identifier.
    }
  }

  return null;
}

function extractAssignedJson(source: string, marker: string): string | null {
  let markerIndex = source.indexOf(marker);
  while (markerIndex >= 0) {
    const start = source.indexOf('{', markerIndex + marker.length);
    if (start >= 0) {
      const json = extractBalancedObject(source, start);
      if (json) return json;
    }
    markerIndex = source.indexOf(marker, markerIndex + marker.length);
  }
  return null;
}

function extractBalancedObject(source: string, start: number): string | null {
  let depth = 0;
  let inString = false;
  let quote = '';
  let escaped = false;

  for (let i = start; i < source.length; i++) {
    const ch = source[i]!;

    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (ch === '\\') {
        escaped = true;
      } else if (ch === quote) {
        inString = false;
      }
      continue;
    }

    if (ch === '"' || ch === "'") {
      inString = true;
      quote = ch;
      continue;
    }

    if (ch === '{') depth += 1;
    if (ch === '}') {
      depth -= 1;
      if (depth === 0) return source.slice(start, i + 1);
    }
  }

  return null;
}

function primarySubtag(lang: string | undefined | null): string {
  if (!lang) return '';
  return lang.split('-')[0]!.trim().toLowerCase();
}

function currentVideoId(): string {
  if (location.hostname === 'youtu.be') return location.pathname.replace(/^\/+/, '');
  return new URLSearchParams(location.search).get('v') ?? '';
}

function defaultTiming(): { startMs: number; endMs: number } {
  const video = document.querySelector<HTMLVideoElement>('video.html5-main-video, video.video-stream');
  const t = video ? video.currentTime * 1000 : 0;
  return { startMs: Math.max(0, t - 2000), endMs: t + 2000 };
}
