# NinerLog API — Operations: Metrics, Dashboards & Alerts

This folder contains everything needed to **watch the operational health** of the
NinerLog API and to **get paged when something that matters breaks**.

| What | Where |
|------|-------|
| Metric catalogue (all metrics emitted) | [`docs/METRICS.md`](../METRICS.md) and the table below |
| Grafana dashboards | [`dashboards/`](./dashboards/) |
| Prometheus alerting rules | [`alerts/prometheus-rules.yml`](./alerts/prometheus-rules.yml) |
| Example Alertmanager routing | [`alerts/alertmanager.yml`](./alerts/alertmanager.yml) |

The API exposes Prometheus metrics at `GET /metrics` (no auth). Disable with
`METRICS_ENABLED=false`.

---

## Metrics catalogue

### HTTP (RED — Rate, Errors, Duration)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `http_requests_total` | Counter | `method`, `path`, `status` | Total HTTP requests |
| `http_request_duration_seconds` | Histogram | `method`, `path` | Request latency (default buckets) |
| `http_response_size_bytes` | Histogram | `method`, `path` | Response body size |
| `http_requests_in_flight` | Gauge | — | Requests currently being processed |
| `api_panics_recovered_total` | Counter | — | Panics recovered by the recovery middleware |
| `rate_limit_hits_total` | Counter | `path` | Requests rejected by rate limiting |

> `path` is the Gin route template (e.g. `/api/v1/flights/:id`) to avoid
> high-cardinality label explosions; unmatched routes use `/*unmatched`.

### Authentication

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `auth_login_attempts_total` | Counter | `result` | `success`, `invalid_credentials`, `account_locked`, `account_disabled`, `2fa_required`, `email_not_verified`, `error` |
| `auth_token_refresh_total` | Counter | `result` | `success`, `invalid` |
| `auth_2fa_attempts_total` | Counter | `result` | `success`, `invalid_token`, `invalid_code` |

### Application & health

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `app_info` | Gauge (const 1) | `version`, `go_version` | Build information |
| `app_uptime_seconds` | Gauge | — | Seconds since server start |
| `health_check_status` | Gauge | — | 1 = healthy, 0 = unhealthy (includes DB ping) |

### Database connection pool

| Metric | Type | Description |
|--------|------|-------------|
| `db_connections_open` | Gauge | Current open connections |
| `db_connections_in_use` | Gauge | Active (in-use) connections |
| `db_connections_idle` | Gauge | Idle connections |
| `db_connections_max_open` | Gauge | Max open connections configured |
| `db_wait_count_total` | Counter | Total connections waited for |
| `db_wait_duration_seconds_total` | Counter | Total seconds spent waiting for connections |

### Notifications / background job

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `notification_check_runs_total` | Counter | — | Background notification check runs |
| `notification_check_duration_seconds` | Histogram | — | Duration of each check run |
| `notification_check_errors_total` | Counter | — | Check runs that aborted early due to an error |
| `notification_last_success_timestamp_seconds` | Gauge | — | Unix timestamp of the last successful check run (staleness signal) |
| `notifications_sent_total` | Counter | `type` | Notifications sent, by category |

### Email delivery

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `email_send_total` | Counter | `result` | `success`, `failure`, `dry_run`, `invalid_address` |
| `email_send_duration_seconds` | Histogram | — | SMTP delivery latency (successful sends only) |

### Go runtime (built-in collectors)

`go_goroutines`, `go_gc_duration_seconds`, `go_memstats_*`,
`process_cpu_seconds_total`, `process_resident_memory_bytes`, …

---

## Quick start

### 1. Scrape the API

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'ninerlog-api'
    scrape_interval: 15s
    metrics_path: /metrics
    static_configs:
      - targets: ['api:3000']   # docker-compose service name; or localhost:3000
```

### 2. Load the alerting rules

```yaml
# prometheus.yml
rule_files:
  - /etc/prometheus/rules/ninerlog-rules.yml   # alerts/prometheus-rules.yml

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']
```

Validate the rules before deploying:

```bash
promtool check rules alerts/prometheus-rules.yml
```

### 3. Route alerts so they wake you up

Copy [`alerts/alertmanager.yml`](./alerts/alertmanager.yml), fill in your real
receiver (PagerDuty / Opsgenie / Slack / email) and start Alertmanager.
`severity="critical"` alerts are routed to the paging receiver; `warning`
alerts go to a non-paging channel.

### 4. Import the dashboards

In Grafana: **Dashboards → New → Import → Upload JSON file** and pick the files
in [`dashboards/`](./dashboards/). Select your Prometheus data source when
prompted (the panels reference a templated `${DS_PROMETHEUS}` data source).

---

## Dashboards

| File | Focus |
|------|-------|
| [`dashboards/ninerlog-api-overview.json`](./dashboards/ninerlog-api-overview.json) | RED method: request rate, error rate, latency percentiles, in-flight, panics, rate-limit hits |
| [`dashboards/ninerlog-operational.json`](./dashboards/ninerlog-operational.json) | Service health, DB connection pool, notification job freshness, email delivery, auth failures, Go runtime |

## Alerts

See [`alerts/prometheus-rules.yml`](./alerts/prometheus-rules.yml). Critical
(paging) alerts include: target down, service unhealthy, high 5xx error rate,
DB pool exhaustion, email delivery failing, and the notification background job
going stale. Warning alerts cover elevated latency, recovered panics, a spike in
login failures (possible brute force), and rate-limit saturation.
