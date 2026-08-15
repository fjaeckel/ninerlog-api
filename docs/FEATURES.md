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
  (single-use 1h token), `…/password-reset`. A reset never bypasses 2FA: an account with
  TOTP enabled must also supply a TOTP or recovery code, the enrolment survives the reset,
  and the owner is mailed a "your password was changed" notice either way. See
  [AUTHENTICATION.md](./AUTHENTICATION.md#password-reset).
- **Brute-force protection** — failed logins increment `FailedLoginAttempts`; after the
  threshold the account is locked (`LockedUntil`).
- **Display preferences** — `PATCH /users/me` stores how a client should present the
  logbook: time format, date format, decimal separator, interface language, the
  informational recency toggles, and which optional columns the flights list shows
  (`flightListColumnMode` `auto`/`custom` plus `flightListColumns`). The API stores and
  validates the choice; rendering it — including hiding columns again when the list is
  too narrow — is the client's job. See [DATA_MODEL.md](./DATA_MODEL.md#core-entities).
- **Code**: `internal/service/auth.go`, handlers in `internal/api/handlers/auth.go`,
  repositories for users / refresh / reset / verification tokens.

### Two-factor authentication (TOTP)
`POST /auth/2fa/setup` provisions a TOTP secret + recovery codes; `…/verify` enables it;
`…/login` completes a 2FA-gated login; `…/disable` turns it off. Implemented in
`internal/service/twofactor.go` (TOTP via `pquerna/otp`). Recovery codes are the
self-service route back in when the authenticator is lost — they satisfy the 2FA step of a
password reset as well as a login — so the admin force-reset is a last resort.

### WebAuthn / passkeys
Optional (enabled when `WEBAUTHN_RP_ID` is set). Registration and login each use an
**options → verify** ceremony; credentials can be listed and deleted. Implemented in
`internal/service/webauthn.go` using `go-webauthn/webauthn`, with credential and session
repositories (`internal/repository/postgres/webauthn.go`). Transient ceremony state lives
in `WebAuthnSession`; persisted passkeys in `WebAuthnCredential`.

The two halves of a ceremony are bound by a single-use handle returned as `sessionId`,
stored only as its SHA-256 and consumed with a single `DELETE … RETURNING`. Because the
state is in Postgres rather than process memory, `options` and `verify` may be served by
different instances and a ceremony survives a restart. A user may hold several ceremonies
open at once, bounded per user by `WEBAUTHN_MAX_OPEN_CEREMONIES` (oldest evicted).
See [AUTHENTICATION.md](./AUTHENTICATION.md#passkeys-webauthn).

### OIDC single sign-on
Optional and off by default; enabled by `OIDC_ISSUER`. It is a **mode switch**, not an
extra login button: with a provider configured, the identity provider owns every account
and NinerLog's own credential paths (password login, registration, email verification,
password reset, TOTP, passkeys) all answer 503.

Authorization-code flow with PKCE, implemented in `internal/service/oidc.go` using
`coreos/go-oidc`. `GET /auth/oidc/authorize` starts the login, the provider returns the
browser to `GET /auth/oidc/callback`, and the SPA redeems a single-use handoff code at
`POST /auth/oidc/exchange` so tokens never travel in a URL. Accounts are provisioned on
first login and keyed by `(issuer, sub)`; email, name and verification status are
re-applied from the ID token on every sign-in.

Clients discover the mode from the public `GET /auth/providers`. Setup, provider recipes,
migration and troubleshooting: [OIDC.md](./OIDC.md).

## Pilot data management

- **Licenses** (`internal/service/license.go`) — CRUD plus per-license statistics. A user
  may hold several licenses across authorities.
- **Class ratings** (`internal/service/class_rating.go`) — nested under a license;
  `ClassType` enum and `ExpiryDate` drive currency and notifications.
- **Aircraft** (`internal/service/aircraft.go`) — the pilot's aircraft; the aircraft class
  links flights to the correct currency bucket.
- **Credentials** (`internal/service/credential.go`) — medicals, language proficiency,
  security clearances, German radio certificates (BZF II / BZF I / AZF); expiry feeds
  notifications.
- **Document files** (`internal/service/document_file.go`) — up to 5 reference
  photos/scans per licence or credential, 5 MB each, **JPEG, PNG or PDF**. Every request
  is authenticated, downloads included: there is no public URL, so a client fetches the
  bytes with its bearer token and renders or downloads the blob.

  The format is decided from the file's own bytes, never from the declared type, but the
  two families give different guarantees:

  - **Images** are verified by parsing the header, and the dimensions that header declares
    are capped so a small file cannot claim a huge canvas. Validation deliberately stops
    at the header — proving every byte means a full decode, which allocates the whole
    pixel buffer, the very cost the dimension cap exists to avoid. A valid header followed
    by trailing bytes is therefore stored as-is.
  - **PDFs** cannot be verified that way at all: nothing in the standard library parses
    PDF, and adding a third-party parser for untrusted input would add attack surface
    rather than remove it. The check is the `%PDF-` signature plus a `%%EOF` trailer, which
    rejects truncated downloads and renamed archives and nothing more.

  Serving is what makes both safe. Responses always carry the sniffed content type behind
  `X-Content-Type-Options: nosniff` and require a bearer token. Images may be rendered
  inline; a **PDF is always sent as an attachment**, because it is an active format that
  can carry scripts and embedded files and we cannot see inside it.

  Uploading and serving user-supplied blobs is an abuse surface an operator may not want
  open at all, so the feature has an off switch: `DOCUMENT_FILES_ENABLED=false` makes
  every `/files` endpoint answer `403` — reads included, since serving the bytes is the
  bandwidth half of the problem. Stored files are retained and reappear if it is switched
  back on. `GET /features` reports the current state and limits so clients hide the UI
  instead of discovering the `403`.
- **Contacts / people** (`internal/service/contact.go`) — reusable people
  (crew/instructors) with search, so names aren't retyped per flight.

  A contact **is** its name: unique per user, case-insensitive, whitespace-trimmed. The
  cockpit role is not part of that identity — it belongs to the crew entry — so one
  person stays one contact whether they fly as PIC, instructor or passenger, on one
  flight or a hundred.

  Contacts are created as a side effect of logging. Every path that writes crew rows —
  `POST`/`PUT /flights`, spreadsheet import confirm, and JSON backup restore — runs the
  names through `ContactService.CrewLinker`, which finds or creates one contact per
  distinct name and links the crew row to it. (Until migration 60 only import did this,
  so a pilot who logged flights in the UI ended up with an empty address book beside a
  logbook full of crew names; that migration also merged the duplicates already stored.)

  Editing follows the link. Renaming a contact rewrites the crew entries on the user's
  unsigned flights so a corrected spelling reaches the logbook, and stops at flights
  carrying a completed instructor signature, whose crew names are attested content.
  Deleting a contact removes only the address-book entry: the crew entry keeps the name
  it was logged with, so nothing in the logbook changes and the delete is always allowed.

  `GET /exports/vcard` hands the whole address book to a phone or mail client as a
  vCard 3.0 `.vcf`, with each person's logged crew roles as `CATEGORIES`.
- **Baseline** (`internal/service/flight.go` + `FlightBaseline`) — carried-over totals from
  a previous logbook so statistics, the Reports page and the printed PDF logbook all
  reflect full history.

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
- Airport names: flight responses carry read-only `departureAirportName` /
  `arrivalAirportName`, resolved per request from the in-memory airport database
  (`internal/airports`) for display. Only the location itself is stored; the names are
  `null` when it does not resolve — off-airport glider and helicopter sites are stored as
  free text — and clients then show the raw stored value.

## Currency

`GET /currency` and `GET /licenses/{id}/currency` evaluate regulatory currency via the
evaluator-registry engine in `internal/service/currency` (handlers in
`internal/api/handlers/currency.go`). Full design in
[DOMAIN.md](./DOMAIN.md#currency-engine).

## Statistics, reports & maps

- **Statistics** — hour totals and breakdowns per user/license (`reports.go`,
  `admin_dashboard.go`, service aggregation).
- **Reports** — `GET /reports/analytics` backs the whole Reports page from one request
  (`reports_analytics.go`); trends and stats-by-class live in `reports.go`. Some report
  routes are registered manually via `RegisterReportsRoutes` (not generated from the spec).
- **Initial hours** — a per-user snapshot of pre-existing experience (`FlightBaseline`).
  It is added to the totals of both the statistics endpoint and the Reports analytics
  totals whenever the requested range reaches back to its cutoff date, so the dashboard
  and the Reports page show the same career totals. The PDF logbook export applies it
  too, as the opening carried-forward balance on the sheets.
- **Maps** — airport lookup/search backed by the in-memory airport database
  (`internal/airports`), plus route and airport-activity statistics (`maps.go`).
  The database merges two upstream datasets — OurAirports (CSV) and mwgg/Airports
  (JSON) — record by record, keeping the more complete entry for each ICAO code and
  filling its gaps from the other. It is loaded at startup and refetched every
  `AIRPORT_REFRESH_INTERVAL` (default 24h); a failed refresh keeps the data already in
  memory. See [METRICS.md](./METRICS.md) for the fetch/load/lookup metrics it exposes.

## Import & export

- **Import** (`internal/api/handlers/import.go`, `import_json.go`) — upload a file
  (CSV/XLSX, including ForeFlight exports) → preview → confirm, or import JSON directly.
  Import sessions are tracked (history endpoints).
  Confirming an import also fills in the entities the flights reference: contacts for
  crew names (the same auto-creation that flight create/update performs — see
  **Contacts / people** under Pilot data management), and fleet entries for every registration in the file that the
  user does not own yet (from a ForeFlight export's Aircraft Table when present, otherwise from
  the flight rows' registration/type columns). Rows skipped as duplicates still
  contribute their aircraft, so re-importing a file backfills a fleet an earlier import
  left empty. The confirm response reports both counts as `contactsCreated` and
  `aircraftCreated`.
  Restoring a JSON backup does the same: contacts are not carried in the backup format, so
  the crew names are re-linked by name against the destination account's address book and
  created where they are new, reported as `contactsCreated` in the restore summary.
- **Export** (`export.go`, `export_pdf.go`, `export_pdf_easa.go`,
  `export_pdf_faa.go`, `export_pdf_baseline.go`, `export_crew.go`,
  `export_vcard.go`) — CSV, JSON, PDF (rendered with `go-pdf/fpdf`) and vCard.
  PDF logbooks come in EASA AMC1 FCL.050 and FAA 14 CFR § 61.51 layouts, each as
  a book-style two-page spread (default) or a condensed single-page landscape
  layout, in A4/A5/Letter. Every page carries per-page / carried-forward /
  running totals and a signature strip. The initial-hours snapshot (below) opens
  the carried-forward balance, so the TOTAL TIME row is a career total and not
  just what NinerLog holds; the columns a snapshot cannot supply are documented
  in `export_pdf_baseline.go` and disclosed on the printed page.
  `GET /exports/vcard` exports the address book as a vCard 3.0 `.vcf` for a phone or mail
  client, carrying each contact's logged crew roles as `CATEGORIES` and a stable `UID` so
  re-importing updates the existing cards.
- **Leaving** (`internal/service/portability`) — the CSV and PDF exports above target
  *authorities*; these target *other software*. `GET /exports/logbook?target=…` writes the
  logbook in another product's own import format (ForeFlight Logbook, LogTen Pro,
  MyFlightbook, CrewLounge PILOTLOG), so migrating is one upload rather than a hand-mapped
  spreadsheet; `GET /exports/targets` enumerates the destinations with their caveats so
  clients never hard-code the list. Aircraft flown but never added to the fleet are
  reconstructed from the flights, and training devices are declared as simulators rather
  than aeroplanes, so nothing is dropped and simulator hours do not become flight time.
  Every vendor format is lossy — none carries licences, medicals, contacts, instructor
  signatures or the pre-NinerLog opening balance — so `GET /exports/archive` ships the
  complete account as a documented, versioned ZIP of plain CSV and JSON that needs no
  NinerLog software to read. Full specification, per-destination support matrix and the
  list of layouts still awaiting a live round-trip: [PORTABILITY.md](./PORTABILITY.md).

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
- **Security notices** — the same template set also carries the account-security mails that
  are not currency notifications and are never opt-out: `VerifyEmail`, `PasswordReset`,
  `PasswordChanged` (sent after every completed reset) and `TwoFactorReset` (sent when an
  admin clears a user's 2FA).
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

- **Users** — list, disable/enable, unlock, reset 2FA (mails the user a notice), delete
  (`admin_users.go`).
- **Platform** — stats/dashboard (`admin_dashboard.go`), audit log (`AdminAuditLog`,
  migration 27), config view. `totalContacts` sits alongside the flight and aircraft
  counts: contacts accumulate on their own as crew names are logged, so it is a growth
  number, not a configuration one.
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
- **Idempotent writes** — any authenticated `POST`/`PUT`/`PATCH`/`DELETE` accepts an
  optional `Idempotency-Key` header; a retry with the same key replays the original
  response instead of re-executing, so a client that queues writes while offline cannot
  create duplicate logbook entries. Opt-in per request — omitting the header keeps the
  previous behaviour exactly. See [API.md](./API.md#idempotent-writes).
- **Delta sync** — the list endpoints for flights, aircraft, contacts, credentials and
  licenses accept an `updatedSince` date-time and return only records changed strictly
  after it, so a client that has already pulled a logbook can fetch just what changed
  rather than paging the whole thing. Opt-in per request. See
  [API.md](./API.md#delta-sync-updatedsince).
- **Deletion feed** — `GET /sync/deletions?since=` reports what was deleted across those
  same five collections, which `updatedSince` cannot express: a removed record just stops
  appearing. Tombstones are recorded by database triggers, so bulk and cascaded deletes are
  covered too, and are kept for 90 days by default; a client whose watermark is older than
  that is told to fall back to a full reconciliation. See
  [API.md](./API.md#deletions-get-syncdeletions).
- **Partial updates via JSON Merge Patch** — on the flight, aircraft, credential and class
  rating update endpoints, an explicit `null` in the request body clears a nullable field
  (e.g. `remarks`, `launchMethod`, `expiryDate`) while an omitted field is left unchanged —
  previously `null` was indistinguishable from omitted, so nullable fields (including dates)
  could not be cleared through the API at all. See
  [API.md](./API.md#partial-updates-json-merge-patch).

> When you add a feature, document it here and update the related deep-dive document
> (DATA_MODEL / DOMAIN / API) in the same PR.
