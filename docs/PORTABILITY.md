# Data portability — taking your logbook elsewhere

A pilot logbook is a legal record that spans a career. Somebody who logs twenty
years of flying in NinerLog must be able to walk away with all of it, in a shape
the next product actually ingests, without hand-mapping columns in a
spreadsheet.

Leaving is a supported operation, not an afterthought. This document specifies
what ships, what each destination can and cannot represent, and what still needs
validating.

## The three endpoints

| Endpoint | Purpose |
| --- | --- |
| `GET /exports/targets` | Enumerate the destinations this deployment supports, with their caveats. Clients render this rather than hard-coding a list. |
| `GET /exports/logbook?target=…` | One file in the destination product's own import format. |
| `GET /exports/archive` | The complete account as an open, documented ZIP. |

All three require authentication and are scoped to the calling pilot by the
service layer. They share the `expensive` rate-limit bucket (15/min per user) —
deliberately generous, because a rate limit must never be what stops somebody
retrieving their own logbook.

These sit alongside the existing exports (`/exports/csv`, `/exports/pdf`,
`/exports/json`, `/exports/vcard`), which are unchanged. The CSV and PDF exports
target *authorities* — an examiner or a licensing office. These target *other
software*.

## Supported destinations

| Target | Product | Format | Round-tripped? |
| --- | --- | --- | --- |
| `foreflight` | ForeFlight Logbook | Two-table ForeFlight import template (CSV) | **Not yet** |
| `logten` | LogTen Pro | Flat CSV with LogTen field names as headers | **Not yet** |
| `myflightbook` | MyFlightbook | Flat CSV, decimal hours, Yes/No flags | **Not yet** |
| `crewlounge` | CrewLounge PILOTLOG (formerly mccPILOTLOG) | Flat CSV, HH:MM times, EASA columns | **Not yet** |

> **Round-tripped means:** the file has been fed through a live import of the
> destination product and the result checked against the source. Every layout
> here was built from the product's published import template but **has not yet
> been confirmed end to end**. `GET /exports/targets` reports this per target as
> `verified`, the admin console lists unverified targets under
> `exportTargetsUnverified`, and the UI tells the pilot rather than implying a
> guarantee that has not been tested.
>
> Flip a target's `Verified` flag in `internal/service/portability/target.go`
> once it has actually been round-tripped — and only then.

### Where each layout lives

One file per destination, each holding its column table at the top:

```
internal/service/portability/
  portability.go   Bundle + Gatherer (data collection, no format logic)
  values.go        shared value conversions — units, dates, booleans
  target.go        the registry: adding a product means one entry + one file
  foreflight.go    ForeFlight Logbook
  logten.go        LogTen Pro
  myflightbook.go  MyFlightbook
  crewlounge.go    CrewLounge PILOTLOG
  archive.go       the open archive
  metrics.go       Prometheus counters
  testdata/        golden files, one per target
```

Every layout is pinned by a golden-file test. A reviewer cannot eyeball a
forty-column CSV and see that a value shifted one column left; a golden diff
makes it obvious. Regenerate deliberately and read the diff:

```bash
go test ./internal/service/portability/ -update
```

## What survives, and what does not

Every vendor format is a **lossy projection**. A destination can only store what
it models.

| Record | ForeFlight | LogTen | MyFlightbook | CrewLounge | Open archive |
| --- | --- | --- | --- | --- | --- |
| Flights, times, landings | ✅ | ✅ | ✅ | ✅ | ✅ |
| Aircraft fleet | ✅ (own table) | ✅ (per row) | ✅ (per row) | ✅ (per row) | ✅ |
| Structured approaches | ✅ (6 max) | partial | count only | count only | ✅ |
| Crew / people on board | ✅ (6 max) | ✅ | partial | ✅ | ✅ |
| EASA multi-pilot time | ❌ | ✅ | as a property | ✅ | ✅ |
| SP-SE / SP-ME split | ❌ | ❌ | ❌ | ✅ | ✅ |
| FSTD (simulator) sessions | as sim rows | ✅ | ✅ | ✅ (own columns) | ✅ |
| Licences and class ratings | ❌ | ❌ | ❌ | ❌ | ✅ |
| Medicals, radio certificates | ❌ | ❌ | ❌ | ❌ | ✅ |
| Contact details | ❌ | ❌ | ❌ | ❌ | ✅ |
| Instructor signatures + images | ❌ | ❌ | ❌ | ❌ | ✅ |
| Pre-NinerLog opening balance | ❌ | ❌ | ❌ | ❌ | ✅ |

**No vendor format carries everything.** That is why the open archive exists and
why the UI recommends downloading it alongside whichever vendor file the pilot
needs.

### Decisions worth knowing about

These are the places where a faithful export and a useful one pull apart. Each
was resolved deliberately.

**Aircraft flown but never added to the fleet are reconstructed from the
flights.** ForeFlight rejects a flight whose `AircraftID` has no row in the
aircraft table, so exporting only the fleet would silently drop a pilot's oldest
entries — exactly the ones they most need to keep. `aircraft.csv` marks these
rows `inFleet=false`.

**Simulators are declared as simulators, not aeroplanes.** A registration that
only ever appears on FSTD rows and is not in the fleet is exported as a training
device (ForeFlight `EquipmentType=sim`; no aircraft category or class in
LogTen). Labelling an FNPT as a single-engine aeroplane would let the
destination count simulator hours as flight time — an error a pilot discovers at
a licence renewal, long after the migration.

