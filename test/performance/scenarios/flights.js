// k6 scenario: Flight CRUD
// create → list (paginated) → get by ID → update → delete
//
// Run: k6 run test/performance/scenarios/flights.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { API, jsonHeaders, authHeaders, seededUser, loginUser } from '../helpers.js';

const flightErrors = new Rate('flight_errors');
const createDuration = new Trend('flight_create_duration', true);
const listDuration = new Trend('flight_list_duration', true);
const getDuration = new Trend('flight_get_duration', true);
const updateDuration = new Trend('flight_update_duration', true);
const deleteDuration = new Trend('flight_delete_duration', true);

export const options = {
  scenarios: {
    flight_crud: {
      executor: 'constant-vus',
      vus: 50,
      duration: '3m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    http_req_failed: ['rate<0.01'],
    flight_errors: ['rate<0.01'],
    flight_create_duration: ['p(95)<500'],
    flight_list_duration: ['p(95)<200'],
    flight_get_duration: ['p(95)<100'],
    flight_update_duration: ['p(95)<500'],
    flight_delete_duration: ['p(95)<200'],
  },
};

export function setup() {
  // Login as a seeded user for each VU to use
  const tokens = [];
  for (let i = 0; i < 50; i++) {
    const user = seededUser(i);
    const auth = loginUser(user.email, user.password);
    if (auth) {
      tokens.push(auth.token);
    }
  }
  return { tokens };
}

export default function (data) {
  const token = data.tokens[__VU % data.tokens.length];
  if (!token) return;
  const headers = authHeaders(token);

  const today = new Date().toISOString().split('T')[0];

  // 1. Create flight
  let res = http.post(`${API}/flights`, JSON.stringify({
    date: today,
    aircraftReg: `D-EABC-${__VU % 100}`,
    aircraftType: 'C172',
    departureIcao: 'EDNY',
    arrivalIcao: 'EDDS',
    offBlockTime: '08:00',
    onBlockTime: '09:30',
    landings: 1,
    remarks: `k6 perf test flight VU${__VU} iter${__ITER}`,
  }), { headers, tags: { name: 'POST /flights' } });

  const createOk = check(res, { 'create 201': (r) => r.status === 201 });
  if (!createOk) {
    flightErrors.add(1);
    return;
  }
  createDuration.add(res.timings.duration);

  const flightId = JSON.parse(res.body).id;

  sleep(0.3);

  // 2. List flights (paginated)
  res = http.get(`${API}/flights?page=1&pageSize=20`, {
    headers, tags: { name: 'GET /flights' },
  });
  check(res, { 'list 200': (r) => r.status === 200 }) || flightErrors.add(1);
  listDuration.add(res.timings.duration);

  sleep(0.2);

  // 3. Get flight by ID
  res = http.get(`${API}/flights/${flightId}`, {
    headers, tags: { name: 'GET /flights/:id' },
  });
  check(res, { 'get 200': (r) => r.status === 200 }) || flightErrors.add(1);
  getDuration.add(res.timings.duration);

  sleep(0.2);

  // 4. Update flight
  res = http.put(`${API}/flights/${flightId}`, JSON.stringify({
    date: today,
    aircraftReg: `D-EABC-${__VU % 100}`,
    aircraftType: 'C172',
    departureIcao: 'EDNY',
    arrivalIcao: 'EDDM',
    offBlockTime: '08:00',
    onBlockTime: '10:00',
    landings: 2,
    remarks: `k6 updated flight VU${__VU}`,
  }), { headers, tags: { name: 'PUT /flights/:id' } });
  check(res, { 'update 200': (r) => r.status === 200 }) || flightErrors.add(1);
  updateDuration.add(res.timings.duration);

  sleep(0.2);

  // 5. Delete flight
  res = http.del(`${API}/flights/${flightId}`, null, {
    headers, tags: { name: 'DELETE /flights/:id' },
  });
  check(res, { 'delete 204': (r) => r.status === 204 }) || flightErrors.add(1);
  deleteDuration.add(res.timings.duration);

  sleep(0.5);
}
