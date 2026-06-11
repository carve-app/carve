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
    await this.persistMembership(lemma, 'learning');
  }

  /**
   * Mark a lemma as known and persist.
   */
  async markKnown(lemma: string): Promise<void> {
    this.known.add(lemma);
    this.learning.delete(lemma);
    this.ignored.delete(lemma);
    await this.persistMembership(lemma, 'known');
  }

  /**
   * Mark a lemma as ignored and persist. Ignored entries are suppressed during
   * annotation like known words, but are kept out of the learner's known-word
   * list.
   */
  async markIgnored(lemma: string): Promise<void> {
    this.ignored.add(lemma);
    this.learning.delete(lemma);
    this.known.delete(lemma);
    await this.persistMembership(lemma, 'ignored');
  }

  /**
   * Persist a single lemma's membership across the three lemma sets, merging
   * with the latest stored values rather than overwriting from this context's
   * (possibly stale) page-load snapshot. Without this, a lemma marked known,
   * learning, or ignored in another tab — or by the background action handlers
   * — after this page loaded would be silently dropped on the next write.
   */
  private async persistMembership(
    lemma: string,
    target: 'known' | 'learning' | 'ignored',
  ): Promise<void> {
    const [knownArr, learningArr, ignoredArr] = await Promise.all([
      storageGet('knownLemmas'),
      storageGet('learningLemmas'),
      storageGet('ignoredLemmas'),
    ]);
    const known = new Set(knownArr ?? []);
    const learning = new Set(learningArr ?? []);
    const ignored = new Set(ignoredArr ?? []);

    // Apply this mutation: the lemma belongs to exactly one set now.
    known.delete(lemma);
    learning.delete(lemma);
    ignored.delete(lemma);
    if (target === 'known') {
      known.add(lemma);
    } else if (target === 'learning') {
      learning.add(lemma);
    } else {
      ignored.add(lemma);
    }

    // Keep the in-memory sets consistent with what we just merged + wrote.
    this.known = known;
    this.learning = learning;
    this.ignored = ignored;

    await Promise.all([
      storageSet('knownLemmas', Array.from(known)),
      storageSet('learningLemmas', Array.from(learning)),
      storageSet('ignoredLemmas', Array.from(ignored)),
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
