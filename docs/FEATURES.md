# Feature Catalogue

This document catalogues every product feature and how it is implemented end-to-end
(HTTP → service → repository → data). Use it to find the code that backs a given
capability. For the data shapes see [DATA_MODEL.md](./DATA_MODEL.md); for the HTTP surface
see [API.md](./API.md); for aviation rules see [DOMAIN.md](./DOMAIN.md).

## Authentication & accounts

End-to-end account lifecycle. See [AUTHENTICATION.md](./AUTHENTICATION.md) for the full
token/2FA/WebAuthn deep-dive.

- **Registration & email verification** — `POST /auth/register` creates a user (bcrypt
  password hash via `pkg/hash`) and issues a single-use email-verification token (24h);
  `POST /auth/verify-email` confirms it, `…/resend` re-sends.
- **Login & tokens** — `POST /auth/login` returns a JWT **access** token plus a **refresh**
  token; `POST /auth/refresh` exchanges a valid refresh token for new tokens. JWTs are
  minted/validated by `pkg/jwt`.
- **Password management** — `POST /auth/change-password`, `…/password-reset-request`
  (single-use 1h token), `…/password-reset`.
- **Brute-force protection** — failed logins increment `FailedLoginAttempts`; after the
  threshold the account is locked (`LockedUntil`).
- **Code**: `internal/service/auth.go`, handlers in `internal/api/handlers/auth.go`,
  repositories for users / refresh / reset / verification tokens.

### Two-factor authentication (TOTP)
`POST /auth/2fa/setup` provisions a TOTP secret + recovery codes; `…/verify` enables it;
`…/login` completes a 2FA-gated login; `…/disable` turns it off. Implemented in
`internal/service/twofactor.go` (TOTP via `pquerna/otp`).

### WebAuthn / passkeys
Optional (enabled when `WEBAUTHN_RP_ID` is set). Registration and login each use an
**options → verify** ceremony; credentials can be listed and deleted. Implemented in
`internal/service/webauthn.go` using `go-webauthn/webauthn`, with credential and session
repositories (`internal/repository/postgres/webauthn.go`). Transient ceremony state lives
in `WebAuthnSession`; persisted passkeys in `WebAuthnCredential`.

## Pilot data management

- **Licenses** (`internal/service/license.go`) — CRUD plus per-license statistics. A user
  may hold several licenses across authorities.
- **Class ratings** (`internal/service/class_rating.go`) — nested under a license;
  `ClassType` enum and `ExpiryDate` drive currency and notifications.
- **Aircraft** (`internal/service/aircraft.go`) — the pilot's aircraft; the aircraft class
  links flights to the correct currency bucket.
- **Credentials** (`internal/service/credential.go`) — medicals, language proficiency,
  security clearances; expiry feeds notifications.
- **Contacts** (`internal/service/contact.go`) — reusable people (crew/instructors) with
  search, so names aren't retyped per flight.
- **Baseline** (`internal/service/flight.go` + `FlightBaseline`) — carried-over totals from
  a previous logbook so statistics reflect full history.

## Flight logging

The core feature. `internal/service/flight.go` orchestrates CRUD and delegates
auto-calculations to `flightcalc.ApplyAutoCalculations`, which composes the helpers in
`internal/service/flightrules` (night/day split, solo, cross-country, distance, crew,
roles, IFR, FSTD, remarks). See [DOMAIN.md](./DOMAIN.md) for the calculation and validation
rules.

- CRUD: `POST/GET/PUT/DELETE /flights`.
- Bulk: `DELETE /flights/delete-all` (`bulk_delete.go`).
- Recalculate: `POST /flights/recalculate` re-runs auto-calculations across flights while
  respecting manual `*Override` flags.
- Rich data: structured approaches, crew members, endorsements, FSTD, launch method for
  gliders.

## Currency

