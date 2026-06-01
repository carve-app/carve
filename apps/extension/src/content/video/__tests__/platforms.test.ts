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

interface CueLike { text: string; startMs: number; endMs: number }

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
