# Package Reference

A map of every package in `internal/` (private application code) and `pkg/` (reusable,
dependency-light utilities). Use this to find where a responsibility lives. For how they
fit together see [ARCHITECTURE.md](./ARCHITECTURE.md).

## `cmd/`

| Path | Purpose |
| --- | --- |
| `cmd/api/main.go` | Application entry point. Loads config, opens the DB and runs migrations, initialises the airport DB, constructs repositories/services/handlers, builds the Gin router and middleware chain, registers routes, starts background workers, and serves with graceful shutdown. |

## `internal/`

### Transport layer

| Package | Responsibility |
| --- | --- |
| `internal/api/handlers` | Gin handlers. `APIHandler` aggregates all services and implements the generated `ServerInterface`; one file per domain (`auth.go`, `flight.go`, `license.go`, `aircraft.go`, `aircraft_reminder.go`, `credential.go`, `currency.go`, `contact.go`, `notification.go`, `backup.go`, `admin*.go`, `import*.go`, `export*.go`, `maps.go`, `reports.go`, `baseline.go`, `bulk_delete.go`, `twofactor.go`, `webauthn.go`, `oidc.go`, `announcements.go`, `flight_util.go`). |
| `internal/api/middleware` | Cross-cutting HTTP concerns: `auth.go` (JWT), `admin.go` (admin gate), `ratelimit.go`, `idempotency.go` (replay of `Idempotency-Key` writes), `metrics.go` + `app_metrics.go`, `recovery.go`, `logger.go`, `security_headers.go`. |
| `internal/api/generated` | **Auto-generated** from `api-spec/openapi.yaml` via `oapi-codegen` (`types.go`, `server.go`, `spec.go`, `generate.go`). Do not hand-edit. See [API.md](./API.md). |

### Business logic

