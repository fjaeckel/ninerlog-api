# ninerlog-rules: design contract

This directory is a **self-contained Go module** that holds pilot currency, recency and
validity rules as audited plain text (YAML), plus an evaluator, a code generator and a
coverage gate. It lives inside `ninerlog-api` for now and must be extractable at any time
with `git subtree split --prefix=rules` and no code changes.

Every contributor, human or agent, follows this document. Changing it is a design change:
say so explicitly in the commit.

## 1. Independence

- `rules/go.mod` declares `module github.com/fjaeckel/ninerlog-rules`. Nothing under `rules/`
  imports `github.com/fjaeckel/ninerlog-api/...`, reads files outside `rules/`, or assumes the
  API's database, models or HTTP types.
- Allowed third-party dependencies: `gopkg.in/yaml.v3`, `github.com/santhosh-tekuri/jsonschema/v6`.
  Anything else needs a reason in `docs/adr/`.
- The API may depend on this module later through a `replace` directive, never the reverse.
- The parent module ignores `rules/` because it has its own `go.mod`. `go test ./...` in the
  API root must stay unaffected.
- CI: `.github/workflows/rules.yml` runs only for changes under `rules/**`: schema
  validation, `go vet`, `go test ./...`, the generator up-to-date check and the coverage gate.

## 2. Layout

```text
rules/
  go.mod, go.sum
  README.md            what this is, how to add a rule, how to run the gate
  DESIGN.md            this contract
  LICENSE              Apache-2.0
  NOTICE               "not legal advice", source attribution
  CHANGELOG.md         written for pilots, one entry per behaviour change
  vocabulary.yaml      single source of truth: metrics, filters, window kinds, statuses,
                       subjects, events, units (the closed vocabulary, section 5)
  messages/keys.yaml   every message, requirement-name, remedy and rule-description key
                       with its params (mirrors ninerlog-api docs/CURRENCY_MESSAGES.md)
  schema/              JSON Schema 2020-12: rule, case, pack, vocabulary, messages
  catalogue/<authority>/<instrument>/<rule-id>.yaml
  cases/<rule-id>/<case-name>.yaml
  sources/<authority>/<article>.md   verbatim article text, URL, retrieval date
  packs/<authority>.yaml             authority packs (class names, fly/no-fly gates, layout)
  inventory/           research inputs: code-rules.yaml, articles-*.yaml (section 8)
  engine/              Go evaluator, package `engine`
  gen/                 generated Go (never edited by hand)
  cmd/rulesgen/        the generator
  cmd/rulescheck/      schema validation + coverage gate (section 7)
  docs/adr/            architecture decision records
```

Authority directory names: `easa`, `faa`, `de` (German national law, e.g. LuftPersV),
`other`. Instrument directory names: `part-fcl`, `part-sfcl`, `part-med`, `14cfr61`,
`luftpersv`, and so on.

## 3. Rule identifiers

`<authority>.<instrument>.<article>[.<qualifier>]`, lower case, dots and hyphens only, e.g.
`easa.part-fcl.fcl-740-a.sep`, `easa.part-fcl.fcl-060-b.passengers`,
`faa.14cfr61.61-57-c.instrument`, `de.luftpersv.45.three-axis`. The file name is the id.

## 4. Input and output (engine boundary)

The engine never sees the API's models. It takes one neutral **record** and an `asOf` date:

```yaml
asOf: 2026-08-16
licences:   [{ id, authority, type, issued, number? }]
ratings:    [{ id, licenceId, class, ulKind?, issued, expires?, notes? }]
privileges: [{ id, licenceId, kind, detail?, issued?, expires? }]
credentials: [{ id, type, issued, expires? }]          # medicals, language, radio
flights:
  - date, class, ulKind?, typeDesignator?, registration?, launchMethod?
    isSimulator, fstdType?
    minutes: { total, pic, dual, spic, picus, sic, dualGiven, examiner, multiPilot,
               night, ifr, actualInstrument, simulatedInstrument, crossCountry }
    takeoffs: { day, night }       landings: { day, night }
    fullStopNightLandings?         # absent = unknown, never 0
    launches, approaches, holds
    interceptAndTrack?             # absent = unknown
    flags: { proficiencyCheck, flightReview, ipc, trainingFlight, towFlight, outlanding,
             instructorOnBoard, examinerOnBoard }
    towedGliders?, distanceKm?
```

