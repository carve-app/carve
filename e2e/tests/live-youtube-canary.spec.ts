import { chromium, expect, test } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const EXTENSION_DIR = path.resolve(__dirname, '..', '..', 'apps', 'extension', 'dist', 'chrome');
// A stable, spoken TEDx talk with public English captions. Do not substitute a
// music video: several have no captions, which used to let this canary pass on
// an empty overlay without proving subtitle integration.
const YOUTUBE_URL = process.env.YOUTUBE_CANARY_URL ?? 'https://www.youtube.com/watch?v=-moW9jvvMr4';

test('public YouTube watch page produces a packaged-extension subtitle cue', async ({ browserName: _browserName }, testInfo) => {
  test.setTimeout(150_000);
  test.skip(process.env.LIVE_PROVIDER_TESTS !== '1', 'live provider tests are opt-in');
  test.skip(!fs.existsSync(EXTENSION_DIR), 'packaged Chrome extension is unavailable');

  const context = await chromium.launchPersistentContext(testInfo.outputPath('youtube-profile'), {
    headless: false,
    args: [
      `--disable-extensions-except=${EXTENSION_DIR}`,
      `--load-extension=${EXTENSION_DIR}`,
      '--autoplay-policy=no-user-gesture-required',
    ],
  });
  try {
    let [worker] = context.serviceWorkers();
    if (!worker) worker = await context.waitForEvent('serviceworker');
    await worker.evaluate(async () => {
      await chrome.storage.local.set({ targetLanguage: 'en', annotateLatinSites: true });
    });
    const page = await context.newPage();
    const response = await page.goto(YOUTUBE_URL, { waitUntil: 'domcontentloaded', timeout: 45_000 });
    expect(response?.ok(), `YouTube returned ${response?.status()}`).toBe(true);
    await expect(page.locator('#carve-sub-overlay')).toBeAttached({ timeout: 20_000 });

    // YouTube commonly inserts a paused pre-roll in fresh browser profiles.
    // A mounted overlay on that ad proves injection but cannot prove subtitle
    // integration, so start playback, skip when possible, and wait until the
    // actual long-form video is advancing.
    const video = page.locator('video.html5-main-video, video.video-stream');
    await expect(video).toBeAttached({ timeout: 20_000 });
    await video.evaluate(async (element: HTMLVideoElement) => {
      element.muted = true;
      await element.play();
    });
    const skipAd = page.locator('.ytp-skip-ad-button, .ytp-ad-skip-button-modern');
    try {
      await skipAd.waitFor({ state: 'visible', timeout: 15_000 });
      await skipAd.click();
    } catch {
      // Some regions receive a short non-skippable ad; the condition below
      // waits for it to finish instead of mistaking its empty captions for a
      // Carve failure.
    }
    await page.waitForFunction(() => {
      const player = document.querySelector('.html5-video-player');
      const current = document.querySelector<HTMLVideoElement>('video.html5-main-video, video.video-stream');
      return Boolean(current && !player?.classList.contains('ad-showing') && current.duration > 120 && current.currentTime > 0);
    }, undefined, { timeout: 90_000 });

    const target = page.locator('#cso-target');
    await expect(target).not.toHaveText('', { timeout: 45_000 });
    await expect(target).not.toContainText('Subtitles will appear here');
  } finally {
    await context.close();
  }
});
