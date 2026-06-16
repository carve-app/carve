import { browser } from '../../shared/browser';

export interface CaptureResult {
  imageBlob: Blob | null;
  audioBlob: Blob | null;
  startMs: number;
  endMs: number;
}

/**
 * Finds the <video> element for the current platform.
 * Netflix embeds inside a shadow root; YouTube uses `.video-stream`.
 */
export function getVideoElement(): HTMLVideoElement | null {
  // YouTube
  const yt = document.querySelector<HTMLVideoElement>('video.html5-main-video, video.video-stream');
  if (yt) return yt;

  // Netflix — video lives inside shadow DOM of the player container
  const nfContainer = document.querySelector('.watch-video--player-view, .NFPlayer');
  if (nfContainer) {
    const all = nfContainer.querySelectorAll<HTMLVideoElement>('video');
    if (all.length) return all[0];
  }

  // Generic fallback: largest video by rendered size
  const videos = Array.from(document.querySelectorAll<HTMLVideoElement>('video'));
  if (!videos.length) return null;
  return videos.reduce((best, v) =>
    v.videoWidth * v.videoHeight > best.videoWidth * best.videoHeight ? v : best,
  );
}

/**
 * Captures the current video frame as a JPEG blob.
 */
export async function captureFrame(video: HTMLVideoElement): Promise<Blob | null> {
  try {
    const w = video.videoWidth || 1280;
    const h = video.videoHeight || 720;

    if (typeof OffscreenCanvas !== 'undefined') {
      const canvas = new OffscreenCanvas(w, h);
      const ctx = canvas.getContext('2d') as OffscreenCanvasRenderingContext2D | null;
      if (!ctx) return null;
      ctx.drawImage(video, 0, 0);
      return await canvas.convertToBlob({ type: 'image/jpeg', quality: 0.8 });
    }

    // Fallback: regular canvas (works in content scripts)
    const canvas = document.createElement('canvas');
    canvas.width = w;
    canvas.height = h;
    const ctx2 = canvas.getContext('2d');
    if (!ctx2) return null;
    ctx2.drawImage(video, 0, 0);
    return new Promise(resolve => canvas.toBlob(resolve, 'image/jpeg', 0.8));
  } catch {
    return null;
  }
}

/**
 * Records audio from the video element for `durationMs` milliseconds.
 * Returns a webm blob (or null if capture is unavailable/blocked).
 */
export function recordAudioClip(
  video: HTMLVideoElement,
  durationMs: number,
): Promise<Blob | null> {
  return new Promise(resolve => {
    try {
      // captureStream() is non-standard but supported in Chrome/Firefox
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const stream: MediaStream = (video as any).captureStream?.() ?? (video as any).mozCaptureStream?.();
      if (!stream) { resolve(null); return; }

      const audioTracks = stream.getAudioTracks();
      if (!audioTracks.length) { resolve(null); return; }

      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : 'audio/webm';

      const recorder = new MediaRecorder(new MediaStream(audioTracks), { mimeType });
      const chunks: Blob[] = [];

      recorder.ondataavailable = e => { if (e.data.size > 0) chunks.push(e.data); };
      recorder.onstop = () => resolve(new Blob(chunks, { type: 'audio/webm' }));
      recorder.onerror = () => resolve(null);

      recorder.start();
      setTimeout(() => {
        if (recorder.state !== 'inactive') recorder.stop();
      }, durationMs);
    } catch {
      resolve(null);
    }
  });
}

/**
 * Captures frame + audio simultaneously.
 * The audio clip length is `cueDurationMs` (default 4 seconds).
 */
export async function captureVideoMedia(
  video: HTMLVideoElement,
  cueDurationMs = 4000,
): Promise<CaptureResult> {
  const startMs = Math.max(0, Math.round(video.currentTime * 1000) - Math.round(cueDurationMs / 2));
  const endMs = startMs + cueDurationMs;

  // Start audio recording first (needs time)
  const audioProm = recordAudioClip(video, cueDurationMs);
  // Capture frame immediately
  const imageBlob = await captureFrame(video);

  return {
    imageBlob,
    audioBlob: await audioProm,
    startMs,
    endMs,
  };
}

