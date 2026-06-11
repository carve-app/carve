export interface TextTrackCueSnapshot {
  text: string;
  startMs: number;
  endMs: number;
}

function primarySubtag(lang: string | undefined | null): string {
  if (!lang) return '';
  return lang.split('-')[0]!.trim().toLowerCase();
}

function cleanCueText(raw: string): string {
  return raw
    .replace(/<[^>]*>/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

function isSubtitleTrack(track: TextTrack): boolean {
  return track.kind === 'subtitles' || track.kind === 'captions';
}

function firstCue(track: TextTrack): VTTCue | null {
  const cues = track.activeCues;
  if (!cues || cues.length === 0) return null;
  return cues[0] as VTTCue;
}

function cueSnapshot(track: TextTrack): TextTrackCueSnapshot | null {
  const cues = track.activeCues;
  if (!cues || cues.length === 0) return null;

  const parts: string[] = [];
  let start = Number.POSITIVE_INFINITY;
  let end = 0;

  for (let i = 0; i < cues.length; i++) {
    const cue = cues[i] as VTTCue;
    const text = cleanCueText(cue.text ?? '');
    if (text) parts.push(text);
    start = Math.min(start, cue.startTime);
    end = Math.max(end, cue.endTime);
  }

  const text = parts.join(' ').trim();
  if (!text) return null;
  return {
    text,
    startMs: Number.isFinite(start) ? Math.round(start * 1000) : 0,
    endMs: end > 0 ? Math.round(end * 1000) : 0,
  };
}

function chooseTargetTrack(video: HTMLVideoElement | null, targetLang: string): TextTrack | null {
  if (!video?.textTracks?.length) return null;
  const target = primarySubtag(targetLang);
  let unlabeled: TextTrack | null = null;

  for (let i = 0; i < video.textTracks.length; i++) {
    const track = video.textTracks[i];
    if (!isSubtitleTrack(track)) continue;

    const lang = primarySubtag(track.language);
    if (target && lang === target) return track;
    if (!lang && !unlabeled) unlabeled = track;
  }

  return unlabeled;
}

export function hasTargetTextTrack(video: HTMLVideoElement | null, targetLang: string): boolean {
  return chooseTargetTrack(video, targetLang) != null;
}

export function readTargetTextTrackCue(
  video: HTMLVideoElement | null,
  targetLang: string,
): TextTrackCueSnapshot | null {
  const track = chooseTargetTrack(video, targetLang);
  if (!track) return null;

  if (track.mode === 'disabled') {
    try {
      track.mode = 'hidden';
    } catch {
      // Some streaming players expose read-only track objects. In that case,
      // fall through and use active cues if the browser still provides them.
    }
  }

  return cueSnapshot(track);
}

export function readAnyActiveTextTrackTiming(video: HTMLVideoElement | null): { startMs: number; endMs: number } | null {
  if (!video?.textTracks?.length) return null;

  for (let i = 0; i < video.textTracks.length; i++) {
    const track = video.textTracks[i];
    if (!isSubtitleTrack(track)) continue;
    if (track.mode !== 'showing' && track.mode !== 'hidden') continue;
    const cue = firstCue(track);
    if (!cue) continue;
    return {
      startMs: Math.round(cue.startTime * 1000),
      endMs: Math.round(cue.endTime * 1000),
    };
  }

  return null;
}
