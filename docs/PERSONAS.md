# Personas

**This document is binding on both `ninerlog-api` and `ninerlog-frontend`.** Every feature,
field, report, currency rule and skill is checked against the personas below before it ships.
A change that makes one persona's experience worse without a stated reason is a regression,
exactly like a failing test.

The personas exist because NinerLog serves pilots whose flying has almost nothing in common:
a club glider pilot logging six winch circuits on a Saturday, an ultralight pilot flying from a
grass strip with no ICAO code, and an airline first officer logging IFR multi-crew sectors.
One undifferentiated form serves none of them well.

The persona check is run with the `personas` skill in either repo
(`.claude/skills/personas/SKILL.md`) and by the `persona-reviewer` agent.

---

## 1. The relevance principle

> A feature is only interesting to pilots who hold the rating it serves, fly the aircraft it
> serves, or are training toward one of them.

NinerLog derives, per user, which **disciplines** are relevant, and adapts every surface to
them. The design is specified in [plans/ADAPTIVE_DISCIPLINES.md](./plans/ADAPTIVE_DISCIPLINES.md).
Four invariants bind every implementation:

1. **Fold, never hide.** An element that does not serve an active discipline moves into a
   "More" drawer on the same screen. It is never removed, and never unreachable.
2. **Data always wins.** An element that holds data on the record in hand (a flight with IFR
   time, an aircraft with a launch method) is always shown, whatever the disciplines say.
3. **Fail open.** While the profile is loading, has failed, or holds a discipline the client
   does not know, everything is shown.
