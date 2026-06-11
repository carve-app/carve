/**
 * L11 — recorded streaming-platform fixture replay.
 *
 * Each .html in __tests__/fixtures/ is a snapshot of the platform's
 * player DOM around an active cue. We load it via jsdom, instantiate the
 * matching platform hook with a stub overlay, trigger the mutation
 * observer, and assert that the overlay receives the cue text we baked
 * into the fixture.
 *
 * When Disney+, Netflix, or any other platform changes its DOM, the
 * canonical selectors stop matching, the test fails, and the developer
 * updates both the platform glue and the fixture in one PR. That's the
 * load-bearing property of this layer.
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';

import { NetflixHook }     from '../platforms/netflix';
import { YouTubeHook }     from '../platforms/youtube';
import { DisneyPlusHook }  from '../platforms/disneyplus';
import { AmazonPrimeHook } from '../platforms/amazonprime';
import { CrunchyrollHook } from '../platforms/crunchyroll';
import { VikiHook }        from '../platforms/viki';

interface CueLike { text: string; startMs: number; endMs: number; nativeText?: string }

function makeStubOverlay() {
  const cues: CueLike[] = [];
  return {
    cues,
    hideNativeContainer: vi.fn(),
    showNativeContainer: vi.fn(),
    onCue: vi.fn((cue: CueLike) => { cues.push(cue); }),
    destroy: vi.fn(),
  };
}

/**
 * jsdom does not implement TextTrack cue timing, so we install a minimal fake
 * `textTracks` list on the fixture's <video> element. Each entry mimics the
 * shape `extractNativeCueText` / `getActiveCueTiming` read: `mode`, `kind`,
 * `language`, and an `activeCues` array of `{ text, startTime, endTime }`.
 */
interface FakeCue { text: string; startTime: number; endTime: number }
interface FakeTrack {
  kind: string;
  language: string;
  mode: 'showing' | 'hidden' | 'disabled';
  activeCues: FakeCue[];
}

function installTextTracks(selector: string, tracks: FakeTrack[]): void {
  const video = document.querySelector(selector);
  if (!video) throw new Error(`fixture missing <video> for selector ${selector}`);
  const list = tracks as unknown as TextTrackList & FakeTrack[];
  // TextTrackList is array-indexed with a `.length`; the fake array satisfies
  // both the index access and `.length` reads the hooks perform.
  Object.defineProperty(video, 'textTracks', {
    configurable: true,
    get: () => list,
  });
}

// Verifies both that the overlay receives a native line at cue time and that
// it is sourced from the *non-target* track, not the target track.
const DUAL_TRACKS: FakeTrack[] = [
  {
    kind: 'subtitles',
    language: 'ja',
    mode: 'showing',
    activeCues: [{ text: '日本語のセリフ', startTime: 42.5, endTime: 45.0 }],
  },
  {
    kind: 'subtitles',
    language: 'en',
    mode: 'showing',
    activeCues: [{ text: 'English translation line.', startTime: 42.5, endTime: 45.0 }],
  },
];

const fixturesDir = path.join(__dirname, 'fixtures');
function loadFixture(name: string): string {
  return fs.readFileSync(path.join(fixturesDir, name), 'utf8');
}

interface PlatformCase {
  name: string;
  ctor: any;
  fixture: string;
  expected: RegExp;
}

const CASES: PlatformCase[] = [
  { name: 'Netflix',       ctor: NetflixHook,     fixture: 'netflix.html',     expected: /active cue/i },
  { name: 'YouTube',       ctor: YouTubeHook,     fixture: 'youtube.html',     expected: /youtube caption/i },
  { name: 'Disney+',       ctor: DisneyPlusHook,  fixture: 'disneyplus.html',  expected: /active cue/i },
  { name: 'Amazon Prime',  ctor: AmazonPrimeHook, fixture: 'amazonprime.html', expected: /active.*caption/i },
  { name: 'Crunchyroll',   ctor: CrunchyrollHook, fixture: 'crunchyroll.html', expected: /active subtitle/i },
  { name: 'Viki',          ctor: VikiHook,        fixture: 'viki.html',        expected: /active subtitle/i },
];

