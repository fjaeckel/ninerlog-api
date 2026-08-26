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
  respecting manual `*Override` flags. It also canonicalises the user's fleet and flight
  registrations into the notation their state of registry uses, reporting the outcome as
  `aircraftNormalized`/`aircraftConflicts` — see
  [AIRCRAFT_REGISTRATIONS.md](./AIRCRAFT_REGISTRATIONS.md).
- Rich data: structured approaches, crew members, endorsements, launch method for gliders.
- Fleet facts that change derivation: `isComplex`, `isHighPerformance`, `isTailwheel` and
  `isMultiPilot` on `/aircraft`. Changing `isMultiPilot` takes effect on the next save of a
  flight, or across the whole logbook via `POST /flights/recalculate`.
- FSTD sessions: `isSimulator: true` logs a simulator session (FNPT, FTD, FFS, BATD/AATD)
  instead of a flight. It records the device type and session duration, keeps the
  instrument work (approaches, holds, simulated instrument time), and carries no flight
  time, route or block times at all — training time is never summed with flying time. The
  admin dashboard reports sessions separately as `totalSimulatorSessions`. See
  [DOMAIN.md](./DOMAIN.md#fstd-simulator-sessions).
- Co-pilot time: logged only where the operation provides a co-pilot seat — a
  multi-pilot aircraft (`isMultiPilot` on the fleet entry), a required safety pilot
  (`SafetyPilot` crew role), or a seat the pilot declares by listing themselves as `SIC`
  or entering `sicTime`. Where none applies and another person is pilot-in-command, the
  user was carried rather than crewed: the flight is stored as a passenger flight
  (`isPassenger`), keeping its route and block times and logging no flight time. This is
  what keeps a GA pilot from logging co-pilot time they may not log, while an airline
  first officer's line flying is unaffected. See
  [DOMAIN.md](./DOMAIN.md#who-may-log-co-pilot-time).
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

- **Custom currency rules** (`/custom-currency`, `internal/service/currency/custom*.go`,
  `internal/api/handlers/custom_currency.go`) — a pilot writes their own rule as a
  declarative document: a rolling window, filters selecting which flights count, and
  "at least N" requirements over aggregated flight metrics, combined with AND. The engine
  evaluates it exactly as it does a regulatory rule; the difference is that the rule is user
  data (`custom_currency_rules`, definition stored as JSONB) rather than hardcoded regulation.
  Rules can be paused, opted into expiry emails per rule, and shared read-only by token for
  another pilot to import as their own copy. Because they are stored server-side, they follow
  the pilot to every device they sign in on, and they travel in the JSON export.

## Statistics, reports & maps

- **Statistics** — hour totals and breakdowns per user/license (`reports.go`,
  `admin_dashboard.go`, service aggregation).
- **Reports** — `GET /reports/analytics` backs the whole Reports page from one request
  (`reports_analytics.go`); trends and stats-by-class live in `reports.go`.
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
  Clients that need offline nearest-airport matching (the iOS Share Extension) download
  the whole database as a gzip-compressed pack via `GET /airports/pack`, and poll
  `GET /airports/pack/status` for its content `etag` — hashed over the airport data
  alone, so a refresh that produced identical data does not trigger a re-download.

## Import & export

- **Import** (`internal/api/handlers/import.go`, `import_json.go`) — upload a file
  (CSV/XLSX) → preview → confirm, or import JSON directly.
  Import sessions are tracked (history endpoints).
- **Import templates** (`internal/service/importtemplate`) — the catalogue of logbook
  export formats NinerLog reads. Each template is data: header aliases pointing at import
  fields, the signature columns that identify the source application, a date-format hint,
  and the export instructions shown on the import screen. Upload matches a file's header
  row against the catalogue and records the winning template as the import's
  `format`, which is also what `GET /admin/stats` groups `importsByFormat` by — i.e. which
  logbook pilots are migrating from. `GET /imports/templates` serves the catalogue so the
  import screen can list supported logbooks and how to export from each.

  Covered today: ForeFlight, LogTen Pro, MyFlightbook, capzlog.aero, FLYLOG.io, Wader,
  Vereinsflieger (both the standard and the extended club export), SkyDemon, the generic
  EASA (AMC1 FCL.050) and FAA column layouts, and NinerLog's own CSV export. One vendor may
  need more than one template: Vereinsflieger's two exports differ by three columns out of
  sixteen and share every other alias, so they are told apart by the columns only one of
  them has, and recorded as separate formats because the standard export carries no block
  times and therefore totals airborne time instead.

  Every template in the catalogue rests on a file, or at least a header row, that the
  vendor's own application produced — `testdata/importsamples/` holds them, and its
  `wanted` list is where a new logbook starts. Templates carry a `confidence` of `exact` or
  `best-effort`; `best-effort` means the aliases were inferred rather than seen, and nothing
  in the catalogue is in that state today. It is kept because the distinction is what the
  import screen tells the pilot, and because the evidence says inference does not work here:
  every template ever written from vendor documentation was wrong. FLYLOG.io's was wrong in
  every column but the date. LogTen files were detected as FAA logbooks and imported as
  nothing. Vereinsflieger's missed the aircraft registration, so a club export was detected
  and then failed every row on a field the file was carrying all along.

  Detection never blocks an import. A file that matches nothing is recorded as `CSV` and
  mapped through a cross-vendor alias table, then adjusted on the mapping screen — so an
  unknown logbook degrades to manual mapping rather than a rejection. Two mapping rules
  exist to absorb format differences: a `landingsTotal` column is reconciled against the
  day/night split by taking the larger (so touch-and-goes counted only in a total column
  survive), and departure/arrival are derived from the first and last waypoint of the
  route when the source has no separate airport columns, as MyFlightbook does.

  Adding a logbook means adding a `Template` in `importtemplate/sources.go`, the matching
  `ImportFormat` member in `api-spec/openapi.yaml`, and the value in the `import_format`
  database enum — no handler or service changes.
- **Round-trip guarantee** — *a CSV NinerLog exports is always a CSV NinerLog can import.*
  This is the one interchange path where we own both ends, so it is the one that must never
  regress: a pilot moving between installations, restoring an archived export, or splitting a
  logbook across accounts depends on it. All three export layouts round-trip, each detected as
  its own template — standard → `NINERLOG_CSV`, EASA → `EASA_CSV`, FAA → `FAA_CSV`.

  Two levels of coverage, both mandatory:
  `internal/api/handlers/export_import_roundtrip_test.go` drives the real export writers
  through the real import pipeline across every column layout × date format × decimal
  separator (24 cases, no Docker), and
  `test/e2e/export_import_roundtrip_e2e_test.go` asserts the same invariant over real HTTP
  against a real database, including that flights land in the target account, the fleet is
  backfilled, and re-importing an export into the account it came from is deduplicated rather
  than doubling the logbook.

  Total time survives exactly where the layout carries block times (standard, EASA); the FAA
  layout has no time-of-day columns, so its total goes through a decimal-hours cell rounded to
  0.1h and is asserted within ±3 minutes. Only the standard layout honours the user's
  date-format and decimal-separator preferences — EASA and FAA hardcode their regulatory
  conventions (`export.go:326`, `export.go:362`) — but the full matrix is run against all
  three so that wiring a preference in later lands on existing coverage.
- **Import samples** (`internal/api/handlers/testdata/importsamples/`) — real and generated
  export files run through the whole pipeline and checked against `manifest.json`. This is the
  only place a template meets a complete file rather than a header row it was written from,
  which is what catches metadata preambles, unexpected delimiters and date conventions that
  differ from the template's declaration. Each sample records its `provenance`: `generated`
  (our own export code, authoritative — and a backwards-compatibility guard for files older
  versions produced), `synthetic` (hand-written from a vendor's documented columns; proves
  only self-consistency), or `real` (an anonymised vendor export). Promoting a template from
  `best-effort` to `exact` means replacing its synthetic sample with a real one. The manifest's
  `wanted` list is the standing request for the exports still missing, and a test keeps that
  list in step with the catalogue. Contribution and anonymisation rules: the directory's
  `README.md`.

  The first four real exports each disproved the template written for them: FLYLOG.io was
  wrong in every column but the date, LogTen Pro's Dynamic Export was being claimed by
  `FAA_CSV` (it uses the FAA short column names) and imported as nothing, MyFlightbook was
  putting a marketing description into the aircraft type because the type lives in
  `ICAO Model` rather than `Model`, and Wader used camelCase against a template that assumed
  EASA column names, so it matched nothing and failed every row on four required fields.
  SkyDemon and capzlog.aero then turned out to have no date column at all — both date a
  flight by a timestamped time column, which the importer now falls back on; SkyDemon has no
  total-time column either, so its total is derived from the block times. SkyDemon also
  writes its places as "ICAO Name" ("EDOI Bienenfarm"), so the leading code is extracted
  from the value's own shape rather than by an airport-database lookup — the database is
  fetched at startup and refreshed in the background, and depending on it would make the
  same file import differently on different instances.

  They also turned up five importer defects a header row cannot expose: a UTF-8 BOM breaking
  quoted-header parsing, bare four-digit clock times (`1003`) reaching Postgres unparsed,
  FLYLOG's `SELF` crew marker becoming a contact, Wader's `00:00` placeholder times deriving
  a 777-minute block time for a one-hour flight, and an export of an empty logbook being
  reported as an unparseable file rather than as one with no flights in it. Three products —
  FLYLOG.io, Wader and capzlog.aero — write a literal self-marker into a crew cell for the
  logbook's owner, which the importer drops rather than turning into a contact. A
  `best-effort` template should be read as a hypothesis, not as support.
  Confirming an import also fills in the entities the flights reference: contacts for
  crew names (the same auto-creation that flight create/update performs — see
  **Contacts / people** under Pilot data management), and fleet entries for every registration in the file that the
  user does not own yet (from a ForeFlight export's Aircraft Table when present, otherwise from
  the flight rows' registration/type columns). Rows skipped as duplicates still
  contribute their aircraft, so re-importing a file backfills a fleet an earlier import
  left empty. Registrations are keyed in canonical notation while collecting them, so a
  file that spells one aircraft two ways yields one fleet entry, not two. The confirm
  response reports both counts as `contactsCreated` and `aircraftCreated`.
  Restoring a JSON backup does the same: contacts are not carried in the backup format, so
  the crew names are re-linked by name against the destination account's address book and
  created where they are new, reported as `contactsCreated` in the restore summary.
- **Export** (`export.go`, `export_pdf.go`, `export_pdf_easa.go`,
  `export_pdf_faa.go`, `export_pdf_baseline.go`, `export_pdf_signature.go`,
  `export_crew.go`, `export_vcard.go`) — CSV, JSON, PDF (rendered with
  `go-pdf/fpdf`) and vCard.
  PDF logbooks come in EASA AMC1 FCL.050 and FAA 14 CFR § 61.51 layouts, each as
  a book-style two-page spread (default) or a condensed single-page landscape
  layout, in A4/A5/Letter. Every page carries per-page / carried-forward /
  running totals and a signature strip. The initial-hours snapshot (below) opens
  the carried-forward balance, so the TOTAL TIME row is a career total and not
  just what NinerLog holds; the columns a snapshot cannot supply are documented
  in `export_pdf_baseline.go` and disclosed on the printed page. Every logged row is
  printed, co-pilot (SIC) flights included — co-pilot time is part of total time
  of flight and the EASA layout has a CO-PILOT column for it. Rows that log no
  flight time at all are the exception: a leg the pilot was carried on as a
  passenger is left out of every PDF format, because it would print an empty row
  and total nothing. An FSTD session still prints, in the FSTD columns. CSV and
  JSON carry both. A flight locked by a completed instructor signature prints that
  sign-off in its remarks column — the captured ink, the signer's name and their
  credential number — so a printed logbook carries its endorsements the way a paper
  one does; `export_pdf_signature.go` holds the block's layout ladder and the raster
  bounds it embeds within.
  `GET /exports/vcard` exports the address book as a vCard 3.0 `.vcf` for a phone or mail
  client, carrying each contact's logged crew roles as `CATEGORIES` and a stable `UID` so
  re-importing updates the existing cards.
- **Full backup / restore** (`GET /exports/json`, `POST /imports/json`) — everything the
  pilot owns in one document: flights with crew, aircraft, licences and class ratings,
  credentials, contacts, custom currency rules, notification preferences and the
  carried-forward hours baseline. `cloudbackup.Payload` is the single definition of that
  shape, shared with cloud backup runs, so a manual export and a scheduled backup always
  carry the same data. Restores are additive and regenerate all IDs, so a backup moves
  between installations; `internal/service/cloudbackup/coverage_test.go` fails the build if a
  new table holding user rows is left out.

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
  retention; `jsonbuilder.go` gathers the user's data set into `cloudbackup.Payload` — the
  same payload `GET /exports/json` writes — and gzips it. A SHA-256 over the payload
  (excluding `exportedAt`) skips a run whose data has not changed.
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
  number, not a configuration one. `totalFlights` counts flights only;
  `totalSimulatorSessions` counts FSTD sessions
  (see [DOMAIN.md](./DOMAIN.md#fstd-simulator-sessions)) and `totalPassengerFlights`
  counts flights whose owner was carried rather than crewing
  (see [DOMAIN.md](./DOMAIN.md#passenger-flights)), each kept apart for the same reason
  the logbook keeps them apart. Config view also reports `registrationPrefixCount`
  and `registrationPrefixesReviewed` — the size of the vendored nationality-mark table
  and when it was last checked against upstream, since the table is vendored rather than
  fetched (see [AIRCRAFT_REGISTRATIONS.md](./AIRCRAFT_REGISTRATIONS.md)).
- **Maintenance** — cleanup expired tokens, SMTP test, manually trigger the notification
  check.
- **Update availability** — `GET /admin/update` reports what each component is running
  against what has been published, so a self-hosted operator sees in the admin console
  that an upgrade is waiting. A tagged build is compared against the newest GitHub
  release; a `latest` build, which carries only the commit its image was built from, is
  compared against the head of the tracked branch and reports how many commits it is
  behind. The API knows its own version and commit from its build stamps; the frontend
  reports its own in the request. `UPDATE_CHECK_ENABLED=false` turns the outbound lookup
  off, after which every component reports `unknown`.
- **Announcements** — create/delete platform-wide banners (`SystemAnnouncement`,
  served publicly at `GET /announcements`; managed in `announcements.go`).

## Observability & operations

- **Health** — `GET /health` (used by the Docker healthcheck).
- **Metrics** — `GET /metrics` (Prometheus), plus a DB-stats collector. See
  [METRICS.md](./METRICS.md).
- **Release check** — a daily background lookup of the newest published release per
  component (`internal/updatecheck`), surfaced in the admin console and as
  `app_update_available`. Opt out with `UPDATE_CHECK_ENABLED=false`.
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