4. **Explain yourself.** Every adaptive decision can say why ("shown because: SPL licence
   #1234"), and the pilot can overrule it (on, off, training toward, or "show everything").

## 2. Disciplines

| Discipline | Served by |
| --- | --- |
| `AEROPLANE` | SEP/MEP/SET ratings, PPL(A)/LAPL(A)/CPL/ATPL, three-axis UL credit |
| `TMG` | TMG class rating (Part-FCL or SPL extension), UL three-axis motorgliders |
| `SAILPLANE` | SPL, LAPL(S), FAA glider, `GLIDER` rating, UL sailplanes |
| `ULTRALIGHT` | UL licences (DULV, DAeC, LBA), `ULTRALIGHT` ratings, qualified by UL kind |
| `GYROPLANE` | GPL, `GYROPLANE` rating, UL gyroplanes |
| `HELICOPTER` | `(H)` licences, UL helicopters |
| `IFR` | IR rating, logged IFR time or approaches |
| `MULTI_CREW` | ATPL/MPL, multi-pilot aircraft, logged SIC/multi-pilot/relief time |
| `INSTRUCTOR` | FI/CRI/FI(S)/examiner privileges, logged dual-given or examiner time |
| `SIMULATOR` | FSTD sessions |

Each discipline has a status per user: `active`, `training`, `dormant` or `off`.

---

## 3. The personas

Each persona lists the disciplines their profile must resolve to, their jobs, what they must
see, what must fold away for them, and acceptance scenarios. Scenarios are written to be
turned directly into e2e tests (API `test/e2e/`, frontend Playwright) and screenshot-harness
fixtures (`scripts/screenshots/fixtures.mjs`, one fixture set per persona).

Primary personas (P1–P6) are the gliding and ultralight pilots this work is for. Guard
personas (G1–G3) are there to stop the gliding and UL work from degrading everyone else.
Core personas (C1–C4) are the powered-aeroplane pilots most accounts belong to: an FAA
instrument pilot, an EASA aircraft owner, a flight instructor and a pilot migrating decades
of records.

### P1 — Lena, club glider pilot

- 29, software developer, flies at a German club airfield on weekends from April to October.
- **Licence:** SPL (LBA), launch methods winch and aerotow. No medical issues, LAPL medical.
- **Aircraft:** club ASK 21 (`D-1234`), LS4 (`D-5678`); occasionally aerotowed behind a DR400.
- **Volume:** ~70 launches and ~40 h a season, often 4–8 winch circuits a day of 6–12 minutes.
- **Club system:** Vereinsflieger — the club's flight log is the source of truth for billing.
- **Disciplines:** `SAILPLANE` active. Everything else `off`/no evidence.

**Jobs**
1. "Am I legal to fly this weekend — including winch, and with a passenger?"
2. Log a Saturday of winch circuits in under a minute, standing at the launch point.
3. Pull the season from Vereinsflieger without retyping it, launch methods included.
4. At season start: know exactly what is missing (15 launches, 2 training flights, 5 h) and
   book the check flight in time.

**Must see:** launch method on every glider flight; launches count; SFCL.160(a) recency with
the 15 launches and 2 training flights; per-method SFCL.155(c) launch recency; passenger
recency (3 launches in 90 days); season stats (launches by method, hours, longest flight).

**Must fold away:** off-block/on-block times (she logs take-off and landing), IFR and approach
fields, multi-pilot/SIC/relief/PICUS, examiner, night tiles, fuel, route waypoints, the
"3 landings in 90 days per model" aeroplane recency table.

**Acceptance scenarios**
- L1: Logging 6 winch circuits of 8 minutes each at one airfield takes one form submission
  (batch entry), yields 6 launches and 48 minutes, and each circuit counts toward winch recency.
- L2: A Vereinsflieger export with `S.-Art` W/F/E imports with the launch method set, and the
  imported gliders are classed `GLIDER`.
- L3: Opening the flight form with `D-1234` selected shows the launch method and take-off/
  landing times as primary inputs; no IFR section is visible without opening "More".
- L4: The currency page answers "legal this weekend?" for solo, winch, aerotow and passengers
  in one glance, and names what is missing in plain language.
- L5: Her printed logbook has launch-method and launches columns (AMC1 SFCL.050), not a
  single-pilot single-engine column.

### P2 — Jonas, student glider pilot

- 16, school student, training toward SPL at Lena's club. No licence yet.
- **Aircraft:** club ASK 21 (dual) and ASK 23 (first solos).
- **Disciplines:** `SAILPLANE` training (intent `goal`, evidence: dual flights, no rating).

**Jobs**
1. See progress toward the SPL skill test (SFCL.130: hours, launches, solo, cross-country).
2. Have the instructor sign training flights on the club phone or his own.
3. Log supervised solo flights correctly (they count as PIC time under supervision).

**Must see:** training progress toward SPL, dual and supervised-solo flights, instructor
signature, launch methods trained (SFCL.155(a)).

**Must fold away:** licence recency and passenger currency (he holds no licence yet), and
everything Lena folds away.

**Acceptance scenarios**
- J1: An account with no licence and three dual glider flights resolves `SAILPLANE` to
  `training`, and the dashboard shows training progress instead of an empty currency page.
- J2: The instructor can sign a flight without Jonas typing anything but his PIN or passkey.
- J3: Obtaining the SPL (adding the licence) turns `SAILPLANE` to `active` and carries all his
  training flights into recency without re-entry.

### P3 — Karl, TMG touring pilot

- 67, retired engineer, flies a club SF 25 Falke and a Super Dimona.
- **Licence:** SPL with TMG extension (SFCL.150). No Part-FCL licence.
- **Disciplines:** `TMG` active, `SAILPLANE` dormant (flew gliders until 2019).

**Jobs**
1. Know SFCL.160(b) status: 12 h / 6 h on TMG / 12 take-offs and landings / 1 h training flight.
2. Carry his grandchildren legally (3 take-offs and landings in a TMG in 90 days).
3. Log touring flights between real airfields quickly, with block times optional.

**Must see:** SFCL.160(b) with each row, TMG passenger currency, cross-country and distance.

**Must fold away:** launch method (a TMG is always self-launched — derive it, never ask),
winch/aerotow recency, IFR, multi-crew.

**Acceptance scenarios**
- K1: Selecting the SF 25 does not show a launch-method field; the flight counts toward
  self-launch recency.
- K2: His dormant glider history still appears in totals and reports, but glider recency does
  not nag him on the dashboard.
- K3: A flight with only take-off and landing times (no block times) is accepted and its total
  time is take-off to landing.

### P4 — Petra, cross-country and self-launch glider pilot

- 44, physician, competition pilot. Owns an ASG 29E (self-launching, classed `GLIDER`).
- **Licence:** SPL with aerotow and self-launch privileges, cloud-flying rating (SFCL.215); FI(S).
- **Also:** tow pilot on the club DR400 under a PPL(A) with SEP and a sailplane-towing rating.
- **Tools:** LX9000 IGC logger, WeGlide, OLC.
- **Disciplines:** `SAILPLANE`, `AEROPLANE`, `INSTRUCTOR` active.

**Jobs**
1. Import IGC files: launch method, take-off/landing times, release, outlanding, distance.
2. See the season in soaring terms: km, hours, outlandings, longest flight, speed.
3. Track SFCL.205 towing recency (5 tows in 24 months), SFCL.215 cloud-flying recency and
   SFCL.360 FI(S) validity next to her SEP revalidation.
4. Log instructing (dual given) in the ASK 21 and her own flights without switching apps.

**Must see:** everything Lena sees, plus self-launch recency, cloud flying, towing, IGC
import, distance/outlanding, instructor time.

**Must fold away:** IFR approaches, multi-crew. She flies aeroplanes (towing), so aeroplane
fields must be one tap away when a DR400 is selected — the aircraft decides.

**Acceptance scenarios**
- P1: Selecting `D-KXYZ` (ASG 29E, `GLIDER`) shows glider fields; selecting the DR400 in the
  same session shows SEP fields and a "tow flight" option — no global mode switch.
- P2: An IGC file of a self-launched out-and-return with an outlanding imports as one flight
  with launch method `self-launch`, an outlanding flag and task distance.
- P3: Her SEP revalidation never counts towed glider launches (towed-launch exclusion holds).

### P5 — Mehmet, three-axis ultralight pilot

- 52, owns a share in a C42 (`D-MXYZ`, 600 kg class) at a UL airfield without ICAO code.
- **Licence:** German UL licence ("Sportpilotenlizenz", issued by DULV — not the EASA SPL),
  three-axis kind, with passenger authorisation. Also a PPL(A) he rarely uses.
- **Disciplines:** `ULTRALIGHT` (three-axis) active, `AEROPLANE` dormant — unless he flies a
  C172 this year.

**Jobs**
1. Know §45 LuftPersV recency for three-axis, and §45a passenger recency.
2. Know which of his UL hours count toward the SEP revalidation of his PPL (FCL.035(a)(4)),
   and the reverse.
3. Be reminded of the aircraft side: Jahresnachprüfung (every 12 months), insurance,
   rescue-system repack and rocket expiry (manufacturer intervals).
4. Log from a strip with no ICAO code without fighting the form.

**Must see:** §45 recency per kind, §45a passengers, UL-to-SEP crediting in both directions,
aircraft reminders, free-text departure/arrival.

**Must fold away:** launch method, IFR, multi-crew, night fields (a German UL may not fly at
night; logging night time on a UL should warn, not hide).

**Acceptance scenarios**
- M1: A flight on `D-MXYZ` with departure "UL-Platz Musterstadt" saves, shows no airport-lookup
  error, and counts for three-axis recency.
- M2: The currency page shows three-axis §45 with the SEP credit named, and the PPL SEP card
  shows which UL minutes it counted.
- M3: A quick-added aircraft `D-M…` cannot be saved without a class and UL kind.
- M4: Night time logged on a UL flight triggers a warning, not a silent save.

### P6 — Sabine, trike and powered-paraglider pilot

- 38, flies a weight-shift trike (`D-MTRK`) and a powered paraglider (no registration) from a
  farm strip.
- **Licence:** DULV, kinds weight-shift and powered paraglider.
- **Disciplines:** `ULTRALIGHT` (weight-shift, powered paraglider) active.

**Jobs**
1. Keep weight-shift and powered-paraglider recency apart — they are different kinds.
2. Log a paraglider flight without inventing a registration.

**Must see:** per-kind recency, kinds never pooled.

**Must fold away:** everything aeroplane: launch method, IFR, multi-crew, complex/high-
performance/tailwheel aircraft flags, SEP crediting.

**Acceptance scenarios**
- S1: A flight on an aircraft with no UL kind never counts toward both kinds; the app asks her
  to set the kind instead.
- S2: A powered-paraglider flight can be logged without a registration.
- S3: No screen she visits in a normal week shows the words "IFR", "SIC" or "block".

### G1 — Mark, airline first officer (guard)

- 34, A320 first officer, ATPL(A) frozen, IR, MCC. Also a club PPL(A) SEP holder.
- **Disciplines:** `AEROPLANE`, `IFR`, `MULTI_CREW`, `SIMULATOR` active.

**Guard scenarios**
- A1: Nothing glider or UL appears on any screen he uses: no launch method, no UL kind, no
  sailplane reports, no trike class in pickers without opening "More classes".
- A2: His flight form, table columns, dashboard and PDF are unchanged by the adaptive work
  (screenshot diff shows no regression).

### G2 — Anna, PPL(A) weekend pilot converting to gliders (guard)

- 41, PPL(A) SEP, starts SPL training this season (SFCL.140 credit for PPL holders).
- **Disciplines:** `AEROPLANE` active, `SAILPLANE` training.

**Guard scenarios**
- N1: Her first dual glider flight turns on the glider toolkit with a one-time, dismissible
  explanation — without hiding any aeroplane feature.
- N2: Her aeroplane recency is not polluted by towed glider launches.

### G3 — Ruth, new account, no data (guard)

- Just registered. No licences, aircraft or flights.
- **Disciplines:** none resolved.

**Guard scenarios**
- R1: The onboarding asks "What do you fly, and what are you training for?" and sets intents
  from the answer.
- R2: If she skips it, she sees the full app (fail-open), never an empty shell.

### C1 — Tom, FAA private pilot with an instrument rating

- 52, US engineer, owns a Cessna 182T (`N734TB`) based at Frederick, Maryland (`KFDK`).
- **Licence:** FAA Private Pilot, airplane single-engine land, instrument airplane. FAA third-
  class medical.
- **Volume:** ~60 h a year, mostly IFR trips across the Mid-Atlantic, some legs at night.
- **Preferences:** `MM/DD/YYYY`, 12-hour clock, decimal hours, dot as decimal separator.
- **Disciplines:** `AEROPLANE`, `IFR` active.

**Jobs**
1. "Am I legal to fly IFR with my family tonight?" — flight review, §61.57(a)/(b) passenger
   currency with full-stop night landings, §61.57(c) instrument currency.
2. Log a trip with approaches, holds and night time as fast as his old paper logbook.
3. Hand the examiner or insurer a logbook that looks like the ASA/Jeppesen one they know.
4. Fly into small US fields that have no ICAO code (`W29`, `1O2`).

**Must see:** the §61.56 flight review and when it lapses; §61.57(c) approaches and holds in the
6 calendar months; night passenger currency counted in full-stop landings; FAA terms (ASEL,
IPC, flight review) and US formats everywhere.

**Must fold away:** EASA revalidation windows and FCL references, launch methods, UL kinds,
multi-crew, German radio licences.

**Acceptance scenarios**
- T1: Every date, time and duration he sees, in the app, the PDF and the CSV, follows his
  preferences (`03/31/2027`, `5:40 PM`, `1.3`).
- T2: The dashboard's "What you can fly today" includes the flight review: with it lapsed, no
  airplane row says he may fly.
- T3: Logging an IPC (`isIpc`) with approaches and a hold restores instrument currency that
  lapsed more than 12 months ago; §61.57(c) counts calendar months.
- T4: Night passenger currency counts only full-stop night landings (§61.57(b)).
- T5: With an FAA licence, the logbook PDF defaults to the FAA layout without `format=faa`.
- T6: A flight to `W29` or `1O2` keeps the identifier and gets coordinates, distance and night
  time like any ICAO airport.
- T7: No screen he visits in a normal week says "SEP (Land)", "revalidation" or "FCL";
  classes read as FAA classes (ASEL).

### C2 — Claudia, PPL(A) aircraft owner with SEP, MEP and IR

- 48, dentist in Mannheim, owns a Mooney M20J (`D-EMCK`), rents a school PA-44 Seminole
  (`D-GSEM`) to keep her MEP rating.
- **Licence:** EASA PPL(A) with SEP, MEP and IR(A) (single- and multi-engine). Class 2 medical,
  ICAO level 5, BZF I.
- **Volume:** ~100 h a year, IFR business and family trips in Germany and neighbouring
  countries; one MEP/IR proficiency check a year.
- **Disciplines:** `AEROPLANE`, `IFR` active.

**Jobs**
1. Keep three ratings with three different revalidation rules valid without a spreadsheet.
2. Keep her own aircraft airworthy: ARC, annual inspection, ELT battery, insurance.
3. "Can I take my family to Zürich on Friday night?" — FCL.060(b) with the IR waiver.
4. Show the flight school and the examiner what she has flown in the Seminole.

**Must see:** SEP (FCL.740.A(b)(1)), MEP and IR (FCL.625.A) each with its own expiry, window and
check; aircraft reminders on the dashboard before they are due; IFR time, approaches and
holds on every flight form.

**Must fold away:** launch methods, UL kinds, multi-crew, SIC and relief time, training
progress.

**Acceptance scenarios**
- O1: SEP, MEP and IR each show their own expiry and revalidation window; an MEP or IR
  proficiency check on a flight revalidates exactly the ratings it covers.
- O2: MEP revalidation requires the proficiency check (FCL.740.A); route sectors alone do not
  revalidate it.
- O3: IR revalidation follows FCL.625.A — a proficiency check in the 3 months before expiry —
  and asks for no experience the rule does not require.
- O4: An ARC due in 23 days shows on the dashboard and in a reminder email; completing it rolls
  the due date by its interval.
- O5: With a valid IR her passenger currency needs no night landing (FCL.060(b)(2)); without it,
  one night landing in 90 days.
- O6: Hobbs or tach time can be recorded per flight and drives hour-based reminders
  (50 h / 100 h checks), next to the date-based ones.

### C3 — Ines, full-time flight instructor FI(A)

- 38, FI(A) at an approved training organisation in Egelsbach, CPL(A) with SEP and night.
- **Students:** 6 PPL(A) and 2 LAPL(A) students at a time, 3–5 lessons a day in C172s and a
  DA40; occasional refresher flights for club pilots (she flew Heinz's, C4).
- **Volume:** ~800 flights and ~900 h of instruction a year.
- **Disciplines:** `AEROPLANE`, `INSTRUCTOR` active.

**Jobs**
1. Log five lessons at the end of the day in minutes, each with the student and the exercise.
2. Sign her students' flights in their logbooks, many a day, without a login per student.
3. Keep her FI(A) certificate valid (FCL.940.FI): instruction hours, seminar, assessment of
   competence, 3-year validity.
4. Answer "how many hours have I given, and to whom?" for the ATO and her own CV.

**Must see:** dual given time as the primary time; the student as crew on every lesson; her
FI(A) validity and what FCL.940.FI still needs; instruction given per student.

**Must fold away:** launch methods, UL kinds, multi-crew, IFR sections unless she logs IFR.

**Acceptance scenarios**
- I1: Logging a lesson with a student from contacts sets dual given time and the student
  crew role in one step, and "log another like this" keeps aircraft, student and airfield.
- I2: Her FI(A) (FCL.940.FI) is a recorded privilege with an expiry and its revalidation
  requirements (instruction hours, refresher seminar, assessment of competence), like FI(S).
- I3: She can sign several students' flights in one session, with her own passkey, and each
  student's logbook shows her signature.
- I4: Reports show instruction given per student and per programme, not only per instructor
  received.
- I5: A PPL(A) or LAPL(A) student's training progress (FCL.210.A / FCL.110.A) is tracked like
  the SPL's.

### C4 — Heinz, 40 years of flying, migrating in

- 68, retired pharmacist in Aachen, PPL since 1986 (converted to EASA PPL(A) in 2012), SEP with
  night, ~3,200 h.
- **Records:** four paper logbooks 1986–2011, carried in as one initial-hours snapshot; 2011
  onward (~1,550 flights) imported from his previous logbook app.
- **Aircraft:** a 1/4 syndicate share in a PA-28 (`D-EHVG`), club C172; two inactive aircraft
  from the import.
- **Disciplines:** `AEROPLANE` active.

**Jobs**
1. Move 40 years of records in one evening and trust the totals match his paper logbook.
2. Re-run the import after fixing a mapping without doubling any flight.
3. Keep using NinerLog on 10,000 flights as quickly as on 100.
4. Print a logbook for the examiner that carries his paper totals forward.

**Must see:** the initial snapshot on every total with its cut-off date; import preview with
duplicates flagged; lifetime totals that reconcile with the snapshot plus imported flights.

**Must fold away:** everything Claudia folds away, plus IFR.

**Acceptance scenarios**
- H1: Totals, reports and the PDF read snapshot + imported flights, and say which part is
  which; the sums reconcile to the minute.
- H2: Importing the same file twice, or restoring the same JSON backup twice, creates no
  duplicate flight.
- H3: An import of 10,000 rows previews and commits within a minute, and a failure part-way
  leaves no partial import.
- H4: With 10,000 flights, the flights list, search, dashboard and currency page each answer in
  under a second.
- H5: Inactive aircraft from the import stay attached to their flights and fold out of pickers.

---

## 4. The persona check

Every change is checked against this matrix before it is committed. The `personas` skill has
the procedure.

| Question | Asked for |
| --- | --- |
| Which personas does this change serve? Name at least one, or say it is persona-neutral. | Every change |
| Which personas see it, and is that intended? | Every rendered change |
| Does any persona see something that does not serve their disciplines, without "More"? | Every rendered change |
| Can any persona lose access to something they have data for? | Every gated change |
| Which acceptance scenarios does it close, and are they tested? | Every feature |
| Does it pass all three guard personas unchanged? | Every gliding/UL change |
| Does it pass the four core personas unchanged? | Every change to shared screens, currency or import |

## 5. Changing this document

Add a persona only when a real group of pilots is not represented, and add its acceptance
scenarios in the same PR. Never delete a scenario to make a change pass; mark it
`(superseded by …)` with a reason. Both repos' `personas` skills point here — there is one
copy.