/**
 * Uploads captured media blobs to POST /v1/cards/{id}/media.
 */
export async function uploadCardMedia(
  cardId: string,
  capture: CaptureResult,
  opts: { sourceUrl?: string; subtitleTranslation?: string } = {},
): Promise<void> {
  const [{ accessToken }, { apiBaseUrl }] = await Promise.all([
    browser.storage.local.get('accessToken'),
    browser.storage.local.get('apiBaseUrl'),
  ]);
  const base = (apiBaseUrl as string | undefined) ?? 'http://localhost:8080';
  const token = accessToken as string | undefined;
  if (!token) return;

  const form = new FormData();
  if (capture.imageBlob) form.append('image', capture.imageBlob, 'frame.jpg');
  if (capture.audioBlob) form.append('audio', capture.audioBlob, 'clip.webm');
  form.append('subtitle_start_ms', String(capture.startMs));
  form.append('subtitle_end_ms', String(capture.endMs));
  if (opts.sourceUrl) form.append('video_source_url', opts.sourceUrl);
  if (opts.subtitleTranslation) form.append('subtitle_translation', opts.subtitleTranslation);

  await fetch(`${base}/v1/cards/${cardId}/media`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  }).catch(() => {/* non-fatal */});
}

// ── DRM-resilient capture path ────────────────────────────────────────────────
//
// captureFrame() above draws the <video> to a canvas, which the browser blanks
// to black on EME/DRM-protected streams (Netflix, Disney+, Prime). The path
// below instead records audio in the page (captureStream — works wherever the
// media element exposes a stream) and hands the *frame* capture to the
// background worker, which screenshots the rendered tab via captureVisibleTab
// and crops to the video rect. That composited screenshot includes DRM video,
// matching how Migaku captures Netflix frames.

function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => {
      const result = reader.result as string;
      // result is a data URL: "data:<mime>;base64,<payload>"
      const comma = result.indexOf(',');
      resolve(comma >= 0 ? result.slice(comma + 1) : '');
    };
    reader.onerror = () => reject(reader.error ?? new Error('read failed'));
    reader.readAsDataURL(blob);
  });
}

/**
 * Seek `video` to `timeSec` and resolve once the frame at that position is
 * actually painted (the `seeked` event). Falls back to a timeout so a media
 * element that never fires `seeked` (e.g. an unsourced <video> in a test, or a
 * platform that swallows the event) can't hang the mining flow.
 *
 * Resolves to `true` only if the playhead actually landed near `timeSec`. A
 * live stream with no seekable window (YouTube Live, Twitch) never moves, so it
 * resolves `false` — letting the caller avoid capturing a frame/audio from the
 * wrong (current) moment and silently corrupting the card.
 */
function seekTo(video: HTMLVideoElement, timeSec: number, timeoutMs = 1500): Promise<boolean> {
  return new Promise(resolve => {
    const target = Math.max(0, timeSec);
    const landed = () => Math.abs(video.currentTime - target) < 0.35;
    // Already there (within a frame) — nothing to wait for.
    if (landed()) { resolve(true); return; }

    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      video.removeEventListener('seeked', finish);
      resolve(landed());
    };
    video.addEventListener('seeked', finish, { once: true });
    try {
      video.currentTime = target;
    } catch {
      finish();
      return;
    }
    setTimeout(finish, timeoutMs);
  });
}

function seekabilityAt(video: HTMLVideoElement, timeSec: number): 'seekable' | 'unseekable' | 'unknown' {
  const ranges = video.seekable;
  if (!ranges || ranges.length === 0) return 'unknown';

  for (let i = 0; i < ranges.length; i++) {
    const start = ranges.start(i);
    const end = ranges.end(i);
    if (end - start < 0.25) continue;
    if (timeSec >= start - 0.25 && timeSec <= end + 0.25) return 'seekable';
  }

  return 'unseekable';
}

function isInsideCueWindow(timeSec: number, startMs: number, endMs: number): boolean {
  const timeMs = timeSec * 1000;
  return timeMs >= startMs - 250 && timeMs <= endMs + 250;
}

