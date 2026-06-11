/**
 * Tests for the exact-sentence capture contract of attachVideoMedia().
 *
 * These pin the behaviors the audit found broken in the old "record forward
 * from the playhead" implementation:
 *   - the FRAME and AUDIO are captured at the CUE's source timing, not wherever
 *     the playhead currently sits (history navigation / pause / latency),
 *   - the stored subtitle_start_ms/end_ms equal the cue window,
 *   - the user's original playhead + paused state are restored,
 *   - an invisible (zero-size) video never triggers a frame capture,
 *   - hasImage/hasAudio reflect the SERVER response, not the local blobs.
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';

const sendMessage = vi.hoisted(() => vi.fn());

vi.mock('../../../shared/browser', () => ({
  browser: { runtime: { sendMessage } },
}));

import { attachVideoMedia } from '../VideoCapture';

// A fake <video> that records seeks and play/pause, and fires `seeked`.
function makeVideo(opts: { currentTime?: number; paused?: boolean; rect?: { width: number; height: number }; withStream?: boolean } = {}) {
  const listeners: Record<string, Array<() => void>> = {};
  const seeks: number[] = [];
  let _time = opts.currentTime ?? 12;
  let _paused = opts.paused ?? true;
  const playCalls: number[] = [];
  const pauseCalls: number[] = [];

  const video: any = {
    get currentTime() { return _time; },
    set currentTime(v: number) {
      _time = v;
      seeks.push(v);
      // Fire `seeked` asynchronously like a real element.
      queueMicrotask(() => (listeners['seeked'] ?? []).forEach((fn) => fn()));
    },
    get paused() { return _paused; },
    play() { _paused = false; playCalls.push(_time); return Promise.resolve(); },
    pause() { _paused = true; pauseCalls.push(_time); },
    addEventListener(ev: string, fn: () => void) { (listeners[ev] ??= []).push(fn); },
    removeEventListener(ev: string, fn: () => void) {
      listeners[ev] = (listeners[ev] ?? []).filter((f) => f !== fn);
    },
    getBoundingClientRect() {
      const r = opts.rect ?? { width: 480, height: 270 };
      return { left: 40, top: 50, width: r.width, height: r.height, right: 40 + r.width, bottom: 50 + r.height };
    },
    textTracks: [],
  };
  if (opts.withStream) {
    video.captureStream = () => ({ getAudioTracks: () => [] }); // no audio tracks → null audio
  }
  return { video, seeks, playCalls, pauseCalls };
}

describe('attachVideoMedia — exact-cue capture', () => {
  beforeEach(() => {
    sendMessage.mockReset();
    (globalThis as any).window = { devicePixelRatio: 1 };
  });

  it('captures the FRAME at the cue start (not the current playhead) and restores position', async () => {
    // Server reports image stored, no audio.
    sendMessage.mockImplementation(async (msg: any) => {
      if (msg.type === 'CAPTURE_VIDEO_FRAME') return { imageBase64: 'AAAA' };
      if (msg.type === 'ATTACH_VIDEO_MEDIA') return { success: true, hasImage: true, hasAudio: false };
      return {};
    });

    // Playhead is at 12s; we mine a cue at [5s, 8s].
    const { video, seeks } = makeVideo({ currentTime: 12, paused: false });

    const res = await attachVideoMedia(video, 'card-1', { startMs: 5000, endMs: 8000 });

    // It seeked to the cue start (5s) before capturing, then back to 12s.
    expect(seeks[0]).toBeCloseTo(5, 3);
    expect(seeks[seeks.length - 1]).toBeCloseTo(12, 3);

    // The ATTACH message carried the CUE window, not the playhead-derived one.
    const attach = sendMessage.mock.calls.map((c) => c[0]).find((m) => m.type === 'ATTACH_VIDEO_MEDIA');
    expect(attach.startMs).toBe(5000);
    expect(attach.endMs).toBe(8000);

    expect(res.hasImage).toBe(true);
    expect(res.success).toBe(true);
  });

  it('does NOT request a frame when the video element is invisible (zero-size)', async () => {
    sendMessage.mockImplementation(async (msg: any) => {
      if (msg.type === 'ATTACH_VIDEO_MEDIA') return { success: false, hasImage: false, hasAudio: false, error: 'no media capturable' };
      return {};
    });
    const { video } = makeVideo({ rect: { width: 0, height: 0 } });

    await attachVideoMedia(video, 'card-2', { startMs: 1000, endMs: 3000 });

    const sentFrame = sendMessage.mock.calls.some((c) => c[0].type === 'CAPTURE_VIDEO_FRAME');
    expect(sentFrame).toBe(false);
  });

  it('reports hasImage/hasAudio from the SERVER response, not local blobs', async () => {
    // We "sent" an image, but the server says it did not persist (e.g. 413).
    sendMessage.mockImplementation(async (msg: any) => {
      if (msg.type === 'CAPTURE_VIDEO_FRAME') return { imageBase64: 'AAAA' };
      if (msg.type === 'ATTACH_VIDEO_MEDIA') return { success: false, hasImage: false, hasAudio: false, error: 'media upload failed: image' };
      return {};
    });
    const { video } = makeVideo();

    const res = await attachVideoMedia(video, 'card-3', { startMs: 2000, endMs: 5000 });
    expect(res.hasImage).toBe(false);
    expect(res.success).toBe(false);
    expect(res.error).toMatch(/upload failed/);
  });

  it('restores a paused video to paused after capturing', async () => {
    sendMessage.mockImplementation(async (msg: any) => {
      if (msg.type === 'CAPTURE_VIDEO_FRAME') return { imageBase64: 'AAAA' };
      if (msg.type === 'ATTACH_VIDEO_MEDIA') return { success: true, hasImage: true, hasAudio: false };
      return {};
    });
    const { video } = makeVideo({ currentTime: 30, paused: true, withStream: true });

    await attachVideoMedia(video, 'card-4', { startMs: 10000, endMs: 13000 });

    // Ended paused (it was paused before).
    expect(video.paused).toBe(true);
    // And restored to its original position.
    expect(video.currentTime).toBeCloseTo(30, 3);
  });

  it('clamps an absurd cue duration to a sane recording window', async () => {
    let attachMsg: any = null;
    sendMessage.mockImplementation(async (msg: any) => {
      if (msg.type === 'CAPTURE_VIDEO_FRAME') return { imageBase64: 'AAAA' };
      if (msg.type === 'ATTACH_VIDEO_MEDIA') { attachMsg = msg; return { success: true, hasImage: true, hasAudio: false }; }
      return {};
    });
    const { video } = makeVideo();

    // A malformed cue spanning the whole track (0 → 9999s).
    await attachVideoMedia(video, 'card-5', { startMs: 0, endMs: 9_999_000 });

    expect(attachMsg.endMs - attachMsg.startMs).toBeLessThanOrEqual(12000);
  });
});