`GET /currency` and `GET /licenses/{id}/currency` evaluate regulatory currency via the
evaluator-registry engine in `internal/service/currency` (handlers in
`internal/api/handlers/currency.go`). Full design in
[DOMAIN.md](./DOMAIN.md#currency-engine).

## Statistics, reports & maps

- **Statistics** — hour totals and breakdowns per user/license (`reports.go`,
  `admin_dashboard.go`, service aggregation).
- **Reports** — trends over time and stats-by-class. Some report routes are registered
  manually via `RegisterReportsRoutes` (not generated from the spec).
- **Maps** — airport lookup/search backed by the in-memory airport database
  (`internal/airports`, loaded at startup from OurAirports data), plus route and
  airport-activity statistics (`maps.go`).

## Import & export

- **Import** (`internal/api/handlers/import.go`, `import_json.go`) — upload a file
  (CSV/XLSX, including ForeFlight exports) → preview → confirm, or import JSON directly.
  Import sessions are tracked (history endpoints).
- **Export** (`export.go`, `export_pdf.go`, `export_crew.go`) — CSV, JSON, and PDF
  (rendered with `go-pdf/fpdf`).

## Notifications

A background job reminds pilots before licenses/ratings/medicals expire and before
currency lapses.

- **Categories** (`internal/models/notification.go`, `NotificationCategory`):
  `credential_medical`, `credential_language`, `credential_security`,
  `credential_other`, `rating_expiry`, `currency_passenger`, `currency_night`,
  `currency_instrument`, `currency_flight_review`, `currency_revalidation`.
- **Preferences** — per-user, per-category opt-in with configurable warning windows
  (`NotificationPreference`); sent notifications are recorded in `NotificationLog` for
  deduplication.
- **Scheduler** — `NotificationService.StartBackgroundChecker(ctx, interval)` runs in a
  goroutine started by `main.go`; the interval comes from `GetCheckInterval()`
  (`NOTIFICATION_CHECK_INTERVAL`, default 1h).
- **Delivery** — emails are sent via `pkg/email` (SMTP) using localized templates
  (`templates_en.go` / `templates_de.go`) chosen by the user's `PreferredLocale`. Email
  metrics are recorded (see [METRICS.md](./METRICS.md)).
- **Code**: `internal/service/notification.go`, handlers in
  `internal/api/handlers/notification.go`.

## Cloud backups

Optional (enabled when the backup credentials encryption key is configured). Pilots can
back up their data to their own storage on a schedule.

- **Providers** (`internal/service/cloudbackup/provider`) — pluggable `Provider` interface
  with `s3`, `sftp`, and `webdav` implementations registered into a provider registry in
  `main.go`. S3 uses `minio-go`, SFTP uses `pkg/sftp`, WebDAV uses `gowebdav`.
- **Destinations** (`destinations.go`) — CRUD; provider config plus schedule and retention
  count. Credentials are **AES-256-GCM encrypted** at rest (`pkg/cryptoutil`); the key
  comes from the environment, never the database.
- **Runs** (`runner.go`, `BackupRun`) — execute a backup, record outcome, enforce
  retention; `jsonbuilder.go` serializes the user's data set.
- **Scheduler** (`scheduler.go`) — a goroutine that triggers due backups; manual runs via
  `POST /backups/destinations/{id}/run`, connectivity check via `…/test`.
- **HTTP**: `internal/api/handlers/backup.go` (list providers, manage destinations,
  test/run, run history).

## Administration

Admin-only endpoints (caller must match `ADMIN_EMAIL`; enforced by the admin middleware):

- **Users** — list, disable/enable, unlock, reset 2FA, delete
  (`admin_users.go`).
- **Platform** — stats/dashboard (`admin_dashboard.go`), audit log (`AdminAuditLog`,
  migration 27), config view.
- **Maintenance** — cleanup expired tokens, SMTP test, manually trigger the notification
  check.
- **Announcements** — create/delete platform-wide banners (`SystemAnnouncement`,
  served publicly at `GET /announcements`; managed in `announcements.go`).

## Observability & operations

- **Health** — `GET /health` (used by the Docker healthcheck).
- **Metrics** — `GET /metrics` (Prometheus), plus a DB-stats collector. See
  [METRICS.md](./METRICS.md).
- **Profiling** — optional pprof server when `PPROF_ENABLED=true`. See
  [PERFORMANCE.md](./PERFORMANCE.md).
- **Structured logging, panic recovery, security headers, CORS, rate limiting** — see the
  middleware chain in [ARCHITECTURE.md](./ARCHITECTURE.md).

> When you add a feature, document it here and update the related deep-dive document
> (DATA_MODEL / DOMAIN / API) in the same PR.