/**
 * Record the EXACT audio of a subtitle cue.
 *
 * The cue's audio only exists while the element is *playing* — a paused
 * <video>'s captureStream() yields a stream with no flowing audio (so the
 * original "record forward from the playhead" code produced silence whenever
 * the user paused to read before mining). The caller seeks the element to the
 * cue start first; here we start playback, capture the live stream for
 * `durationMs`, then stop. The caller restores the playhead + paused state.
 */
async function recordCueAudio(video: HTMLVideoElement, durationMs: number): Promise<Blob | null> {
  // captureStream() is non-standard but supported in Chrome/Firefox.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const stream: MediaStream | undefined =
    (video as any).captureStream?.() ?? (video as any).mozCaptureStream?.();
  if (!stream) return null;

  // Kick playback so audio frames actually flow into the captured stream. A
  // paused element yields a stream with no live audio (the original bug:
  // mining while paused-on-subtitle recorded pure silence).
  try {
    const playResult = video.play();
    if (playResult && typeof playResult.catch === 'function') {
      await playResult.catch(() => {/* autoplay may be blocked; try recording anyway */});
    }
  } catch {/* play() may throw synchronously in odd states */}

  // Audio tracks appear only once the element starts producing audio — poll
  // briefly rather than giving up on the first (often empty) read.
  let audioTracks = stream.getAudioTracks();
  for (let i = 0; i < 20 && audioTracks.length === 0; i++) {
    await new Promise(r => setTimeout(r, 50));
    audioTracks = stream.getAudioTracks();
  }
  if (audioTracks.length === 0) return null;

  return new Promise<Blob | null>(resolve => {
    let settled = false;
    const done = (b: Blob | null) => { if (!settled) { settled = true; resolve(b); } };
    try {
      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : 'audio/webm';
      const recorder = new MediaRecorder(new MediaStream(audioTracks), { mimeType });
      const chunks: Blob[] = [];
      recorder.ondataavailable = e => { if (e.data.size > 0) chunks.push(e.data); };
      recorder.onstop = () => done(chunks.length ? new Blob(chunks, { type: 'audio/webm' }) : null);
      recorder.onerror = () => done(null);

      recorder.start();
      setTimeout(() => {
        if (recorder.state !== 'inactive') recorder.stop();
      }, durationMs);
      // Hard safety net: never let a stuck recorder hang the mining flow.
      setTimeout(() => done(null), durationMs + 4000);
    } catch {
      done(null);
    }
  });
}

export interface VideoMediaResult {
  success: boolean;
  hasImage: boolean;
  hasAudio: boolean;
  error?: string;
}

/**
 * Capture screenshot + EXACT-sentence audio for a mined video card and attach
 * them to `cardId`.
 *
 * `cue` carries the subtitle's real source timing (from the platform's
 * textTracks), which is what makes the capture *exact* regardless of where the
 * playhead currently sits — the user may have scrolled back through subtitle
 * history, or paused seconds ago, or be watching live. We:
 *
 *   1. remember the user's position + paused state,
 *   2. seek to the cue start and grab the FRAME there (paused, so autoplay
 *      can't advance past the mined moment — and DRM-safe via the worker),
 *   3. play from the cue start and record the AUDIO for the cue's duration,
 *   4. restore the user's original position + paused state.
 *
 * The stored subtitle_start_ms/subtitle_end_ms are the cue's true source
 * timing, so the card links back to the exact moment in the video.
 *
 * Returns what actually landed (per the server) so the caller can give honest
 * feedback ("media unavailable on this site").
 */
