# ninerlog-rules: design contract

This directory is a **self-contained Go module** that holds pilot currency, recency and
validity rules as audited plain text (YAML), plus an evaluator, a code generator and a
coverage gate. It lives inside `ninerlog-api` for now and must be extractable at any time
with `git subtree split --prefix=rules` and no code changes.

Every contributor, human or agent, follows this document; sections 11 to 13 amend the
earlier ones and win where they disagree. Changing it is a design change:
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

## 13. Integration of the first porting round (2026-09-27)

The four porting families (easa-fcl, easa-sfcl, faa, de-other) were merged into one
catalogue. Where this section and an earlier one disagree, this section wins. Every change
to the contract made at integration, and why:

1. **Coverage of rows NinerLog cannot count (gate rule fixed).** A leaf with an
   `aggregate: none` metric (`not_recorded`) is always `tracked: false`, so the tags
   `requirement:<id>:met` / `:unmet` required by section 7 were unreachable and porters had to
   drop such rows. The gate now requires `requirement:<id>:untracked` for these leaves
   instead (and no `unknown:not_recorded`), observes `:untracked` for every untracked row,
   and derives no `any_of:<node>:<branch>` tag for a branch that is informational or a
   `not_recorded` leaf, since neither can be met on its own. Restored with it: the
   SFCL.115(a)(2) passenger-competence flight (informational row, as the API shows it); the
   DAeC safety or performance training for weight-shift trikes (informational row); and the
   FAA 61.217(a) ground-instructor activity (an untracked alternative, so a ground instructor
   with no recorded activity is `unknown`, never `lapsed`). The FAA porter's 61.56 flight
   review needed no restoration: every (d) and (e) alternative is an event NinerLog can count.
2. **Source kinds.** `source_kind` gains `app_policy` (NinerLog fallbacks with no regulation
   behind them: `source` has a `cite` only, no file, no quote). The four `other.ninerlog.*`
   rules use it instead of borrowing LuftPersV § 9(1). `association` sources (DULV, DAeC)
   are cited without a file, because association documents are never stored in `sources/`;
   the delegating statute (LuftPersV § 45(4)) moved to `related_sources`. The gate limits an
   association quote to 300 characters (copyright hygiene). `regulation` and `national_law`
   sources, and every `related_sources` entry, still need url, file and a verbatim quote
   (schema `storedSource`). `sources/README.md` records the copyright status of each origin.
3. **Missing sources fetched.** `sources/de/luftpersv-42.md` (official XML) and
   `sources/easa/sfcl-130.md` (EUR-Lex consolidation via the Cellar) were added. The
   LuftPersV § 42 training rules now quote § 42(4) and (5) (their hours matched); the SFCL.130
   rule quotes SFCL.130(a)(2) and (b), which showed that TMG instruction counts toward the
   15 hours (new divergence `tmg-instruction`, new rows for the 7 hours and 3 hours dual in
   sailplanes). SFCL.130 and LuftPersV § 42 were added to the article inventories.
4. **Stage condition `valid_until_within: { days: n }`** with param source
   `days_to_valid_until`: true when the evaluation has a validUntil at most n days after
   asOf. The FAA flight review shows `expiring` in its last 90 days again, as the API does
   (the former divergence `no-expiring` is gone).
5. **Type-rated subjects.** `applies_to.typeRated: true|false` selects ratings with or
   without a type designator. For `passengers` with `typeRated: true` there is one subject
   per class and type designator, and the subject's `detail` is the type designator.
   `faa.14cfr61.61-57-a.passengers-type` and `faa.14cfr61.61-57-b.night-passengers-type`
   count takeoffs and landings per type; the class rules take `typeRated: false`. 61.58 stays
   `not_supported`: not every FAA type rating is for a multi-pilot or turbojet aircraft, and
   the record cannot tell which.
6. **Licence kind `LAPL_H`** (alias `LAPL(H)`, no longer a `HELICOPTER` alias). FCL.140.H is
   now a partial rule; FCL.740.H excludes LAPL(H); MED.A.030's LAPL rule covers it. Declined:
   splitting `HELICOPTER` further into PPL(H), CPL(H) and ATPL(H) kinds for MED.A.030 class 1
   and class 2; no rule other than MED.A.030 needs it and the request only asked for LAPL(H).
   The MED.A.030 note says the helicopter licences are not evaluated.
7. **Engine count and maximum take-off mass** are optional *flight* inputs (`flight.engines`,
   existing `flight.mtomKg`) with filters `maxEngines` and `maxMtomKg` (absent = unknown),
   not rating fields as requested: every other aircraft property the vocabulary reads is on
   the flight, an FSTD session carries the data of the type it represents, and a filter keeps
   missing data three-valued, which a boolean `when` on the rating could not. FCL.740.H
   offers the (a)(2)(ii) route (6 hours as PIC and 1 hour refresher training) on flights of
   single-engine types up to 3 175 kg; without the data the result is unknown.
8. **Mountain landings**: optional `flight.mountainLandings` and metric `mountain_landings`.
   FCL.815(d) is now supported (six landings on designated surfaces or a proficiency check in
   24 months).
