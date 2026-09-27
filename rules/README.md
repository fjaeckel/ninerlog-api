# ninerlog-rules

Pilot currency, recency and validity rules as audited plain text, with the evaluator that
runs them, a code generator and a coverage gate. Module
`github.com/fjaeckel/ninerlog-rules`; it lives inside `ninerlog-api` for now and can be split
out with `git subtree split --prefix=rules`. [DESIGN.md](DESIGN.md) is the binding contract.

Not legal advice: see [NOTICE](NOTICE).

## What is here

| Path | What |
| --- | --- |
| `vocabulary.yaml` | The closed vocabulary: metrics, filters, windows, combinators, stage conditions, events, subjects, statuses, units, record fields |
| `messages/keys.yaml` | Every message, requirement-name, remedy and rule-description key with its params; mirrors `ninerlog-api/docs/CURRENCY_MESSAGES.md` |
| `catalogue/<authority>/<instrument>/<rule-id>.yaml` | One rule per file, with its verbatim source quote and its divergences from the API |
| `cases/<rule-id>/<case>.yaml` | One record, one date and the expected evaluations |
| `sources/` | Verbatim regulation texts |
| `inventory/` | Research inputs: what the API does today, which articles are in scope |
| `schema/` | JSON Schema 2020-12 for every YAML file |
| `engine/` | The evaluator (`engine.Evaluate`, `engine.EvaluateRule`) and escape hatches (`engine/hatches`) |
| `gen/` | Generated constants and one test per rule (never edit) |
| `cmd/rulesgen` | The generator (`go generate ./...`) |
| `cmd/rulescheck` | Schema validation and the coverage gate |
| `docs/porting-guide.md` | The recipe for adding or porting a rule |

## Using the engine

```go
cat, err := engine.LoadCatalogue("path/to/rules")
evs := engine.Evaluate(cat, &record, engine.MustDate("2026-08-16"))
```

`record` is the neutral input of DESIGN.md section 4 (`engine.Record`); each `Evaluation`
has the shape of section 4. Absent optional input is unknown, never zero: a requirement
whose metric is unknown for every flight in its window is `tracked: false` and never met.

## Adding a rule

Follow [docs/porting-guide.md](docs/porting-guide.md). In short:

1. Put the verbatim article in `sources/` if it is not there yet.
2. Write `catalogue/<authority>/<instrument>/<id>.yaml` using only vocabulary names; quote the
   source verbatim; add `divergences` for every deliberate difference from the API and a line
   in `CHANGELOG.md` naming `<id>#<divergence>`.
3. Write cases under `cases/<id>/` until the gate shows no missing tags.
4. Set `maps_to` in `inventory/code-rules.yaml` and `inventory/articles-*.yaml`.
5. `go generate ./... && go test ./... && go run ./cmd/rulescheck -report -rule <id>`.

## Running the gate

```bash
cd rules
go vet ./...
go test ./...                               # engine, generated per-rule case tests, rulescheck in -report mode
go generate ./... && git diff --exit-code   # generated code is up to date
go run ./cmd/rulescheck -strict             # the CI gate
go run ./cmd/rulescheck -report             # same report, always exits 0
go run ./cmd/rulescheck -report -rule faa.14cfr61.61-57-c.instrument
```

`-strict` fails on schema errors, rule and case errors, missing coverage tags, unused or
unimplemented vocabulary, unmapped `inventory/code-rules.yaml` entries (except
`scope: consumer`), articles without a catalogue entry, missing CHANGELOG lines for
divergences, and engine statement coverage below 95 %. **CI runs `-strict` and fails until
the porting of every inventory entry and article is complete; that is expected.** Use
`-report` to track progress.

## Design choices

Where DESIGN.md leaves room, this module decided:

- **Name checking lives in `rulescheck`, not in the schemas.** The schemas check structure;
  `rulescheck` checks every metric, filter, window, condition, subject, unit and key against
  `vocabulary.yaml` and `messages/keys.yaml`, with messages that name the rule and field.
  Adding a vocabulary entry therefore never touches the schemas.
- **Types are hand-written in `engine/`; the generator emits** the metric dispatch the engine
  must implement (`engine/zz_metrics_gen.go`: a missing metric is a compile error),
  constants for rule ids, message keys and metric names (`gen/constants.go`), and one test
  per rule (`gen/rule_<id>_test.go`).
- **Coverage tags are observed, not declared.** The gate evaluates every case and derives the
  tags it actually exercised; a case's `covers:` list is checked against that and serves as
  documentation.
- **Windows.** `calendar_months: n` starts on the first day of the nth month before the month
  of `asOf` and runs through `asOf` (the month to date counts). `rolling_months` clamps to
  the end of the month (31 March minus one month is 28 or 29 February).
- **Records** carry, beyond DESIGN.md section 4: `holder.dateOfBirth`, `events` (checks,
  tests, courses that are not flight rows), `variants`, `validFrom` on ratings, privileges
  and credentials, and optional flight fields (`cruiseMinutes`, `soleManipulator`,
  `pilotFlying`, `tailwheel`, `mtomKg`, `towKind`, `fullStopLandings`, `checkRating`).
- **Requirement rows** may carry `lastDate` and a `messageKey` besides the fields of section 4.
- **Subjects** include `launch_method` (SFCL.155) besides the kinds of section 4.
- **Stage conditions** include `holds` (what the holder holds, e.g. a valid IR) and
  `undetermined` (the tree is neither met nor definitely unmet because input is missing).