describe('streaming-platform hooks against recorded fixtures', () => {
  for (const { name, ctor, fixture, expected } of CASES) {
    describe(name, () => {
      beforeEach(() => {
        document.documentElement.innerHTML = loadFixture(fixture);
      });

      it('fires a cue with the recorded text', async () => {
        const overlay = makeStubOverlay();
        const hook = new ctor(overlay);
        hook.mount();
        // Give the MutationObserver a microtask to flush.
        await new Promise((r) => setTimeout(r, 0));
        // checkSubtitle is invoked synchronously on mount() in every hook
        // we ship; if a future hook becomes async, this test will catch
        // the missing flush.
        expect(overlay.onCue).toHaveBeenCalled();
        expect(overlay.cues[0].text).toMatch(expected);
        expect(typeof overlay.cues[0].startMs).toBe('number');
        expect(typeof overlay.cues[0].endMs).toBe('number');
      });

      it('hides the native subtitle container', () => {
        const overlay = makeStubOverlay();
        const hook = new ctor(overlay);
        hook.mount();
        expect(overlay.hideNativeContainer).toHaveBeenCalled();
      });
    });
  }
});

/**
 * Dual-subtitle capture: when a native-language TextTrack is showing alongside
 * the target track, the hook must read the native track's active cue at cue
 * time and pass it as `nativeText` so the overlay shows both lines live and
 * mined cards carry the human translation.
 */
describe('dual-subtitle native text capture (textTracks)', () => {
  // Each platform's <video> selector as the hook queries it.
  const DUAL_CASES: { name: string; ctor: any; fixture: string; videoSelector: string }[] = [
    { name: 'YouTube', ctor: YouTubeHook, fixture: 'youtube.html', videoSelector: 'video.video-stream' },
    { name: 'Netflix', ctor: NetflixHook, fixture: 'netflix.html', videoSelector: 'video' },
  ];

  for (const { name, ctor, fixture, videoSelector } of DUAL_CASES) {
    describe(name, () => {
      beforeEach(() => {
        document.documentElement.innerHTML = loadFixture(fixture);
      });

      it('captures the native (English) cue when a second track is showing', async () => {
        installTextTracks(videoSelector, DUAL_TRACKS);
        const overlay = makeStubOverlay();
        const hook = new ctor(overlay, 'ja');
        hook.mount();
        await new Promise((r) => setTimeout(r, 0));

        expect(overlay.onCue).toHaveBeenCalled();
        const cue = overlay.cues[0];
        // Target line still comes from the on-page DOM, not the track.
        expect(cue.nativeText).toBe('English translation line.');
        // Timing is read from the (target) showing track's active cue.
        expect(cue.startMs).toBe(42500);
        expect(cue.endMs).toBe(45000);
      });

      it('leaves nativeText undefined when only the target track is showing', async () => {
        installTextTracks(videoSelector, [DUAL_TRACKS[0]]);
        const overlay = makeStubOverlay();
        const hook = new ctor(overlay, 'ja');
        hook.mount();
        await new Promise((r) => setTimeout(r, 0));

        expect(overlay.onCue).toHaveBeenCalled();
        expect(overlay.cues[0].nativeText).toBeUndefined();
      });

      it('strips VTT markup from the native cue', async () => {
        installTextTracks(videoSelector, [
          DUAL_TRACKS[0],
          {
            kind: 'subtitles',
            language: 'en',
            mode: 'showing',
            activeCues: [{ text: '<c.yellow>Tagged</c> <b>line</b>', startTime: 0, endTime: 1 }],
          },
        ]);
        const overlay = makeStubOverlay();
        const hook = new ctor(overlay, 'ja');
        hook.mount();
        await new Promise((r) => setTimeout(r, 0));

        expect(overlay.cues[0].nativeText).toBe('Tagged line');
      });
    });
  }
});