9. **No double evaluation (new gate check "Overlaps").** Two supported or partial rules fail
   the gate when they can select the same subject: same subject kind and intersecting
   authorities (with the licence kinds they allow; DULV and DAeC licences are always `UL`),
   classes, ultralight kinds, `typeRated`, privilege kinds, credential types, launch methods,
   training programme and effective period. `holds` is not considered (conservative). An
   overlap is resolved by `supersedes: [rule ids]` (the engine's `Evaluate` then drops the
   superseded rule's evaluation of any subject the superseding rule evaluated) or by a shared
   `group: <name>` declared in `vocabulary.yaml` `rule_groups`, whose description states how
   the members combine and their precedence (every group so far: no precedence, each member
   reports its own condition). A group needs two members. Resolutions of the overlaps found:
   the Part-FCL, Part-SFCL and FAA passenger rules form the groups `easa_fcl_passengers`,
   `easa_sfcl_passengers` and `faa_passengers`; the FAA medical durations
   `faa_medical_duration`; the two FAA flight instructor regimes `faa_cfi`;
   `easa.part-fcl.fcl-240-g.gpl` and `easa.part-sfcl.sfcl-160-b.tmg` supersede
   `other.ninerlog.expiry-only` (licences of other authorities); FE_S left
   `other.ninerlog.privilege-expiry`; LAUNCH_METHOD_TRAINED is evaluated by
   `easa.part-sfcl.sfcl-155-a.launch-method-trained` except on FAA, DULV and DAeC licences,
   where `other.ninerlog.privilege-expiry.launch-method` takes it; FCL.710 excludes
   gyroplanes (FCL.240.G(b) covers them); FCL.140.H applies to EASA and LBA licences like
   FCL.740.H. `other.ninerlog.expiry-only.faa-ultralight` overlaps nothing (Part 61 has no
   ultralight rating) and stays.
10. **Unused vocabulary pruned, reservations possible.** Removed because no rule and no article
    in scope needs them: metrics `distance_km`, `landings.day`, `takeoffs.day`,
    `minutes.crossCountry`, `minutes.examiner`, `minutes.instrument`, `minutes.multiPilot`,
    `minutes.night`, `minutes.picus`, `minutes.sic`; filters `excludeLaunchMethods`,
    `minMinutes`; stage condition `untracked` (a `not_recorded` row is always untracked);
    rule event `resets` (engine code removed too); units `flight`, `km`. Section 12.7's
    reservation is `reserved_for: { <section>: { <name>: <article or reason> } }` at the top
    of `vocabulary.yaml`; the gate accepts a reserved unused entry and fails on a
    reservation for an undeclared or used entry. Nothing is reserved today. Re-adding an
    entry follows section 5.
11. **CHANGELOG**: grouped by authority, one line per divergence ending in the stable id
    `<rule id>#<divergence id>`. The gate now also fails on a line naming a divergence that
    does not exist and on a divergence listed on more than one line.
12. **Parallel-work directories** (`docs/key-requests/`, `docs/changelog-fragments/`,
    `docs/mapping/`, `docs/vocab-requests/`) stay, empty between rounds, each with a README
    describing its format and how the integration step merges it.
13. **Only safe material is kept (copyright hygiene, enforced).** `vocabulary.yaml`
    `source_origins` is an allow-list of the origins a file under `sources/` may have:
    `us-federal` (17 U.S.C. § 105: US federal works are not copyrighted), `de-amtliches-werk`
    (§ 5(1) UrhG: statutes and ordinances are not protected) and `eu-legal-act` (Commission
    Decision 2011/833/EU: reuse with the attribution "© European Union,
    https://eur-lex.europa.eu" and without distorting the meaning). Every file declares its
    origin and attribution in its header (retrofitted to all 113 files; the verbatim text
    was not touched), and the gate section "Sources" fails on a file that is not Markdown
    (no raw XML or HTML downloads), has no allowed origin, sits outside its origin's
    directory, lacks the attribution, cites a URL outside the origin's hosts, or names AMC,
    GM, Easy Access Rules, ICAO, DULV, DAeC or an association in its header or a heading.
    The own-words AMC/GM summaries that 19 EASA source headers carried moved to
    `docs/amc-gm-notes.md`, so `sources/` holds only allowed-origin text. Association
    sources should carry no quote; the gate caps one at 300 characters. The catalogue's
    quotes, notes and divergences were scanned for verbatim AMC/GM or association text; none
    was found (the quoted passages in notes are statute text or NinerLog's own API strings).
    NOTICE and `sources/README.md` state the status of each origin; the catalogue, engine and
    tools are Apache-2.0, the texts under `sources/` keep their own status.
14. **Strict gate.** With the above, `go run ./cmd/rulescheck -strict` passes; CI enforces
    it for every change under `rules/`. Open items that need a human (association document
    titles and thresholds, marked `TODO: verify` in the DULV and DAeC rules) do not fail it,
    because those rules are `partial` and their notes say so.
