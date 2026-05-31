import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import { SubtitleOverlay } from './SubtitleOverlay';
import { NetflixHook } from './platforms/netflix';
import { YouTubeHook } from './platforms/youtube';

export class SubtitleHook {
  private overlay: SubtitleOverlay | null = null;

  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
  ) {}

  mount(): void {
    const url = window.location.href;
    if (!/netflix\.com\/watch|youtube\.com\/watch|youtu\.be\//.test(url)) return;

    this.overlay = new SubtitleOverlay(this.lang, this.vocabCache, this.popupManager);

    if (/netflix\.com\/watch/.test(url)) {
      new NetflixHook(this.overlay).mount();
    } else if (/youtube\.com\/watch|youtu\.be\//.test(url)) {
      new YouTubeHook(this.overlay).mount();
    }
  }

  destroy(): void {
    this.overlay?.destroy();
    this.overlay = null;
  }
}