**Absent optional data is unknown, not zero.** A requirement whose metric is unknown for
every flight in its window is reported `tracked: false` and never counts as met.

The engine returns one **evaluation** per (rule, subject):

```yaml
ruleId, subject: { kind: rating|licence|privilege|credential|passengers|flight_review|type,
                   id?, class?, ulKind? }
status: current|expiring|expired|lapsed|unknown|not_applicable
messageKey, messageParams?
ruleDescriptionKey
expiresOn?, windowOpensAt?, validUntil?
requirements:
  - { id, nameKey, metric, current, required, unit, met, tracked, validUntil?,
      remedyKey?, remedyParams? }
citations: [ "EASA FCL.740.A(b)(1)" ]
```

Status meanings and message, requirement and remedy keys match
`ninerlog-api/docs/CURRENCY_MESSAGES.md` so the API can adopt the engine without changing
its responses. A new key is added to `messages/keys.yaml` first.

## 5. The closed vocabulary

Rules may only use what `vocabulary.yaml` declares. There is no expression language.

- **Metrics**: named counters over the flights in a window after filtering, e.g.
  `flights`, `minutes.total`, `minutes.pic`, `minutes.picOrDual`, `landings.total`,
  `landings.day`, `landings.night`, `full_stop_night_landings`, `takeoffs_and_landings`,
  `launches`, `approaches`, `holds`, `intercept_and_track`, `training_flights`,
  `longest_training_flight_minutes`, `route_sectors`, `proficiency_checks`, `tows`, …
- **Filters**: `classes`, `ulKinds`, `launchMethods`, `typeDesignators`, `roles` (`pic`,
  `dual`, `spic`, `picus`, `sic`, `instructor`), `simulator` (`exclude|only|include`),
  `flags`, `minMinutes` (per-flight threshold, e.g. "one flight of at least 60 minutes").
- **Windows**: `rolling_days: n`, `rolling_months: n`, `calendar_months: n` (the n calendar
  months preceding the month of `asOf`), `before_expiry_months: n` (anchored to the subject's
  expiry), `since_issue`, `lifetime`.
- **Combinators**: `all_of`, `any_of`, `n_of: { n, of: [...] }`.
- **Stages**: an ordered list evaluated top to bottom; the first whose `when` holds sets
  `status` and `messageKey`. `when` may only reference requirement ids, `expired`,
  `in_window`, `grace` stages and events.
- **Events**: `restored_by` (e.g. `ipc`, `proficiency_check`, `flight_review`), `resets`.
- **Escape hatches**: `escape_hatch: <name>` names logic implemented in Go under
  `engine/hatches/`, each with its own cases. Use one only when the vocabulary cannot express
  the article faithfully; record why in the rule file.

Adding to the vocabulary means: `vocabulary.yaml`, schema, engine, generator output, and at
least one case that exercises it.

## 6. Rule files

```yaml
id: faa.14cfr61.61-57-c.instrument
title: Instrument experience
authority: faa
instrument: 14 CFR Part 61
article: 61.57(c)(1), (d)
support: supported            # supported | partial | not_supported
effective_from: 2018-11-27
effective_to: null
source:
  cite: 14 CFR 61.57(c)(1)
  url: https://www.ecfr.gov/current/title-14/section-61.57
  file: sources/faa/61.57.md  # verbatim text lives here
  quote: >-                   # the exact sentence(s) this rule encodes, verbatim
    ...
applies_to: { subject: rating, classes: [IR], authorities: [faa] }
window: { calendar_months: 6 }
requirements:
  all_of:
    - { id: approaches, metric: approaches, min: 6, nameKey: requirement.approaches, unit: approaches }
    - { id: holds, metric: holds, min: 1, nameKey: requirement.holds, unit: holds }
    - { id: track, metric: intercept_and_track, min: 1, nameKey: requirement.intercept_track, unit: flights }
stages:
  - { when: all_met, status: current, messageKey: rating.ir_current }
  - { when: { met_within: { calendar_months: 12 } }, status: expiring, messageKey: rating.ir_lapsed_safety_pilot }
  - { when: always, status: expired, messageKey: rating.ir_expired_ipc }
restored_by: [{ event: ipc }]
ruleDescriptionKey: faa_ir
notes: >-
  Differences from the API's hand-written rule, if any, with the reason.
```

