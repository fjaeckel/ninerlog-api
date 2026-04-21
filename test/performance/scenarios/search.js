// k6 scenario: Flight search & filtering
// Search with date range, airport, aircraft type filters + pagination
//
// Run: k6 run test/performance/scenarios/search.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { API, authHeaders, seededUser, loginUser } from '../helpers.js';

const searchErrors = new Rate('search_errors');
const searchDuration = new Trend('search_duration', true);
const filterDuration = new Trend('filter_duration', true);
const paginationDuration = new Trend('pagination_duration', true);

export const options = {
  scenarios: {
    search_filter: {
      executor: 'constant-vus',
      vus: 50,
      duration: '2m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<300', 'p(99)<600'],
    http_req_failed: ['rate<0.01'],
    search_errors: ['rate<0.01'],
    search_duration: ['p(95)<200'],
    filter_duration: ['p(95)<200'],
    pagination_duration: ['p(95)<200'],
  },
};

export function setup() {
  const tokens = [];
  for (let i = 0; i < 50; i++) {
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

  // 1. Search by text
  let res = http.get(`${API}/flights?search=Performance+test&page=1&pageSize=20`, {
    headers, tags: { name: 'GET /flights?search' },
  });
  check(res, { 'search 200': (r) => r.status === 200 }) || searchErrors.add(1);
  searchDuration.add(res.timings.duration);

  sleep(0.3);

  // 2. Filter by date range
  res = http.get(`${API}/flights?startDate=2024-06-01&endDate=2024-12-31&page=1&pageSize=20`, {
    headers, tags: { name: 'GET /flights?dateRange' },
  });
  check(res, { 'date filter 200': (r) => r.status === 200 }) || searchErrors.add(1);
  filterDuration.add(res.timings.duration);

  sleep(0.3);

  // 3. Filter by departure airport
  res = http.get(`${API}/flights?departureIcao=EDNY&page=1&pageSize=20`, {
    headers, tags: { name: 'GET /flights?airport' },
  });
  check(res, { 'airport filter 200': (r) => r.status === 200 }) || searchErrors.add(1);
  filterDuration.add(res.timings.duration);

  sleep(0.3);

  // 4. Filter by aircraft registration
  res = http.get(`${API}/flights?aircraftReg=D-EABC-${__VU % 100}&page=1&pageSize=20`, {
    headers, tags: { name: 'GET /flights?aircraftReg' },
  });
  check(res, { 'aircraft filter 200': (r) => r.status === 200 }) || searchErrors.add(1);
  filterDuration.add(res.timings.duration);

  sleep(0.3);

  // 5. Pagination (various pages)
  for (let page = 1; page <= 3; page++) {
    res = http.get(`${API}/flights?page=${page}&pageSize=20&sortBy=date&sortOrder=desc`, {
      headers, tags: { name: 'GET /flights?paginated' },
    });
    check(res, { [`page ${page} 200`]: (r) => r.status === 200 }) || searchErrors.add(1);
    paginationDuration.add(res.timings.duration);
    sleep(0.1);
  }

  // 6. Combined filters
  res = http.get(`${API}/flights?departureIcao=EDNY&startDate=2024-01-01&endDate=2025-12-31&sortBy=date&sortOrder=desc&page=1&pageSize=10`, {
    headers, tags: { name: 'GET /flights?combined' },
  });
  check(res, { 'combined 200': (r) => r.status === 200 }) || searchErrors.add(1);
  filterDuration.add(res.timings.duration);

  sleep(0.5);
}