export async function attachVideoMedia(
  video: HTMLVideoElement,
  cardId: string,
  cue: { startMs: number; endMs: number },
  opts: { sourceUrl?: string; subtitleTranslation?: string } = {},
): Promise<VideoMediaResult> {
  // The stored window is the cue's TRUE source timing. Audio is recorded over
  // this same window, so content and metadata always agree.
  const startMs = Math.max(0, Math.round(cue.startMs));
  const rawDuration = Math.round(cue.endMs - cue.startMs);
  // Clamp the recorded clip to a sane length (a malformed/whole-track cue must
  // not trigger a multi-minute recording).
  const duration = Math.min(Math.max(rawDuration || 4000, 1000), 12000);
  const endMs = startMs + duration;

  const originalTime = video.currentTime;
  const wasPaused = video.paused;
  const cueStartSec = startMs / 1000;
  // Some progressive/video-fixture streams play normally but report no useful
  // seekable range. If the user mines while already inside the cue, keep the
  // current rendered moment instead of seeking backward and losing all media.
  const captureAtCurrentCueMoment =
    isInsideCueWindow(originalTime, startMs, endMs)
    && seekabilityAt(video, cueStartSec) === 'unseekable';

  let imageBase64: string | null = null;
  let audioBlob: Blob | null = null;
  try {
    // 1) Move to the cue start so both frame and audio reflect the mined
    //    sentence — not wherever the playhead drifted to. If the cue start is
    //    unseekable but the playhead is already inside this cue, capture the
    //    current rendered sentence moment instead of throwing away media.
    const canCaptureMedia = captureAtCurrentCueMoment || await seekTo(video, cueStartSec);

    if (canCaptureMedia) {
      // 2) Capture the frame at the cue start, while paused. Skip entirely when
      //    the video element is not actually rendered (zero-size / display:none)
      //    so we never fall back to a whole-viewport screenshot of unrelated
      //    page chrome.
      const r = video.getBoundingClientRect();
      if (r.width >= 16 && r.height >= 16) {
        const rect = { x: r.left, y: r.top, width: r.width, height: r.height };
        const dpr = window.devicePixelRatio || 1;
        const frameResult = await browser.runtime.sendMessage({ type: 'CAPTURE_VIDEO_FRAME', rect, dpr });
        imageBase64 = frameResult?.imageBase64 ?? null;
      }

      // 3) Record cue audio. In the normal path this plays from the cue start;
      //    in the unseekable-current-cue fallback it records the remaining cue.
      if (!captureAtCurrentCueMoment || !wasPaused) {
        const recordDuration = captureAtCurrentCueMoment
          ? Math.min(duration, Math.max(Math.round(endMs - originalTime * 1000), 1000))
          : duration;
        audioBlob = await recordCueAudio(video, recordDuration);
      }
    }
  } finally {
    // 4) ALWAYS restore the user exactly where they were — even if frame or
    //    audio capture threw. Leaving the real player seeked-away or in the
    //    wrong play state would be a jarring, visible regression.
    if (!captureAtCurrentCueMoment) {
      try {
        await seekTo(video, originalTime);
      } catch {/* best-effort restore */}
    }
    if (wasPaused) {
      video.pause();
    } else {
      const p = video.play();
      if (p && typeof p.catch === 'function') {
        p.catch((err: unknown) => {
          // Restoring playback can be blocked by autoplay policy on some
          // embeds. Nothing we can do programmatically (no user gesture left),
          // but surface non-autoplay failures for debugging rather than
          // swallowing every error.
          const name = (err as { name?: string } | null)?.name;
          if (name !== 'NotAllowedError' && name !== 'AbortError') {
            console.warn('carve: could not resume video playback after mining', err);
          }
        });
      }
    }
  }

  let audioBase64: string | null = null;
  let audioMime: string | null = null;
  if (audioBlob && audioBlob.size > 0) {
    try {
      audioBase64 = await blobToBase64(audioBlob);
      audioMime = audioBlob.type || 'audio/webm';
    } catch {
      audioBase64 = null;
    }
  }

  // Upload the already-captured frame + audio through the background worker
  // (which holds the host permission, sidestepping the content-script CORS
  // wall in prod). hasImage/hasAudio come back from the SERVER, reflecting what
  // actually persisted — not just what we sent.
  const result = await browser.runtime.sendMessage({
    type: 'ATTACH_VIDEO_MEDIA',
    cardId,
    imageBase64,
    audioBase64,
    audioMime,
    startMs,
    endMs,
    sourceUrl: opts.sourceUrl,
    subtitleTranslation: opts.subtitleTranslation,
  });

  return {
    success: !!result?.success,
    hasImage: !!result?.hasImage,
    hasAudio: !!result?.hasAudio,
    error: result?.error,
  };
}
