// k6 scenario: Authentication flow
// register → login → refresh token → logout
//
// Run: k6 run test/performance/scenarios/auth.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { API, jsonHeaders, authHeaders } from '../helpers.js';

const authErrors = new Rate('auth_errors');
const loginDuration = new Trend('login_duration', true);
const refreshDuration = new Trend('refresh_duration', true);

export const options = {
  scenarios: {
    auth_flow: {
      executor: 'constant-vus',
      vus: 100,
      duration: '2m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    http_req_failed: ['rate<0.01'],
    auth_errors: ['rate<0.01'],
    login_duration: ['p(95)<300'],
    refresh_duration: ['p(95)<200'],
  },
};

export default function () {
  const uniqueId = `${__VU}-${__ITER}-${Date.now()}`;
  const email = `perf-auth-${uniqueId}@ninerlog-perf.com`;
  const password = 'PerfTestAuth123!';

  // 1. Register
  let res = http.post(`${API}/auth/register`, JSON.stringify({
    email, password, name: `Auth Perf ${uniqueId}`,
  }), { headers: jsonHeaders(), tags: { name: 'POST /auth/register' } });

  check(res, { 'register 201': (r) => r.status === 201 }) || authErrors.add(1);

  // 2. Login
  res = http.post(`${API}/auth/login`, JSON.stringify({
    email, password,
  }), { headers: jsonHeaders(), tags: { name: 'POST /auth/login' } });

  const loginOk = check(res, { 'login 200': (r) => r.status === 200 });
  if (!loginOk) {
    authErrors.add(1);
    return;
  }
  loginDuration.add(res.timings.duration);

  const body = JSON.parse(res.body);
  const token = body.accessToken;
  const refreshToken = body.refreshToken;

  sleep(0.5);

  // 3. Refresh token
  res = http.post(`${API}/auth/refresh`, JSON.stringify({
    refreshToken,
  }), { headers: jsonHeaders(), tags: { name: 'POST /auth/refresh' } });

  check(res, { 'refresh 200': (r) => r.status === 200 }) || authErrors.add(1);
  refreshDuration.add(res.timings.duration);

  sleep(0.5);

  // 4. Access protected endpoint (verify token works)
  res = http.get(`${API}/users/me`, {
    headers: authHeaders(token),
    tags: { name: 'GET /users/me' },
  });

  check(res, { 'get profile 200': (r) => r.status === 200 }) || authErrors.add(1);

  sleep(0.5);
}
