# Sailplanes and TMGs

What NinerLog needs to know about gliders, self-launching sailplanes and touring motor
gliders (TMGs): how to classify the aircraft, which EASA rules apply to their flights, and
how the currency engine implements each point. FAA gliders are covered in
[FAA gliders](#faa-gliders-14-cfr-part-61). The general engine design is in
[DOMAIN.md](./DOMAIN.md#currency-engine).

Source: Commission Implementing Regulation (EU) 2018/1976, Annex III (Part-SFCL), added by
Regulation (EU) 2020/357. Part-SFCL replaced Part-FCL Subpart S (`FCL.140.S`, `FCL.110.S` …)
for sailplane pilots; anything citing `FCL.140.S` is out of date.

## Aircraft and classes

Definitions (Regulation 2018/1976 Art. 2 and Regulation 1178/2011 Art. 2):

| Term | Meaning |
| --- | --- |
| Sailplane | Heavier-than-air aircraft supported by the dynamic reaction of the air against its fixed lifting surfaces, whose free flight does not depend on an engine |
| Powered sailplane | A sailplane with one or more engines that has, with engine(s) inoperative, the characteristics of a sailplane |
| Self-launching sailplane (SLG) | A powered sailplane that can take off under its own power, usually with a retractable engine or propeller |
| TMG (touring motor glider) | A specific class of powered sailplane with an integrally mounted, non-retractable engine and non-retractable propeller, capable of taking off and climbing under its own power |

Every TMG is a sailplane in Part-SFCL, so "on sailplanes" includes TMGs and the rules
say "excluding TMGs" where they mean otherwise. A self-launching sailplane is a sailplane
and not a TMG, even though it has an engine.

German registrations don't tell them apart: `D-K…` marks every *Motorsegler* (SLG and
TMG), `D-0…`…`D-9…` pure sailplanes, `D-M…` ultralights including UL motorgliders.

How to set the aircraft's `aircraft_class` in NinerLog:

| Aircraft | `aircraft_class` | Counts toward |
| --- | --- | --- |
| Sailplane, including self-launching (e.g. ASK 21, DG-808, Arcus M) | `GLIDER` | `GLIDER` rating (SFCL.160(a)), self-launch launch method when `launchMethod` is `self-launch` |
| TMG (e.g. SF 25, Dimona, Super Dimona) | `TMG` | SPL TMG rating (SFCL.160(b)), the 5 h of SFCL.160(a), self-launch launch method, PPL/LAPL TMG and SEP pools (FCL.740.A, FCL.140.A) |
| UL sailplane or UL motorglider | `ULTRALIGHT` | German UL rules (LuftPersV §45) |

The class decides. A self-launching sailplane classed `TMG` would feed the TMG and
aeroplane rules instead of the sailplane rules; the engine cannot detect that mistake.

## Launch methods (SFCL.155)

`Flight.launchMethod` is one of `winch`, `car`, `aerotow`, `self-launch`, `bungee`.

SFCL.155(a): a pilot may use only launch methods they trained for. Initial training:

| Method | Training |
| --- | --- |
| Winch, car | 10 dual launches + 5 supervised solo launches |
| Aerotow, self-launch | 5 dual launches + 5 supervised solo launches; self-launch dual may be in a TMG |
| Bungee | 3 launches dual or supervised solo |

SFCL.155(c), verbatim: "In order to maintain the privileges for each launching method …
SPL holders shall complete a minimum of five launches during the last two years, except
for bungee launch, in which case they shall complete only two launches. In the case of
self-launch, launches may be done in self-launch or through take-offs in TMGs or a
combination of these."

SFCL.155(d): a pilot who lapses flies the missing launches dual or supervised solo.

Winch, car, aerotow and bungee are **towed** launches: they count only toward a `GLIDER`
rating or a sailplane-licence rule, never toward a powered class, its proficiency check or
its passenger currency. Self-launches are not towed.

### Launch method in and out

Every write path accepts only those five values (`models.ValidateFlightTextFields`,
`ErrInvalidLaunchMethod`); the service trims and lower-cases the value first and treats a
blank one as none. A flight create or update with any other value is a 400, and so is a
`POST /imports/json` restore: the restore stops at that flight with
`Failed to import flight on <date> (<reg>): invalid launch method: <value>`, like any other
invalid flight in a backup.

CSV import maps a launch column to the `launchMethod` import field
(`importtemplate.ParseLaunchMethod`, case-insensitive, trimmed):

| Cell | Launch method |
| --- | --- |
| `W`, `Winde`, `Windenstart`, `winch` | `winch` |
| `F`, `F-Schlepp`, `Flugzeugschlepp`, `aerotow` | `aerotow` |
| `E`, `Eigenstart`, `self-launch` | `self-launch` |
| `A`, `Autoschlepp`, `car` | `car` |
| `G`, `Gummiseil`, `Gummiseilstart`, `bungee` | `bungee` |

The single letters are Vereinsflieger's `S.-Art` codes; the generic German `Startart`
header and English `Launch Method`/`Launch Type` headers are recognised too. Any other
value imports the flight with no launch method; the row does not fail.

An imported launch method (from a launch column or a `[Launch: …]` remarks marker) is kept
only when `models.LaunchMethodApplies` to the flight's aircraft: the class already in the
fleet, or for an aircraft the import creates, the class it infers. It applies to no class
at all, `GLIDER`, `TMG`, and `ULTRALIGHT` with no kind or kind `SAILPLANE` or
`THREE_AXIS_MOTORGLIDER`; for every other class (`SEP*`, `MEP*`, `SET*`, `GYROPLANE`, other
ultralight kinds, `OTHER`) it is dropped. Vereinsflieger writes `E` for powered aircraft
too, so a club Cessna classed `SEP_LAND` imports with no launch method and no launch marker
in its remarks. The preview applies the same rule. `POST`/`PUT /flights` do not: a launch
method entered by hand is stored whatever the aircraft.

Exports: the standard CSV layout has a `LaunchMethod` column, followed by the glider flight
facts and the aircraft's `AircraftClass`/`ULKind`. The EASA, FAA and
vsimakhin/web-logbook CSV layouts and the EASA and FAA PDFs have no launch column, so
`flightrules.CombinedRemarks` appends `[Launch: <method>]` to the remarks cell after the
function-time annotations. On import a `[Launch: <method>]` marker in a remarks column is
read back as the launch method (a launch column wins) and removed from the remarks, so all
four CSV layouts round-trip it. The JSON backup carries the field as is. The sailplane PDF
has its own launch columns and prints no marker; see [Printed logbook](#printed-logbook).

Aircraft an import creates get a class so the currency engine counts their flights; see
[Aircraft class on import](./AIRCRAFT_REGISTRATIONS.md#aircraft-class-on-import). A towed
launch on an aircraft with no other class evidence makes it `GLIDER`.

### Flight times: take-off to landing

Glider flights have no block time: there is no engine start or chocks, so AMC1 SFCL.050
records the departure and arrival times as take-off and landing. A flight is logged with
`departureTime` (take-off) and `arrivalTime` (landing) alone; `offBlockTime` and
`onBlockTime` stay empty. `totalTime` is then take-off to landing (block time wins only
when both block times are present, as a TMG pilot may record them), and night time,
night take-offs/landings, cross-country time and every recency count derive from the same
pair. The EASA exports print take-off and landing in their departure/arrival time
columns, an import of a take-off/landing-only logbook needs no block columns, and
tap-to-log opens a session at `takeoff` and completes it at `landing`. See
[DOMAIN.md](./DOMAIN.md#total-time-and-pilot-function-time).

### Launches and series entries

`Flight.launches` is the number of launches a flight row records. It is derived as the
take-off count (day + night, which is one per landing), at least one for a flight, and 0
for an FSTD session or a passenger flight. A pilot may set it, which sets
`launchesOverride`; `null` on `PUT` returns it to derivation. A row stored before the field
existed has no value and counts its take-offs, at least one.

Every launch count in the currency engine reads it: the 15 launches of SFCL.160(a)(1)(i)
and the per-method SFCL.155(c) counts (`GetProgressByAircraftClass`, `GetLaunchCounts`).
TMG take-offs toward self-launch count the TMG flights' launches the same way.

Winch circuits can be logged two ways:

- **One row per circuit** (the default). Each circuit is a flight with its own take-off
  and landing, one launch each. `POST /flights/batch` logs a series of them at once; see
  [Batch circuits](#batch-circuits).
- **One row for the series**, as AMC1 FCL.050 allows for a series of flights on one day
  at one site: a single flight whose take-off and landing span the series, with
  `launches` set to the number of launches (and `landings` to the number of landings).
  Six winch launches in one row count as six launches for SFCL.155 and SFCL.160.

### Batch circuits

`POST /flights/batch` takes a `template` (a full flight create body: date, aircraft,
launch method, crew, departure and arrival, remarks …) and 1–50 `legs`. Each leg carries
its take-off (`departureTime`) and landing (`arrivalTime`), and may carry `landings`
(default: the template's, else 1), `launches` and `remarks`. Each leg is the template with
the leg laid over it and is validated exactly like `POST /flights`; the legs are stored in
one database transaction, so an invalid leg rejects the whole batch with a 400 naming the
zero-based leg index and nothing is created. The response lists the created flights in leg
order. Lena's six 8-minute winch circuits are one request: six flights, 48 minutes, six
launches.

### Outlandings, tow flights and release height

- `isOutlanding` marks a landing away from the planned site (Außenlandung). An outlanding
  is not a cross-country flight by virtue of its landing place, so no cross-country time
  is derived from departure ≠ arrival; a cross-country time the pilot enters is kept.
- `isTowFlight` marks a flight on which the pilot flew the tug, towing a sailplane. It is
  meaningful on a powered aircraft and is not validated beyond being a boolean. The
  towing privileges count it (SFCL.205, 14 CFR 61.69, DULV; see [Privileges](#privileges)).
- `releaseHeightM` is the tow or winch release height in whole metres, 0–20000; anything
  else is a 400.

All three, and `launches` with its override flag, travel in the JSON backup, the standard
CSV layout (`Launches`, `Outlanding`, `TowFlight`, `ReleaseHeightM` after `LaunchMethod`),
the sailplane PDF (outlanding and release height as remarks markers) and CSV import (`Starts`/`Launches`, `Außenlandung`/`Outlanding`, `Tow Flight`,
`Release Height`; a boolean reads true for `true`, `yes`, `ja`, `x`, `1` or a positive
number). An imported launch count is stored as the pilot's value. They are logbook query
fields (`launches`, `outlanding`, `towflight`, `releaseHeight`), and `is_outlanding` and
`is_tow_flight` are custom-currency filters beside the `launches` metric.

### IGC import

`POST /flights/igc/preview` analyses an FAI IGC flight recorder file without storing it;
`POST /flights/igc` stores it with a flight, either one the pilot names (`flightId`) or one
created from the file through the same path as `POST /flights`. The file is kept verbatim
(`flight_files`, decision D4), can be downloaded again from
`GET /flights/{id}/files/{fileId}`, and travels in the JSON backup.

**Parser** (`pkg/igc`, pure, no I/O). Reads the A record (logger manufacturer and serial),
the H records `DTE` (both `HFDTE150708` and `HFDTEDATE:150708,01`), `PLT`, `GTY`, `GID`
(registration) and `CID`, the first I record (the `ENL` and `MOP` extensions by byte
offset), B fixes (time, latitude, longitude, validity, pressure and GNSS altitude) and E
events. Lines may end in CRLF, LF or CR; unknown records are ignored; malformed and
out-of-order fixes are skipped and counted. A fix more than 12 hours earlier than the one
before it is on the next UTC day, so a file crossing midnight UTC keeps its order. Every
input is bounded: 5 MB, 200,000 fixes, 4,096 bytes per line, 1,000 events, 64 extensions,
100 runes per header value. A control character other than tab, a file that does not start
with an A record, one without a valid date, or one without a valid fix is rejected with a
typed `*igc.ParseError` naming the reason and line, which the API returns as a 400.

**Analysis** (`igc.Analyze`). Only valid (`A`) fixes with a position count. The altitude is
GNSS when more than half the valid fixes carry one, else pressure altitude.

| Fact | Rule |
| --- | --- |
| Ground speed | Distance to the latest fix at least 4 s earlier, over the elapsed time |
| Take-off | Start of the first run above 25 km/h that lasts 20 s |
| Landing | Start of the first run below 5 km/h, at least 20 s after take-off, that lasts 60 s or reaches the end of the file; without one the last fix, and `landingDetected` is false |
| Flight time | Take-off to landing, rounded to minutes as `POST /flights` rounds clock times |
| Self-launch | The file declares ENL or MOP and the larger of the two is at least 500 (of 999) for 30 s, starting within 2 min of take-off. Release is the start of the first 30 s with the engine below 500; an engine that never stops gives no release height. Confidence 0.9, or 0.75 without an engine stop |
| Winch | Top of the climb within 90 s of take-off gains 250–700 m (150–900 m at lower confidence) at a mean of at least 4 m/s, and over the next 30 s the climb rate falls below half of that. Release is the top. Confidence 0.85, or 0.6 outside 250–700 m |
| Aerotow | From 1 min after take-off, the first fix after which the glider turns 200° within 30 s (circling) or sinks at a mean below −0.3 m/s over 20 s ends the tow; release is the highest fix up to there. The tow must last 2 min, gain 200 m and climb at 1–6 m/s. Confidence 0.75 for at least 3 min and 300 m, else 0.7 |
| Unknown | None of the above; confidence 0 and no release height |
| Release height | Altitude at release minus altitude at take-off, whole metres |
| Maximum altitude | Highest altitude between take-off and landing (metres MSL) |
| Free distance | Largest great-circle distance from the take-off point to any fix in flight, km |
| Out-and-return distance | Twice the free distance: an approximation that assumes the pilot turned at the farthest point and flew back toward the start. It is not a scored task distance |

The checks run in the order self-launch, winch, aerotow. A self-launching sailplane
recorded without ENL or MOP climbs like an aerotow and is reported as one; a powerful tug
climbing above 6 m/s is reported as unknown.

**Places and outlanding.** Take-off and landing are named by the airport within 3 km in the
airport database (`internal/airports`), else by coordinates (`50.49889N 9.95389E`, used as
the flight's `departureIcao`/`arrivalIcao` text). A landing is an **outlanding** when it was
detected, lies more than 3 km from the take-off point and more than 3 km from every known
airport. Landing back at an unlisted home field is therefore not an outlanding, and with
the airport database unavailable no landing is one.

**Matching.** The preview's `matchingFlightId` is a flight of the pilot on the take-off's
UTC date, on the same registration (canonical notation), whose take-off/landing (else
block) span overlaps the file; a flight on that date and glider without a complete time
pair matches when no overlapping flight does.

**Creating the flight.** The aircraft is the `HFGIDGLIDERID` registration; a file without
one can only be attached to an existing flight. A registration missing from the fleet is
created with the header's glider type as type, make and model, and the class the importers
infer (`models.InferImportedAircraftClass`: `D-[0-9]{4}` and every other registration
`GLIDER`, `D-M…` `ULTRALIGHT`). The flight gets the take-off UTC date, take-off and landing
times (no block times, so total time is take-off to landing), departure and arrival, one
landing, the outlanding flag, and the launch method with its release height when the method
is detected and `models.LaunchMethodApplies` to the aircraft. Launches follow from the
take-off as for any flight. Attaching (`flightId`) never changes the flight.

**Storage and limits.** A flight holds at most 5 files of at most 5 MB; the same file (by
SHA-256) is stored once per account, so a second import of it is a 409 naming the flight
that holds it. Files are listed as metadata and downloaded as `application/octet-stream`
with `Content-Disposition: attachment`. The JSON backup carries each file gzipped and
base64-encoded under `flightFiles`, keyed by its flight's id in the backup; a restore
revalidates it as IGC and attaches it to the restored flight.

### Printed logbook

`GET /exports/pdf?format=sailplane` prints an AMC1 SFCL.050 logbook, one landscape page per
batch of flights:

| Column | Source |
| --- | --- |
| Date | `date` |
| Aircraft: type, registration | `aircraftType`, `aircraftReg` |
| Take-off: place, time | `departureIcao`, `departureTime` |
| Landing: place, time | `arrivalIcao`, `arrivalTime` |
| Flight time | `totalTime` (take-off to landing) |
| Launch: method, launches | `launchMethod` (`Winch`, `Aerotow`, `Self-launch`, `Car`, `Bungee`), `launches` |
| Pilot function time: PIC, dual, FI(S) | PIC + PICUS + SPIC, `dualTime`, `dualGivenTime` |
| Remarks and endorsements | `flightrules.SailplaneRemarks`: remarks, endorsements, function-time annotations, `[Outlanding]`, `[Release <n> m]` |

The take-off and landing columns print `departureTime`/`arrivalTime`; a flight with only
block times prints those. There are no single-/multi-pilot, night, IFR or FSTD columns and
no `[Launch: …]` marker. Every page ends with the three totals rows (this page, previous
pages, total time) for flight time, launches, PIC, dual and FI(S) time, and the pilot's
certification and signature line. A totals summary page follows: flights, flight time,
launches in total and per method, PIC, dual, FI(S) time and outlandings. A prior-experience
baseline opens the time balances; it has no launch count, so the launch totals cover logged
flights only and the summary says so. Instructor sign-offs print in the remarks cell as in
the EASA layout.

The licence-scoped export (`logbookLicenseId`) picks this layout by itself for an SPL,
LAPL(S) or FAA glider licence (`service.LogbookFormatForLicence`); an explicit `format`
wins. The `layout` parameter (spread/single) does not apply.

## Statistics

Soaring statistics count **soaring flights**: flights whose aircraft (matched by
registration in the pilot's fleet) is classed `GLIDER`, is an `ULTRALIGHT` of kind
`SAILPLANE`, or is classed `TMG` when the flight has a launch method. FSTD sessions and
passenger flights never count, and neither do flights on a registration missing from the
fleet. The rule is one SQL predicate (`soaringFlightSQL`,
`internal/repository/postgres/soaring.go`) shared by both surfaces below, so they agree.

- `GET /reports/soaring-season?year=YYYY` — the dashboard season card: one calendar year
  (UTC, default the current year, 1900 to next year, else 400) with flights, launches,
  launches per method (`winch`, `aerotow`, `selfLaunch`, `car`, `bungee`, and
  `unspecified` for soaring flights without a method), total and average minutes,
  outlandings, the longest flight by total time, and the five departure places with the
  most soaring flights. A year without soaring flights is all zeros, not an error.
- Custom reports (`/reports/custom`) take the metrics `launches` (the launch count of
  soaring flights only, so a pilot's aeroplane take-offs never read as launches),
  `outlandings` and `towFlights` (flights marked `isOutlanding` / `isTowFlight`, any
  aircraft), and the groupings `launchMethod`, `aircraftClass` and `ulKind`. CSV and PDF
  exports add a soaring column only when the report charts it or its total is not zero.

Launches read `Flight.launches` exactly as the currency engine does (`launchCountSQL`: the
stored count, else the take-offs, at least one).

The longest flight is by total time. Neither surface uses `flights.distance`: it is the
great-circle distance between departure and arrival airports, 0 for a local or
out-and-return soaring flight and unknown for an outlanding field, so it says nothing about
the distance flown. IGC import (WP-27) computes free and out-and-return distance for a
file but does not store them on the flight, so a soaring distance metric still waits for a
stored flown or task distance.

## Recency (SFCL.160)

### (a) Sailplanes, excluding TMGs

> SPL holders shall exercise SPL privileges, excluding TMGs, only if in the last 24 months
> before the planned flight they:
> (1) completed, on sailplanes, at least five hours of flight time as PIC or flying dual or
> solo under the supervision of an FI(S), including, on sailplanes, excluding TMGs, at
> least: (i) 15 launches; and (ii) two training flights with an FI(S); or
> (2) passed a proficiency check with an FE(S) on a sailplane, excluding TMGs; the
> proficiency check shall be based on the skill test for SPL.

Reading the text:

- The 5 h may be flown on any sailplane, TMGs included ("on sailplanes"); the 15 launches
  and the two training flights must be on sailplanes excluding TMGs.
- Dual and supervised solo time count toward the 5 h, not PIC time alone.
- "Two training flights" is a number of flights, with no minimum duration. Two short
  winch circuits with an instructor count; one long dual flight does not.

### (b) TMGs

> SPL holders shall exercise their TMG privileges only if in the last 24 months before the
> planned flight they:
> (1) completed at least 12 hours of flight time as PIC or flying dual or solo under the
> supervision of an FI(S), including, on TMGs, at least: (i) six hours flight time;
> (ii) 12 take-offs and landings; and (iii) a training flight of at least one hour total
> flight time with an instructor; or
> (2) passed a proficiency check with an examiner … based on the skill test as specified
> in point SFCL.150(b)(2).

- The 12 h may include sailplane hours; 6 h, 12 take-offs and landings and the training
  flight must be on TMGs.
- The training flight is one flight of at least 1 h total time, not 1 h of dual time
  added up over several flights.

### (c) Part-FCL TMG holders

SPL holders who also hold a Part-FCL licence with TMG privileges (e.g. PPL(A) or LAPL(A)
with a TMG class rating) are exempt from (b); the Part-FCL rules for that licence apply
instead (FCL.740.A or FCL.140.A).

### (d) Logbook

Dual flights, supervised flights, training flights and proficiency checks under (a) and (b)
must be entered in the logbook and signed by the FI(S) or FE(S).

### (e) Passengers

> SPL holders shall carry passengers only if in the preceding 90 days they have carried out
> as PIC, at least: (1) three launches in sailplanes, excluding TMGs, if passengers are to
> be carried in sailplanes, excluding TMGs; or (2) three take-offs and landings in TMGs, if
> passengers are to be carried in a TMG. For carrying passengers at night in a TMG, at
> least one of those take-offs and landings shall be carried out at night.

SFCL.115(a)(2) additionally requires, once after licence issue, 10 h or 30 launches as PIC
plus a passenger-competence training flight (or an FI(S)/BI(S) certificate).

## How NinerLog implements it

Code: `internal/service/currency/easa.go` (`easaSPLRule`, `easaSPLTMGRule`,
`easaLaunchMethodCurrency`, `EvaluatePassengerCurrency`); data:
`internal/repository/postgres/currency_flight_data.go`.

| Rule | Requirement (`nameKey`) | Measured as | Classes |
| --- | --- | --- | --- |
| SFCL.160(a)(1) | `requirement.flight_time` ≥ 300 min | PIC + dual + SPIC minutes | `GLIDER` + `TMG` |
| SFCL.160(a)(1)(i) | `requirement.launches` ≥ 15 | the flights' `launches` | `GLIDER` |
| SFCL.160(a)(1)(ii) | `requirement.training_flights` ≥ 2 | flights with dual time | `GLIDER` |
| SFCL.160(a)(2) | `requirement.proficiency_check` | a proficiency check flight in 24 months | `GLIDER` |
| SFCL.160(b)(1) | `requirement.flight_time` ≥ 720 min | PIC + dual + SPIC minutes | `GLIDER` + `TMG` |
| SFCL.160(b)(1)(i) | `requirement.tmg_time` ≥ 360 min | PIC + dual + SPIC minutes | `TMG` |
| SFCL.160(b)(1)(ii) | `requirement.tmg_landings` ≥ 12 | landings | `TMG` |
| SFCL.160(b)(1)(iii) | `requirement.tmg_training_flight` ≥ 60 min | longest total time of a flight with dual time | `TMG` |
| SFCL.160(b)(2) | `requirement.proficiency_check` | a proficiency check flight in 24 months | `TMG` |
| SFCL.160(c) | `rating.sfcl_tmg_exempt`, no requirements | the pilot holds a `TMG` rating on a Part-FCL licence | — |
| SFCL.155(c) | `launchMethodCurrency[]` | `launches` per `launchMethod`; `self-launch` adds the `TMG` flights' launches | `GLIDER` (+ `TMG`) |
| SFCL.155(a) | `launchMethodCurrency[].trained` | a `LAUNCH_METHOD_TRAINED` privilege for the method on the rating's licence | — |
| SFCL.160(e)(1) | passenger currency, `easa_spl_pax` | landing days of flights with PIC time | `GLIDER` |
| SFCL.160(e)(2) | passenger currency, `easa_spl_tmg_pax` | landing days of flights with PIC time; night landings when the licence holds `TMG_NIGHT` | `TMG`, SPL licence only |
| SFCL.115(a)(2) | passenger currency `requirements`, informational | PIC minutes and launches since the licence's issue date | `GLIDER` + `TMG` |

- Recency is met by either all experience rows or the proficiency check. When neither is,
  the rating reports status `lapsed` with `rating.recency_not_met`: the licence stays valid,
  its privileges may not be exercised until recency is restored. `expired` is reserved for a
  date expiry.
- <a id="supervised-solo"></a>**Supervised solo.** "Solo under the supervision of an
  FI(S)" counts toward the hours of SFCL.160(a)(1) and (b)(1). A student logs it as
  `spicTime` (student pilot-in-command), which carves out of PIC time, and the hours rows
  sum PIC, dual and SPIC minutes (`progress.spicMinutes`). A supervised solo logged as PIC
  time counts as PIC time. SPIC time on `ULTRALIGHT` aircraft is not credited, like their
  dual time.
- The glider rule applies to a `GLIDER` rating on any licence and to every non-TMG rating
  on an `SPL` or `LAPL(S)` licence; only a `GLIDER` rating pools TMG hours.
- `launchMethodCurrency` lists every method the pilot has ever logged on the rating's
  class, so a lapsed method shows as `0 / 5`, and every method with a
  `LAUNCH_METHOD_TRAINED` privilege on the rating's licence, logged or not (a trained
  `self-launch` never logged on a sailplane counts the TMG take-offs in the window).
  `trained` marks the methods the pilot recorded training for (SFCL.155(a)). It is
  informational: the rating status does not depend on it.
- Passenger currency for `GLIDER` never reports night privilege.
- Each met row and each met launch method carries `validUntil`, the last day it holds if
  the pilot does not fly again: with exactly 15 launches the launches row lasts until the
  oldest of them + 24 months − 1 day, a method with 5 launches until its fifth-newest
  launch + 24 months − 1 day. An unmet launch method carries
  `remedy.launch_method_dual` with the missing count: SFCL.155(d) lets the pilot restore it
  by flying the missing launches dual or solo under supervision.
- `GET /currency/readiness?aircraftReg=D-1234&passengers=true&date=…` answers a club
  pilot's "legal this Saturday?" (L4): the `GLIDER` rating, each launch method she has
  logged, sailplane passenger recency and her medical, evaluated on the flights already on
  record as of that date. A `GLIDER` aircraft selects `GLIDER` ratings only; a `TMG`
  selects the SPL or Part-FCL `TMG` rating. See [DOMAIN.md](./DOMAIN.md#readiness-get-currencyreadiness).
- SFCL.160(c) is applied by `Service.EvaluateAll`: an SPL TMG result is reported current
  with `rating.sfcl_tmg_exempt` when any EASA-evaluated licence that is neither a sailplane
  nor an ultralight licence (PPL, LAPL, CPL, ATPL) carries a `TMG` rating.
- AMC1 SFCL.160 credits Annex I sailplanes toward the hourly requirements only. PIC time on
  `ULTRALIGHT` aircraft of kind `SAILPLANE` or `THREE_AXIS_MOTORGLIDER` counts toward the 5 h
  of (a) and the 12 h of (b), and `THREE_AXIS_MOTORGLIDER` PIC time toward the 6 h on TMGs;
  their dual time, launches, landings and training flights never count, because a training
  flight needs an ORA.ATO.135-authorised aircraft. The result lists the kinds in
  `creditedUltralightKinds`.

- Passenger currency for `TMG` on an SPL reports night privilege only when the licence holds
  a current `TMG_NIGHT` privilege (SFCL.210). Night status then needs one night landing as
  PIC in the preceding 90 days (SFCL.160(e)(2)), with `nightExpiresOn`, and the message is
  `pax.current_day_night` or `pax.day_current_night_not`.
- Sailplane and SPL TMG passenger currency carry the SFCL.115(a)(2) prerequisites in
  `requirements`: `requirement.pax_prerequisite_time` (600 minutes as PIC) and
  `requirement.pax_prerequisite_launches` (30 launches as PIC), alternatives counted on
  `GLIDER` and `TMG` flights with PIC time since the licence's `issueDate`, and
  `requirement.pax_competence_flight`, which is not tracked (`requirement.untracked`). The
  rows never change `dayStatus`: a re-issued licence carries a later issue date than the
  one the rule means.

### Privileges

Ratings, endorsements and authorisations beside the class ratings are recorded as licence
privileges (`/licenses/{licenseId}/privileges`, table `licence_privileges`, migration 77).
`GET /currency` evaluates each one into `privileges[]`
(`internal/service/currency/privileges.go`, a rule per kind). Every privilege is `expired`
past its `expiresOn`; otherwise its recency rule decides between `current` and `lapsed`,
and a kind without one is `current` (`privilege.valid`). `messageParams.date` is the expiry
date when there is one.

| Kind | Rule (`ruleDescriptionKey`) | Rows | Counts |
| --- | --- | --- | --- |
| `SAILPLANE_TOWING` on a non-FAA licence | SFCL.205(c) (`sfcl_205_towing`) | `requirement.tows` ≥ 5 in 24 months | take-offs of `isTowFlight` flights on any class except `ULTRALIGHT` |
| `SAILPLANE_TOWING` on an FAA licence | 14 CFR 61.69(a)(5) (`faa_61_69_towing`) | `requirement.tows` ≥ 3, or `requirement.towed_glider_flights` ≥ 3, in 24 calendar months | tows as above; launches of `aerotow` `GLIDER` flights with PIC time |
| `BANNER_TOWING` | SFCL.205(c) (`sfcl_205_banner_towing`) | `requirement.tows` ≥ 5 in 24 months | the same tow flights as sailplane towing |
| `CLOUD_FLYING` | SFCL.215 (`sfcl_215_cloud_flying`) | `requirement.cloud_flying_time` ≥ 60 minutes or `requirement.cloud_flying_flights` ≥ 5 in 24 months | `ifrTime` of `GLIDER` flights with PIC time |
| `FI_S` | SFCL.360 (`sfcl_360_fi_s`) | `requirement.instruction_time` ≥ 1800 minutes or `requirement.instruction_launches` ≥ 60 in 3 years; `requirement.fi_refresher` (untracked) | `dualGivenTime` and launches of `GLIDER` and `TMG` flights with instruction given |
| `LAUNCH_METHOD_TRAINED` | SFCL.155(a) (`sfcl_155_launch_method`) | none; marks `launchMethodCurrency[].trained` | — |
| `TMG_NIGHT` | expiry (`privilege_expiry`) | none; enables SPL TMG night passengers | — |
| `AEROBATIC_BASIC`, `AEROBATIC_ADVANCED`, `BI_S`, `FE_S` | expiry (`privilege_expiry`) | none | — |

The ultralight kinds (`UL_PASSENGER_AUTH`, `UL_TOWING`, `UL_TYPE_BRIEFING`) are described
in [DOMAIN.md](./DOMAIN.md#ultralights).

- **Tows** are the take-offs of a tow flight (its `launches` when set), so a series row on
  the tug counts each tow. Banner tows have no flag of their own: a sailplane tow counts
  toward banner towing and the reverse.
- **Cloud flying time is IFR time on a sailplane.** A sailplane logs no IFR time otherwise,
  so NinerLog reads `ifrTime` on a `GLIDER` flight as time exercising cloud-flying
  privileges; log cloud flying there. TMGs are excluded, as are flights without PIC time.
- **SFCL.360 instruction** is `dualGivenTime`; a flight with instruction given counts its
  launches toward the 60.
- A met row carries `validUntil` (the date the newest counted flights reach the threshold,
  plus the window, minus one day; for 61.69 the last day of the 24th calendar month after
  it). An unmet SFCL.205 or SFCL.215 row carries `remedy.privilege_with_instructor`: fly the
  missing amount dual or under the supervision of an instructor. Other unmet rows carry
  `remedy.fly_more`.
- A privilege never changes a class rating's status. Petra's towing, cloud flying and FI(S)
  sit next to her SEP revalidation (P job 3); her tow flights never count toward it or
  toward her `GLIDER` rating (P3).
- A privilege with an `expiresOn` gets the rating-expiry email (category `rating_expiry`) at
  the pilot's warning days.

### Known gaps

- SFCL.155(a) initial launch-method training counts (10 dual + 5 supervised solo winch
  launches, …) are not evaluated; a `LAUNCH_METHOD_TRAINED` privilege records the outcome.
- SFCL.115(a)(2) is reported, not enforced (see above); the passenger-competence training
  flight is not tracked.
- The SFCL.360 refresher and the 9-year FI(S) assessment of competence are not tracked;
  `requirement.fi_refresher` is informational.
- SFCL.215 has no proficiency-check alternative in NinerLog: an `isProficiencyCheck` flight
  on a sailplane is the SFCL.160 check.
- Banner and sailplane tows share `isTowFlight`.

## FAA gliders (14 CFR Part 61)

A glider rating under FAA rules has no recency rule of its own. Whether the pilot may act
as PIC depends on the §61.56 flight review; whether they may carry passengers depends on
§61.57(a). The two are reported separately, like every other FAA rating.

### §61.56 flight review

> (a) … a flight review consists of a minimum of 1 hour of flight training and 1 hour of
> ground training. … (b) Glider pilots may substitute a minimum of three instructional
> flights in a glider, each of which includes a flight to traffic pattern altitude, in lieu
> of the 1 hour of flight training required in paragraph (a) of this section.

A flight review is valid until the end of the 24th calendar month after the month it was
completed in (§61.56(c)).

### §61.57(a) passengers

> … no person may act as a pilot in command of an aircraft carrying passengers … unless
> that person has made at least three takeoffs and three landings within the preceding 90
> days, and— (i) The person acted as the sole manipulator of the flight controls; and
> (ii) The required takeoffs and landings were performed in an aircraft of the same
> category, class, and type (if a type rating is required).

§61.57(a) governs passengers only. It does not decide whether a glider pilot may fly solo.
Night passenger currency (§61.57(b)) does not apply to gliders.

### How NinerLog implements it

Code: `internal/service/currency/faa.go` (`faaGliderRule`, `EvaluatePassengerCurrency`,
`EvaluateFlightReview`). The glider rule applies to a `GLIDER` rating on any FAA licence
and to every rating on an FAA `GLIDER` licence.

| Rule | Where | Measured as | Classes |
| --- | --- | --- | --- |
| §61.56(a) | rating, `requirement.flight_review` | the latest flight flagged `isFlightReview`, on any class | all |
| §61.56(b) | rating, `requirement.training_flights` ≥ 3 | flights with dual time since the first day of the calendar month 24 months ago | `GLIDER`, towed launches included |
| §61.57(a) | passenger currency, `faa_glider` | landing days of flights with PIC time in the last 90 days | the rating's class, towed launches included |
| §61.56 (per pilot) | `flightReview` | the latest `isFlightReview` flight | all |

- The rating is `current` when the flight review is current or the §61.56(b) row is met;
  otherwise it takes the flight review's status (`expiring`, `expired`), or `expired` with
  `flight_review.none_on_record` when no review is logged. Its `ruleDescriptionKey` is
  `faa_flight_review`; see [CURRENCY_MESSAGES.md](./CURRENCY_MESSAGES.md) for the keys.
- Landings in the last 90 days never change the rating status.
- "Sole manipulator of the controls" is approximated by PIC time: dual flights do not count
  toward passenger currency.
- Passenger currency for `GLIDER` never reports night privilege, on any FAA licence.
- The top-level `flightReview` is per pilot and ignores the §61.56(b) alternative; a glider
  rating can therefore be `current` while `flightReview` is `expired`.

### Known gaps

- The §61.56(b) alternative is inferred from dual flights only. The 1 hour of ground
  training, the flight to traffic pattern altitude and the instructor's endorsement are not
  recorded, and the alternative carries no expiry date.
- The alternative counts all three flights in the window, so a set of flights straddling
  its start is not recognised.
- §61.69 towing recency is evaluated for a `SAILPLANE_TOWING` privilege on an FAA licence
  ([Privileges](#privileges)); the other §61.69 prerequisites (100 hours PIC, the
  endorsement) are not, and unpowered ultralight tows are not told apart from glider tows.
