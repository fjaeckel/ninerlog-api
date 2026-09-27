# ninerlog-rules

Pilot currency, recency and validity rules as audited plain text, with the evaluator that
runs them, a code generator and a coverage gate. Module
`github.com/fjaeckel/ninerlog-rules`; it lives inside `ninerlog-api` for now and can be split
out with `git subtree split --prefix=rules`. [DESIGN.md](DESIGN.md) is the binding contract;
[docs/adr/0001-rules-catalogue.md](docs/adr/0001-rules-catalogue.md) explains why it exists.

Not legal advice: see [NOTICE](NOTICE).

## Status

`go run ./cmd/rulescheck -strict` passes: every rule, case, coverage tag, overlap,
vocabulary entry, inventory entry, source file and CHANGELOG reference is green, and engine
statement coverage is above 95 %. CI (`.github/workflows/rules.yml`) enforces it for every
change under `rules/`.

On 2026-09-27 the catalogue holds 145 rules and 646 cases:

| Authority | supported | partial | not_supported | total |
| --- | --- | --- | --- | --- |
| `easa` (Part-FCL, Part-SFCL, Part-BFCL, Part-MED) | 25 | 26 | 29 | 80 |
| `faa` (14 CFR Parts 61, 68) | 4 | 16 | 17 | 37 |
| `de` (LuftPersV, LuftVZO, DULV, DAeC) | 5 | 9 | 10 | 24 |
| `other` (NinerLog policy) | 4 | 0 | 0 | 4 |
| **total** | **38** | **51** | **56** | **145** |

`partial` rules evaluate what the record allows and say in `notes` what they leave out;
`not_supported` entries map an article in scope that NinerLog cannot evaluate. All 51 rules
the API evaluates today (`inventory/code-rules.yaml`, 6 of them consumers that stay in the
API) and all 113 articles in scope (`inventory/articles-*.yaml`) map to catalogue entries.
`messages/keys.yaml` has 269 keys; the vocabulary has 9 subjects, 29 metrics, 26 filters, 7
windows, 16 stage conditions and 5 rule groups. `sources/` holds 113 verbatim texts: 66
`eu-legal-act`, 32 `us-federal`, 15 `de-amtliches-werk`.

Open items that need a person rather than the gate: the titles and thresholds of the DULV
and DAeC association rules are marked `TODO: verify` (the documents are private and are
never stored here); see DESIGN.md section 13 for the other decisions taken at integration.

## What is here

| Path | What |
| --- | --- |
| `vocabulary.yaml` | The closed vocabulary: metrics, filters, windows, combinators, stage conditions, events, subjects, statuses, units, record fields, rule groups, allowed source origins |
| `messages/keys.yaml` | Every message, requirement-name, remedy and rule-description key with its params; mirrors `ninerlog-api/docs/CURRENCY_MESSAGES.md` |
| `catalogue/<authority>/<instrument>/<rule-id>.yaml` | One rule per file, with its verbatim source quote and its divergences from the API |
| `cases/<rule-id>/<case>.yaml` | One record, one date and the expected evaluations |
| `sources/` | Verbatim regulation texts of allowed origin only ([sources/README.md](sources/README.md)) |
| `inventory/` | Research inputs: what the API does today, which articles are in scope |
| `schema/` | JSON Schema 2020-12 for every YAML file |
| `engine/` | The evaluator (`engine.Evaluate`, `engine.EvaluateRule`) and escape hatches (`engine/hatches`) |
| `gen/` | Generated constants and one test per rule (never edit) |
| `cmd/rulesgen` | The generator (`go generate ./...`) |
| `cmd/rulescheck` | Schema validation and the coverage gate |
| `docs/porting-guide.md` | The recipe for adding or porting a rule |
| `docs/amc-gm-notes.md` | Own-words notes on EASA AMC and GM that bear on counting |
| `docs/adr/` | Architecture decision records |
| `docs/key-requests/`, `docs/changelog-fragments/`, `docs/mapping/`, `docs/vocab-requests/` | Fragments for parallel porting rounds (empty between rounds; each has a README) |

## Using the engine

```go
cat, err := engine.LoadCatalogue("path/to/rules")
evs := engine.Evaluate(cat, &record, engine.MustDate("2026-08-16"))
```

`record` is the neutral input of DESIGN.md section 4 (`engine.Record`, authoritative
definition `schema/record.schema.json`); each `Evaluation` has the shape of section 4.
Absent optional input is unknown, never zero: a requirement whose metric is unknown for every
flight in its window is `tracked: false` and never met. `Evaluate` drops a rule's evaluation
of a subject when a rule that `supersedes` it evaluated the same subject.

## Adding a rule

Follow [docs/porting-guide.md](docs/porting-guide.md). In short:

1. Put the verbatim article in `sources/` if it is not there yet, with its `Origin` and
   `Attribution` header lines.
2. Write `catalogue/<authority>/<instrument>/<id>.yaml` using only vocabulary names; quote the
   source verbatim; add `divergences` for every deliberate difference from the API and one
   line in `CHANGELOG.md` ending in `<id>#<divergence>`.
3. Write cases under `cases/<id>/` until the gate shows no missing tags.
4. Set `maps_to` in `inventory/code-rules.yaml` and `inventory/articles-*.yaml`.
5. `go generate ./... && go test ./... && go run ./cmd/rulescheck -report -rule <id>`.

## Running everything

```bash
cd rules
go vet ./...
go test ./...                               # engine, generated per-rule case tests, rulescheck
go generate ./... && git diff --exit-code   # generated code is up to date
go run ./cmd/rulescheck -strict             # the CI gate (exit 1 on any problem)
go run ./cmd/rulescheck -report             # same report, always exits 0
go run ./cmd/rulescheck -report -rule faa.14cfr61.61-57-c.instrument
```

`-strict` fails on schema errors; rule and case errors; missing coverage tags; overlapping
rules without `supersedes` or a shared `group`; CHANGELOG lines missing, stale or
duplicated; vocabulary entries unused (unless `reserved_for`) or not implemented; source
files without an allowed origin, attribution or host, or naming material that is never
stored; unmapped `inventory/code-rules.yaml` entries (except `scope: consumer`); articles
without a catalogue entry; and engine statement coverage below 95 %.

## Design choices

Where DESIGN.md leaves room, this module decided:

- **Name checking lives in `rulescheck`, not in the schemas.** The schemas check structure;
  `rulescheck` checks every metric, filter, window, condition, subject, unit and key against
  `vocabulary.yaml` and `messages/keys.yaml`, with messages that name the rule and field.
  Adding a vocabulary entry therefore never touches the schemas, except for new rule fields.
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
  `pilotFlying`, `tailwheel`, `mtomKg`, `engines`, `towKind`, `fullStopLandings`,
  `mountainLandings`, `checkRating`).
- **Requirement rows** may carry `lastDate` and a `messageKey` besides the fields of section 4.
- **Subjects** include `launch_method` (SFCL.155) besides the kinds of section 4; ratings and
  passengers can be split by `typeRated`.
- **Stage conditions** include `holds` (what the holder holds, e.g. a valid IR),
  `undetermined` (the tree is neither met nor definitely unmet because input is missing) and
  `valid_until_within`.

## Licence

The catalogue, engine and tools are licensed under the Apache License 2.0 ([LICENSE](LICENSE)).
The texts under `sources/` are not relicensed: each keeps its own status (US federal works
in the public domain, German official works, EU legal texts reused under Commission Decision
2011/833/EU with attribution), as [NOTICE](NOTICE) and [sources/README.md](sources/README.md)
describe.