**CrewLounge gets FSTD sessions in its simulator columns only.** NinerLog stores
a session's duration in `totalTime` (a flight row requires it), and PILOTLOG has
dedicated simulator columns that stay out of the flight totals. Writing both
would count the session twice and inflate the pilot's aeroplane hours, so the
session goes to the simulator columns alone. **A pilot's PILOTLOG total will
therefore be lower than their NinerLog total by the sum of their simulator
time.** This is PILOTLOG modelling the distinction that NinerLog folds together,
not data loss — the sessions are all present.

**"SELF" is resolved to the pilot's actual name** for every destination except
CrewLounge. `flightrules.DisplayPICName` renders `SELF` when the account holder
was PIC — the EASA logbook convention, and what the EASA PDF prints. That
convention does not travel: a product that treats the column as a person would
create a crew member literally called SELF. CrewLounge's own format uses `SELF`,
so it keeps it.

**Products without departure/arrival columns get the airports in the route
field.** MyFlightbook stores airports only in `Route`. Passing through an empty
route would strand every flight logged without an explicit one with no airports
at all, so `DEP ARR` is synthesised.

**The instructor is not repeated as a passenger.** ForeFlight has a dedicated
`InstructorName` column as well as `Person1…6`; listing the instructor in both
makes them appear as instructor *and* passenger on every training flight.

**Day/night landings map to the destinations' full-stop columns.** NinerLog does
not distinguish full-stop from touch-and-go, and uses its day/night landing
counts for its own currency calculations. Mapping them to the full-stop columns
keeps the destination agreeing with what NinerLog itself shows the pilot;
carrying only a total would lose the day/night split that night currency
depends on. A pilot who logs touch-and-goes as landings will see the same
overstatement in both products, not a new one introduced by the export.

**Approaches beyond six are truncated in ForeFlight**, which has six columns.
Where `approachesCount` exceeds the structured entries, the remainder is emitted
as one generic entry so the *total* still matches. The open archive carries the
full structured list.

## The open portability archive

A plain ZIP of UTF-8 CSV and JSON. No tool reads it today; any tool can be made
to, and it stays readable with nothing but a text editor long after every
importer targeted here has changed. **This is the actual portability guarantee —
the vendor formats are the convenience.**

Format id `ninerlog-portability-archive`, currently version `1.0`
(`ArchiveFormatVersion` in `archive.go`). Bump the version when the layout
changes in a way a reader would notice.

### Members

| Path | Contents |
| --- | --- |
| `manifest.json` | Machine-readable index: format id and version, export timestamp, pilot, every file with a description and row count, and the unit conventions. |
| `README.md` | The same information for a human, generated per export. |
| `flights.csv` | Every logged flight, one row each, keyed by `flightId`. |
| `aircraft.csv` | The fleet plus reconstructed aircraft; `inFleet` distinguishes them. |
| `licenses.csv` | Pilot licences. |
| `class-ratings.csv` | Ratings, linked to licences by `licenseId`. |
| `credentials.csv` | Medicals, language proficiency, radio certificates. |
| `contacts.csv` | People recorded in the logbook. |
| `crew.csv` | Who was on board each flight and in what role, linked by `flightId`. |
| `signatures.csv` | Instructor sign-off records, referencing images by `imageFile`. |
| `signatures/<id>.png` | Captured signature images. |
| `baseline.json` | Hours flown before this logbook began. **Omitted entirely when the pilot has no baseline** — its presence is meaningful. |

### Conventions

- Durations are decimal hours, two decimal places (`1.50` = one hour thirty).
- Dates are ISO 8601 (`YYYY-MM-DD`); times of day are UTC `HH:MM`.
- Distances are nautical miles.
- Booleans are `true` / `false`.
- IDs are stable within one archive and link the files to each other. They are
  NinerLog's own UUIDs, so two exports of an unchanged account produce the same
  IDs.
- A cell beginning with `'` had a leading `=`, `+`, `-` or `@`. See below.

### Why `baseline.json` matters

The opening balance is not a flight, so **every flight-shaped export drops it** —
including all four vendor formats. A pilot who recorded 310 hours of prior
experience and then reconciles totals after a migration would find hundreds of
hours missing with no indication why. The archive exports it explicitly, the
file explains that the hours appear in no flight row, and the generated
`README.md` points a confused pilot at it.

## Security

Every cell in every export goes through `pkg/csvsafe`, which neutralises
spreadsheet formula injection (CWE-1236) by prefixing a leading `=`, `+`, `-`,
`@`, tab or carriage return with an apostrophe. Logbook text is user-controlled
and these files exist to be handed to somebody else, so a remarks field
containing `=HYPERLINK("http://evil.test")` would otherwise execute on the
recipient's machine.

The guard lives at the writer, not at call sites, so a newly added column cannot
reintroduce the hole. `pkg/csvsafe` is shared with the older handler exports in
`internal/api/handlers/export.go`; there is exactly one implementation.

## Operating it

See [METRICS.md](./METRICS.md#data-portability-metrics) for the three metrics,
`docs/metrics/dashboards/ninerlog-operational.json` for the panels, and
`docs/metrics/alerts/prometheus-rules.yml` (group `ninerlog-api.portability`)
for the alerts.

A failing export is not a degraded feature — it is a pilot being held in by a
broken door. `portability_exports_total{result="error"}` alerts on its own
rather than being tolerated inside a success ratio.

## Adding a destination

1. Add a `Descriptor` to `registry` in `target.go`, with `Verified: false` and
   honest `Notes` about what the destination cannot represent.
2. Add `<product>.go` with the column table at the top and one `write<Product>`
   function taking `(io.Writer, *Bundle)`.
3. Add the value to `ExportTargetId` in `api-spec/openapi.yaml`, then
   `make generate`.
4. Add a golden test and run with `-update`; **read the generated file** before
   committing it.
5. Update the support matrix above and the frontend's target notes.
6. Round-trip it through the real product, then set `Verified: true`.
