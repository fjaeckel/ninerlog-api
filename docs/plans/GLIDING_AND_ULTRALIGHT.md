# Plan: gliding, TMG and ultralight experience

**Status: proposed, not implemented.** This plan comes from an audit of both repos on
2026-09-26. Every work package below (WP) can go to a single implementer agent. The personas
in [../PERSONAS.md](../PERSONAS.md) are the acceptance criteria, and each WP lists the
scenario IDs it closes. The adaptive UI that ties it together is specified in
[ADAPTIVE_DISCIPLINES.md](./ADAPTIVE_DISCIPLINES.md).

## 1. Where we stand

**Strong.** The currency engine already covers these rules:

- Part-SFCL: SFCL.160(a)(b)(c)(e) and SFCL.155(c) launch-method recency
  (`internal/service/currency/easa.go:437-794`)
- German UL, per kind: LuftPersV §45/§45a (`german_ul.go:45-280`)
- Annex I credit toward SEP, LAPL and SPL (`credited_classes.go`)
- GPL (`gpl.go`)

The towed-launch exclusion keeps winch and aerotow launches out of powered ratings. All of
this has e2e coverage (`test/e2e/currency_glider_ul_e2e_test.go`,
`currency_ultralight_e2e_test.go`, `currency_gpl_e2e_test.go`).

**Weak.** Everything outside currency is still a powered-aircraft logbook with gliders and ULs
forced into it:

- **Block times are required, and they define total time** (`openapi.yaml:7252-7256`,
  `internal/api/handlers/flight_kind.go:41-43`, `internal/models/flight.go:199-217`). Gliders
  log take-off to landing. Pilots invent block times to get past the form.
- **Launch method is lost everywhere except the JSON backup.** It is missing from CSV, both
  PDFs, web-logbook and custom-report exports. The Vereinsflieger importer ignores `S.-Art`
  on purpose, with a comment that is now stale (`internal/service/importtemplate/sources.go:251,914-918`).
- **Imported and quick-added aircraft have no class.** See `internal/api/handlers/import.go:537-560`
  and `ninerlog-frontend/.../FlightForm.tsx:403-412`. Glider currency joins on the class
  (`currency_flight_data.go:208,243`), so a pilot's imported season counts for nothing.
- **No batch entry.** Six winch circuits take six full forms. "Fill from last flight" does not
  copy the launch method or the crew, and it clears the arrival field
  (`FlightForm.tsx:422-428`).
- **Launch-method field gated wrongly.** It shows for TMGs, where it is meaningless, and is
  missing for UL sailplanes (`FlightForm.tsx:442-447`).
- **Glider-shaped data not modelled at all:** outlandings, tow flights, release height, IGC,
  task distance, supervised solo, towing and cloud-flying ratings, and FI(S) validity.
- **UL-shaped data not modelled at all:**
  - aircraft reminders: Jahresnachprüfung, insurance, rescue-system repack
  - Einweisung
  - passenger authorisation (§84a)
  - registration is always required, which does not fit an unregistered powered paraglider
- **The German copy leans on English.** "Recency" is used as a German noun; the trike label
  says "schwerkraftgesteuert" where it should say "gewichtskraftgesteuert"; "Off-Block" appears
  on glider screens.

**Correctness.** These must ship before any polish, because each one can show a pilot as
legal when they are not:

| # | Finding | Evidence | Regulation |
| --- | --- | --- | --- |
| C1 | The §45(2) and (2a) training flight is summed from dual minutes across flights. Three 20-minute dual circuits satisfy it. | `german_ul.go` (`mInstructorMinutes`) | "ein Übungsflug von mindestens einer Stunde", i.e. one flight of at least 1 h ([§45](https://www.gesetze-im-internet.de/luftpersv/__45.html)) |
| C2 | Flights on a UL with no kind count toward every kind, §45a passenger recency included. A pilot rated for both trike and three-axis double-counts. | `german_ul.go:96,256` (`IncludeUnspecified: true`) | §45a requires "derselben Art" |
| C3 | A UL rating with no kind is evaluated as three-axis, so SEP/TMG time gets credited and three-axis thresholds apply. | `german_ul.go:63-68` | §45(4): other kinds follow DULV/DAeC rules |
| C4 | FAA §61.57(a) passenger currency is reported as the glider *rating* status, so "Expired" shows for a pilot who may legally fly solo. | `faa.go:162-200` | §61.57(a) governs passengers only |
| C5 | An FAA Private licence with a glider rating gets a night requirement. | `faa.go:230` | Glider night passenger rule does not apply |
| C6 | The licence-filtered PDF compares the raw `aircraft_class` case-sensitively, ignores the UL kind, and drops SEP flights that were credited toward §45(2). | `internal/api/handlers/export_pdf.go:555-581` | — |
| C7 | `SET*` classes land in the SP-ME column. | `internal/service/flightrules/display.go:197` | AMC1 FCL.050 |
| C8 | The LAPL(A) training flight has the same cumulative pattern as C1, for every LAPL(A) holder. | `easa.go:357` (`easaLAPLRule`, `mInstructorMinutes`) | FCL.140.A(a)(1): "one refresher training flight of at least 1 hour total flight time with an instructor" |
| C9 | FAA SEP and other powered ratings report §61.57(a)/(b) passenger currency as the rating *status*, the same pattern WP-02 fixed for gliders. It affects every FAA pilot (guard persona Mark's FAA peers). Found during WP-02; not yet scheduled. | `faa.go` (`faaPassengerRatingRule`) | §61.57 governs passengers; §61.56 governs acting as PIC |

Checked and **not** a finding: §45(2) allows the 12 h to be flown on three-axis UL, TMG or SEP.
Pooling SEP/TMG time is correct, as long as the training flight is on a three-axis UL, which
it is.

## 2. Decisions the owner must make first

The implementers are told not to invent these. Each WP that depends on a decision names it.

| ID | Question | Recommendation |
| --- | --- | --- |
| D1 | Unmet UL §45 recency: `expiring`, `expired`, or something else? | **Decided (2026-09-26):** a new status `lapsed`, meaning "licence valid, privileges not exercisable until recency is restored". `expired` stays reserved for a date expiry. |
| D2 | Winch circuits: one logbook row per circuit, or one row with `launches = N`? | **Decided (2026-09-26):** one row per circuit by default, plus a "one line" option for AMC1 FCL.050-style series entries. Both need a `launches` field separate from take-offs. |
| D3 | Block times optional for every class, or only for gliders and ULs? | **Decided (2026-09-26):** every flight accepts either block times or take-off/landing times. Total time is the block span when present, otherwise take-off to landing. |
| D4 | IGC files: parse and discard, or store them? | **Decided (2026-09-26):** store them as documents attached to the flight, and include them in export (rule 6). |
| D5 | OGN / WeGlide / Vereinsflieger integrations? | **Decided (2026-09-26): nothing paid.** No Vereinsflieger API (it needs a paid, per-club key). Vereinsflieger data comes in only through the free CSV export the pilot downloads, which the importer already reads (WP-21 adds `S.-Art`). WeGlide through the pilot's own free API key (WP-41). OGN is free under ODbL (WP-42). |
| D6 | Scope for UK NPPL(M) and FAA Sport/Part 103? | After the German/EASA work is finished (wave 4). |

## 3. Work packages

Order: **wave 0 (correctness) → wave 1 (toolkits foundation) → waves 2 and 3 in parallel →
wave 4**. Inside a wave, WPs that touch different files can run in parallel.

Every WP's definition of done:

- The `personas` skill has been run, and its report line names the scenario IDs closed.
- Tests name the scenario.
- The gates are green: `make fmt lint test route-check migrate-check` plus the e2e suite, and
  on the frontend `npx vitest run`, `npm run type-check` and `npm run lint`.
- Docs are in the same PR (`docs-sync`), and rules 5 and 6 are covered where they apply (admin
  surface, export/import).
- Frontend WPs attach before/after screenshots for every persona fixture they touch.

### Wave 0: correctness (api)

**WP-01. UL recency correctness** (`endpoint-implementer`; needs D1) — **implemented**; the
rules are in [DOMAIN.md](../DOMAIN.md#ultralights), the `lapsed` status in
[CURRENCY_MESSAGES.md](../CURRENCY_MESSAGES.md).

- The §45(2)/(2a) training flight becomes *one* flight of at least 60 min with instructor
  (dual) time, on the rating's kind. The same fix applies to the LAPL(A) training flight
  (C8). The SEP refresher of FCL.740.A(b)(1)(ii) stays cumulative, because it says "1 hour of
  flight training", not one flight. Add a `longestDualFlight` read beside
  `mInstructorMinutes`, as `easaSPLTMGRule` already does for SFCL.160(b)(1)(iii).
- A kindless UL flight counts only when the user holds exactly one UL kind. Otherwise it
  counts for none, and the result reports `unclassifiedFlights: n`. That is a new optional
  integer on `ClassRatingCurrency`, in the spec.
- A UL rating with no kind no longer defaults to three-axis. It evaluates to status
  `unknown` with `nameKey` `rating.ul_kind_required`.
- §45a counts take-offs and landings, not landings alone.
- Tests: unit tables in `german_ul_test.go`, plus e2e for the three-by-20-minute case (M2) and
  for trike plus three-axis with a kindless flight (S1).
- Docs: `DOMAIN.md#ultralights`, `CURRENCY_MESSAGES.md`.
- Closes S1, M2, C1, C2, C3 and C8.

**WP-02. FAA glider correctness** (`endpoint-implementer`)

- Report §61.57(a) as `PassengerCurrency`, not as rating status. Count only PIC landings.
- Add the §61.56 glider alternative (3 instructional flights instead of 1 h of training).
- Suppress the night requirement for a glider rating on any FAA licence.
- Docs: `SAILPLANES.md` (a new FAA section) and `DOMAIN.md`. Tests in `currency_glider_ul_e2e_test.go`.
- Closes C4 and C5.

**WP-03. Export correctness** (`endpoint-implementer`)

- The licence-filtered PDF uses the normalised class (trimmed, upper-case), filters ULs by
  kind, and includes credited SEP/TMG flights under a "credited" marker.
- `SET*` goes to SP-SE.
- Tests: handler tests plus `export_formats_e2e_test.go`.
- Closes C6 and C7.

### Wave 1: toolkits foundation

WP-10 to WP-15 are phases 0 to 3c of [ADAPTIVE_DISCIPLINES.md](./ADAPTIVE_DISCIPLINES.md#phases).
WP-12 (persona fixtures in the screenshot harness) blocks every frontend WP after it.

### Wave 2: glider experience

**WP-20. Take-off-to-landing time model** (api + fe; needs D3)

- API — **implemented**: a flight needs block times **or** take-off and landing times.
  `totalTime` comes from the block span, otherwise from take-off to landing. The handler
  validates (no `oneOf`; `FlightCreate` already carried its conditional requirements in the
  description), through `models.FlightClocks` (`internal/models/flight_times.go`). Night
  time, night take-off/landing, total-time recovery, CSV import, the EASA CSV/PDF and
  web-logbook exports and tap-to-log sessions (`takeoff` opens, `landing` completes) all
  use the same pair. e2e: `test/e2e/time_model_e2e_test.go` and
  `TestFlightTimePairValidation` in `validation_e2e_test.go`. See DOMAIN.md
  "Total time and pilot function time".
- FE: when the selected aircraft is `GLIDER`, `TMG` or `ULTRALIGHT`, take-off/landing come
  first and block times fold into "More". Quick Log (`QuickLogPage.tsx`) gets the same order.
- Closes L3 and K3.

**WP-21. Launch method end to end** (api + fe)

- API:
  - validate the enum in `models/validation.go:41`
  - add a `launchMethod` import field
  - map Vereinsflieger `S.-Art`/`startart` to it. Verify the codes against a real export: W
    winch, F aerotow, E self-launch, A car, G bungee.
  - add a CSV export column
  - add the method to PDF remarks until WP-22 lands
  - classify auto-created aircraft. Use the Vereinsflieger aircraft type when present.
    Otherwise use the registration: `D-[0-9]{4}` → `GLIDER`; `D-K…` → leave unset and flag,
    because SLG vs TMG is ambiguous; `D-M…` → `ULTRALIGHT` with the kind unset and flagged.
- FE:
  - show the field for aircraft `GLIDER` or UL `SAILPLANE`
  - hide it for `TMG` and UL `THREE_AXIS_MOTORGLIDER` (implicitly self-launch)
  - quick-add requires a class (and a kind for UL)
  - add an "Unclassified aircraft" banner on the aircraft page listing aircraft with flights
    but no class
- Closes L2, K1, M3 and B5.

**WP-22. Sailplane logbook PDF** (api)

- API — **implemented**: `GET /exports/pdf?format=sailplane`, an AMC1 SFCL.050 layout with
  launches and launch-method columns, PIC/dual/FI(S) time, take-off to landing, outlanding
  and release height in the remarks, and no multi-pilot/IFR/night columns or `[Launch: …]`
  marker. The licence-scoped export picks it for SPL/LAPL(S)/FAA glider licences when no
  `format` is given. See [SAILPLANES.md](../SAILPLANES.md#printed-logbook).
- FE: offer the layout in the PDF export dialog (and default to it for a sailplane logbook).
- Text-extraction test (`export_pdf_discipline_test.go`) and e2e.
- Closes L5.

**WP-23. Batch circuits and "log another"** (api + fe; needs D2)

- API — **implemented** (migration 76): a nullable `launches` column with
  `launchesOverride`, derived from the take-off count and read by every launch count in
  the currency engine; the D2 "one line" series entry is a single flight with `launches`
  set. `POST /flights/batch`: a `FlightCreate` template plus 1–50
  `legs[{departureTime, arrivalTime, landings?, launches?, remarks?}]`, validated like a
  single create and stored in one transaction; an invalid leg is a 400 naming its index.
  See [SAILPLANES.md](../SAILPLANES.md#launches-and-series-entries).
- FE:
  - A "Circuits" mode in the flight form: aircraft, launch method, crew and site entered once,
    then one line per circuit with take-off and landing.
  - A "Log another like this" action after save. It copies reg, crew, launch method and
    departure, and sets arrival to departure for local sites.
  - "Fill from last flight" also copies the launch method and crew.
- Closes L1.

**WP-24. "Ready to fly?" and what's missing** (api + fe)

- API — **implemented**: every requirement row gets `validUntil` (the date the count falls
  below the threshold as flights age out) and a `remedyKey` (e.g. SFCL.155(d): "fly 2
  launches dual or supervised solo"). `GET /currency/readiness?date=&aircraftReg=&passengers=`
  answers per rating, per launch method, passengers and medical. See
  [DOMAIN.md](../DOMAIN.md#readiness-get-currencyreadiness).
- FE: a dashboard card and a currency-page header. Lena's version reads "Saturday: solo ✓
  winch ✓ aerotow ✗ (2 launches) passengers ✓". Also a season-start planner listing the
  minimum flights needed.
- Closes L4.

**WP-25. Glider flight facts** (api + fe; `migration-author` for the columns)

- API — **implemented** (migration 76): flight flags `isOutlanding` and `isTowFlight`, plus
  `releaseHeightM` (0–20000 m). Cross-country is no longer derived from departure ≠ arrival
  when `isOutlanding` is set. All three, and `launches`, go into the JSON backup, the
  standard CSV layout, CSV import, logbook search and custom-currency filters.
- Supervised solo: **decided and implemented without a crew role** — it is logged as
  `spicTime`, and SFCL.160(a)(1) and (b)(1) count PIC + dual + SPIC minutes (closes the
  known gap in `SAILPLANES.md`). The FI(S)/BI signature uses the existing flight signatures. Fields fold unless `SAILPLANE` or `TMG` is active
  (tow flight: `AEROPLANE` + `SAILPLANE`).
- Closes P1 and J2 (partly).

**WP-26. Sailplane ratings and privileges** (api + fe) — **API half implemented**
(migration 77, `/licenses/{id}/privileges`, `privileges[]` on `GET /currency`, backup and
restore inside the licence entry, admin count `licencePrivileges`); rules in
[SAILPLANES.md](../SAILPLANES.md#privileges), keys in
[CURRENCY_MESSAGES.md](../CURRENCY_MESSAGES.md#privilegecurrencymessagekey). Cloud-flying time
is IFR time on `GLIDER` flights. The FI(S) refresher, the 9-year assessment and SFCL.155(a)
initial training counts are reported as not tracked. Frontend half open.

- Structured privilege records on a licence (new table, portable, with an admin count):
  - SFCL.205 sailplane or banner towing: 5 tows in 24 months, counted from `isTowFlight`
  - SFCL.215 cloud flying: 1 h or 5 flights in 24 months; needs a cloud-flying time or flag
  - SFCL.360 FI(S): 3-year refresher, 30 h or 60 launches, 9-year assessment
  - SFCL.155(a) initial launch-method training
  - SFCL.115(a)(2) passenger prerequisites
  - SFCL.160(e)(2) night TMG passengers
  - FAA §61.69 tow recency
  - DULV towing authorisation (Schleppberechtigung): 10 tows per type in 24 months
- Each gets an evaluator in the registry and a docs row in `SAILPLANES.md`.
- Closes Petra's P-jobs 3 and 4.

**WP-27. IGC import** (api + fe; needs D4) — **API half implemented** (migration 78
`flight_files`, `pkg/igc` parser and analysis, `POST /flights/igc/preview`,
`POST /flights/igc`, `/flights/{id}/files`, `flightFiles` in the JSON backup, admin count
`flightFiles`); rules in [SAILPLANES.md](../SAILPLANES.md#igc-import). Distances are
reported, not stored on the flight, and task distance (a declared C-record task) is not
computed: free and out-and-return (twice the free distance) only. Frontend half open.

- Parse the H, B and E records and ENL.
- Derive take-off and landing times, launch method (winch: steep climb to 300–600 m within
  60 s; aerotow: slow climb then release; self-launch: ENL/MOP), release height, outlanding
  (landing away from any known airfield), task or free distance, and max altitude.
- Match to an existing flight, or create one. Store the file as a flight document.
- Fuzz-test the parser; fixture files cover each launch method.
- Closes P2.

**WP-28. Soaring statistics** (api + fe) — **API half implemented**
(`GET /reports/soaring-season`; custom-report metrics `launches`, `outlandings`,
`towFlights` and groupings `launchMethod`, `aircraftClass`, `ulKind`); scope in
[SAILPLANES.md](../SAILPLANES.md#statistics). Launches by method is the `launchMethod`
grouping of the `launches` metric. No distance metric: `flights.distance` is
airport-to-airport, so a distance metric waits for WP-27's task distance. Frontend half
open.

- Custom-report metrics: launches, launches by method, outlandings, distance.
- A season card on the dashboard (`SAILPLANE` active): launches by method, hours, longest
  flight, outlandings.
- Reports get a "Soaring" section that folds unless `SAILPLANE`.
- Closes L-job 4 and P-job 2.

**WP-29. Training progress** (api + fe)

- Syllabus templates:
  - SPL (SFCL.130): 15 h, 10 h dual, 2 h supervised solo, 45 launches, cross-country
  - TMG extension (SFCL.150)
  - German UL §42: three-axis 30 h/5 h solo; weight-shift 25 h/10 h dual/5 h solo
- Progress is computed from flights, shown when a toolkit is `training`, and signed by the
  instructor.
- Closes J1 and J3.

### Wave 3: ultralight experience

**WP-30. Aircraft reminders** (api + fe; `migration-author`) — **API half implemented**
(migration 75, `/aircraft/{id}/reminders`, `/aircraft-reminders`, notification category
`aircraft_reminder`, backup/restore, admin count); semantics in
[DOMAIN.md](../DOMAIN.md#aircraft-reminders). No job was added (the existing notification
checker carries it), so no new metric. Attaching documents to aircraft is not part of it.
Frontend half open.

- An `aircraft_reminders` table. Types:
  - Jahresnachprüfung (12 months)
  - insurance
  - rescue-system repack
  - rocket expiry
  - ARC
  - ELT battery
  - custom, with interval and due date
- Documents can attach to aircraft.
- Due items go into the notification digest. Add a metric and dashboard row if a job is added
  (`metrics-dashboards`), plus an admin count and export/import.
- Closes M-job 3.

**WP-31. UL logging fit** (api + fe) — API half **implemented**: powered-paraglider names in
place of a registration, save warnings `ul_night_flight`, `ul_mtom_exceeds_600` and
`ul_120kg_class` (API.md "Save warnings"). The 472.5 kg class and the §45(1) medical rule are
not evaluated.

- Registration optional for UL `POWERED_PARAGLIDER`, keyed to a named aircraft instead.
- A warning (not a rejection) when a German-UL flight has night time.
- MTOM checks by UL class (120 kg single-seater, 472.5 kg, 600 kg), plus the §45(1) medical
  rule above 120 kg empty mass.
- Closes S2 and M4.

**WP-32. Passenger authorisation and Einweisung** (api + fe) — **API half implemented**
with WP-26: `UL_PASSENGER_AUTH`, `UL_TYPE_BRIEFING` (detail: aircraft type) and `UL_TOWING`
(detail: kind) privileges; German UL passenger currency is current only with the
authorisation recorded (`pax.ul_authorisation_missing` otherwise) and carries the §84a
progress rows; see [DOMAIN.md](../DOMAIN.md#ultralights). Frontend half open.

- §84a DULV passenger-authorisation progress: 5 cross-country flights, 2 with landings,
  200 km in total, flown with an FI.
- Structured Einweisung (type familiarisation) records per aircraft type.
- Stored as licence privileges (same table as WP-26).
- Closes M-job 1.

**WP-33. UL in stats and exports** (api)

Custom-report group-by UL kind (`ulKind`) is implemented with WP-28.

- UL kind in CSV, stats-by-class breakdowns and custom-report group-by.
  - API — **implemented** for CSV and stats-by-class: the standard CSV ends with
    `AircraftClass` and `ULKind`; `GET /reports/stats-by-class` keeps one `ULTRALIGHT` row and
    adds its per-kind split as `byUlKind` (additive, so current clients are unaffected).
    Custom-report group-by is separate work.
- A UL PDF layout: kind and passengers columns, no IFR/MP. Chosen automatically for UL
  licences.
  - API — **implemented**: `GET /exports/pdf?format=ultralight`, picked for UL licences. No
    passengers column: a flight does not record a passenger count. See
    [DOMAIN.md](../DOMAIN.md#printed-logbook-layouts).
- FE: render `byUlKind` under the ultralight bar, and offer the UL layout in the export dialog.
- Closes M and S reporting.

**WP-34. German copy pass** (fe, `i18n-sync`)

- Replace "Recency" as a German noun with "Fortlaufende Flugerfahrung" or "Flugpraxis", after
  agreeing a glossary in `docs/TRANSLATION_GUIDE.md`.
- "gewichtskraftgesteuert".
- Show "F-Schlepp" in the launch-method options.
- Glider-specific help text in place of "Bremsklötze weg".
- Translate "PIC under supervision".
- Closes F5 across all personas.

### Wave 4: ecosystem (after D5 and D6)

- ~~WP-40: Vereinsflieger API sync~~ — dropped (D5: paid per-club key). The CSV import covers it.
- WP-41: WeGlide link. The pilot pastes their own API key (Profile → Settings → Advanced →
  API Key on weglide.org), which is free and sent as `X-API-Key`. The key allows 60 requests a
  day, and a user can hold at most 2. It only reads the pilot's own flights and IGC files.
  WeGlide grants OAuth only to apps with 1,000 or more users, so there is no "Sign in with
  WeGlide". Store the key encrypted (`pkg/cryptoutil`, like backup credentials), and sync
  at most once a day per user. The key is user-owned secret material: document it as exempt
  from export, with the reason.
- WP-42: OGN flight suggestions. The data is free, under ODbL, and comes from the public
  APRS feed. OGN rules: no re-distribution of data older than 24 hours; `no-track` devices
  are never received, and `no-ident` must not be shown. The pilot links their FLARM ID
  per aircraft. A server-side APRS listener turns take-off and landing events into a
  *suggestion* that expires after 24 hours. Only the times and site the pilot confirms are
  stored, as their own flight; raw positions are never stored. The only cost is operating
  an always-on connection, which needs a metric, a Grafana panel and an admin config flag
  (rule 5). An opt-in env var keeps self-hosters who don't want it unaffected. Confirm the
  24-hour interpretation with OGN before building.
- WP-43: UK NPPL(M)/microlight "12 in 24" evaluator.
- WP-44: FAA Sport Pilot (MOSAIC) and a Part 103 light mode.
- WP-45: offline club-day mode for the launch point.

## 4. Handing a WP to an agent

Paste this template with the WP filled in. Implementers are told not to invent missing
decisions, so a WP that depends on an unanswered D-item must not be handed out.

```
You are implementing WP-<nn> from docs/plans/GLIDING_AND_ULTRALIGHT.md in <repo>.
Read first: CLAUDE.md, docs/PERSONAS.md, the WP text, and the skills it names
(personas, api-change, aviation-domain, migrations, user-data-portability, admin-surface,
e2e-sync, docs-sync — frontend: personas, screenshots, api-layer, design-system, i18n).
Decisions already made: <D-items with the owner's answers>.
Contract: <fields, status codes, validation, ownership — copied from the WP>.
Acceptance: scenarios <IDs> from docs/PERSONAS.md, each as a named test; guards A1/A2/N2
unchanged.
Do not: change behaviour outside the WP, weaken a test, commit security findings.
Finish with the gates green, docs updated, and the persona report line.
```

Review every WP with the `persona-reviewer` agent before merge.
