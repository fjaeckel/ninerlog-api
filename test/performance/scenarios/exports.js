// k6 scenario: Report generation (exports)
// PDF export + CSV export + JSON export for users with many flights
//
// Run: k6 run test/performance/scenarios/exports.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { API, authHeaders, seededUser, loginUser } from '../helpers.js';

const exportErrors = new Rate('export_errors');
const pdfDuration = new Trend('export_pdf_duration', true);
const csvDuration = new Trend('export_csv_duration', true);
const jsonDuration = new Trend('export_json_duration', true);

export const options = {
  scenarios: {
    exports: {
      executor: 'constant-vus',
      vus: 10,
      duration: '2m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<2000', 'p(99)<5000'],
    http_req_failed: ['rate<0.01'],
    export_errors: ['rate<0.01'],
    export_pdf_duration: ['p(95)<2000'],
    export_csv_duration: ['p(95)<1000'],
    export_json_duration: ['p(95)<1000'],
  },
};

export function setup() {
  const tokens = [];
  for (let i = 0; i < 10; i++) {
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

  // 1. PDF export
  let res = http.get(`${API}/exports/pdf`, {
    headers, tags: { name: 'GET /exports/pdf' },
    responseType: 'binary',
  });
  check(res, {
    'pdf 200': (r) => r.status === 200,
    'pdf has content': (r) => r.body && (r.body.byteLength || r.body.length) > 100,
  }) || exportErrors.add(1);
  pdfDuration.add(res.timings.duration);

  sleep(1);

  // 2. CSV export
  res = http.get(`${API}/exports/csv`, {
    headers, tags: { name: 'GET /exports/csv' },
  });
  check(res, {
    'csv 200': (r) => r.status === 200,
    'csv has content': (r) => r.body && r.body.length > 100,
  }) || exportErrors.add(1);
  csvDuration.add(res.timings.duration);

  sleep(1);

  // 3. JSON export
  res = http.get(`${API}/exports/json`, {
    headers, tags: { name: 'GET /exports/json' },
  });
  check(res, {
    'json 200': (r) => r.status === 200,
    'json has content': (r) => r.body && r.body.length > 100,
  }) || exportErrors.add(1);
  jsonDuration.add(res.timings.duration);

  sleep(1);
}