| Package | Responsibility |
| --- | --- |
<<<<<<< HEAD
| `internal/service` | Domain services: `auth.go`, `flight.go`, `license.go`, `class_rating.go`, `aircraft.go`, `aircraft_reminder.go`, `credential.go`, `contact.go`, `notification.go` (+ `notification_metrics.go`), `twofactor.go`, `webauthn.go`, `oidc.go` (+ `oidc_config.go`), `idempotency.go`, `deletion.go` (tombstone feed + reaper), `save_warnings.go` (save-time checks behind the `warnings` response field: `flightChecks`, `aircraftChecks`), `logbook_scope.go` (`LogbookScope`: resolves a licence logbook into native and credited flights, using `currency.Service.CreditScope`; shared by `GET /flights`, `GET /exports/csv`, `GET /exports/pdf` and custom reports through `logbookLicenseId`). Each takes repository interfaces + `pkg` utilities. |
=======
| `internal/service` | Domain services: `auth.go`, `flight.go`, `license.go`, `class_rating.go`, `aircraft.go`, `aircraft_reminder.go`, `credential.go`, `contact.go`, `notification.go` (+ `notification_metrics.go`), `twofactor.go`, `webauthn.go`, `oidc.go` (+ `oidc_config.go`), `idempotency.go`, `deletion.go` (tombstone feed + reaper), `logbook_scope.go` (`LogbookScope`: resolves a licence logbook into native and credited flights, using `currency.Service.CreditScope`; shared by `GET /flights`, `GET /exports/csv`, `GET /exports/pdf` and custom reports through `logbookLicenseId`). `soaring_season.go` (`SoaringSeasonService`: `GET /reports/soaring-season` over `repository.SoaringRepository`). Each takes repository interfaces + `pkg` utilities. |
>>>>>>> wip/stats
| `internal/service/currency` | The currency engine: `Evaluator`/`Registry`/`FlightDataProvider` (`evaluator.go`), `Service` (`service.go`), authority evaluators (`easa.go`, `faa.go`, `german_ul.go`, `other.go`), and shared logic (`engine.go`, `types.go`). The PostgreSQL implementations of `FlightDataProvider` and `CustomFlightDataProvider` live in `internal/repository/postgres` (`currency_flight_data.go`, `currency_flight_data_daily.go`, `custom_currency_data.go`). `clock.go` carries the evaluation instant in the context, `daily.go` the per-date reads and their per-request cache, `projection.go` the `validUntil` projection and remedy keys. See [DOMAIN.md](./DOMAIN.md#currency-engine). |
| `internal/service/readiness` | `GET /currency/readiness`: evaluates currency as of a date through `currency.Service.EvaluateAsOf` and answers per rating, launch method, passengers and medical certificate, optionally for one aircraft. See [DOMAIN.md](./DOMAIN.md#readiness-get-currencyreadiness). |
| `internal/service/flightcalc` | `ApplyAutoCalculations(flight, userName)` — the single entry point that derives flight fields. |
| `internal/service/flightrules` | Composable flight rules used by `flightcalc`: `night.go` (day/night via solar), `crew.go`, `roles.go`, `names.go`, `ifr.go`, `fstd.go`, `remarks.go`, `display.go`. |
| `internal/service/importtemplate` | The logbook-import template catalogue: `field.go` (import-field constants), `template.go` (the `Template` type + registry), `sources.go` (the templates themselves — ForeFlight, LogTen Pro, MyFlightbook, capzlog.aero, FLYLOG.io, Wader, Vereinsflieger standard + extended, SkyDemon, vsimakhin/web-logbook, generic EASA/FAA, NinerLog), `detect.go` (header normalisation, scored detection, mapping suggestion), `values.go` (cell-value vocabularies: launch-method codes, ForeFlight aircraft classes). Pure data + lookup; imports no generated types, so the handler converts at the edge. |
| `internal/service/cloudbackup` | Cloud backup orchestration: `service.go`, `destinations.go`, `runner.go`, `scheduler.go`, `jsonbuilder.go`. |
| `internal/service/cloudbackup/provider` | Pluggable storage `Provider` interface + registry, with `s3/`, `sftp/`, and `webdav/` implementations. |
| `internal/service/pilotprofile` | Pilot profile (adaptive disciplines): `derive.go` holds the pure `Derive` function (licences, ratings, fleet, flight aggregate, stored intents and `now` → one `DisciplineState` per discipline) and `PendingAcknowledgement`; `service.go` holds `Get`/`Update`/`Settings`/`Replace` over `PilotProfileRepository` and `DisciplineEvidenceSource`. See [DOMAIN.md](./DOMAIN.md#pilot-profile-and-disciplines). |
| `internal/service/customreport` | Custom reports: validation and normalisation of a saved definition, per-account quota, window resolution (`lastMonths`/`yearToDate`/`range`/`all`), and result assembly — gap-filled chronological keys for time groupings, ranked-and-limited keys otherwise (`service.go`). |

### Data layer

| Package | Responsibility |
| --- | --- |
| `internal/repository` | Repository **interfaces** (`interfaces.go`) — e.g. `UserRepository`, `FlightRepository`, `LicenseRepository`, `ClassRating`, `Credential`, `Aircraft`, `Contact`, `FlightCrew`, `Notification`, `RefreshToken`, `PasswordResetToken`, `EmailVerificationToken`, `WebAuthnCredential`/`WebAuthnSession`, `BackupDestination`/`BackupRun`, `FlightBaseline`, `Idempotency`, `Deletion` (read-and-sweep over trigger-written tombstones), `CustomReport` (CRUD, reordering, and CTE-based aggregation over `flights`), `PilotProfile` (get/upsert of the stored intents) and `DisciplineEvidenceSource` (the single grouped flight query behind the pilot profile), plus the direct-access interfaces `Admin`, `Announcement`, `FlightImport`, `Reports` (analytics/trends/map aggregates) and `UserContent` (transactional account-content wipe). |
| `internal/repository/postgres` | PostgreSQL implementations of those interfaces (one file per entity). Parameterized SQL only; returns domain models. |

### Supporting

| Package | Responsibility |
| --- | --- |
| `internal/models` | Domain structs + validation helpers (no I/O): `user.go`, `license.go`, `class_rating.go`, `aircraft.go`, `aircraft_reminder.go`, `credential.go`, `contact.go`, `flight.go`, `flight_baseline.go`, `notification.go`, `pilot_profile.go` (disciplines, intents, statuses, evidence), `licence_kind.go` (`ClassifyLicence`, the single free-text licence-type classifier), `backup.go`, `webauthn.go`, `oidc.go`, `idempotency.go`, `deletion.go`, plus `validation.go` (text-length limits and the launch-method enum), `launch_method.go` (launch-method values, towed launches), `aircraft_class_infer.go` (class of an imported aircraft from its registration), `warning.go` (save-time warning codes) and `errors.go` (shared error types). |
| `internal/config` | Loads typed configuration from environment variables. |
| `internal/airports` | In-memory airport database merged from OurAirports (CSV) and mwgg/Airports (JSON). `Init()` at startup, `StartRefresher()` refetches on a timer. Lock-free reads over an atomically swapped snapshot: ICAO map for exact lookups, sorted code list for prefix search, 1°×1° grid for `Nearest`. Used for coordinates/distance and airport lookup/search. |
| `internal/updatecheck` | Release update check. Holds the running version and commit (link-time stamps, else `APP_VERSION`/`APP_COMMIT`), reads the newest published GitHub release per component on a timer, and compares by semantic version — or, for an untagged `latest` build, compares the build commit against the tracked branch head. Serves `GET /admin/update`; disabled by `UPDATE_CHECK_ENABLED=false`. |
| `internal/testutil` | Shared test fixtures, database setup/teardown, and an API client for tests. |

## `pkg/`

Reusable utilities with minimal dependencies, safe to use from any layer.

| Package | Responsibility |
| --- | --- |
| `pkg/jwt` | `Manager` — minting and validating JWT access/refresh tokens. |
| `pkg/hash` | bcrypt password hashing/verification and SHA-256 token hashing. |
| `pkg/cryptoutil` | AES-256-GCM (`AEAD`) for encrypting stored backup credentials; key helpers (`New`, `NewFromBase64`, `GenerateKey`, `GenerateKeyBase64`). |
| `pkg/duration` | Convert/format flight durations: minutes ↔ decimal hours, `HH:MM`, parsing. See [DOMAIN.md](./DOMAIN.md#time-and-duration-handling). |
| `pkg/registration` | `Normalize`/`Canonical` — rewrites an aircraft registration into the canonical notation of its state of registry, against a vendored ICAO nationality-mark table (`prefixes.go`); `Clean` only uppercases and trims (powered-paraglider names). See [AIRCRAFT_REGISTRATIONS.md](./AIRCRAFT_REGISTRATIONS.md). |
| `pkg/email` | SMTP sender (`smtp.go`) with localized templates (`templates_en.go`, `templates_de.go`) and email metrics (`metrics.go`). Recipients go through the SMTP envelope, not message headers (anti-injection). The send path runs the SMTP conversation command by command so a refusal can be attributed to the recipient or to our own setup; `delivery.go` defines those outcomes and the `DeliveryRecorder` interface that lets `internal/service` persist them without `pkg/email` depending on a database. |
| `pkg/solar` | Sunrise/sunset/twilight (`Calculate`, `CivilTwilight`, `IsNight`) wrapping `go-solar`; powers the day/night flight split. |

## Generated vs hand-written code

- **Generated** (do not edit): `internal/api/generated/*`. Regenerate with `make generate`
  after editing `api-spec/openapi.yaml`.
- **Everything else** is hand-written and reviewed normally.

> When you add a package or move a responsibility, update this reference.
