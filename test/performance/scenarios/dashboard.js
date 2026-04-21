// k6 scenario: Dashboard & statistics
// GET /currency + GET /users/me/statistics + GET /reports/* concurrent
//
// Run: k6 run test/performance/scenarios/dashboard.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { API, authHeaders, seededUser, loginUser } from '../helpers.js';

const dashErrors = new Rate('dashboard_errors');
const currencyDuration = new Trend('currency_duration', true);
const statsDuration = new Trend('stats_duration', true);
const reportsDuration = new Trend('reports_duration', true);

export const options = {
  scenarios: {
    dashboard: {
      executor: 'constant-vus',
      vus: 100,
      duration: '2m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1500'],
    http_req_failed: ['rate<0.01'],
    dashboard_errors: ['rate<0.01'],
    currency_duration: ['p(95)<300'],
    stats_duration: ['p(95)<200'],
    reports_duration: ['p(95)<300'],
  },
};

export function setup() {
  const tokens = [];
  for (let i = 0; i < 100; i++) {
    const user = seededUser(i);
    const auth = loginUser(user.email, user.password);
    if (auth) tokens.push(auth.token);
  }
  return { tokens };
}

export default function (data) {
  const token = data.tokens[__VU % data.tokens.length];
  if (!token) return;
  const headers = authHeaders(token);

  // 1. Currency status (complex two-tier evaluation)
  let res = http.get(`${API}/currency`, {
    headers, tags: { name: 'GET /currency' },
  });
  check(res, { 'currency 200': (r) => r.status === 200 }) || dashErrors.add(1);
  currencyDuration.add(res.timings.duration);

  sleep(0.2);

  // 2. User statistics
  res = http.get(`${API}/users/me/statistics`, {
    headers, tags: { name: 'GET /users/me/statistics' },
  });
  check(res, { 'stats 200': (r) => r.status === 200 }) || dashErrors.add(1);
  statsDuration.add(res.timings.duration);

  sleep(0.2);

  // 3. Report endpoints (trends, routes, airport stats)
  const reportEndpoints = [
    { path: '/reports/trends?months=12', name: 'GET /reports/trends' },
    { path: '/reports/routes', name: 'GET /reports/routes' },
    { path: '/reports/airport-stats', name: 'GET /reports/airport-stats' },
    { path: '/reports/stats-by-class', name: 'GET /reports/stats-by-class' },
  ];

  for (const endpoint of reportEndpoints) {
    res = http.get(`${API}${endpoint.path}`, {
      headers, tags: { name: endpoint.name },
    });
    check(res, { [`${endpoint.name} 200`]: (r) => r.status === 200 }) || dashErrors.add(1);
    reportsDuration.add(res.timings.duration);
    sleep(0.1);
  }

  // 4. User profile
  res = http.get(`${API}/users/me`, {
    headers, tags: { name: 'GET /users/me' },
  });
  check(res, { 'profile 200': (r) => r.status === 200 }) || dashErrors.add(1);

  sleep(0.5);
}
