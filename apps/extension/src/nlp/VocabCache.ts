import { storageGet, storageSet } from '../shared/storage';

export class VocabCache {
  private known: Set<string>;
  private learning: Set<string>;
  private ignored: Set<string>;

  constructor() {
    this.known = new Set();
    this.learning = new Set();
    this.ignored = new Set();
  }

  /**
   * Load vocab sets from chrome.storage.local.
   */
  async load(): Promise<void> {
    const [knownArr, learningArr, ignoredArr] = await Promise.all([
      storageGet('knownLemmas'),
      storageGet('learningLemmas'),
      storageGet('ignoredLemmas'),
    ]);
    this.known = new Set(knownArr ?? []);
    this.learning = new Set(learningArr ?? []);
    this.ignored = new Set(ignoredArr ?? []);
  }

  /**
   * Get the status of a lemma.
   */
  getStatus(lemma: string): 'known' | 'learning' | 'unknown' {
    if (this.known.has(lemma) || this.ignored.has(lemma)) return 'known';
    if (this.learning.has(lemma)) return 'learning';
    return 'unknown';
  }

  /**
   * Mark a lemma as learning and persist.
   */
  async markLearning(lemma: string): Promise<void> {
    this.learning.add(lemma);
    this.known.delete(lemma);
    this.ignored.delete(lemma);
    await Promise.all([
      storageSet('learningLemmas', this.getLearningLemmas()),
      storageSet('knownLemmas', this.getKnownLemmas()),
      storageSet('ignoredLemmas', Array.from(this.ignored)),
    ]);
  }

  /**
   * Mark a lemma as known/ignored and persist.
   */
  async markKnown(lemma: string): Promise<void> {
    this.ignored.add(lemma);
    this.learning.delete(lemma);
    this.known.delete(lemma);
    await Promise.all([
      storageSet('ignoredLemmas', Array.from(this.ignored)),
      storageSet('learningLemmas', this.getLearningLemmas()),
      storageSet('knownLemmas', this.getKnownLemmas()),
    ]);
  }

  /**
   * Get all known lemmas as an array.
   */
  getKnownLemmas(): string[] {
    return Array.from(this.known);
  }

  /**
   * Get all learning lemmas as an array.
   */
  getLearningLemmas(): string[] {
    return Array.from(this.learning);
  }
}
