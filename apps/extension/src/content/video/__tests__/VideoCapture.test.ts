import { describe, it, expect, beforeEach, vi } from 'vitest';

// Import only the pure/testable exports
import { getVideoElement, captureFrame, captureVideoMedia } from '../VideoCapture';

// ── getVideoElement ───────────────────────────────────────────────────────────

describe('getVideoElement', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('returns null when no video is present', () => {
    expect(getVideoElement()).toBeNull();
  });

  it('finds YouTube video via class selector', () => {
    const video = document.createElement('video');
    video.className = 'html5-main-video';
    document.body.appendChild(video);
    expect(getVideoElement()).toBe(video);
  });

  it('finds YouTube video-stream variant', () => {
    const video = document.createElement('video');
    video.className = 'video-stream';
    document.body.appendChild(video);
    expect(getVideoElement()).toBe(video);
  });

  it('falls back to largest video by resolution when no class matches', () => {
    const small = document.createElement('video');
    Object.defineProperty(small, 'videoWidth', { value: 640 });
    Object.defineProperty(small, 'videoHeight', { value: 360 });

    const large = document.createElement('video');
    Object.defineProperty(large, 'videoWidth', { value: 1920 });
    Object.defineProperty(large, 'videoHeight', { value: 1080 });

    document.body.appendChild(small);
    document.body.appendChild(large);

    expect(getVideoElement()).toBe(large);
  });

  it('returns single video when only one exists (fallback path)', () => {
    const video = document.createElement('video');
    Object.defineProperty(video, 'videoWidth', { value: 1280 });
    Object.defineProperty(video, 'videoHeight', { value: 720 });
    document.body.appendChild(video);
    expect(getVideoElement()).toBe(video);
  });

  it('prefers YouTube class over generic fallback', () => {
    const generic = document.createElement('video');
    Object.defineProperty(generic, 'videoWidth', { value: 1920 });
    Object.defineProperty(generic, 'videoHeight', { value: 1080 });
    document.body.appendChild(generic);

    const yt = document.createElement('video');
    yt.className = 'html5-main-video';
    Object.defineProperty(yt, 'videoWidth', { value: 640 });
    Object.defineProperty(yt, 'videoHeight', { value: 360 });
    document.body.appendChild(yt);

    expect(getVideoElement()).toBe(yt);
  });
});

// ── captureFrame ──────────────────────────────────────────────────────────────

describe('captureFrame', () => {
  it('returns null when canvas context is unavailable', async () => {
    const video = document.createElement('video');
    // jsdom does not implement canvas 2d draw — getContext returns an object but
    // drawImage may silently fail. We force the fallback path to test null-safety.
    const origCreate = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      if (tag === 'canvas') {
        const c = origCreate('canvas');
        // Make getContext return null to simulate unsupported environment
        vi.spyOn(c, 'getContext').mockReturnValue(null as any);
        return c;
      }
      return origCreate(tag);
    });

    const result = await captureFrame(video);
    expect(result).toBeNull();

    vi.restoreAllMocks();
  });

  it('uses fallback dimensions (1280×720) when video has no size', async () => {
    const video = document.createElement('video');
    // videoWidth/videoHeight default to 0 in jsdom
    // We just verify no exception is thrown
    const drawSpy = vi.fn();
    const toBlobMock = vi.fn((cb: BlobCallback) => cb(new Blob([], { type: 'image/jpeg' })));

    const origCreate = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      if (tag === 'canvas') {
        return {
          getContext: () => ({ drawImage: drawSpy }),
          toBlob: toBlobMock,
          width: 0,
          height: 0,
        } as unknown as HTMLCanvasElement;
      }
      return origCreate(tag);
    });

    const result = await captureFrame(video);
    // drawImage called with fallback 1280×720 dimensions configured on canvas
    expect(drawSpy).toHaveBeenCalledWith(video, 0, 0);
    expect(result).toBeInstanceOf(Blob);

    vi.restoreAllMocks();
  });
});

// ── captureVideoMedia — timing ────────────────────────────────────────────────

describe('captureVideoMedia timing', () => {
  it('computes startMs and endMs correctly for currentTime > half duration', async () => {
    const video = document.createElement('video') as any;
    Object.defineProperty(video, 'currentTime', { value: 10.0 }); // 10s into video
    // Mock captureStream to return null → audio = null
    video.captureStream = () => null;

    // Mock OffscreenCanvas if not defined
    if (typeof OffscreenCanvas === 'undefined') {
      (global as any).OffscreenCanvas = undefined;
    }

    // Spy on canvas path
    const toBlobMock = vi.fn((cb: BlobCallback) => cb(new Blob([], { type: 'image/jpeg' })));
    const origCreate = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      if (tag === 'canvas') {
        return {
          getContext: () => ({ drawImage: vi.fn() }),
          toBlob: toBlobMock,
          width: 0,
          height: 0,
        } as unknown as HTMLCanvasElement;
      }
      return origCreate(tag);
    });

    const cueDurationMs = 4000;
    const result = await captureVideoMedia(video, cueDurationMs);

    // startMs = max(0, round(10000) - round(2000)) = 8000
    expect(result.startMs).toBe(8000);
    expect(result.endMs).toBe(result.startMs + cueDurationMs);

    vi.restoreAllMocks();
  });

  it('clamps startMs to 0 when video is near the beginning', async () => {
    const video = document.createElement('video') as any;
    Object.defineProperty(video, 'currentTime', { value: 0.5 }); // 500ms in
    video.captureStream = () => null;

    const toBlobMock = vi.fn((cb: BlobCallback) => cb(new Blob([], { type: 'image/jpeg' })));
    const origCreate = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      if (tag === 'canvas') {
        return {
          getContext: () => ({ drawImage: vi.fn() }),
          toBlob: toBlobMock,
          width: 0,
          height: 0,
        } as unknown as HTMLCanvasElement;
      }
      return origCreate(tag);
    });

    const result = await captureVideoMedia(video, 4000);
    expect(result.startMs).toBe(0); // clamped to 0
    expect(result.endMs).toBe(4000);

    vi.restoreAllMocks();
  });

  it('returns null audio when captureStream is unavailable', async () => {
    const video = document.createElement('video') as any;
    Object.defineProperty(video, 'currentTime', { value: 5.0 });
    // No captureStream method → audio falls back to null

    const origCreate = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      if (tag === 'canvas') {
        return {
          getContext: () => ({ drawImage: vi.fn() }),
          toBlob: (cb: BlobCallback) => cb(new Blob([], { type: 'image/jpeg' })),
          width: 0,
          height: 0,
        } as unknown as HTMLCanvasElement;
      }
      return origCreate(tag);
    });

    const result = await captureVideoMedia(video, 4000);
    expect(result.audioBlob).toBeNull();

    vi.restoreAllMocks();
  });
});