`support: not_supported` entries still carry `source`, `applies_to` and a `notes` line saying
what is missing. They make the catalogue a complete map of the regulation, not only of the
code.

**Quotes are verbatim.** They come from the official consolidated text (EUR-Lex, eCFR,
gesetze-im-internet.de) and are stored in `sources/`. If a text cannot be fetched, write
`TODO: verify` and never paraphrase from memory.

## 7. Cases and the coverage gate

A case is one record, one `asOf`, and the expected evaluations for one rule:

```yaml
rule: faa.14cfr61.61-57-c.instrument
name: lapsed-within-grace
covers: [stage:expiring, requirement:approaches:unmet, window:edge-out]
asOf: 2026-08-16
record: { ... }
expect:
  - subject: { kind: rating, id: r-ir }
    status: expiring
    messageKey: rating.ir_lapsed_safety_pilot
    requirements: { approaches: { current: 4, met: false }, holds: { met: true } }
```

`cmd/rulescheck` derives the **required coverage tags** from each supported or partial
rule's structure, and fails when a case set misses any:

- `stage:<status>` for every stage;
- `requirement:<id>:met` and `requirement:<id>:unmet` for every requirement;
- `any_of:<id>:<branch>` for each branch satisfied on its own;
- `window:edge-in` and `window:edge-out` (the first day inside and outside the window);
- `event:<name>` for every `restored_by` or `resets` event;
- `unknown:<metric>` for every metric with optional input;
- `expiry:before`, `expiry:on`, `expiry:after` when the subject has an expiry.

The gate also fails when:

1. a key used by a rule is missing from `messages/keys.yaml`;
2. a vocabulary entry is not used by any rule, or used but not implemented by the engine;
3. an entry in `inventory/code-rules.yaml` (rules the API implements today) has no catalogue
   rule mapped to it;
4. an entry in `inventory/articles-*.yaml` (regulation articles in scope) has no catalogue
   entry, supported or not;
5. engine statement coverage from `go test -cover ./engine/...` is below 95 %.

## 8. Inventory inputs

- `inventory/code-rules.yaml`: every rule, branch, status, message key and edge case the API
  evaluates today, with `file:line`, from the gliding/UL branch (the superset). Each entry
  has `id`, `where`, `article`, `subject`, `window`, `requirements`, `stages`, `messages`,
  `edge_cases`, `suspected_divergence` (and why), `maps_to` (the catalogue rule id, filled in
  when ported).
- `inventory/articles-easa.yaml`, `articles-faa.yaml`, `articles-de.yaml`: every article in
  scope (section 9), with `cite`, `url`, `subject`, `summary` (one line, own words),
  `source_file` (the verbatim text in `sources/`), `in_app` (yes, partial or no), and
  `maps_to`.

## 9. Scope of "every article"

Articles that decide whether a pilot may exercise a licence, rating, certificate or
privilege on a date, or carry passengers: recent experience, revalidation and renewal,
validity periods, recency for privileges, flight reviews and proficiency checks, and
medical validity. For EASA: Part-FCL, Part-SFCL, Part-BFCL (balloons; not in the app, so
`not_supported`) and Part-MED validity. For FAA: 14 CFR Part 61, and Part 68 (BasicMed).
For Germany: LuftPersV for ultralights. Training syllabi and skill-test content are out of
scope, apart from the recency and experience they require.

