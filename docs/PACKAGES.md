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
| `internal/api/handlers` | Gin handlers. `APIHandler` aggregates all services and implements the generated `ServerInterface`; one file per domain (`auth.go`, `flight.go`, `license.go`, `aircraft.go`, `credential.go`, `currency.go`, `contact.go`, `notification.go`, `backup.go`, `admin*.go`, `import*.go`, `export*.go`, `maps.go`, `reports.go`, `baseline.go`, `bulk_delete.go`, `twofactor.go`, `webauthn.go`, `announcements.go`, `flight_util.go`). |
| `internal/api/middleware` | Cross-cutting HTTP concerns: `auth.go` (JWT), `admin.go` (admin gate), `ratelimit.go`, `metrics.go` + `app_metrics.go`, `recovery.go`, `logger.go`, `security_headers.go`. |
| `internal/api/generated` | **Auto-generated** from `api-spec/openapi.yaml` via `oapi-codegen` (`types.go`, `server.go`, `spec.go`, `generate.go`). Do not hand-edit. See [API.md](./API.md). |

### Business logic

| Package | Responsibility |
| --- | --- |
| `internal/service` | Domain services: `auth.go`, `flight.go`, `license.go`, `class_rating.go`, `aircraft.go`, `credential.go`, `contact.go`, `notification.go` (+ `notification_metrics.go`), `twofactor.go`, `webauthn.go`. Each takes repository interfaces + `pkg` utilities. |
| `internal/service/currency` | The currency engine: `Evaluator`/`Registry`/`FlightDataProvider` (`evaluator.go`), `Service` (`service.go`), authority evaluators (`easa.go`, `faa.go`, `german_ul.go`, `other.go`), shared logic (`engine.go`, `types.go`), and PostgreSQL aggregation (`flight_data.go`). See [DOMAIN.md](./DOMAIN.md#currency-engine). |
| `internal/service/flightcalc` | `ApplyAutoCalculations(flight, userName)` — the single entry point that derives flight fields. |
| `internal/service/flightrules` | Composable flight rules used by `flightcalc`: `night.go` (day/night via solar), `crew.go`, `roles.go`, `names.go`, `ifr.go`, `fstd.go`, `remarks.go`, `display.go`. |
| `internal/service/cloudbackup` | Cloud backup orchestration: `service.go`, `destinations.go`, `runner.go`, `scheduler.go`, `jsonbuilder.go`. |
| `internal/service/cloudbackup/provider` | Pluggable storage `Provider` interface + registry, with `s3/`, `sftp/`, and `webdav/` implementations. |

### Data layer

| Package | Responsibility |
| --- | --- |
| `internal/repository` | Repository **interfaces** (`interfaces.go`) — e.g. `UserRepository`, `FlightRepository`, `LicenseRepository`, `ClassRating`, `Credential`, `Aircraft`, `Contact`, `FlightCrew`, `Notification`, `RefreshToken`, `PasswordResetToken`, `EmailVerificationToken`, `WebAuthnCredential`/`WebAuthnSession`, `BackupDestination`/`BackupRun`, `FlightBaseline`. |
| `internal/repository/postgres` | PostgreSQL implementations of those interfaces (one file per entity). Parameterized SQL only; returns domain models. |

### Supporting

| Package | Responsibility |
| --- | --- |
| `internal/models` | Domain structs + validation helpers (no I/O): `user.go`, `license.go`, `class_rating.go`, `aircraft.go`, `credential.go`, `contact.go`, `flight.go`, `flight_baseline.go`, `notification.go`, `backup.go`, `webauthn.go`, plus `validation.go` (text-length limits) and `errors.go` (shared error types). |
| `internal/config` | Loads typed configuration from environment variables. |
| `internal/airports` | In-memory airport database (OurAirports data), `Init()` at startup; used for coordinates/distance and airport lookup/search. |
| `internal/testutil` | Shared test fixtures, database setup/teardown, and an API client for tests. |

## `pkg/`

Reusable utilities with minimal dependencies, safe to use from any layer.

| Package | Responsibility |
| --- | --- |
| `pkg/jwt` | `Manager` — minting and validating JWT access/refresh tokens. |
| `pkg/hash` | bcrypt password hashing/verification and SHA-256 token hashing. |
| `pkg/cryptoutil` | AES-256-GCM (`AEAD`) for encrypting stored backup credentials; key helpers (`New`, `NewFromBase64`, `GenerateKey`, `GenerateKeyBase64`). |
| `pkg/duration` | Convert/format flight durations: minutes ↔ decimal hours, `HH:MM`, parsing. See [DOMAIN.md](./DOMAIN.md#time-and-duration-handling). |
| `pkg/email` | SMTP sender (`smtp.go`) with localized templates (`templates_en.go`, `templates_de.go`) and email metrics (`metrics.go`). Recipients go through the SMTP envelope, not message headers (anti-injection). |
| `pkg/solar` | Sunrise/sunset/twilight (`Calculate`, `CivilTwilight`, `IsNight`) wrapping `go-solar`; powers the day/night flight split. |

## Generated vs hand-written code

- **Generated** (do not edit): `internal/api/generated/*`. Regenerate with `make generate`
  after editing `api-spec/openapi.yaml`.
- **Everything else** is hand-written and reviewed normally.

> When you add a package or move a responsibility, update this reference.
