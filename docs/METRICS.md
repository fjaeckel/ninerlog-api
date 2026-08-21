# NinerLog API — Prometheus Metrics

The NinerLog API exposes Prometheus metrics at `GET /metrics` (no authentication required).

> **Operations bundle:** Ready-to-import Grafana dashboards and Prometheus
> alerting rules live in
> [`docs/metrics/`](./metrics/). Start there to wire up monitoring and paging.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `METRICS_ENABLED` | `true` | Set to `false` to disable metrics collection |
| `APP_VERSION` | `dev` | Version string exposed in the `app_info` gauge, used only when the binary carries no build stamp |
| `AIRPORT_REFRESH_INTERVAL` | `24h` | How often the airport database is refetched. `off` or `0s` disables the refresher |
| `UPDATE_CHECK_ENABLED` | `true` | Set to `false` to stop looking up published releases on GitHub. No `update_check_*` series are emitted when off |
| `UPDATE_CHECK_INTERVAL` | `24h` | How often the release lookup runs. `off` or `0s` leaves only the lookup performed at startup |
| `UPDATE_CHECK_API_REPO` | `fjaeckel/ninerlog-api` | `owner/name` repository the API's releases are read from |
| `UPDATE_CHECK_FRONTEND_REPO` | `fjaeckel/ninerlog-frontend` | `owner/name` repository the frontend's releases are read from |
| `UPDATE_CHECK_BRANCH` | `main` | Branch an untagged (`latest`) build's commit is compared against |
| `APP_COMMIT` | unset | Commit this build came from, used only when the binary carries no build stamp |

Rate limiting is not a metrics setting, but it is what the rate-limit metrics
below are for:

| Env Var | Default | Description |
|---------|---------|-------------|
| `SEARCH_RATE_LIMIT_PER_MINUTE` | `60` | Requests per minute per user for flight search (`GET /flights?q=`). Values that are unparseable or `<= 0` are ignored with a warning and the default is kept — a limiter of zero would reject every search |
| `DISABLE_RATE_LIMIT` | unset | `true` disables **all** rate limiting. Intended for tests and local development |

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
| `notification_check_errors_total` | Counter | — | Check runs that aborted early due to an error (e.g. failing to load preferences) |
| `notification_last_success_timestamp_seconds` | Gauge | — | Unix timestamp of the last successfully completed check run. Use for staleness alerting |

### OIDC Login Metrics

Emitted only in OIDC mode (`OIDC_ISSUER` set). Browser-facing errors are coarse by
design, so this metric is where the actual failure reason surfaces. See
[OIDC.md](./OIDC.md).

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `auth_oidc_login_attempts_total` | Counter | `result` | Steps of the OIDC login flow. `result`: `authorize`, `authorize_native`, `callback_success`, `success`, `authorize_failed`, `provider_error`, `provider_unavailable`, `invalid_state`, `email_missing`, `email_conflict`, `account_disabled`, `handoff_invalid`, `error` |

A gap between `authorize` and `callback_success` means users are dropping out at the
provider. `invalid_state` rising means expired logins, replayed callbacks, or a proxy
eating the binding cookie; `email_conflict` means addresses collide with pre-existing
local accounts (see the migration guidance in OIDC.md).

### WebAuthn Ceremony Metrics

Emitted only when passkeys are enabled (`WEBAUTHN_RP_ID` set). See
[AUTHENTICATION.md](./AUTHENTICATION.md) for the ceremony design.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `webauthn_sessions_created_total` | Counter | `ceremony` | Ceremony sessions opened. `ceremony`: `registration`, `login` |
| `webauthn_sessions_consumed_total` | Counter | `ceremony`, `result` | Finish attempts. `result`: `ok`, `rejected`. A rejection covers every unusable handle — expired, replayed, wrong ceremony, or forged |
| `webauthn_sessions_evicted_total` | Counter | — | Sessions dropped by the per-user open-ceremony cap. Sustained growth means users are abandoning ceremonies |
| `webauthn_sessions_expired_total` | Counter | — | Expired rows removed by the cleanup tick. Sustained growth on the discoverable-login path is the signal worth alerting on, since those writes are unauthenticated |

A sustained rise in `rejected` relative to `ok` is worth attention: it indicates
either replay attempts or clients that are losing the handle between the two
requests.

### Airport Database Metrics

The in-memory airport database (`internal/airports`) merges two upstream datasets —
`ourairports` (CSV) and `mwgg` (JSON) — at startup and on every refresh.