## 10. Working rules for agents

- Work only inside `rules/` (and `.github/workflows/rules.yml` when told). Never change API
  code outside `rules/` unless your task says so.
- Do not commit, push, or create branches. The coordinator commits.
- Run `go test ./...` inside `rules/` before you finish, and report its output.
- Report: files written, anything that did not fit this contract, and open questions.

## 11. Decisions (2026-09-27, open to review)

1. **Dates, not instants.** Every window works on calendar dates. `rolling_days: n` covers
   `asOf - n days` through `asOf`, both inclusive; `rolling_months` and
   `before_expiry_months` likewise include their first day. No clock time is involved.
2. **Valid through the expiry date.** A rating, certificate, privilege or medical with
   expiry date D is valid on D and expired from D + 1, uniformly. (Today the API treats
   ratings and medicals as expired on D but privileges and the flight review as valid on D.)
3. **Authorities match case-insensitively** after trimming (`easa`, `EASA`, ` Easa `).
4. **Rule sources have a kind**: `regulation` (EU, CFR), `national_law` (LuftPersV),
   `association` (DULV, DAeC rules with no statute behind them). Association rules live
   under `catalogue/de/dulv/` and `catalogue/de/daec/` and cite the association document.
5. **Scope of the gate.** Regulation-derived rules, including training-programme
   requirements (subject `training`), are in the catalogue and the coverage gate. API
   consumers that compose rules (readiness, the night privilege flag, the UL night save
   warning) stay in the API; their inventory entries are marked `scope: consumer` and are
   excluded from gate check 3.
6. **Divergences are fixed in the catalogue, not copied.** The catalogue encodes the
   article as written. Each deliberate difference from today's API behaviour is a
   `divergence` entry in the rule file (what the API does, what the article says, citation)
   and a CHANGELOG line. The parity harness lists them by name.

## 12. Amendments after the foundation (2026-09-27)

The foundation build showed where sections 4–7 were too thin or wrong. Where this section
and an earlier one disagree, this section wins.

1. **Record (section 4)** also has: `holder.dateOfBirth` (optional), non-flight `events`
   (proficiency checks, skill tests, seminars, assessments of competence), aircraft
   `variants`, `validFrom`, and several optional flight fields listed in
   `schema/record.schema.json`, which is the authoritative definition.
2. **Requirement rows carry `messageKey`**, as the API sends today. A `launch_method`
   subject exists.
3. **Stage conditions (section 5)** also include `holds` (a named condition such as the IR
   night waiver, the SFCL.160(c) exemption, the §84a authorisation) and `undetermined` (a
   required input is unknown). A missing optional input is three-valued: never met.
4. **Windows:** `calendar_months: n` covers the n calendar months before the month of
   `asOf` plus the current month to date. `rolling_months` snaps to the end of the month.
5. **FAA 61.57(c) grace:** instrument currency lapses six calendar months after the
   requirement was last met; the grace runs from that date, not over a 12-month sum. The
   example in section 6 is superseded by the reference rule file.
6. **Coverage tags (section 7):** `stage:<stage-id>` rather than status; per-window
   `window:<id>:edge-in|edge-out` for rules with several windows; `unknown:` tags for
   filters that read optional inputs, not only metrics. Tags are derived from what each
   case exercises, and a case's `covers:` must match.
7. **Unused vocabulary** entries left after porting are pruned or listed with a reason in
   `vocabulary.yaml` (`reserved_for: <article>`); the gate accepts reserved entries.
8. **Parallel porting.** Porting agents do not edit shared files. New message keys go to
   `docs/key-requests/<family>.yaml` and changelog lines to
   `docs/changelog-fragments/<family>.md`; the integration step merges them into
   `messages/keys.yaml` and `CHANGELOG.md`. Until then, key and changelog findings for a
   family's rules are expected in `rulescheck -report`.
9. `effective_from` is set only when the source file states it; otherwise null.
