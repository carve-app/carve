import { browser } from '../../shared/browser';

const FLUSH_INTERVAL_MS = 30_000;
const MIN_SESSION_SEC = 5;

export class ImmersionTracker {
  private startedAt: Date;
  private activeSeconds = 0;
  private lastTickAt: number | null = null;
  private tickInterval: ReturnType<typeof setInterval> | null = null;
  private isVisible: boolean;
  private isActive: boolean;

  constructor(private lang: string) {
    this.startedAt = new Date();
    this.isVisible = !document.hidden;
    this.isActive = true;

    this.bindEvents();
    this.startTick();

    // Flush every 30 seconds
    this.tickInterval = setInterval(() => {
      this.flush();
    }, FLUSH_INTERVAL_MS);
  }

  private bindEvents(): void {
    // Page visibility. Flush BEFORE marking hidden: visibilitychange→hidden
    // fires while the page is still alive (tab switch / close / mobile
    // backgrounding), so the runtime message to the service worker reliably
    // lands — unlike the pagehide/beforeunload path, where the content-script
    // context is torn down before an async message is delivered. This is the
    // primary flush for navigation losses.
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
        this.flush();
        this.pauseTick();
        this.isVisible = false;
      } else {
        this.isVisible = true;
        this.startTick();
      }
    });

    // Activity signals — reset idle timer
    const activityEvents = ['mousemove', 'keydown', 'scroll', 'click'];
    for (const ev of activityEvents) {
      document.addEventListener(ev, () => {
        this.isActive = true;
        if (!this.isVisible) return;
        if (this.lastTickAt === null) {
          this.startTick();
        }
      }, { passive: true });
    }

    // Last-ditch flush on unload. The reliable flush is visibilitychange above
    // (which fires first); this is best-effort for the rare case the page goes
    // straight to unload without a prior hidden transition.
    window.addEventListener('pagehide', () => {
      this.flush();
    });
  }

  private startTick(): void {
    if (this.lastTickAt !== null) return;
    this.lastTickAt = Date.now();
  }

  private pauseTick(): void {
    if (this.lastTickAt !== null) {
      const elapsed = (Date.now() - this.lastTickAt) / 1000;
      this.activeSeconds += elapsed;
      this.lastTickAt = null;
    }
  }

  private flush(): void {
    // Accumulate since last tick
    if (this.lastTickAt !== null && this.isVisible) {
      const elapsed = (Date.now() - this.lastTickAt) / 1000;
      this.activeSeconds += elapsed;
      this.lastTickAt = Date.now();
    }

    const duration = Math.round(this.activeSeconds);
    if (duration < MIN_SESSION_SEC) return;

    // Reset counter
    this.activeSeconds = 0;
    const sessionStart = this.startedAt.toISOString();
    this.startedAt = new Date();

    const payload = {
      type: 'LOG_IMMERSION',
      languageCode: this.lang,
      sessionType: 'reading',
      durationSec: duration,
      startedAt: sessionStart,
      url: window.location.href,
    };

    browser.runtime.sendMessage(payload).catch(() => {});
  }

  destroy(): void {
    if (this.tickInterval !== null) {
      clearInterval(this.tickInterval);
      this.tickInterval = null;
    }
    this.flush();
  }
}
