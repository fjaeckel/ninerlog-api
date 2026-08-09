---
name: admin-surface
description: Keeping the admin console in step with every feature — which of the /admin endpoints (stats, config, maintenance, audit log) a new featureset has to show up in, and what to change to put it there. Use when adding or removing a feature, adding an env-var toggle or optional subsystem, adding a table users accumulate rows in, adding an admin-only action, or changing a rate limit.
---

# Keeping the admin console complete

The admin console is how the operator sees the deployment. A feature that is invisible
there does not exist for whoever has to run this thing: they cannot tell whether it is
switched on, how much it is being used, or who touched it.

**A feature is not finished until the admin console reflects it.** Same PR as the
behaviour change, exactly like docs and e2e tests.

## The four surfaces

| Endpoint | Handler | Answers |
| --- | --- | --- |
| `GET /admin/stats` | `GetAdminStats` (`internal/api/handlers/admin_dashboard.go`) | Is anyone using it? |
| `GET /admin/config` | `GetAdminConfig` (same file) | Is it switched on, and how is it configured? |
| `POST /admin/maintenance/*` | `admin_dashboard.go`, `admin_email.go` | Can the operator repair or trigger it by hand? |
| `GET /admin/audit-log` | `ListAdminAuditLog` (`admin.go`) | Who did what to it? |

Users, email deliveries/suppressions and announcements have their own endpoints
(`admin_users.go`, `admin_email.go`, `announcements.go`); those are surfaces in their own
right, not a template for new features.

## What your feature owes

Work through these four questions. "None of them" is a legitimate answer for a pure
refactor — it is almost never the right answer for a feature.

| If your feature… | Add |
| --- | --- |
| stores rows users accumulate (a new table, a new entity) | a count in `AdminStats` |
| is gated by an env var, or is an optional subsystem wired conditionally in `cmd/api/main.go` | a flag in `AdminConfig` |
| has a provider/backend choice, a retention window, or a limit | the non-secret detail in `AdminConfig` alongside the flag |
| creates rows that expire, go stale, or leak | a maintenance action |
| lets an admin act on a user or on platform state | an audit-log action name |

## Recipes

All of these are API changes, so they start in the spec — see `api-change`.

### A stat

1. `api-spec/openapi.yaml` → `AdminStats`: add the property. Put it in `required` unless it
   can genuinely be absent (optional becomes a pointer in Go and `?` in the frontend, and
   the UI then has to distinguish "zero" from "not reported").
2. `make generate`.
3. `GetAdminStats`: one more `scanCount(h.db.QueryRowContext(...))` line. Use `scanCount` —
   it logs and falls back to 0. The dashboard must not 500 because one table is missing on
   an older deployment.
4. Breakdowns (counts grouped by a dimension) follow `cloudBackupDestinations`: a nested
   object with a `total` and a `byProvider`-style map, initialised to an empty map so the
   JSON never contains `null`.
5. `test/e2e/admin_e2e_test.go`, `admin stats` subtest — assert the new field is present.

### A config flag

1. `api-spec/openapi.yaml` → `AdminConfig`. **Non-secret only.** JWT keys, SMTP passwords,
   the database DSN, the OIDC client secret and `BACKUP_CREDENTIALS_KEY` have no
   representation here and never will. A public issuer URL, a provider name, a retention
   duration and a boolean are fine.
2. `make generate`.
3. `GetAdminConfig`: derive the flag from the **wired dependency**, not from
   `os.Getenv`. `cmd/api/main.go` already decided whether the subsystem exists;
   `h.backupService != nil`, `h.oidcService != nil`, `h.documentFileService.Enabled()`
   are the established forms. Re-reading the environment in the handler is how the console
   ends up claiming a feature is on while nothing is running.
4. When a subsystem is off for a *reason*, report the reason rather than a bare `false` —
   `UnverifiedCleanupDisabledReason` is the pattern, and the enum lives in the spec.
   Conversely, do not report timings for a subsystem that is not running.
5. `test/e2e/admin_e2e_test.go`, `admin config` subtest.

> `rateLimitAuth` and `rateLimitAdmin` are **hard-coded display strings** in
> `GetAdminConfig` (`"10 req/min"`, `"30 req/min"`). Nothing links them to
> `RateLimitByPath`. If you change a limiter, change these too — otherwise the console
> confidently reports the old number.

### A maintenance action

1. Spec it under `/admin/maintenance/…` with a response body that reports **what it did**
   (row counts, not just `"ok"`) — the operator needs to know whether it was a no-op.
2. `requireAdmin(c)` first; nothing else in the handler runs before it.
3. Call `h.logAdminAction(...)` with the counts in the details map.
4. Guard on the optional service being nil; the action must degrade to a clear message,
   not a panic, when the subsystem is not wired.

### An audited action

Any handler that changes platform state or acts on another user calls:

```go
h.logAdminAction(c, adminUserID, "cleanup_tokens", targetUserID, map[string]any{...})
```

- Action names are stable `snake_case` and are rendered **verbatim** in the frontend audit
  tab — pick something an operator can read.
- `details` is a `map[string]any`, marshalled by `encoding/json`. Never assemble the JSON
  by hand: read the comment on `logAdminAction`, which records two live bugs caused by
  exactly that, one of them attacker-triggerable.
- Pass `targetUserID` whenever the action names a user.

## The other half is in the frontend

Adding a field to `AdminStats` or `AdminConfig` does not put it on screen. The frontend
regenerates its client from this spec and then has to render the field explicitly, with
`en` **and** `de` strings. Finish the job in `ninerlog-frontend` — its `admin-surface`
skill is the counterpart to this one.

Proof that this half gets forgotten: `authMode`, `oidcIssuer` and `documentFilesEnabled`
are all in `AdminConfig` today, all populated by `GetAdminConfig`, and none of them are
rendered anywhere in the admin UI.

## Before you commit

- [ ] Spec changed first, `make generate` run, `internal/api/generated/` not hand-edited
- [ ] New fields populated in `GetAdminStats` / `GetAdminConfig`
- [ ] No secret reachable through `/admin/config`
- [ ] New admin action calls `logAdminAction` with a map payload
- [ ] `test/e2e/admin_e2e_test.go` covers the new field or action (`e2e-sync`)
- [ ] `docs/FEATURES.md` "Administration" section updated; `docs/API.md` if endpoints
      changed (`docs-sync`)
- [ ] Frontend counterpart raised or implemented
