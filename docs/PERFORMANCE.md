# NinerLog Performance Baselines & Thresholds

> Last updated: 2026-04-21
> Platform: Apple M2 (macOS), Docker Desktop, PostgreSQL 18

## Table of Contents

- [API Performance Thresholds](#api-performance-thresholds)
- [Go Benchmark Baselines](#go-benchmark-baselines)
- [k6 Load Test Scenarios](#k6-load-test-scenarios)
- [Frontend Bundle Size Baselines](#frontend-bundle-size-baselines)
- [Frontend Performance Thresholds (Lighthouse)](#frontend-performance-thresholds-lighthouse)
- [How to Run Performance Tests](#how-to-run-performance-tests)
- [Interpreting Results](#interpreting-results)

---

## API Performance Thresholds

| Endpoint Category | p95 Threshold | p99 Threshold | Max Error Rate |
|-------------------|---------------|---------------|----------------|
| Read endpoints (GET) | < 200ms | < 500ms | < 1% |
| Write endpoints (POST/PUT/DELETE) | < 500ms | < 1000ms | < 1% |
| Export endpoints (PDF/CSV) | < 2000ms | < 5000ms | < 1% |
| Auth endpoints | < 500ms | < 1000ms | < 1% |
| Spike test (200 VUs) | < 1000ms | < 3000ms | < 5% |

---

## Go Benchmark Baselines

Benchmarked on Apple M2, Go 1.25, `go test -bench=. -benchmem`

### Flight Auto-Calculations (`flightcalc.ApplyAutoCalculations`)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| ApplyAutoCalculations | ~575 | 32 | 2 |
| ApplyAutoCalculations_NightFlight | ~574 | 32 | 2 |
| ApplyAutoCalculations_WithCrew | ~607 | 176 | 3 |

### Currency Evaluation (`currency.Service`)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| EASAEvaluator (single rating) | ~554 | 536 | 11 |
| EvaluateAll (1 license, 1 rating) | ~981 | 1072 | 18 |
| EvaluateAll (1 license, 3 ratings) | ~2889 | 4537 | 46 |

### Flight Service (`service.FlightService`)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| CreateFlight | ~597 | 638 | 2 |
| ListFlights (500 flights) | ~5042 | 9280 | 7 |
| GetFlight (by ID) | ~8 | 0 | 0 |

---

## k6 Load Test Scenarios

### Authentication Flow
- **VUs:** 100 | **Duration:** 2 min
- **Operations:** register → login → refresh → get profile
- **Thresholds:** login p95 < 300ms, refresh p95 < 200ms

### Flight CRUD
- **VUs:** 50 | **Duration:** 3 min
- **Operations:** create → list → get → update → delete
- **Thresholds:** create p95 < 500ms, list p95 < 200ms, get p95 < 100ms

### Search & Filtering
- **VUs:** 50 | **Duration:** 2 min
- **Operations:** text search, date range, airport filter, aircraft filter, pagination
- **Thresholds:** all p95 < 200ms

### Dashboard & Statistics
- **VUs:** 100 | **Duration:** 2 min
- **Operations:** currency status, user statistics, reports (trends, routes, airport-stats, stats-by-class)
- **Thresholds:** currency p95 < 300ms, stats p95 < 200ms, reports p95 < 300ms

### Report Export
- **VUs:** 10 | **Duration:** 2 min
- **Operations:** PDF export, CSV export, JSON export
- **Thresholds:** PDF p95 < 2000ms, CSV p95 < 1000ms, JSON p95 < 1000ms

### Spike Test
- **VUs:** 0 → 200 ramp (30s), hold (1 min), ramp down (30s)
- **Operations:** mixed read operations (flights, currency, statistics, aircraft, reports)
- **Thresholds:** p95 < 1000ms, error rate < 5%

### k6 Baseline Results (Apple M2, Docker Desktop, 2026-04-21)

| Scenario | VUs | Duration | Iterations | Requests | Error Rate | p95 | Status |
|----------|-----|----------|------------|----------|------------|-----|--------|
| Auth Flow | 100 | 2m | 1,239 | 4,956 | 0.00% | 4.87s (login) | ⚠️ Thresholds crossed (local Docker bcrypt overhead) |
| Flight CRUD | 50 | 3m | 6,251 | 31,305 | 0.00% | 23ms (create), 25ms (list) | ✅ All passed |
| Search & Filter | 50 | 2m | 2,950 | 23,650 | 0.00% | 32ms (search), 29ms (filter) | ✅ All passed |
| Dashboard | 100 | 2m | 8,939 | 62,673 | 0.00% | 28ms (currency), 16ms (stats) | ✅ All passed |
| Exports | 10 | 2m | 400 | 1,210 | 0.00% | 58ms (PDF), 23ms (CSV), 38ms (JSON) | ✅ All passed |
| Spike (200 VUs) | 200 | 2m | 18,081 | 18,181 | 0.00% | 11ms | ✅ All passed |

> **Note:** Auth scenario thresholds are crossed due to bcrypt's intentional CPU cost on local Docker.
> In production (with dedicated CPU), expect login p95 < 300ms.

---

## Frontend Bundle Size Baselines

> Build: Vite 7.3, React 19, production build (no sourcemaps)

| Metric | Size | Gzipped |
|--------|------|---------|
| **Total JS** | 1,551 KB | ~470 KB |
| **Total CSS** | 113 KB | ~20 KB |
| **Main vendor chunk** (`index`) | 463 KB | ~151 KB |
| **Largest page chunk** (`ReportsPage`) | 410 KB | ~119 KB |
| **Map page** (`MapPage`) | 160 KB | ~47 KB |
| **Help page** (`HelpPage`) | 162 KB | ~49 KB |
| **API/schemas** | 132 KB | ~43 KB |

### Bundle Size Thresholds

| Metric | Warning | Failure |
|--------|---------|---------|
| Total JS increase vs main | > 5% | > 15% |
| Any single chunk | > 500 KB | > 750 KB |

---

## Frontend Performance Thresholds (Lighthouse)

| Metric | Minimum Score / Max Value |
|--------|--------------------------|
| Performance score | ≥ 90 |
| Accessibility score | ≥ 90 (warn) |
| Best Practices score | ≥ 90 (warn) |
| First Contentful Paint | < 2.0s |
| Largest Contentful Paint | < 2.5s |
| Cumulative Layout Shift | < 0.1 |
| Total Blocking Time | < 300ms |
| Time to Interactive | < 3.5s |

---

## How to Run Performance Tests

### API — Go Benchmarks

```bash
cd ninerlog-api

# All benchmarks
make bench

# Specific package
go test -run='^$' -bench=. -benchmem ./internal/service/flightcalc/
go test -run='^$' -bench=. -benchmem ./internal/service/currency/
go test -run='^$' -bench=. -benchmem ./internal/service/
```

### API — k6 Load Tests

**Prerequisites:** [Docker](https://docs.docker.com/get-docker/), [k6](https://k6.io/docs/get-started/installation/)

```bash
cd ninerlog-api

# Run all scenarios (starts Docker stack, seeds data, runs tests, stops stack)
make test-perf
# or
./scripts/run-perf-tests.sh

# Seed only (keep stack running for manual testing)
./scripts/run-perf-tests.sh --seed-only

# Run specific scenario
./scripts/run-perf-tests.sh auth
./scripts/run-perf-tests.sh flights

# Skip seeding (re-use existing data)
./scripts/run-perf-tests.sh --skip-seed flights

# Keep stack running after tests
./scripts/run-perf-tests.sh --keep
```

### Frontend — Bundle Analysis

```bash
cd ninerlog-frontend

# Build with bundle visualization (generates dist/stats.html)
npm run build:analyze

# Open the treemap
open dist/stats.html
```

### Frontend — Lighthouse CI

```bash
cd ninerlog-frontend

# Build first
npm run build

# Run Lighthouse CI (uses .lighthouserc.js config)
npm run lighthouse
```

---

## Interpreting Results

### k6 Output

k6 prints a summary with key metrics:
- **http_req_duration**: p50, p90, p95, p99, max — main latency metric
- **http_req_failed**: percentage of failed requests
- **iterations**: total completed test iterations
- **vus**: concurrent virtual users

**Threshold failures** are printed as `✗` in the output. Any threshold failure means the scenario failed.

### Go Benchmarks

- **ns/op**: nanoseconds per operation (lower is better)
- **B/op**: bytes allocated per operation (lower is better)
- **allocs/op**: heap allocations per operation (lower is better)

Compare against baselines above. Significant regressions (>20% slower) should be investigated.

### Bundle Size

The `dist/stats.html` treemap shows which dependencies contribute most to bundle size. Large increases usually come from:
- New heavy dependencies (chart libraries, map tiles, etc.)
- Accidentally importing entire libraries instead of tree-shaking

### Lighthouse

Scores are 0-100. Key metrics:
- **LCP** (Largest Contentful Paint): when the main content is visible
- **CLS** (Cumulative Layout Shift): visual stability
- **TBT** (Total Blocking Time): main thread responsiveness
