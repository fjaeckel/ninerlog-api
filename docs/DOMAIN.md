# Aviation Domain

This document explains the aviation-specific logic that makes NinerLog more than a CRUD
app: how flight time is represented, how fields are auto-calculated, how flights are
validated, and how the **currency engine** determines whether a pilot is legally current
under EASA, FAA, and other rule sets.

## Time and duration handling

All flight **durations are stored and manipulated as integer minutes** (migration
`000031` converted every time column from decimal hours to `INTEGER`). This eliminates
floating-point rounding errors (e.g. `1h23m` is exactly `83`, never `1.3833…`).

Conversion and formatting live in `pkg/duration`:

| Function | Purpose |
| --- | --- |
| `MinutesToDecimalHours(min) float64` | minutes → decimal hours (e.g. 83 → 1.38) |
| `DecimalHoursToMinutes(h) int` | decimal hours → minutes |
| `FormatHM(min) string` | `"1h 23m"` |
| `FormatColonHM(min) string` | `"1:23"` |
| `FormatDecimal(min) string` | decimal-hours string |
| `ParseDuration(input) (int, error)` | parse user input (`HH:MM`, decimal, etc.) → minutes |

Block/event times of day (`OffBlockTime`, `OnBlockTime`, `DepartureTime`, `ArrivalTime`)
are stored as `HH:MM:SS` strings in **UTC**, because they are wall-clock instants, not
durations. Per-user display preferences (`TimeDisplayFormat`, `DateFormat`,
`ClockFormat`, `DecimalSeparator`) control how values are rendered for that pilot;
`ClockFormat` affects display only — stored and exported times stay 24-hour.

## Total time and pilot function time

`TotalTime` is **block time** — EASA AMC1 FCL.050 Col 9, "total time of flight". It is
computed by the server from `OffBlockTime` and `OnBlockTime`; clients do not send it.

The *pilot function time* columns (Cols 15–18) **decompose** that total rather than adding
to it:

| Column | Field | Counts toward total time |
| --- | --- | --- |
| PIC | `PICTime` | yes |
| PIC under supervision (PICUS) | `PICUSTime` | yes |
| Student PIC (SPIC) | `SPICTime` | yes |
| Co-pilot (SIC) | `SICTime` | yes |
| Dual received | `DualTime` | yes |
| Cruise relief co-pilot | `ReliefTime` | yes |
| Instructor (dual given) | `DualGivenTime` | no — it *overlays* the others |
| Examiner | `ExaminerTime` | no — it *overlays* the others |

So the invariant is
`PICTime + PICUSTime + SPICTime + SICTime + DualTime + ReliefTime <= TotalTime`, enforced
by `ValidateTimeDistribution()`. Two consequences are easy to get wrong:

- **Co-pilot time counts in full.** An airline first officer's 10-hour sector is 10 hours
  of total time, 10 hours of co-pilot time and zero PIC time, and all 10 count toward the
  1500 hours for an ATPL. Leaving `SICTime` out of the total would understate every
  multi-crew pilot's logbook.
- **Instructor time is not an extra slice.** An FI normally logs the same hour as both PIC
  and instructor time, so adding `DualGivenTime` to the sum would double-count it. It is
  bounded by `TotalTime` alone, not by `PICTime`, because an FI instructing a qualified
  pilot who acts as PIC logs instructor time with no PIC time of their own.

`SoloTime` and `CrossCountryTime` are likewise subsets of the total, not additional slices.

When a row declares `SICTime` but carries no crew list — typical of imported logbooks —
`flightrules.DetermineRole` resolves the user to co-pilot so `PICTime` is not also claimed
for the same minutes. Whether the time may be logged at all is a separate question, covered
next.

### Declared function times: PICUS, SPIC, examiner, relief

`PICUSTime` (PIC under supervision, EASA FCL.030 — the time a first officer logs toward
unfreezing an ATPL), `SPICTime` (student pilot-in-command on an integrated course),
`ExaminerTime` (conducting a check) and `ReliefTime` (cruise relief co-pilot on an
augmented crew) are **declared by the pilot and never auto-derived** — the server cannot
know that a sector was flown as pilot flying under supervision, or that the PIC
countersigned it. They are kept distinct from `PICTime` and `SICTime` in storage,
statistics and analytics, because a pilot needs "actual PIC" and "PICUS" as separate
totals.

Because PICUS, SPIC and relief are function times, derivation carves them out of the
derived column for the resolved role (`flightcalc.derivedFunctionMinutes`): a first
officer who declares a full sector as PICUS logs zero co-pilot time for it, and a partial
relief declaration leaves the remainder as co-pilot time. Examiner time overlays function
time exactly like `DualGivenTime` and is bounded by `TotalTime` alone. Declaring any of
the three carved times also declares the crew seat (`flightrules.HasDeclaredFunctionTime`
feeds `MayLogCoPilotTime` and `DetermineRole`), and any declared function time suppresses
solo time — each implies another pilot on board.