**Fetch and load**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `airport_db_fetch_total` | Counter | `source`, `result` | Fetch attempts per source. Sources: `ourairports`, `mwgg`. Results: `success`, `error` |
| `airport_db_fetch_errors_total` | Counter | `source`, `reason` | Fetch failures. Reasons: `request` (network/DNS), `status` (non-200), `decode` (schema drift or corrupt body), `empty` (0 records parsed) |
| `airport_db_fetch_duration_seconds` | Histogram | `source` | Download + parse latency per source |
| `airport_db_fetch_bytes` | Gauge | `source` | Size of the last successful download |
| `airport_db_source_records` | Gauge | `source` | Records parsed by the last successful fetch, before merging |
| `airport_db_load_duration_seconds` | Histogram | — | Duration of a full reload: both fetches, merge, and index build |
| `airport_db_merge_duration_seconds` | Histogram | — | Merge + index build only, excluding network time |
| `airport_db_reload_total` | Counter | `result` | Reload outcomes: `success` (both sources), `partial` (one source failed), `failed` (no source usable), `rejected` (result below 50% of the live snapshot, swap refused) |

**Merged database**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `airport_db_airports` | Gauge | — | Airports in the active snapshot |
| `airport_db_records_by_origin` | Gauge | `origin` | Where the snapshot's records came from: `ourairports` (only there), `mwgg` (only there), `both` (merged) |
| `airport_db_merge_preferred` | Gauge | `source` | For airports in both datasets, which source supplied the winning base record |
| `airport_db_dropped_records` | Gauge | — | Records discarded in the last merge for unusable coordinates |
| `airport_db_last_success_timestamp_seconds` | Gauge | — | Unix timestamp of the last snapshot swap |
| `airport_db_age_seconds` | Gauge | — | Seconds since the active snapshot was loaded; `0` when none is loaded |

**Lookups**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `airport_lookup_total` | Counter | `operation`, `result` | Read-path calls. Operations: `lookup` (exact ICAO), `search` (ICAO prefix), `nearest` (coordinates). Results: `hit`, `miss`, `unavailable` (no database loaded) |
| `airport_lookup_duration_seconds` | Histogram | `operation` | Latency of the scanning operations (`search`, `nearest`) only — the exact-match path is a single map hit and is not timed |

**Downloadable pack**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `airport_pack_requests_total` | Counter | `endpoint`, `result` | `GET /airports/pack` (`pack`) and `GET /airports/pack/status` (`status`) requests. Results: `success`, `unavailable` (no database loaded) |
| `airport_pack_build_duration_seconds` | Histogram | — | Duration of assembling the gzip-compressed pack (marshal, hash, gzip), once per snapshot on first request |
| `airport_pack_bytes` | Gauge | — | Gzip-compressed size of the current pack |

> **Why this matters:** a `result="unavailable"` rate above zero means flights are being
> served without airport names, distances, or night-time calculations. Alert on
> `airport_db_age_seconds` rather than on reload failures alone — a single failed refresh
> is harmless because the previous snapshot keeps serving, but a database that stops
> refreshing for days is drifting away from the upstream data.

### Release Update Check Metrics

The release check (`internal/updatecheck`) compares the running components against the
newest release published for each component's repository, or — for a build carrying only
a commit, which is what the `latest` image tags are — against the head of
`UPDATE_CHECK_BRANCH`. It is skipped entirely when `UPDATE_CHECK_ENABLED=false`, in which
case none of these series exist.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `update_check_runs_total` | Counter | `result` | Release check runs. Results: `success` (every component read), `error` (at least one failed) |
| `update_check_errors_total` | Counter | `reason` | Lookup failures. Reasons: `request` (network/DNS), `status` (non-200, including GitHub rate limiting), `decode` (unreadable body), `empty` (release without a tag) |
| `update_check_duration_seconds` | Histogram | — | Duration of one run, covering every component queried |
| `update_check_last_success_timestamp_seconds` | Gauge | — | Unix timestamp of the last run in which every component was read |
| `app_update_available` | Gauge | `component` | 1 when a newer build exists, 0 when current. Only `component="api"`: the frontend's running build is known to the browser, not to the API. The series is absent while the API build carries neither a semantic version nor a commit |
| `app_commits_behind` | Gauge | `component` | Commits the tracked branch is ahead of the running build. Only `component="api"`, and only for a build compared by commit (the `latest` tag); absent for a tagged release build |
| `update_check_latest_version_info` | Gauge (const 1) | `component`, `version` | Newest published release per component. Components: `api`, `frontend` |

> **Why this matters:** `app_update_available` is the series to alert on for a
> self-hosted deployment — it is the only signal that a security fix has shipped
> and this instance has not taken it. A failing check is not urgent on its own,
> which is why staleness (`update_check_last_success_timestamp_seconds`) rather
> than `update_check_errors_total` is what the alert rule watches.

