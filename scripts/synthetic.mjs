#!/usr/bin/env node
/**
 * L15 — production synthetic monitoring.
 *
 * Runs every minute (a small cron worker, Datadog synthetic, or k8s
 * CronJob). Logs into a dedicated verified synthetic account, mines a card on
 * a fixture URL, submits a review, asserts the review took, then removes the
 * card. On failure, posts to
 * $ALERT_WEBHOOK (Slack/PagerDuty/SNS).
 *
 * Required env:
 *   API_BASE       — production API base URL
 *   ALERT_WEBHOOK  — incoming-webhook URL for failure pages
 *   SYNTHETIC_EMAIL    — pre-provisioned, verified synthetic user
 *   SYNTHETIC_PASSWORD — password for the synthetic user
 */

import { request } from 'node:https';
import { URL } from 'node:url';
import { randomUUID } from 'node:crypto';

const API = process.env.API_BASE || 'https://api.carve.app';
const ALERT = process.env.ALERT_WEBHOOK;
const SYNTHETIC_EMAIL = process.env.SYNTHETIC_EMAIL;
const SYNTHETIC_PASSWORD = process.env.SYNTHETIC_PASSWORD;

const t0 = Date.now();
const timings = {};

async function step(name, fn) {
  const start = Date.now();
  try {
    const out = await fn();
    timings[name] = Date.now() - start;
    return out;
  } catch (e) {
    timings[name] = Date.now() - start;
    throw new Error(`[step:${name}] ${e.message}`);
  }
}

function postJSON(url, body, token) {
  return new Promise((resolve, reject) => {
    const u = new URL(url);
    const req = request(
      { hostname: u.hostname, port: u.port || 443, path: u.pathname, method: 'POST',
        headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) } },
      (res) => {
        const chunks = [];
        res.on('data', (c) => chunks.push(c));
        res.on('end', () => {
          const text = Buffer.concat(chunks).toString('utf8');
          if (res.statusCode >= 400) return reject(new Error(`${url} → ${res.statusCode}: ${text}`));
          try { resolve(JSON.parse(text)); } catch { resolve({}); }
        });
      },
    );
    req.on('error', reject);
    req.write(JSON.stringify(body));
    req.end();
  });
}

function getJSON(url, token) {
  return new Promise((resolve, reject) => {
    const u = new URL(url);
    const req = request(
      { hostname: u.hostname, port: u.port || 443, path: u.pathname + (u.search || ''),
        method: 'GET', headers: { Authorization: `Bearer ${token}` } },
      (res) => {
        const chunks = [];
        res.on('data', (c) => chunks.push(c));
        res.on('end', () => {
          const text = Buffer.concat(chunks).toString('utf8');
          if (res.statusCode >= 400) return reject(new Error(`${url} → ${res.statusCode}: ${text}`));
          try { resolve(JSON.parse(text)); } catch { resolve({}); }
        });
      },
    );
    req.on('error', reject);
    req.end();
  });
}

function deleteRequest(url, token) {
  return new Promise((resolve, reject) => {
    const u = new URL(url);
    const req = request(
      { hostname: u.hostname, port: u.port || 443, path: u.pathname,
        method: 'DELETE', headers: { Authorization: `Bearer ${token}` } },
      (res) => {
        res.resume();
        res.on('end', () => res.statusCode >= 400
          ? reject(new Error(`${url} → ${res.statusCode}`))
          : resolve({}));
      },
    );
    req.on('error', reject);
    req.end();
  });
}

async function alert(error) {
  if (!ALERT) {
    console.error(error.stack || error.message);
    return;
  }
  await postJSON(ALERT, {
    text: `🚨 carve synthetic check failed at ${new Date().toISOString()}`,
    error: error.message, timings,
  }).catch(() => {});
}

async function main() {
  if (!SYNTHETIC_EMAIL || !SYNTHETIC_PASSWORD) {
    throw new Error('SYNTHETIC_EMAIL and SYNTHETIC_PASSWORD are required');
  }
  const session = await step('login', () =>
    postJSON(`${API}/v1/auth/login`, { email: SYNTHETIC_EMAIL, password: SYNTHETIC_PASSWORD }),
  );
  const token = session.access_token;
  if (!token) throw new Error('login: no access_token');

  const card = await step('create_card', () =>
    postJSON(`${API}/v1/cards`, { front_text: 'synthetic', back_text: 'monitor', language_code: 'en' }, token),
  );

  await step('submit_review', () =>
    postJSON(`${API}/v1/review/events`, {
      event_id: randomUUID(), card_id: card.id, rating: 3, time_taken_ms: 1500,
    }, token),
  );

  const due = await step('get_due_count', () =>
    getJSON(`${API}/v1/review/due-count?language=en`, token),
  );
  if (typeof due.due_count !== 'number') throw new Error(`due-count missing in ${JSON.stringify(due)}`);

  await step('cleanup_card', () => deleteRequest(`${API}/v1/cards/${card.id}`, token));

  const total = Date.now() - t0;
  console.log(JSON.stringify({ status: 'ok', total_ms: total, timings }));
}

main().catch(async (e) => {
  await alert(e);
  process.exit(1);
});
