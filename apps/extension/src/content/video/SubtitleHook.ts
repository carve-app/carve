import type { VocabCache } from '../../nlp/VocabCache';
import type { PopupManager } from '../popup/PopupManager';
import { NetflixHook } from './platforms/netflix';
import { YouTubeHook } from './platforms/youtube';

export class SubtitleHook {
  constructor(
    private lang: string,
    private vocabCache: VocabCache,
    private popupManager: PopupManager,
  ) {}

  mount(): void {
    const url = window.location.href;
    if (/netflix\.com\/watch/.test(url)) {
      new NetflixHook(this.lang, this.vocabCache, this.popupManager).mount();
    } else if (/youtube\.com\/watch|youtu\.be\//.test(url)) {
      new YouTubeHook(this.lang, this.vocabCache, this.popupManager).mount();
    }
  }
}