On the paper-layout exports (EASA/FAA CSV and PDF) the declared times fold into the
conventional columns — PICUS and SPIC into the PIC column per AMC1 FCL.050 (for FAA
layouts, into SIC and dual respectively per 14 CFR §61.51(f)), relief into the co-pilot
column — with the breakdown annotated in remarks by `flightrules.CombinedRemarks`
(`[PICUS 2:05]`). The standard CSV, JSON export and API keep the four as separate fields.

## Who may log co-pilot time

Co-pilot time is not a consequence of another pilot being on board. It requires a co-pilot
seat that the operation actually calls for, and most general aviation flying has none:

- a **multi-pilot aircraft** — certificated for a minimum crew of two pilots (EASA
  FCL.010; 14 CFR §61.51(f)(1), "aircraft type certificated for more than one pilot").
  This is `aircraft.is_multi_pilot`, a fleet fact alongside `is_complex`,
  `is_high_performance` and `is_tailwheel`. `aircraft_class` cannot express it: that
  column is free-form and describes engines and land/sea, not required crew;
- a **required safety pilot** during simulated instrument flight (14 CFR §91.109(b),
  loggable under §61.51(f)(2)) — the user carries the `SafetyPilot` crew role;
- a **declaration by the pilot** — the user lists their own `SIC` crew entry, or enters
  `sicTime` directly. Two-pilot operations mandated by an operations manual rather than by
  the type certificate (EASA FCL.010; §61.51(f)(2), §135.99(c)) are recorded this way.

`flightrules.MayLogCoPilotTime` is the single predicate. **An aircraft absent from the
fleet is treated as single-pilot**: derivation never invents co-pilot time for an aircraft
it knows nothing about.

### Passenger flights

When another person is pilot-in-command and none of the above applies, the user was
carried rather than crewed. `flightrules.DetermineRole` returns `RolePassenger` and the row
is stored with `is_passenger = true`: it keeps its route, block times and distance as the
record of the trip, and carries zero in `total_time` and every pilot-function, landing and
instrument column. Like an FSTD session it contributes to no total, statistic or currency
calculation — `flightrules.CountsAsFlightTime` and the SQL predicate
`NOT is_simulator AND NOT is_passenger` are the two sides of that rule.

### Declared versus derived co-pilot time

`SICTimeOverride` marks a co-pilot time the pilot entered, following the `*Override`
convention used for takeoffs and landings. Derivation trusts a declared value on any
aircraft and leaves it as entered; a value derivation wrote itself carries no override, so
re-running `POST /flights/recalculate` can still correct a row an earlier derivation filled
in. Without that distinction the derived value would justify itself on every subsequent
save.

### Multi-pilot time

The EASA AMC1 FCL.050 multi-pilot column (Col 10) records time flown in aeroplanes
certificated for a minimum crew of two, so `flightrules.IsMultiPilotOperation` requires
`is_multi_pilot` in addition to a two-pilot crew. A safety pilot in a single-pilot
aeroplane is a required crew member and logs co-pilot time, but the aeroplane stays
single-pilot and the time is not multi-pilot time. `PilotingCategoryFor` continues to bucket
a row as MP on `MultiPilotTime > 0`, which is now only ever filled for a multi-pilot
aircraft.

### Existing data

Migration `000065` does not infer `is_multi_pilot` from existing `multi_pilot_time`: that
would flag exactly the single-pilot aircraft this rule exists to correct, cementing the
error. The fleet starts unmarked and the pilot marks it.

What the migration does backfill is the two override flags, and only where derivation
cannot have produced the value. Before it, a co-pilot or multi-pilot time on a row with no
crew list was kept as entered — so it came from the pilot — while one on a row with a crew
list was written by derivation. Marking only the former as declared protects imported and
hand-entered logbooks from being re-derived on the next save, and leaves derived values
free to be corrected. Everything else stays as it is until the pilot marks their fleet and
runs `POST /flights/recalculate`.

## FSTD (simulator) sessions

A session in a flight simulation training device — FNPT, FTD, FFS, BATD/AATD — is training,
not flying. AMC1 FCL.050 records it in its own columns (20–22: date, device type, session
duration) and is explicit that **session time is recorded separately and may not be summed
with flight time**.

NinerLog stores a session as a `flights` row with `IsSimulator = true`. That row:

- carries its duration in `SimulatedFlightTime` and `FSTDType` for the device designation;
- has **zero** in every flight-time column — `TotalTime`, `PICTime`, `DualTime`, `SICTime`,
  `DualGivenTime`, `MultiPilotTime`, `PICUSTime`, `SPICTime`, `ExaminerTime`, `ReliefTime`,
  `SoloTime`, `CrossCountryTime`, `NightTime`, `IFRTime`
  — and zero landings, takeoffs and distance;
- has no `AircraftReg`, no departure/arrival and no block times. A device is not flown
  between places and has nothing to record off- and on-block, which is why those fields are
  required for a flight and rejected for a session. `AircraftType` is still required: it is
  the aircraft the device represents;
