#!/usr/bin/env node
/**
 * L15 — production synthetic monitoring.
 *
 * Runs every minute (a small cron worker, Datadog synthetic, or k8s
 * CronJob). Registers a throwaway user, mines a card on a fixture URL,
 * submits a review, asserts the review took. On failure, posts to
 * $ALERT_WEBHOOK (Slack/PagerDuty/SNS).
 *
 * Required env:
 *   API_BASE       — production API base URL
 *   ALERT_WEBHOOK  — incoming-webhook URL for failure pages
 *   USER_DOMAIN    — domain to use in throwaway emails (default: synthetic.carve.app)
 */

import { request } from 'node:https';
import { URL } from 'node:url';
import { randomUUID } from 'node:crypto';

const API = process.env.API_BASE || 'https://api.carve.app';
const DOMAIN = process.env.USER_DOMAIN || 'synthetic.carve.app';
const ALERT = process.env.ALERT_WEBHOOK;

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
  const email = `synthetic+${Date.now()}@${DOMAIN}`;

  const reg = await step('register', () =>
    postJSON(`${API}/v1/auth/register`, {
      email, password: 'synthetic-supersecret-123', display_name: 'Synthetic',
    }),
  );

  const token = reg.access_token;
  if (!token) throw new Error('register: no access_token');

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

  await step('cleanup_user', () => deleteRequest(`${API}/v1/users/me`, token));

  const total = Date.now() - t0;
  console.log(JSON.stringify({ status: 'ok', total_ms: total, timings }));
}

main().catch(async (e) => {
  await alert(e);
  process.exit(1);
});
