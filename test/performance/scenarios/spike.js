// k6 scenario: Spike test
// Ramp 0 → 200 VUs in 30s, hold 1 min, ramp down
// Tests system behavior under sudden load spikes
//
// Run: k6 run test/performance/scenarios/spike.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { API, authHeaders, seededUser, loginUser } from '../helpers.js';

const spikeErrors = new Rate('spike_errors');
const spikeDuration = new Trend('spike_request_duration', true);

export const options = {
  scenarios: {
    spike: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 200 },  // Ramp up to 200 VUs
        { duration: '1m', target: 200 },    // Hold at 200 VUs
        { duration: '30s', target: 0 },     // Ramp down
      ],
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<1000', 'p(99)<3000'],
    http_req_failed: ['rate<0.05'],
    spike_errors: ['rate<0.05'],
  },
};

export function setup() {
  // Pre-login tokens for seeded users
  const tokens = [];
  for (let i = 0; i < 100; i++) {
    const user = seededUser(i);
    const auth = loginUser(user.email, user.password);
    if (auth) tokens.push(auth.token);
  }
  return { tokens };
}

export default function (data) {
  if (data.tokens.length === 0) return;
  const token = data.tokens[__VU % data.tokens.length];
  if (!token) return;
  const headers = authHeaders(token);

  // Mix of typical API operations
  const operations = [
    () => http.get(`${API}/flights?page=1&pageSize=10`, { headers, tags: { name: 'GET /flights' } }),
    () => http.get(`${API}/currency`, { headers, tags: { name: 'GET /currency' } }),
    () => http.get(`${API}/users/me/statistics`, { headers, tags: { name: 'GET /statistics' } }),
    () => http.get(`${API}/users/me`, { headers, tags: { name: 'GET /users/me' } }),
    () => http.get(`${API}/aircraft`, { headers, tags: { name: 'GET /aircraft' } }),
    () => http.get(`${API}/reports/trends?months=6`, { headers, tags: { name: 'GET /reports/trends' } }),
  ];

  const op = operations[Math.floor(Math.random() * operations.length)];
  const res = op();

  check(res, { 'status 200': (r) => r.status === 200 }) || spikeErrors.add(1);
  spikeDuration.add(res.timings.duration);

  sleep(0.5 + Math.random() * 1.0);
}
