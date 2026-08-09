---
name: metrics-dashboards
description: Instrumenting a feature and keeping Prometheus metrics, docs/METRICS.md, the Grafana dashboards in docs/metrics/dashboards/ and the alert rules in step with each other. Use when adding, renaming or removing a metric, adding a background job, external dependency, cache or rate limiter, or when `make dashboard-check` reports a problem.
---

# Metrics, dashboards and alerts

A metric is four artefacts, not one. Ship all four in the same PR:

| Artefact | Where |
| --- | --- |
| Declaration | `*_metrics.go` next to the code it measures |
| Documentation | `docs/METRICS.md` — the metric reference tables |
| Chart | one of `docs/metrics/dashboards/*.json` |
| Alert | `docs/metrics/alerts/prometheus-rules.yml` — only if a human should act on it |

Dashboards reference metric names as **strings**, so a rename in Go leaves a silently
empty panel and nobody notices until an incident. `make dashboard-check`
(`scripts/check-dashboards.py`) closes that loop: it verifies every metric a panel
references is actually declared in Go, checks panel layout, and warns about metrics that
are emitted but charted nowhere. It is **not wired into CI** — run it yourself before
committing.

## Declaring a metric

Instrumentation lives in a `*_metrics.go` file in the package it measures —
`internal/service/notification_metrics.go`, `internal/airports/metrics.go`,
`pkg/email/metrics.go`. Package-level vars, registered in `init()`:

```go
var NotificationsSentTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notifications_sent_total",
		Help: "Total number of notifications sent.",
	},
	[]string{"type"},
)

func init() {
	prometheus.MustRegister(NotificationsSentTotal, /* … */)
}
```

Rules:

- `snake_case`; counters end `_total`; base units in the name (`_seconds`, `_bytes`) and
  never milliseconds.
- **Document the label values in the Go doc comment.** The checker reads names, not
  meanings; the comment is what stops `result="err"` and `result="error"` coexisting.
- **Labels must be low cardinality.** No user IDs, emails, tokens, flight IDs, or raw URL
  paths. HTTP metrics label `path` with the Gin route template (`/api/v1/flights/:id`);
  unmatched routes collapse to `/*unmatched`. A label whose value comes from user input
  needs a fixed allow-list.
- `scripts/check-dashboards.py` finds names via `prometheus.*Opts{ … Name: "…" }` and
  `NewDesc("…")`. A name built by string concatenation is invisible to it — don't.

## What a new feature should emit

| Feature shape | Emit |
| --- | --- |
| Background job / scheduled worker | runs counter, duration histogram, errors counter, **and a `_last_success_timestamp_seconds` gauge** — staleness is the failure mode a runs-counter cannot show |
| Call to an external service (SMTP, an upstream data source, an object store) | attempts counter labelled by result, a failure counter labelled by *reason*, and a latency histogram |
| Rate limiter | hits/rejections counter labelled by limiter |
| In-memory cache or dataset | size gauge, age gauge, and hit/miss/unavailable counters |
| Optional subsystem | make it obvious in the metrics when it is not running — an absent series and a zero series read very differently on a dashboard |

Then add it to the right table in `docs/METRICS.md`, with type, labels and — for a
`result`-style label — the full list of values.

## Charting it

Pick the dashboard by subject, not by convenience:

| Dashboard | Scope |
| --- | --- |
| `ninerlog-api-overview.json` | RED: request rate, errors, latency, in-flight, panics |
| `ninerlog-operational.json` | Service health, DB pool, background jobs, email, auth |
| `ninerlog-ratelimits.json` | Limiter headroom and rejection ratios |
| `ninerlog-airports.json` | Airport database pipeline and lookups |

If a feature is substantial enough to need its own dashboard, add a new file and
cross-link it from the others' dashboard dropdown, as the existing four do.

Panel JSON constraints the checker enforces:

- unique `uid` across all dashboards; `title`, `panels` and `schemaVersion` present
- every panel has a `gridPos`; `x + w <= 24`; no two panels overlap
- every metric in every `expr` resolves to a declared metric or a known collector built-in

Panels use the templated `${DS_PROMETHEUS}` data source and the `job` template variable
(populated from `label_values(app_info, job)`) rather than a hard-coded job name, so an
operator scraping under a different `job_name` does not have to edit queries.

Deliberately not charting something is a decision you record: add the name to
`UNCHARTED_OK` in `scripts/check-dashboards.py` **with a comment saying why**. Leaving a
permanent warning trains everyone to ignore the warnings.

> Current backlog — declared but charted nowhere: the OIDC login, email delivery,
> suppression, idempotency, unverified-account and WebAuthn-session metrics. If you touch
> one of those areas, chart it rather than adding to the pile.

## Alerting on it

`docs/metrics/alerts/prometheus-rules.yml`, grouped by subject. Add a rule only when a
human should do something about it.

- `severity: critical` pages 24/7; `severity: warning` goes to a non-paging channel.
  Choose deliberately — a paging alert nobody can act on at 03:00 is worse than none.
- Always set `for:` so a scrape blip does not page.
- Alert on **ratios**, not absolute rates, for anything traffic-dependent: a flat
  rejections-per-second threshold cannot tell a busy service shrugging off a scraper from
  a limiter turning away a third of a quiet service's real traffic.
- For background jobs, alert on staleness via the last-success gauge, not on error count.
- Alert expressions are evaluated by Prometheus and cannot use Grafana variables — they
  hard-code `job="ninerlog-api"`. Keep that consistent, and keep it documented in
  `docs/metrics/README.md`.
- Validate with `promtool check rules docs/metrics/alerts/prometheus-rules.yml`.

## Renaming or removing a metric

A rename is a breaking change for every operator running these dashboards. Touch all of:
the Go declaration, `docs/METRICS.md`, every dashboard `expr`, the alert rules, and
`UNCHARTED_OK` if the old name is listed. `make dashboard-check` catches only the
dashboards — grep for the old name across `docs/` before you call it done, and note the
rename in the PR description.

## Before you commit

- [ ] Metric declared in a `*_metrics.go` file and registered in `init()`
- [ ] Label values enumerated in the doc comment; no unbounded label
- [ ] Row added to the right table in `docs/METRICS.md`
- [ ] Panel added to a dashboard, or the name added to `UNCHARTED_OK` with a reason
- [ ] Alert added, or a conscious decision that none is warranted
- [ ] `make dashboard-check` clean
- [ ] `docs/FEATURES.md` "Observability & operations" updated if the feature is new
      (`docs-sync`)
