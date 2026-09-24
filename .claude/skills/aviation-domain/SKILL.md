---
name: aviation-domain
description: NinerLog's aviation invariants — durations as integer minutes, auto-calculated flight fields and their override flags, day/night and solar logic, the EASA/FAA currency evaluator registry, ownership checks and sentinel errors. Use before touching flights, times, statistics, currency, licenses, or any service-layer logic.
---

# Aviation domain rules

Full narrative: `docs/DOMAIN.md`. These are the invariants that are easy to break.

## Durations are integer minutes

Every flight duration is stored and manipulated as **integer minutes** — in the database, in
models, and in JSON. Migration `000031` converted decimal-hour columns to `INTEGER` to kill
floating-point rounding (`1h23m` is exactly `83`). Decimal hours are a *display* concern only;
convert with `pkg/duration` (`MinutesToDecimalHours`, `DecimalHoursToMinutes`, `FormatHM`,
`FormatColonHM`, `FormatDecimal`, `ParseDuration`). Never introduce a float duration field.

Times **of day** are different: `OffBlockTime`, `OnBlockTime`, `DepartureTime`, `ArrivalTime`
are `HH:MM:SS` strings in **UTC** (wall-clock instants, not durations). Per-user display
preferences (`TimeDisplayFormat`, `DateFormat`, `DecimalSeparator`) control rendering.

## Auto-calculation and overrides

`flightcalc.ApplyAutoCalculations(flight, userName)`
(`internal/service/flightcalc/flightcalc.go`) derives night time, day/night landings, total
landings, solo time, cross-country time, great-circle distance, and normalises crew/roles/names/
IFR/FSTD/remarks via helpers in `internal/service/flightrules/`.

Every auto-calculated takeoff/landing field has a companion `*Override` boolean (e.g.
`LandingsDayOverride`), as do `SICTime`, `MultiPilotTime`, `NightTime` and `CrossCountryTime`;
a flag is set when the pilot sends the value and cleared when `PUT` sends it as JSON `null`
(`handlers.applyOverride`). **Any code that recalculates — including
`POST /flights/recalculate` — must respect the override flags** and not clobber manual
entries. The flags are serialised in the backup payload and reported on every flight response.

Day/night classification uses `flightrules.IsNightAt(t, lat, lon)`, backed by sunrise/sunset
from `pkg/solar` and airport coordinates from the in-memory `internal/airports` database (loaded
at startup, refreshed on a timer). Departure/arrival may also be free-text off-airport locations, which have no
coordinates — handle the missing-coordinate case rather than assuming a lookup succeeds.

## Currency engine

`internal/service/currency` answers: given recent flying, may this pilot exercise a rating's
privileges and carry passengers?

- A **registry maps regulatory authority → `Evaluator`**: EASA, FAA, German UL (registered for
  several authorities via `RegisterMulti`), and an expiry-only `Other` fallback. Registration
  happens in `cmd/api/main.go`; adding a regulator is a localized change (new evaluator + one
  registration).
- Optional interfaces layer on extra questions: `PassengerCurrencyEvaluator`
  (EASA FCL.060(b), FAA §61.57) and `FlightReviewEvaluator` (FAA §61.56, 24 calendar months).
  `PeerAwareEvaluator` (EASA) receives the license's other class ratings.
- **Which aircraft classes count toward a rating is declared by the rule**, not assumed to be
  the rating's own class: EASA LAPL(A) pools every aeroplane class and TMG (FCL.140.A), and
  SEP(land)+TMG pool under FCL.740.A(b)(1) when both ratings are held. Passenger currency is
  never pooled. See `credited_classes.go` and DOMAIN.md "Credited classes".
- **Evaluators never write SQL.** They pull aggregates through the `FlightDataProvider`
  interface (`evaluator.go`, PostgreSQL impl in `postgres/currency_flight_data.go`):
  `GetProgressByAircraftClass`, `GetProgressAll`, `GetLastFlightReview`,
  `GetLastProficiencyCheck`, `GetLaunchCounts`. Need new data? Extend the interface, not the
  evaluator.
- Users can also define custom currency rules; see the custom-currency service and handlers.

## Validation, ownership, errors

Validation is layered and lives **below the handler**:

1. `internal/models/flight.go` — `IsValid()` (required fields) and `ValidateTimeDistribution()`
   (component times must be coherent with total time).
2. `internal/models/validation.go` — maximum lengths for every free-text field.
3. `internal/service/flight.go` — ownership checks and orchestration.

Rules:

- Every user-scoped operation verifies the resource belongs to the authenticated user
  (`resource.UserID != requestingUserID` → 403). Do this in the service, not the handler.
- Services return **sentinel errors** (`ErrFlightNotFound`, `ErrUnauthorizedFlight`,
  `ErrInvalidFlight`, `ErrInvalidTimeDistribution`, …); handlers map them to status codes.
- Wrap internal errors with context (`fmt.Errorf("...: %w", err)`) but never return raw error
  text to clients.
- Repositories use parameterized SQL exclusively. The one user-influenced string used as a JSONB
  key (custom fields' `field_key`) is validated against `^[a-z][a-z0-9_]{0,63}$` at the API
  boundary — keep it that way.
- Secrets come from environment variables only; bcrypt for passwords, AES-256-GCM for stored
  backup credentials; never log JWTs.
