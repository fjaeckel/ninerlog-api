// k6 seed data generator for NinerLog performance testing.
// Run with: k6 run test/performance/seed.js
//
// Seeds the perf database with: 100 users, ~10,000 flights, aircraft, licenses, ratings
// Each user gets: 2 aircraft, 1 license with 1 rating, ~100 flights

import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';

const BASE_URL = __ENV.PERF_API_URL || 'http://localhost:3334';
const API = `${BASE_URL}/api/v1`;

const NUM_USERS = 100;
const FLIGHTS_PER_USER = 100;

// Deterministic airport pairs for route variety
const AIRPORTS = [
  ['EDNY', 'EDDS'], ['EDDS', 'EDDM'], ['EDDM', 'EDDK'], ['EDDK', 'EDDF'],
  ['EDDF', 'EDDL'], ['EDDL', 'EDDH'], ['EDDH', 'EDDB'], ['EDDB', 'EDNY'],
  ['EDNY', 'LSZH'], ['LSZH', 'LSZB'], ['LSZB', 'LSGG'], ['LSGG', 'LFPG'],
  ['LFPG', 'EGLL'], ['EGLL', 'EHAM'], ['EHAM', 'EDDF'], ['EDDF', 'LOWW'],
];

const AIRCRAFT_TYPES = [
  { registration: 'D-EABC', type: 'C172', make: 'Cessna', model: '172S', aircraftClass: 'SEP_LAND' },
  { registration: 'D-EMEP', type: 'PA44', make: 'Piper', model: 'Seminole', aircraftClass: 'MEP_LAND', isComplex: true },
];

export const options = {
  vus: 10,
  iterations: NUM_USERS,
  thresholds: {
    http_req_failed: ['rate<0.05'],
  },
};

function formatDate(d) {
  return d.toISOString().split('T')[0];
}

function formatTime(hours, minutes) {
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`;
}

export default function () {
  const userIdx = exec.scenario.iterationInTest;
  const email = `perfuser-${String(userIdx).padStart(4, '0')}@ninerlog-perf.com`;
  const password = 'PerfTest123!Secure';
  const name = `Perf User ${userIdx}`;

  // 1. Register
  let res = http.post(`${API}/auth/register`, JSON.stringify({
    email, password, name,
  }), { headers: { 'Content-Type': 'application/json' } });

  if (res.status !== 201) {
    // User might already exist, try login
    res = http.post(`${API}/auth/login`, JSON.stringify({ email, password }), {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  const loginRes = http.post(`${API}/auth/login`, JSON.stringify({ email, password }), {
    headers: { 'Content-Type': 'application/json' },
  });

  check(loginRes, { 'login succeeded': (r) => r.status === 200 });
  if (loginRes.status !== 200) {
    console.error(`Login failed for ${email}: ${loginRes.status} ${loginRes.body}`);
    return;
  }

  const token = JSON.parse(loginRes.body).accessToken;
  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };

  // 2. Create aircraft
  const aircraftIds = [];
  for (const ac of AIRCRAFT_TYPES) {
    const acBody = {
      registration: `${ac.registration}-${userIdx}`,
      type: ac.type,
      make: ac.make,
      model: ac.model,
      aircraftClass: ac.aircraftClass,
    };
    if (ac.isComplex) acBody.isComplex = true;

    const acRes = http.post(`${API}/aircraft`, JSON.stringify(acBody), { headers });
    if (acRes.status === 201) {
      aircraftIds.push(JSON.parse(acRes.body).id);
    }
  }

  // 3. Create license with rating
  const licRes = http.post(`${API}/licenses`, JSON.stringify({
    regulatoryAuthority: 'EASA',
    licenseType: 'PPL',
    licenseNumber: `DE-PPL-PERF-${userIdx}`,
    issueDate: '2022-01-15',
    issuingAuthority: 'LBA',
  }), { headers });

  let licenseId = null;
  if (licRes.status === 201) {
    licenseId = JSON.parse(licRes.body).id;

    // Add SEP_LAND rating
    http.post(`${API}/licenses/${licenseId}/ratings`, JSON.stringify({
      classType: 'SEP_LAND',
      issueDate: '2022-01-15',
      expiryDate: '2027-01-15',
    }), { headers });
  }

  // 4. Create flights (spread over 2 years)
  const baseDate = new Date('2024-01-01');
  for (let i = 0; i < FLIGHTS_PER_USER; i++) {
    const flightDate = new Date(baseDate);
    flightDate.setDate(baseDate.getDate() + Math.floor(i * 7.3)); // ~weekly flights over 2 years

    const routeIdx = (userIdx + i) % AIRPORTS.length;
    const [dep, arr] = AIRPORTS[routeIdx];
    const acIdx = i % AIRCRAFT_TYPES.length;

    const depHour = 7 + (i % 10); // Flights between 07:00 and 16:00
    const duration = 60 + (i % 120); // 60-180 min
    const arrHour = depHour + Math.floor(duration / 60);
    const arrMin = duration % 60;

    const flight = {
      date: formatDate(flightDate),
      aircraftReg: `${AIRCRAFT_TYPES[acIdx].registration}-${userIdx}`,
      aircraftType: AIRCRAFT_TYPES[acIdx].type,
      departureIcao: dep,
      arrivalIcao: arr,
      offBlockTime: formatTime(depHour, 0),
      onBlockTime: formatTime(arrHour, arrMin),
      departureTime: formatTime(depHour, 10),
      arrivalTime: formatTime(arrHour, Math.max(0, arrMin - 10)),
      landings: 1 + (i % 3),
      ifrTime: i % 4 === 0 ? 30 : 0,
      remarks: `Performance test flight ${i + 1}`,
    };

    if (licenseId && i % 5 === 0) {
      flight.licenseId = licenseId;
    }

    http.post(`${API}/flights`, JSON.stringify(flight), { headers });

    // Small sleep to avoid overwhelming the API during seeding
    if (i % 20 === 0) sleep(0.1);
  }

  console.log(`✅ Seeded user ${userIdx}: ${email} with ${FLIGHTS_PER_USER} flights`);
}
