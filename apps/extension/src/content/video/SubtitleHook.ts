import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import { SubtitleOverlay } from './SubtitleOverlay';
import { NetflixHook } from './platforms/netflix';
import { YouTubeHook } from './platforms/youtube';
import { DisneyPlusHook } from './platforms/disneyplus';
import { AmazonPrimeHook } from './platforms/amazonprime';
import { CrunchyrollHook } from './platforms/crunchyroll';
import { VikiHook } from './platforms/viki';

type PlatformHook = { mount: () => void; unmount?: () => void };
type HookCtor = new (overlay: SubtitleOverlay, lang: string) => PlatformHook;

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
  private platformHook: PlatformHook | null = null;

  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
    private platformHooks = PLATFORM_HOOKS,
  ) {}

  mount(): void {
    const url = window.location.href;
    const platform = this.platformHooks.find(p => p.match.test(url));
    if (!platform) return;

    this.overlay = new SubtitleOverlay(this.lang, this.vocabCache, this.popupManager);
    this.platformHook = new platform.ctor(this.overlay, this.lang);
    this.platformHook.mount();
  }

  destroy(): void {
    this.platformHook?.unmount?.();
    this.platformHook = null;
    this.overlay?.destroy();
    this.overlay = null;
  }
}
