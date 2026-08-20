# SQL layering: where SQL is allowed to live

NinerLog follows a strict **handler → service → repository → models** layering
(see [ARCHITECTURE.md](./ARCHITECTURE.md)). The rule this document pins down:

> **All SQL that the API executes lives in `internal/repository/postgres`.**
> Handlers and services never contain SQL text and never hold a `*sql.DB`.
> They call repository interfaces defined in `internal/repository`.

This became fully true in the SQL-layering migration (2026-08); before it, the
admin console, reports/analytics, imports, announcements, bulk deletes and the
currency engine's data providers queried the database directly from
`internal/api/handlers` and `internal/service/currency`.

## What lives where today

| Surface | Interface (`internal/repository`) | Implementation (`internal/repository/postgres`) |
| --- | --- | --- |
| Admin stats, audit log, user listing, maintenance sweeps | `AdminRepository` | `admin.go` |
| System announcements | `AnnouncementRepository` | `announcement.go` |
| Import history (`flight_imports`) | `FlightImportRepository` | `flight_import.go` |
| Reports, analytics, map aggregates | `ReportsRepository` | `reports.go` |
| Account-content wipe (`DELETE /users/me/data`) | `UserContentRepository` | `user_content.go` |
| Regulatory currency aggregates | `currency.FlightDataProvider` (in `internal/service/currency`) | `currency_flight_data.go` |
| Custom currency rule aggregates | `currency.CustomFlightDataProvider` (in `internal/service/currency`) | `custom_currency_data.go` |

The two currency provider interfaces live in the `currency` package rather than
`internal/repository` because the evaluators define the contract (what to
count, in which shape); the repository layer only implements it. This mirrors
the pre-existing `FlightDataProvider` design.

`APIHandler` reaches these repositories directly (no intermediate service) —
the same pattern as the pre-existing `FlightCrewRepository` field. That is a
deliberate exception to "handlers call services": these surfaces are
read-mostly projections or one-shot maintenance actions with no business rules
of their own, and a pass-through service per repository would add indirection
without adding a seam. Anything with ownership checks, validation, or reuse
from background jobs still goes through `internal/service`.

## Deliberate exceptions — SQL (or SQL-adjacent code) outside the repository layer

The following places contain SQL text or `database/sql` usage outside
`internal/repository/postgres`, each for a structural reason. If you touch one
of them, keep the constraint that justifies it.

### 1. `internal/flightsearch` — the advanced-search compiler

`flightsearch` parses the `q=` search grammar into an AST and **renders SQL
condition fragments** (`sql.go`) with `$n` bind placeholders. It never
executes anything: `Query.Compile` returns `(condition, args)`, and the only
caller is `internal/repository/postgres/flight.go`, which splices the
condition into its own parameterized queries.

Why it stays put:

- It is a *compiler*, not data access. The SQL text is the compiler's output
  format, inseparable from the parser and field table that live beside it
  (`parse.go`, `fields.go`) and from their shared tests.
- Moving the rendering into `postgres/` would either split the AST from its
  renderer across packages (exporting every node type) or drag the whole
  parser into the repository layer, which handlers also need for
  parse-error reporting (`400 Invalid search query`).
- The layering goal — no SQL *executed* outside repositories — is preserved:
  the fragment is powerless until the flight repository binds and runs it.

Guard rails: all identifiers come from the fixed field table; every value is
bound via the builder's `bind()`; `escapeLike` escapes LIKE metacharacters.

### 2. `cmd/api/main.go` — composition root

`main.go` opens the `*sql.DB`, configures the pool, runs `golang-migrate`
(`m.Up()`), and pings the database for `/health`. It contains no query text;
it is the one place allowed to hold the raw handle, because it is where every
repository is constructed. The `withStatementTimeout` DSN helper in
`cmd/api/dsn.go` manipulates the connection string, not SQL.

### 3. `db/migrations/*.sql` — schema

Schema DDL (and the tombstone triggers from migration `000054`) is SQL by
definition and lives in migration files, applied at startup. Notably the
deletion tombstones are *deliberately* written by database triggers rather
than repository code — see [DATA_MODEL.md](./DATA_MODEL.md#deletion-tombstones).

### 4. Test scaffolding and test fixtures

- `internal/testutil/database.go` connects to the Docker test database and
  `TRUNCATE`s tables between tests. It supports repository integration tests;
  it is not production code.
- `internal/repository/postgres/*_test.go` assert against the SQL their
  repositories emit (sqlmock) or run it against the test database — SQL in
  repository tests is the repository layer.
- `test/e2e/pentest_owasp_e2e_test.go` contains SQL-injection *payload
  strings* (e.g. `' OR '1'='1`) sent as attack input. They are fixtures, not
  queries.

### 5. `internal/api/middleware/app_metrics.go`

Holds a `*sql.DB` only to export `sql.DBStats` pool gauges to Prometheus. It
never issues a query.

## Dynamic SQL and the custom-currency vocabulary

User-authored currency rules are the one place where query shape is user data.
The safety contract (unchanged by the move, now enforced in
`postgres/custom_currency_data.go`):

1. Every identifier that reaches SQL comes from a fixed lookup table keyed by
   the rule's controlled vocabulary (`customMetricSQL`, `customMetricRowSQL`,
   `customFilterColumn`, `customBoolPredicate`). No user string is ever
   interpolated as a column, table, or operator.
2. Every user-supplied value is bound as a query parameter.
3. Rule bodies are validated (`models.CustomCurrencyRuleBody.Validate`)
   before persistence or evaluation; an unknown identifier at query time is an
   internal error.

If you add a metric or filter to the vocabulary, update the maps in
`custom_currency_data.go` *and* the model validation together;
`TestCustomMetricMapsStayConsistent` guards the aggregate/per-flight pairing.

## Rules of thumb when adding code

- New query → new (or extended) repository interface + `postgres/`
  implementation. Never `db.Query…` from a handler or service.
- Need a fragment of SQL built from user input? Follow the custom-currency
  pattern: fixed identifier maps, bound values, validation upstream.
- A handler may hold a repository interface directly only for logic-free
  read/maintenance surfaces; anything with domain rules gets a service.
