import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import { SubtitleOverlay } from './SubtitleOverlay';
import { NetflixHook } from './platforms/netflix';
import { YouTubeHook } from './platforms/youtube';
import { DisneyPlusHook } from './platforms/disneyplus';
import { AmazonPrimeHook } from './platforms/amazonprime';
import { CrunchyrollHook } from './platforms/crunchyroll';
import { VikiHook } from './platforms/viki';

type HookCtor = new (overlay: SubtitleOverlay, lang: string) => { mount: () => void; unmount?: () => void };

const PLATFORM_HOOKS: { match: RegExp; ctor: HookCtor }[] = [
  { match: /netflix\.com\/watch/,           ctor: NetflixHook },
  { match: /youtube\.com\/watch|youtu\.be\//, ctor: YouTubeHook },
  { match: /disneyplus\.com\/(?:video|play|browse)/, ctor: DisneyPlusHook },
  { match: /(?:amazon|primevideo)\.com\/.*(?:gp\/video|video\/detail|video\/player)/, ctor: AmazonPrimeHook },
  { match: /crunchyroll\.com\/(?:watch|series)/, ctor: CrunchyrollHook },
  { match: /viki\.com\/videos/,             ctor: VikiHook },
];

export class SubtitleHook {
  private overlay: SubtitleOverlay | null = null;

  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
  ) {}

  mount(): void {
    const url = window.location.href;
    const platform = PLATFORM_HOOKS.find(p => p.match.test(url));
    if (!platform) return;

    this.overlay = new SubtitleOverlay(this.lang, this.vocabCache, this.popupManager);
    new platform.ctor(this.overlay, this.lang).mount();
  }

  destroy(): void {
    this.overlay?.destroy();
    this.overlay = null;
  }
}
