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
    // Page visibility
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
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

    // Flush on page unload
    window.addEventListener('pagehide', () => {
      this.flush(true);
    });
    window.addEventListener('beforeunload', () => {
      this.flush(true);
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

  private flush(synchronous = false): void {
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

    if (synchronous && typeof navigator.sendBeacon === 'function') {
      // Best-effort sync send — fall back to async message on failure
      browser.runtime.sendMessage(payload).catch(() => {});
    } else {
      browser.runtime.sendMessage(payload).catch(() => {});
    }
  }

  destroy(): void {
    if (this.tickInterval !== null) {
      clearInterval(this.tickInterval);
      this.tickInterval = null;
    }
    this.flush(true);
  }
}