- **keeps** its instrument work — `SimulatedInstrumentTime` (capped at the session
  duration), `Holds`, `Approaches`, `IsIPC`, `IsProficiencyCheck` — because that is the
  training-relevant part. `ActualInstrumentTime` is cleared; actual instrument time
  requires real IMC.

`flightcalc.ApplyAutoCalculations` branches to `applySessionCalculations` for these rows, so
none of the flight-shaped derivations (night time, landing split, cross-country, distance,
PIC/dual from crew) run against them.

**Sessions are excluded from every aggregate.** `flightrules.CountsAsFlightTime` is the
Go-side predicate; in SQL the equivalent `NOT is_simulator` is carried by the statistics,
reports, analytics, per-aircraft and currency queries. This keeps a session out of flight
totals, the fleet list and — deliberately — the currency engine. FAA §61.57(c) does permit
instrument recency in an FSTD; crediting it is a separate change, and until then the
conservative answer is that a session never establishes currency.

Custom reports (`/reports/custom`) select flights with the same filter as `GET /flights`,
then apply this rule in the aggregate: every duration metric and `flights`/`landings` count
only rows where
`NOT is_simulator AND NOT is_passenger`. The one exception is `fstdTime`, which sums
`simulated_flight_time` across the matching simulator rows on its own — so a report can
show flight hours and FSTD hours side by side without summing them, matching AMC1 FCL.050.

In exports, sessions populate the FSTD block of the EASA layouts
(`flightrules.IsFSTDRow`, `FSTDFields`) and contribute 0 to the TOTAL TIME column, which is
what the paper form requires.

> Migration `000064` introduced `is_simulator` and backfilled it. Rows carrying an
> `fstd_type` were device sessions logged as flights with a placeholder registration and
> invented block times; the migration recovers the session duration into
> `simulated_flight_time` and clears their flight-time columns. Those values are not
> restored by the down migration.

## Flight auto-calculations

When a flight is created or updated, the service derives several fields so pilots don't
have to compute them by hand. The entry point is
`flightcalc.ApplyAutoCalculations(flight, userName, aircraft)`
(`internal/service/flightcalc/flightcalc.go`), which composes helpers from
`internal/service/flightrules`:

- **Day/night split** — `flightrules.IsNightAt(t, lat, lon)` (`solar.IsNight`) uses
  civil twilight (sun 6° below the horizon) at the relevant airport to classify
  takeoffs/landings as day or night, and `NightTime` counts the block-time minutes that
  are night at the departure airport. The astronomical computation lives in `pkg/solar`;
  see [Day and night](#day-and-night).
- **Total landings** — `AllLandings = LandingsDay + LandingsNight`.
- **Takeoffs** — one takeoff per landing: `TakeoffsDay + TakeoffsNight = AllLandings`,
  all classified day or night by the takeoff time (else off-block time) at the departure
  airport (day when departure or both times are missing or unknown); see
  [Day and night](#day-and-night). Re-derived from the landing count on
  every save, so a stored value is never reused.
- **Solo time** — derived when the flight is neither dual nor flown as PIC with other
  crew.
- **Cross-country time** — derived as the whole block time when departure ≠ arrival
  airport; see [Manual overrides](#manual-overrides) for why a pilot may replace it.
- **Distance** — great-circle distance (nautical miles) from airport coordinates in the
  in-memory airport database (`internal/airports`).
- **Pilot role** — `flightrules.DetermineRole(flight, userName, aircraft)` resolves the
  user to PIC, dual received, dual given, co-pilot or passenger. `aircraft` is the fleet
  entry for the registration flown (`flightrules.AircraftFacts`), resolved by the caller
  via `service.AircraftFactsFor`; `nil` means the registration has no fleet entry. See
  [Who may log co-pilot time](#who-may-log-co-pilot-time). Dual given is resolved from a
  self-listed `Instructor` or a third-party `Student`; a pilot listing themselves as
  `Student` (e.g. a supervised solo with `SPICTime`/`PICUSTime`) stays PIC and logs no
  dual given. With an empty `userName`, every `Student` entry counts as third party.
  Rows saved before this rule are corrected by `POST /flights/recalculate`.
- **Crew / roles / names / IFR / FSTD / remarks / display** — additional helpers in
  `flightrules/` (`crew.go`, `roles.go`, `names.go`, `ifr.go`, `fstd.go`, `remarks.go`,
  `display.go`) normalise crew roles, instructor/PIC names, instrument fields, simulator
  type, and display formatting.

### Manual overrides

Every auto-calculated takeoff/landing field has an `*Override` boolean (e.g.
`LandingsDayOverride`), as do `SICTime`, `MultiPilotTime`, `NightTime` and
`CrossCountryTime`. When a pilot edits the value manually, the override flag is set so
recalculation does not clobber the manual entry. The `POST /flights/recalculate` endpoint
re-runs auto-calculations across a pilot's flights while respecting overrides.

The handlers set a flag when the request carries the corresponding field with a number,
and clear it when `PUT /flights/{id}` carries the field as JSON `null` — the next
calculation then rewrites the value. An omitted field leaves both value and flag alone.
Each flight response reports every flag (`nightTimeOverride`, `crossCountryTimeOverride`,
`takeoffsDayOverride`, `takeoffsNightOverride`, `landingsDayOverride`,
`landingsNightOverride`, `sicTimeOverride`, `multiPilotTimeOverride`) so a client can show
which values are the pilot's and offer a way back to the derived one. The flags travel in
the JSON export and cloud backup and are restored by `POST /users/me/data`.

Night time and cross-country time carry overrides because no derivation matches every
rulebook: night is derived from civil twilight at the departure location over block time,
and cross-country as the whole block time when departure ≠ arrival. FAA 14 CFR 61.1(b)
counts cross-country only with a landing more than 50 NM from departure (no landing needed
for ATP experience); EASA FCL.010 counts any pre-planned route flown with navigation,
including one returning to the departure aerodrome. A value the pilot enters is bounded by
`TotalTime` (`ErrInvalidNightTime`, `ErrInvalidCrossCountryTime`). A night or cross-country
column in a CSV import is taken as the pilot's record and stored as an override, capped at
block time.

A CSV import treats the file's other splits the same way. Separate day and night landing
columns, and day and night takeoff columns, are stored with their override flags, so the
civil-twilight split runs only for a file with a single landings total; landings counted
only in a larger total go to the day landings. A positive SIC or multi-pilot column is stored
as an override capped at block time, and a self-listed SIC crew entry declares the whole
block as SIC time. These declarations are what `MayLogCoPilotTime` reads (below), so a
co-pilot sector does not resolve to a passenger flight. An aircraft created by the import is
marked multi-pilot only when its rows carry multi-pilot time.

### Day and night

An instant is **day** when it lies inside the civil-twilight interval `[dawn, dusk]` of
any local solar day around it, and **night** otherwise (ICAO/EASA: night runs from the
end of evening civil twilight to the beginning of morning civil twilight; boundaries
count as day). `go-solar` computes one `[dawn, dusk]` pair per *local solar day*, the
24 hours centred on mean solar noon at the airport's longitude, so a pair requested for
a UTC date can start or end on the neighbouring UTC date: at KFDK (77° W) the pair for
2026-06-19 ends at 01:11:48Z on 06-20, at YSSY (151° E) the pair for 2026-10-20 starts at
18:42Z on 10-19. `solar.DaylightBetween(from, to, lat, lon)` therefore collects the pairs
for every UTC date from the day before `from` to the day after `to`; `solar.IsNight`
and the `NightTime` minute count both test against that set, so the takeoff/landing
split and night minutes always agree.

- **Polar night** (sun below −6° all solar day): the day contributes no interval.
- **Midnight sun** (sun above −6° all solar day): the day contributes the whole solar
  day, mean solar noon ± 12 h. Consecutive solar days tile without a gap, and a
  midnight-sun day next to a normal day leaves night only where the normal day's
  twilight leaves it.

The instants classified are built by one helper (`flightcalc.instantsOf`): off-block,
takeoff (`DepartureTime`), landing (`ArrivalTime`) and on-block are placed on the flight's
`Date` in UTC, and any of them earlier than off-block (or than takeoff, when off-block is
absent) falls on the following day (a flight across 00:00Z). `NightTime`, the takeoff split
and the landing split all use these instants, so an overnight landing is classified
against the next day's twilight.

- The **takeoff split** classifies the takeoff time at the departure airport, and the
  **landing split** the landing time at the arrival airport. Each falls back to the block
  time (off-block, on-block) when it is absent, unparseable or outside the
  `[off-block, on-block]` window.
- `NightTime` uses block times only.

`NightTime` samples each block-time minute from off-block (`[off, on)`) and is capped at
`TotalTime`. Stored flights keep the values derived when they were last saved;
`POST /flights/recalculate` re-derives non-overridden ones.

## Flight validation

Validation is layered:

1. **Model-level** (`internal/models/flight.go`):
   - `IsValid()` — required fields present. These differ by row kind: a flight needs a
     registration and block time, a session needs `FSTDType` and a positive
     `SimulatedFlightTime` (see [FSTD sessions](#fstd-simulator-sessions)), and a
     passenger flight needs only a registration.
   - `ValidateTimeDistribution()` — function-time consistency: component times must not
     exceed total time,
     `PICTime + PICUSTime + SPICTime + SICTime + DualTime + ReliefTime <= TotalTime`
     (instructor and examiner time overlay instead), PIC/dual logic must be coherent, and
     a session or passenger flight must carry no flight time at all.
2. **Text-field limits** (`internal/models/validation.go`) — enforces maximum lengths on
   free-text fields (registration, type, remarks, notes, …) to prevent abuse and oversized
   payloads.
3. **Service-level** (`internal/service/flight.go`) — ownership checks (the flight's
   aircraft/user belong to the caller) and orchestration of the above.

Validation failures surface as sentinel errors (e.g. `ErrInvalidFlight`,
`ErrInvalidTimeDistribution`) that handlers map to HTTP 400.

## Aircraft registration normalisation

`AircraftService.CreateAircraft`/`UpdateAircraft` and `FlightService.CreateFlight`/
`UpdateFlight` canonicalise `Registration`/`AircraftReg` through `pkg/registration` before
validating: a nationality mark recognised against the vendored ICAO table gets its hyphen
inserted, moved or removed to match how that state writes it (`DEABC` → `D-EABC`,
`N-12345` → `N12345`); anything unrecognised is only uppercased and trimmed. This matters
because `flights.aircraft_reg` is a denormalised string and the join key the fleet and
per-registration statistics group by — two spellings of one aircraft otherwise split into
two fleet entries and two sets of statistics. Full design, the table-maintenance workflow,
and the `POST /flights/recalculate` migration path are in
[AIRCRAFT_REGISTRATIONS.md](./AIRCRAFT_REGISTRATIONS.md).

## Strict logbook mode and the flight audit trail

Some authorities expect a logbook that shows what an entry said before it was corrected.
Strict logbook mode is the opt-in setting that makes NinerLog keep that history. It is a
per-user flag (`users.strict_logbook`); standard mode is unchanged and records nothing.

What strict mode adds, and only in strict mode:

- every committed change to a flight or its crew is kept as a **revision** (the state
  before the change), and a deleted flight is kept as a **void record**;
- a late edit is classified as a **correction** and needs a reason;
- the pilot can **certify** the logbook through a date;
- under a Swiss licence, manual overrides may only **reduce** a calculated value.

Switching strict mode off stops recording; history already captured stays with the logbook
and is never deleted, except together with the account. History starts when strict mode is
switched on (`strict_logbook_since`); there are no baseline revisions for older entries.

### Licence rule registry (`internal/service/logbookrules`)

The registry maps a licence to the authority's rules. It is keyed on the licence's
**issuing country** (`licenses.issuing_country`, never inferred), the licence type and, for
the EASA fallback, the regulatory authority. A licence without an issuing country has no
rule, ever. Saving a licence (create or patch) runs the registry; if any licence now
**requires** strict mode and the user is not strict, strict mode is switched on with a
`licence` event naming the saved licence. It is never switched off automatically, and the
user may switch it off by hand even while a licence requires it.

| Country | Licence types | Requirement | Draft window | Basis |
| --- | --- | --- | --- | --- |
| CH | any | required | `hours_48` | FOCA Accepted Logbook Formats §2.3(7) |
| DE | not UL | required | `flight_day` | NfL 2212-21 §4.3.2 |
| DE | UL (authority DULV or DAeC, a UL type, or a German UL term) | suggested | | §120 LuftPersV |
| AT | national: UL, hang glider, paraglider, national glider | required | `flight_day` | ZLPV 2006 Anlage 6 |
| AT | other (Part-FCL, including the gyroplane licence GPL) | suggested | | AMC1 FCL.050 (e)(2) |
| AU | any | required | `flight_day` | CASR 61.355, RAAP 13 |
| NZ | any | required | `flight_day` | Part 61.29(e) |
| JP | any | required | `flight_day` | 国空乗390号 |
| UY | any | required | `flight_day` | RAU 61.51 |
| DK, BR, GB, US | any | suggested | | BL 6-04 §3.2.6 / CIV Digital / printed extracts / AC 120-78B |
| other country, authority EASA | any | suggested | | AMC1 FCL.050 (e)(2) |
| anything else | | none | | |

Licence types are matched as whole words: the type is split on non-letters and upper-cased
(`ä` and `Ä` included), and a term matches only as one token or, for a multi-word term, a run
of tokens. `PPG` is therefore not `PG`, `UPG` is not `UL`, and `GPL` is not a national
licence.

- **Ultralight (`LooksLikeUL`)** mirrors the frontend's `looksLikeULLicence`: the upper-cased,
  trimmed type is `UL`, starts with `UL ` or `UL-`, or contains `ULTRALIGHT` / `ULTRALEICHT`.
- **DE UL** (suggested, §120 LuftPersV): the licence's `regulatoryAuthority` is `DULV` or `DAeC`
  (case-insensitive, trimmed), or `LooksLikeUL` holds, or the type contains one of
  `GLEITSCHIRM`, `MOTORSCHIRM`, `TRIKE`, `HÄNGEGLEITER`/`HAENGEGLEITER`, `DRACHEN`,
  `TRAGSCHRAUBER` or `SPORTPILOTENLIZENZ` as a whole word. Any other German licence is required.
- **AT national** (required, ZLPV 2006 Anlage 6): `LooksLikeUL`, or one of `UL`, `HG`, `PG`,
  `HÄNGEGLEITER`, `HAENGEGLEITER`, `HANGGLIDER`, `HANG GLIDER`, `PARAGLIDER`, `PARAGLEITER`,
  `GLEITSCHIRM`, `DRACHEN`, `MOTORSCHIRM`, `NATIONAL GLIDER` as whole words. `GPL` is the
  Part-FCL gyroplane licence and only suggests strict mode.

A suggested licence is surfaced in `GET /logbook/mode` (`suggestedBy`) but never switches
anything on. With several requiring licences the **earliest** deadline applies per flight,
and the reported `draftWindow` is the shortest (`flight_day` before `hours_48`). Strict mode
switched on by hand, with no requiring licence, uses `flight_day`.

**Strict mode and the shortest window are account-wide.** They do not depend on which licence
a flight belongs to: one requiring licence puts every flight of the account under strict
mode, and the shortest window across all requiring licences applies to every flight (a pilot
with a Swiss and a German licence gets the Swiss downward-only overrides and the German
flight-day window on all flights). Saving any requiring licence (create or patch) switches
strict mode back on, with a `licence` event, after the pilot switched it off by hand.

#### Regulation text behind each row

The wording below is quoted from the primary texts as read for the ADR
([#260](https://github.com/fjaeckel/ninerlog-api/issues/260)); it is the author's reading,
not legal advice. Where the source holds no quotable correction rule the row says so.

- **CH**, [FOCA GM/INFO Accepted Logbook Formats, valid 01.02.2024](https://www.bazl.admin.ch/dam/en/sd-web/qcDpXt-Ox7VO/foca_gminfo_foca-acccepted-logbook_formats.pdf),
  §2.3-2.5: "Any alteration of values relevant to the experience of the pilot shall be
  traceable (change log)… shall be an integral part of any exports / print outs. An exception
  … for a period of 48 hours after the initial entry"; any alteration "shall invalidate the
  attached signature automatically"; calculated times "may only reduce". Written for
  software; formats exist for "aeroplane, helicopter, sailplane, balloon" (§2.2).
- **DE**, [NfL 2212-21](https://www.regierung.oberbayern.bayern.de/mam/dokumente/bereich2/sg25/nfl__2212-21.pdf):
  annex §1, the logbook is "ein Dokument mit Urkundencharakter"; annex §4.3, every flight is
  recorded "dauerhaft und unmittelbar noch am Tag des jeweiligen Fluges"; annex §4.3.2,
  erroneous entries are struck "so … dass diese erkennbar bleiben", and the correct entry is
  made "unter Angabe des Änderungsdatums". The NfL covers "FCL.050, SFCL.050 (Segelflug) und
  BFCL.050 (Ballon)". The flight day as the draft window is NinerLog's reading of §4.3, not a
  stated rule.
- **DE UL**, [§120 LuftPersV](https://gesetze.legal/bund/luftpersv/120): no correction rule
  (contents, 2-year retention, instructor certification); NfL 2212-21 does not apply to
  Luftsportgeräteführer.
- **AT national**, [ZLPV 2006 §118](https://www.jusline.at/gesetz/zlpv_2006/paragraf/118) and
  [Anlage 6](https://360.lexisnexis.at/media/attachment/2006_ii_205_anlage6/pdf) lit. a:
  "Unrichtige Eintragungen sind so durchzustreichen, dass sie lesbar bleiben"; no pages may
  be removed; electronic logbooks need the authority's approval.
- **AT Part-FCL and the EASA fallback**,
  [AMC1 FCL.050 (e)(2)](https://aviation.bot/EASA/Aircrew-Dec-2024/amc1-fcl050-recording-of-flight-time-pilot-logbook?p=p_sqBQcP178yP&s=1):
  electronic records "readily available at the request of a competent authority …
  certified by the pilot, and in a format acceptable by the competent authority". No
  correction rule.
- **AU**, [CASR 61.355(2)(b)](https://classic.austlii.edu.au/au/legis/cth/consol_reg/casr1998333/s61.355.html):
  each entry "retained unaltered" for 7 years, strict liability;
  [RAAP 13 (2022)](https://www.raa.asn.au/files/download/?id=3524): electronic logbooks
  "should create an audit history of the change".
- **NZ**, Civil Aviation Rules Part 61, rule 61.29(e): "An incorrect entry … may be altered
  only by putting a line through the entry and by adding the correct information"; each page
  is certified by the pilot.
- **JP**, [JCAB 国空乗390号](https://www.mlit.go.jp/notice/noticedata/pdf/201312/00006077.pdf)
  and the glider 要領 (2022): no correction fluid; for gliders, "訂正内容の履歴がわかるように"
  (the correction history must be recognisable).
- **UY**, DINACIA RAU 61, §61.51(d),(f): no erasures or interlineation; an error is fixed by a
  new entry and the wrong line is voided with "errose".
- **DK**, [BL 6-04 (1999)](https://en.trafikstyrelsen.dk/media/16112/BL6-04%20on%20recording%20of%20flight%20time.pdf)
  §3.2.6: amendments must show "what the text was before the amendment and who has recorded
  the amendment" (national non-Part-FCL licences; current force unconfirmed).
- **BR**, [ANAC IS 61-001 Rev. G (2023)](https://pergamum.anac.gov.br/pergamum/vinculos/IS61-001G.pdf)
  §5.3.1: in the CIV Digital lifecycle a draft can be deleted, a submitted entry is never
  edited, only "exclusão solicitada" with date and requester. The official record is CIV
  Digital, hence suggested.
- **GB**: ink, printed-and-signed extracts and false-entry bans, no correction method
  ([UK CAA, guidance on documentation](https://www.caa.co.uk/general-aviation/pilot-licences/applications/documentation/guidance-on-documentation/)).
- **US**, [14 CFR 3.403](https://www.ecfr.gov/current/title-14/chapter-I/subchapter-A/part-3)
  bans fraudulent alteration of any document "(electronic or physical)";
  [AC 120-78B (2024)](https://www.faa.gov/documentLibrary/media/Advisory_Circular/AC_120-78B_FAA_Web.pdf)
  records "unalterable without the appropriate certification" (guidance).

### Draft window and corrections

A flight entry may be edited freely until its **draft deadline**:

- `hours_48`: 48 hours after the flight **landed**: the flight date plus the on-block time
  in UTC, rolled to the next day when the on-block time is earlier than the off-block time
  (`flightcalc.OnBlockInstant`). A flight with no on-block time falls back to the end of
  its flight day. The row's `created_at` plays no part;
- `flight_day`: the end of the flight's date in the departure airport's mean solar offset
  (`round(longitude / 15)` hours; an unknown airport counts as UTC).

Because the window is anchored on the flight, a flight entered after its flight day is
already past a `flight_day` deadline, so its first edit is a correction (a late entry is a
correction under the German rule). Creating a flight is never classified.

For a `user_edit` update of a flight owned by a strict-mode user, the edit is a
**correction** if any of these holds:

1. now is after the draft deadline (the earliest across requiring licences);
2. the flight has ever carried a signature with status `completed` or `voided`;
3. the flight's date (existing or new, whichever is earlier) is on or before the latest
   `certified_through`;
4. a `*Override` flag goes from false to true, or an overridden value changes, and now is
   after the end of the flight day.

A correction needs a non-blank `correctionReason`; the void reason of the latest voided
signature stands in when none is given. Without either, `PUT /flights/{id}` answers `422`
`correction_reason_required` and writes nothing. The classification is stored on the revision
(`is_correction`) with the reason. A save that changes nothing is not classified and records
nothing. Deletes are classified by rules 1 to 3 and stored on the void record; a delete is
never refused for lack of a reason. A replace-mode restore classifies the flights it wipes the
same way (rules 1 and 3; a replace is refused while the account holds signatures, so rule 2
cannot apply).

Changes the system makes are never classified and never need a reason: recalculation
(`recalc`), an aircraft re-registration that renames flights (`aircraft_rename`) and a
contact rename that rewrites crew names (`contact_rename`). They are
recorded with their source so exports and the history card can label them as system changes.
Imports create new flights and record no revision; `import` stays a valid stored source so
revisions restored from another installation keep it.

Renaming a contact rewrites the crew names on the user's unsigned flights (signed flights
keep the name they were signed with). The rewrite is a `contact_rename` system change with
the automatic reason `Crew contact renamed: <old> → <new>`; it never needs a user-supplied
reason, is never refused and is never a correction, so exports mark it with no `†` and label
it "System: contact rename" in the change log.

### Overrides under a Swiss licence

When a requiring licence is Swiss (`overridesDownwardOnly` in `GET /logbook/mode`), each
`*Override` value that a create or update sets or changes must not exceed the
auto-calculated value (FOCA: calculated times "may only reduce"). The service computes the
calculated value on a copy of the flight with the override flags and values cleared and
`flightcalc.ApplyAutoCalculations` run. A violation is `422` `override_exceeds_calculated`
with one `details[].field` per offending field. Overrides already stored and left unchanged
are not rechecked.

### Certification

`POST /logbook/certifications` records that the pilot certifies the logbook through a date
(AMC1 FCL.050 (e)(2)). The date may not be later than today at UTC+14, the latest timezone on Earth, so a pilot
anywhere can certify the current day (`certification_in_future`), nor
earlier than the latest certification (`certification_before_latest`), and strict mode must
be on (`409` `strict_mode_required`). Certifying does not lock flights; later edits to
flights on or before the date are corrections (rule 3).

### How history is captured

Capture is a database concern, not application code: flights are also changed by routes
that never pass through a service (bulk operations, cascades, raw SQL). Migration 79 puts
`BEFORE` triggers on `flights` and `flight_crew_members` that store a pre-image of the flight
and its crew once per transaction, only when the owner is in strict mode, the flight was not
inserted in the same transaction. A deferred
constraint trigger compares the pre-image with the final state at `COMMIT` and writes one
revision per flight per transaction only if something other than `updated_at` and
`signature_id` (or the crew's names and roles) changed.

The application supplies the change source, reason and correction flag through
`repository.ChangeContext` on the `context.Context`. Every postgres write that touches
`flights` or `flight_crew_members` runs in a transaction that first sets the transaction-local
settings `ninerlog.change_source`, `ninerlog.change_reason` and `ninerlog.change_correction`
(`applyChangeContext`); a write with no change context is stored with source `unknown`.
Two consequences for code: a logical user edit must be **one transaction** (the flight update
and the crew replacement are done by `FlightRepository.UpdateWithCrew`, otherwise one edit
would yield several revisions), and a new flight and its crew must be written in **one
transaction** (`FlightRepository.CreateWithCrew`), so the "inserted in this transaction"
check covers the crew rows too. Contacts are linked to the crew inside the write transaction
(`FlightWriteOptions.BeforeWrite`, run by the repository with contact and logbook
repositories bound to that transaction), after every check of the write has passed, so a
rejected request or a failed flight or crew write creates no contacts.

### Signed fields hash

When a signature completes (live or token flow), for every user, the SHA-256 of the flight's
signed fields is stored in `flight_signatures.signed_fields_hash`: the date, aircraft,
route, block and flight times, every time column, landings and takeoffs, remarks, instructor
and PIC names, and the crew sorted by role then name, serialised in a fixed field order
(`service.SignedFieldsHash`). Signature responses carry `fieldsHash` and a `fieldsHashValid`
recomputed on read, so an entry edited after its signature was voided no longer matches the
signature that was attested.

The public signing page (`GET /sign/{token}`) returns the hash of the flight as it shows it
(`fieldsHash`). The signer's `POST /sign/{token}` may send it back; when it no longer matches
the flight's current hash the request is `409` with `code: flight_changed` and nothing is
recorded, so a signature cannot attest fields the signer did not see. A request without
`fieldsHash` completes as before.

### Exports

Only a user in strict mode gets the audit trail in an export; the state is read when the
export is made, so switching strict mode off returns the standard output while the recorded
history stays. `LogbookService.ExportHistory` assembles it from the user's revisions and the current
state of the edited flights in two queries, with the same diff as
`GET /flights/{id}/revisions`, and from `ListVoided`.

- **CSV** (standard, EASA, FAA): `corrected` (`true` when the flight has a correction
  revision, `false` otherwise, like the other boolean columns) and `last_corrected_at`
  (latest correction: UTC date in the user's `dateFormat`, then `HH:MM`).
- **PDF** (EASA, FAA, both layouts): flights with a correction revision end their remarks in
  `†n`; `n` numbers those flights in print order and is the `REF` of their rows in the
  appendix. The marker is placed after truncation, so it is never cut. The appendix lists
  every `update` revision (corrections and system changes alike), the voided entries and the
  mode switches, and each signature's first 12 hash characters. Voided entries and the change
  log cover the whole logbook, not only the exported flights.
- Standard-mode documents are byte-identical to the output before strict mode existed; the
  PDF labels stay English.

### Backup and restore

The JSON backup (`GET /users/me/data`) carries `logbookMode` (state and every switch event),
`flightRevisions` and `logbookCertifications`, so cloud backups carry them too. A restore
(`POST /users/me/data`) writes them in the same transaction as the rest of the backup
(`service.ImportLogbook` through `repository.RestoreTx`). The logbook sections are validated
once while the restore is planned (`LogbookImport.Prepare`); a revision without `changedAt`, a
mode event without `createdAt` or a certification without `certifiedAt` rejects the whole
restore with `400`.

**Flight ids.** A restored flight keeps its id from the backup when no flights row holds
that id after the wipe (whoever owns it), no other user has revisions under it and the
account has no void record for it; otherwise it gets a new id. Its crew goes with it, and
revisions follow the id the flight ends up with. The void records of a deleted flight keep the
flight's id when nothing holds it, and share one fresh id otherwise. Revision numbers,
snapshots (with `id` and `user_id` rewritten), sources, reasons, correction flags and
timestamps are kept; crew snapshots lose their contact links. A revision whose
`(flight_id, revision_no)` already exists is skipped, so the history of a flight that keeps
its id is never duplicated.

This is what makes `replace` safe in a strict account. The wipe and the re-insert happen in
one transaction, so the deferred trigger finds a flight that comes back under its id
unchanged and writes nothing; only flights absent from the backup get a void record (source
`import`, reason "Replaced by backup restore"), flagged as a correction by the delete rules.
Replacing an account with its own backup any number of times changes neither the voided list
nor the history of a live flight.

If the backup was strict and the account is not, strict mode is switched on with an `import`
event after the backup's own events. A restored flight keeps its `createdAt`; a missing value
becomes the flight date at 00:00 UTC, and no value is later than the time of the restore.
`MaxRestoreRevisions` is 1,000,000; the 200 MiB request body limit is the constraint that
binds first.
