#!/usr/bin/env node
/**
 * L14 — LLM-as-judge polish review.
 *
 * Boots the SvelteKit web app + mock API, captures one screenshot per
 * route, ships them to the Claude API with the rubric from
 * docs/13-design-rubric.md, parses the per-route score JSON, and writes
 * reports/polish/polish-scores.json.
 *
 * CI invokes via `make test-polish`. The job fails when any route's
 * aggregate drops by >= 0.5 vs reports/polish/baseline.json.
 *
 * Required env:
 *   ANTHROPIC_API_KEY — your Claude API key
 *   WEB_BASE_URL      — optional; defaults to http://localhost:5173
 */

import fs from 'node:fs';
import path from 'node:path';
import { chromium } from 'playwright';
import Anthropic from '@anthropic-ai/sdk';

const REPO_ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const RUBRIC_PATH   = path.join(REPO_ROOT, 'docs', '13-design-rubric.md');
const SCREENS_DIR   = path.join(REPO_ROOT, 'reports', 'polish', 'screenshots');
const SCORES_PATH   = path.join(REPO_ROOT, 'reports', 'polish', 'polish-scores.json');
const BASELINE_PATH = path.join(REPO_ROOT, 'reports', 'polish', 'baseline.json');

const ROUTES = [
  { name: 'landing',         url: '/' },
  { name: 'login',           url: '/login' },
  { name: 'register',        url: '/register' },
  { name: 'pricing',         url: '/pricing' },
  { name: 'privacy',         url: '/privacy' },
  { name: 'forgot-password', url: '/forgot-password' },
];

const BASE = process.env.WEB_BASE_URL || 'http://localhost:5173';
const REGRESSION_THRESHOLD = 0.5;

async function captureScreenshots() {
  fs.mkdirSync(SCREENS_DIR, { recursive: true });
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const page = await ctx.newPage();
  const out = [];
  for (const r of ROUTES) {
    const file = path.join(SCREENS_DIR, `${r.name}.png`);
    await page.goto(`${BASE}${r.url}`, { waitUntil: 'networkidle' }).catch(() => {});
    await page.waitForTimeout(400);
    await page.screenshot({ path: file, fullPage: true });
    out.push({ ...r, file });
  }
  await browser.close();
  return out;
}

async function judgeRoute(client, rubric, route, b64) {
  const sys = `You are a senior product-design reviewer auditing screenshots against a precise rubric. Always reply with a single valid JSON object matching the schema in the rubric. No prose, no markdown.`;
  const userMsg = `Rubric:\n\n${rubric}\n\nRoute: ${route.url}\nName: ${route.name}\n\nReview the attached screenshot and produce the JSON described in the "Output schema" section.`;

  const res = await client.messages.create({
    model: 'claude-opus-4-7',
    max_tokens: 1500,
    system: sys,
    messages: [
      {
        role: 'user',
        content: [
          { type: 'image', source: { type: 'base64', media_type: 'image/png', data: b64 } },
          { type: 'text', text: userMsg },
        ],
      },
    ],
  });

  const text = res.content.filter((c) => c.type === 'text').map((c) => c.text).join('');
  const m = text.match(/\{[\s\S]*\}/);
  if (!m) throw new Error(`Judge for ${route.name} returned no JSON:\n${text}`);
  return JSON.parse(m[0]);
}

function checkRegression(current, baseline) {
  const failures = [];
  for (const r of current.routes) {
    const prev = baseline?.routes.find((p) => p.name === r.name);
    if (!prev) continue;
    const delta = prev.aggregate - r.aggregate;
    if (delta >= REGRESSION_THRESHOLD) {
      failures.push({
        route: r.name,
        before: prev.aggregate,
        after: r.aggregate,
        delta,
        blockers: r.blockers ?? [],
      });
    }
  }
  return failures;
}

async function main() {
  if (!process.env.ANTHROPIC_API_KEY) {
    console.error('ANTHROPIC_API_KEY not set');
    process.exit(2);
  }
  const rubric = fs.readFileSync(RUBRIC_PATH, 'utf8');
  const client = new Anthropic();

  console.log('Capturing screenshots…');
  const captured = await captureScreenshots();

  console.log(`Judging ${captured.length} routes…`);
  const routes = [];
  for (const r of captured) {
    const b64 = fs.readFileSync(r.file).toString('base64');
    const score = await judgeRoute(client, rubric, r, b64);
    routes.push(score);
    console.log(`  ${r.name}: aggregate ${score.aggregate}`);
  }

  const report = { generated_at: new Date().toISOString(), routes };
  fs.mkdirSync(path.dirname(SCORES_PATH), { recursive: true });
  fs.writeFileSync(SCORES_PATH, JSON.stringify(report, null, 2));

  const baseline = fs.existsSync(BASELINE_PATH)
    ? JSON.parse(fs.readFileSync(BASELINE_PATH, 'utf8'))
    : null;
  if (baseline) {
    const regressions = checkRegression(report, baseline);
    if (regressions.length > 0) {
      console.error('Polish regressions detected:');
      for (const r of regressions) {
        console.error(`  ${r.route}: ${r.before} → ${r.after} (-${r.delta.toFixed(2)})`);
        for (const b of r.blockers) console.error(`    blocker: ${b}`);
      }
      process.exit(1);
    }
  } else {
    fs.writeFileSync(BASELINE_PATH, JSON.stringify(report, null, 2));
    console.log('No baseline; wrote one. Re-run on the next branch to detect regressions.');
  }
}

main().catch((e) => { console.error(e); process.exit(1); });
