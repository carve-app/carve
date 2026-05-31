/// <reference types="chrome" />

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
    chrome.storage.local.get('accessToken'),
    chrome.storage.local.get('apiBaseUrl'),
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
