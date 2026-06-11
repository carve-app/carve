/**
 * Shared helper for capturing the *native-language* (dual) subtitle line at
 * cue time.
 *
 * Migaku-style dual subtitles show the learner's target language plus their
 * native language simultaneously. The target line is read from each platform's
 * on-page subtitle DOM (see the platform hooks). The native line, when the
 * viewer has enabled a second subtitle track, is exposed by the browser as an
 * additional `TextTrack` on the <video> element. This helper reads the active
 * cue text of that native track so the hooks can pass it into `onCue` and the
 * overlay can render both lines live (and mined cards carry the human
 * translation).
 *
 * This is deliberately a standalone module (not a method on SubtitleOverlay)
 * so the platform hooks can call it at cue time without importing the overlay
 * and creating an import cycle.
 */

/** Normalize a BCP-47 language tag to its lowercased primary subtag. */
function primarySubtag(lang: string | undefined | null): string {
  if (!lang) return '';
  return lang.split('-')[0]!.trim().toLowerCase();
}

/** Strip HTML/VTT markup and collapse whitespace from a cue payload. */
function cleanCueText(raw: string): string {
  return raw
    .replace(/<[^>]*>/g, '') // VTT/HTML tags, e.g. <c>, <v Bob>, <b>
    .replace(/\s+/g, ' ')
    .trim();
}

/** Read the concatenated text of a track's currently-active cues. */
function activeCueText(track: TextTrack): string {
  const cues = track.activeCues;
  if (!cues || cues.length === 0) return '';
  const parts: string[] = [];
  for (let i = 0; i < cues.length; i++) {
    const cue = cues[i] as VTTCue;
    const text = cleanCueText(cue.text ?? '');
    if (text) parts.push(text);
  }
  return parts.join(' ').trim();
}

/**
 * Scan a video element's text tracks for a *native-language* track (one whose
 * language differs from the learner's target language) that is currently
 * displaying cues, and return its active-cue text.
 *
 * Returns `undefined` when no such track is active so the caller can leave
 * `ActiveCue.nativeText` unset (the overlay handles a missing native line).
 *
 * @param video      the platform's <video> element, or null
 * @param targetLang the learner's target-language code (e.g. "ja"); the native
 *                   track is any *showing* track whose primary subtag differs.
 */
export function extractNativeCueText(
  video: HTMLVideoElement | null,
  targetLang: string,
): string | undefined {
  if (!video) return undefined;
  const tracks = video.textTracks;
  if (!tracks || tracks.length === 0) return undefined;

  const target = primarySubtag(targetLang);

  // Prefer a track that is explicitly a different language from the target.
  // Fall back to any non-target showing track if language metadata is absent
  // but there is clearly a second showing subtitle/captions track.
  let fallback: string | undefined;

  for (let i = 0; i < tracks.length; i++) {
    const track = tracks[i];
    // 'showing' tracks render cues; 'hidden' tracks still populate activeCues
    // without drawing, which some players use for the secondary line.
    if (track.mode !== 'showing' && track.mode !== 'hidden') continue;
    if (track.kind !== 'subtitles' && track.kind !== 'captions') continue;

    const lang = primarySubtag(track.language);
    const text = activeCueText(track);
    if (!text) continue;

    if (lang && target && lang !== target) {
      // Definite native track: language is known and differs from target.
      return text;
    }
    if (!lang && fallback === undefined) {
      // Language metadata missing — remember as a weaker candidate.
      fallback = text;
    }
  }

  return fallback;
}
