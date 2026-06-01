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
const TOKEN = __ENV.ACCESS_TOKEN || '';

const HEADERS = TOKEN
  ? { Authorization: `Bearer ${TOKEN}`, 'Content-Type': 'application/json' }
  : { 'Content-Type': 'application/json' };

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

export function listCards() {
  const r = http.get(`${API}/v1/cards?language=en&limit=50`, {
    headers: HEADERS, tags: { name: 'list' },
  });
  check(r, { 'status 200/401': (res) => res.status === 200 || res.status === 401 });
  sleep(0.2);
}

export function reviewSession() {
  const r = http.get(`${API}/v1/review/session?language=en&limit=20`, {
    headers: HEADERS, tags: { name: 'session' },
  });
  check(r, { 'status 200/401': (res) => res.status === 200 || res.status === 401 });
  sleep(0.1);
}

export function submitReview() {
  const r = http.post(
    `${API}/v1/review/events`,
    JSON.stringify({
      card_id: '00000000-0000-0000-0000-000000000000',
      rating: 3,
      time_taken_ms: 1500,
    }),
    { headers: HEADERS, tags: { name: 'event' } },
  );
  check(r, { 'status 200/400/401/404': (res) => [200, 400, 401, 404].includes(res.status) });
  sleep(0.05);
}
