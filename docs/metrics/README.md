# NinerLog API — Operations: Metrics, Dashboards & Alerts

This folder contains everything needed to **watch the operational health** of the
NinerLog API and to **get paged when something that matters breaks**.

| What | Where |
|------|-------|
| Metric catalogue (all metrics emitted) | [`docs/METRICS.md`](../METRICS.md) and the table below |
| Grafana dashboards | [`dashboards/`](./dashboards/) |
| Prometheus alerting rules | [`alerts/prometheus-rules.yml`](./alerts/prometheus-rules.yml) |

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

> `path` is the Gin route template (e.g. `/api/v1/flights/:id`) to avoid
> high-cardinality label explosions; unmatched routes use `/*unmatched`.

### Rate limiting

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rate_limit_requests_total` | Counter | `limiter`, `path` | Requests evaluated by a limiter (allowed **and** rejected) |
| `rate_limit_hits_total` | Counter | `limiter`, `path` | Requests rejected by a limiter |

`limiter` is one of `general`, `search`, `expensive`, `auth`, `admin`, `sign`,
`signature_email`. Both metrics use the same route-template `path` as
`http_requests_total`.

> Read them as a ratio, not in isolation: a rejection rate means nothing
> without the traffic it is a fraction of. And because limiters stack while
> `path` excludes the query string, the `limiter` label is the only way to tell
> a throttled `GET /flights?q=…` search from a throttled plain listing — both
> are `/api/v1/flights`.

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

### Airport database

Full reference (including the fetch/merge internals): [`../METRICS.md`](../METRICS.md#airport-database-metrics).

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `airport_db_fetch_total` | Counter | `source`, `result` | Fetch attempts per upstream (`ourairports`, `mwgg`) |
| `airport_db_fetch_errors_total` | Counter | `source`, `reason` | Fetch failures: `request`, `status`, `decode`, `empty` |
| `airport_db_fetch_duration_seconds` | Histogram | `source` | Download + parse latency |
| `airport_db_reload_total` | Counter | `result` | `success`, `partial`, `failed`, `rejected` |
| `airport_db_airports` | Gauge | — | Airports in the active snapshot |
| `airport_db_age_seconds` | Gauge | — | Age of the active snapshot (staleness signal) |
| `airport_lookup_total` | Counter | `operation`, `result` | `lookup`/`search`/`nearest` × `hit`/`miss`/`unavailable` |

### Email delivery

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `email_send_total` | Counter | `result` | `success`, `failure`, `dry_run`, `invalid_address` |
| `email_send_duration_seconds` | Histogram | — | Latency of the SMTP send call (successful and failed attempts) |
| `email_delivery_total` | Counter | `type`, `status` | `delivered`, `hard_bounce`, `soft_bounce`, `rejected`, `invalid_address`, `suppressed`, `server_error`, `dry_run` |
| `email_suppressed_addresses` | Gauge | — | Addresses refused after a permanent delivery failure |
| `unverified_account_reminders_total` | Counter | `result` | `sent`, `undeliverable`, `deferred`, `error` |
| `unverified_accounts_deleted_total` | Counter | — | Accounts reaped for never confirming their address |

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
      - targets: ['api:3000']   # replace 'api' with your service name (see docker-compose.yml); or use localhost:3000 when running the binary directly
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

Configure Alertmanager to route alerts by severity: `severity="critical"`
alerts should go to a paging receiver (PagerDuty / Opsgenie), while `warning`
alerts go to a non-paging channel (Slack / email).

### 4. Import the dashboards

In Grafana: **Dashboards → New → Import → Upload JSON file** and pick the files
in [`dashboards/`](./dashboards/). Select your Prometheus data source when
prompted (the panels reference a templated `${DS_PROMETHEUS}` data source).

Each dashboard also has a **Job** dropdown at the top, populated from
`label_values(app_info, job)`. It defaults to `ninerlog-api`, but if your
`scrape_configs` entry uses a different `job_name` just pick yours from the
dropdown — no editing of panel queries required.

> The **alerting rules are not** parameterized this way: Prometheus evaluates
> them server-side and has no access to a Grafana variable. If your scrape job
> is not named `ninerlog-api`, search and replace `job="ninerlog-api"` in
> [`alerts/prometheus-rules.yml`](./alerts/prometheus-rules.yml) before loading
> it.

---

## Dashboards

All five are tagged `ninerlog` and cross-link to each other through the
dashboard dropdown in the top-right.

| File | Focus |
|------|-------|
| [`dashboards/ninerlog-api-overview.json`](./dashboards/ninerlog-api-overview.json) | RED method: request rate, error rate, latency percentiles (global and per-route), in-flight, panics, response sizes, 4xx breakdown, rate-limit hits by limiter |
| [`dashboards/ninerlog-operational.json`](./dashboards/ninerlog-operational.json) | Service health and version, DB pool and utilization, notification job freshness and errors, email delivery and SMTP latency, login/refresh/2FA, Go runtime |
| [`dashboards/ninerlog-ratelimits.json`](./dashboards/ninerlog-ratelimits.json) | **Start here to tune a limit.** Rejection ratio per limiter, search headroom and latency cost, rejections by route, and a cross-check against the 429s actually served |
| [`dashboards/ninerlog-airports.json`](./dashboards/ninerlog-airports.json) | Airport database: snapshot age and size, reload outcomes, upstream fetch failures by source and reason, merge composition, lookup hit/miss/unavailable rates |
| [`dashboards/ninerlog-accounts.json`](./dashboards/ninerlog-accounts.json) | Sign-in and account lifecycle: OIDC login flow by result, WebAuthn ceremonies started vs completed vs expired, verification reminders and unverified-account deletions |

`ninerlog-operational.json` carries both email views: `email_send_total` for the
SMTP transport and `email_delivery_total` broken down by status, which is the
one that separates a dead mailbox from a broken mail setup.

### Keeping them honest

Dashboards reference metric names as strings, so a rename in Go leaves a
silently empty panel. `make dashboard-check` (or
`python3 scripts/check-dashboards.py`) verifies that every metric referenced by
a panel is actually declared in the Go source, that panels do not overlap or
run past the 24-column grid, and warns about metrics that are emitted but
charted nowhere. Run it after changing either a dashboard or a metric — CI runs
it too, so an uncharted or misspelled metric fails the build rather than
quietly producing an empty panel.

## Alerts

See [`alerts/prometheus-rules.yml`](./alerts/prometheus-rules.yml). Critical
(paging) alerts include: target down, service unhealthy, high 5xx error rate,
DB pool exhaustion, email delivery failing, an empty airport database, and the
notification background job going stale. Warning alerts cover elevated latency,
recovered panics, a spike in login failures (possible brute force), a limiter
rejecting more than 5% of its own traffic, flight search being throttled at all
(a tighter 1% threshold, because search is interactive), a stale airport
database, a failing airport source, a growing email suppression list, and the
unverified-account reaper deleting in bulk or sending its final warning into a
void.

The two account alerts exist because deletion is irreversible and the affected
user is by definition not around to complain: a spike in
`unverified_accounts_deleted_total` is far more often broken verification mail
than a wave of abandoned signups.

The two rate-limit alerts fire on a **ratio**, not an absolute rejection rate:
a flat threshold on rejections/s cannot distinguish a busy service shrugging
off a scraper from a limiter turning away a third of a quiet service's real
traffic.
