// k6 shared helpers for NinerLog performance tests

import http from 'k6/http';

export const BASE_URL = __ENV.PERF_API_URL || 'http://localhost:3334';
export const API = `${BASE_URL}/api/v1`;

// JSON headers without auth
export function jsonHeaders() {
  return { 'Content-Type': 'application/json' };
}

// JSON headers with auth
export function authHeaders(token) {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };
}

// Register a new user, return { email, password }
export function registerUser(email, password, name) {
  const res = http.post(`${API}/auth/register`, JSON.stringify({
    email, password, name,
  }), { headers: jsonHeaders() });
  return res;
}

// Login and return { token, refreshToken }
export function loginUser(email, password) {
  const res = http.post(`${API}/auth/login`, JSON.stringify({
    email, password,
  }), { headers: jsonHeaders() });

  if (res.status === 200) {
    const body = JSON.parse(res.body);
    return { token: body.accessToken, refreshToken: body.refreshToken };
  }
  return null;
}

// Get a seeded user's credentials by VU index
export function seededUser(vuIdx) {
  const idx = vuIdx % 100; // 100 seeded users
  return {
    email: `perfuser-${String(idx).padStart(4, '0')}@ninerlog-perf.com`,
    password: 'PerfTest123!Secure',
  };
}
