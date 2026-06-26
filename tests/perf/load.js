// k6 load test — exercises the hottest authenticated endpoints with a
// realistic mix of read/write traffic. Run with:
//
//     k6 run tests/perf/load.js \
//         --env API_BASE=http://localhost:8080 \
//         --env ACCESS_TOKEN=$(cat .test_token)
//
// CI invocation: `make test-perf` (see Makefile). p95 thresholds match
// the perf budget called out in docs/12-testing-strategy.md.

import http from 'k6/http';
import { check, sleep } from 'k6';

const API = __ENV.API_BASE || 'http://localhost:8080';
const PROVIDED_TOKEN = __ENV.ACCESS_TOKEN || '';
const PROVIDED_CARD_ID = __ENV.CARD_ID || '';

function headers(token) {
  return { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
}

function uuid() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = Math.floor(Math.random() * 16);
    return (c === 'x' ? r : (r & 0x3) | 8).toString(16);
  });
}

export function setup() {
  if (PROVIDED_TOKEN && PROVIDED_CARD_ID) {
    return { token: PROVIDED_TOKEN, cardId: PROVIDED_CARD_ID };
  }
  const email = `k6+${Date.now()}-${uuid()}@example.com`;
  const register = http.post(`${API}/v1/auth/register`, JSON.stringify({
    email, password: 'k6-performance-password', display_name: 'k6',
  }), { headers: { 'Content-Type': 'application/json' } });
  if (register.status !== 200) throw new Error(`register failed: ${register.status} ${register.body}`);
  const token = register.json('access_token');
  const card = http.post(`${API}/v1/cards`, JSON.stringify({
    language_code: 'en', lemma: `performance-${uuid()}`, back_text: 'load test',
  }), { headers: headers(token) });
  if (card.status !== 200 && card.status !== 201) throw new Error(`card seed failed: ${card.status} ${card.body}`);
  return { token, cardId: card.json('id') };
}

export const options = {
  scenarios: {
    list_cards: {
      executor: 'constant-arrival-rate',
      rate: 1, timeUnit: '1s', duration: '60s',
      preAllocatedVUs: 5, maxVUs: 20,
      exec: 'listCards',
    },
    review_session: {
      executor: 'constant-arrival-rate',
      rate: 5, timeUnit: '1s', duration: '60s',
      preAllocatedVUs: 10, maxVUs: 30,
      exec: 'reviewSession',
    },
    submit_review: {
      executor: 'constant-arrival-rate',
      rate: 5, timeUnit: '1s', duration: '60s',
      preAllocatedVUs: 10, maxVUs: 30,
      exec: 'submitReview',
    },
  },
  thresholds: {
    'http_req_failed':                  ['rate<0.01'],
    'http_req_duration{name:list}':     ['p(95)<250'],
    'http_req_duration{name:session}':  ['p(95)<250'],
    'http_req_duration{name:event}':    ['p(95)<300'],
  },
};

export function listCards(data) {
  const r = http.get(`${API}/v1/cards?language=en&limit=50`, {
    headers: headers(data.token), tags: { name: 'list' },
  });
  check(r, { 'list status 200': (res) => res.status === 200 });
  sleep(0.2);
}

export function reviewSession(data) {
  const r = http.get(`${API}/v1/review/session?language=en&limit=20`, {
    headers: headers(data.token), tags: { name: 'session' },
  });
  check(r, { 'session status 200': (res) => res.status === 200 });
  sleep(0.1);
}

export function submitReview(data) {
  const r = http.post(
    `${API}/v1/review/events`,
    JSON.stringify({
      event_id: uuid(),
      card_id: data.cardId,
      rating: 3,
      time_taken_ms: 1500,
    }),
    { headers: headers(data.token), tags: { name: 'event' } },
  );
  check(r, { 'event status 200': (res) => res.status === 200 });
  sleep(0.05);
}