### Email Delivery Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `email_send_total` | Counter | `result` | Email send attempts. Results: `success`, `failure`, `dry_run`, `invalid_address` |
| `email_send_duration_seconds` | Histogram | — | Latency of the SMTP send call (both successful and failed attempts) |
| `email_delivery_total` | Counter | `type`, `status` | Send attempts by message type and SMTP outcome. Statuses: `delivered`, `hard_bounce`, `soft_bounce`, `rejected`, `invalid_address`, `suppressed`, `server_error`, `dry_run` |
| `email_suppressed_addresses` | Gauge | — | Addresses currently refused after a permanent delivery failure |
| `unverified_account_reminders_total` | Counter | `result` | Follow-up verification reminders. Results: `sent`, `undeliverable`, `deferred`, `error` |
| `unverified_accounts_deleted_total` | Counter | — | Accounts reaped for never confirming their address |

> **Why this matters:** every user-facing notification is delivered over SMTP.
> `notifications_sent_total` only increments on success, so SMTP outages are
> invisible without `email_send_total{result="failure"}`.
>
> `email_send_total` stays deliberately coarse for existing dashboards.
> `email_delivery_total` is the one that separates a dead address
> (`hard_bounce`) from a broken mail setup (`server_error`) — the two look
> identical in the coarse counter, and only the first should ever stop mail to a
> user. A rising `email_suppressed_addresses` means real users are losing mail;
> a jump in `unverified_accounts_deleted_total` is worth alerting on, because
> account deletion is irreversible.

### Rate Limiting Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `rate_limit_requests_total` | Counter | `limiter`, `path` | Requests **evaluated** by a limiter, allowed and rejected alike |
| `rate_limit_hits_total` | Counter | `limiter`, `path` | Requests **rejected** by a limiter |

`limiter` names the bucket that evaluated the request: `general`, `search`,
`expensive`, `auth`, `admin`, `sign`, `signature_email`. See
[API.md](./API.md#security-model) for each one's budget and scope.

`path` is the Gin route template, the same normalization `http_requests_total`
uses, so the two can be joined.

> **Always read these as a ratio.** `rate_limit_hits_total` on its own cannot
> tell you whether a limit is correctly sized — 2 rejections/s is a non-event
> against 200 req/s of traffic and an outage against 3 req/s.
> `rate_limit_requests_total` is the denominator that makes it interpretable.

> **The `limiter` label is load-bearing, not decoration.** Limiters stack, and
> the `path` label does not include the query string, so a 429 on
> `/api/v1/flights` is ambiguous without it: the request could have exhausted
> the coarse `general` budget or the `search` budget. Grouping rejections by
> route alone cannot distinguish a throttled free-text search from a throttled
> plain logbook listing.

### Idempotency Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `idempotency_requests_total` | Counter | `outcome` | Requests carrying an `Idempotency-Key` header |

Only requests that opt in are counted, so this doubles as adoption tracking:
today's frontend sends no key and contributes nothing.

| `outcome` | Meaning |
|---|---|
| `executed` | Key claimed, request ran, response stored for replay |
| `replayed` | Stored response returned; the handler did not run |
| `released` | 5xx or panic — the key was freed so the client can retry |
| `in_progress` | 409: an earlier request with this key is still running |
| `mismatch` | 422: the key was already used for a different payload |
| `not_replayable` | 409: the original response was too large to store |
| `invalid_key` | 400: malformed key |
| `body_error` | 413: the request body could not be read |
| `unavailable` | 503: the replay store was unreachable, so the request was refused |

Sustained `mismatch` points at a client deriving keys from too little of the
payload — it is a client bug, not load. Any `unavailable` at all means writes
were refused, and should be read alongside the DB pool metrics above.

See [API.md](./API.md#idempotent-writes) for the full contract and the
`IDEMPOTENCY_*` settings.

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

# Email delivery failure ratio (last 15m)
sum(rate(email_send_total{result="failure"}[15m]))
  / sum(rate(email_send_total[15m]))

# Background notification checker staleness (seconds since last success)
time() - notification_last_success_timestamp_seconds

# Rejection ratio per limiter — the number to tune a limit against
sum by (limiter) (rate(rate_limit_hits_total[5m]))
  / clamp_min(sum by (limiter) (rate(rate_limit_requests_total[5m])), 0.0001)

# Are flight searches being throttled?
sum(rate(rate_limit_hits_total{limiter="search"}[15m]))
  / clamp_min(sum(rate(rate_limit_requests_total{limiter="search"}[15m])), 0.0001)

# Search headroom: allowed searches per second
sum(rate(rate_limit_requests_total{limiter="search"}[5m]))
  - sum(rate(rate_limit_hits_total{limiter="search"}[5m]))

# Cross-check the limiter counters against the 429s actually served
sum(rate(http_requests_total{status="429"}[5m]))
```
