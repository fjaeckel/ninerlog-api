# NinerLog API — Prometheus Metrics

The NinerLog API exposes Prometheus metrics at `GET /metrics` (no authentication required).

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `METRICS_ENABLED` | `true` | Set to `false` to disable metrics collection |
| `APP_VERSION` | `dev` | Version string exposed in `app_info` gauge |

## Metric Reference

### HTTP Request Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `http_requests_total` | Counter | `method`, `path`, `status` | Total HTTP requests |
| `http_request_duration_seconds` | Histogram | `method`, `path` | Request latency in seconds (default buckets) |
| `http_response_size_bytes` | Histogram | `method`, `path` | Response body size in bytes |
| `http_requests_in_flight` | Gauge | — | Number of requests currently being processed |
| `api_panics_recovered_total` | Counter | — | Total panics recovered by the recovery middleware |

> **Path normalization:** The `path` label uses Gin route templates (e.g. `/api/v1/flights/:id`) instead of concrete URLs to prevent high-cardinality label explosions. Unmatched routes use `/*unmatched`.

### Authentication Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `auth_login_attempts_total` | Counter | `result` | Login attempts. Results: `success`, `invalid_credentials`, `account_locked`, `account_disabled`, `2fa_required`, `error` |
| `auth_token_refresh_total` | Counter | `result` | Token refresh attempts. Results: `success`, `invalid` |
| `auth_2fa_attempts_total` | Counter | `result` | 2FA verification attempts. Results: `success`, `invalid_token`, `invalid_code` |

### Application Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `app_info` | Gauge (const 1) | `version`, `go_version` | Build information for Grafana dashboards |
| `app_uptime_seconds` | Gauge | — | Seconds since server start |
| `health_check_status` | Gauge | — | 1 = healthy, 0 = unhealthy (includes DB ping) |

### Database Connection Pool Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `db_connections_open` | Gauge | — | Current open database connections |
| `db_connections_in_use` | Gauge | — | Active (in-use) connections |
| `db_connections_idle` | Gauge | — | Idle connections |
| `db_connections_max_open` | Gauge | — | Max open connections configured |
| `db_wait_count_total` | Counter | — | Total connections waited for |
| `db_wait_duration_seconds_total` | Counter | — | Total seconds spent waiting for connections |

### Notification / Background Job Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `notification_check_runs_total` | Counter | — | Total background notification check runs |
| `notification_check_duration_seconds` | Histogram | — | Duration of each check run |
| `notifications_sent_total` | Counter | `type` | Notifications sent. Types: `credential_expiry`, `revalidation`, `passenger_currency`, `night_currency`, `flight_review`, `rating_expiry`, `currency_revalidation`, `currency_flight_review` |

### Rate Limiting Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rate_limit_hits_total` | Counter | `path` | Requests rejected by rate limiting |

### Go Runtime Metrics (built-in)

The default Prometheus Go collector provides:

| Metric | Description |
|--------|-------------|
| `go_goroutines` | Number of goroutines |
| `go_gc_duration_seconds` | GC pause duration summary |
| `go_memstats_alloc_bytes` | Bytes allocated and in use |
| `go_memstats_heap_*` | Heap memory stats |
| `process_cpu_seconds_total` | Total CPU time |
| `process_resident_memory_bytes` | Resident memory size |

## Prometheus Scrape Configuration

```yaml
scrape_configs:
  - job_name: 'ninerlog-api'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:3000']
    metrics_path: /metrics
```

For Docker Compose deployments, use the service name:

```yaml
scrape_configs:
  - job_name: 'ninerlog-api'
    scrape_interval: 15s
    static_configs:
      - targets: ['api:3000']
```

## Example PromQL Queries

```promql
# Request rate per route (req/s)
rate(http_requests_total[5m])

# Error rate (4xx + 5xx as % of total)
sum(rate(http_requests_total{status=~"4..|5.."}[5m])) / sum(rate(http_requests_total[5m])) * 100

# P95 latency by route
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

# P99 latency by route
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))

# Login failure rate
rate(auth_login_attempts_total{result!="success"}[5m])

# DB connection pool utilization
db_connections_in_use / db_connections_max_open * 100

# Notification send rate by type
rate(notifications_sent_total[1h])
```
