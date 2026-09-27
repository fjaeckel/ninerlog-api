# Porting guide

The recipe for adding a catalogue rule, whether it ports a rule the API evaluates today
(`inventory/code-rules.yaml`) or maps an article in scope (`inventory/articles-*.yaml`).
DESIGN.md is the contract; this guide is how to satisfy it. The two reference rules are the
templates to copy:

- `catalogue/easa/part-fcl/easa.part-fcl.fcl-740-a.sep.yaml` (expiry-anchored, any_of with a
  nested any_of, class pools, ultralight credit, six stages) and its 17 cases;
- `catalogue/faa/14cfr61/faa.14cfr61.61-57-c.instrument.yaml` (calendar months, an optional
  input, a grace stage, a restoring event) and its 10 cases.

## What you may edit

| You edit | You never edit |
| --- | --- |
| `catalogue/**`, `cases/**` for your family | `engine/`, `schema/`, `vocabulary.yaml`, `cmd/` |
| `docs/mapping/<family>.yaml` (your `maps_to`) | `gen/` (generated) |
| `docs/changelog-fragments/<family>.md` | `CHANGELOG.md`, `inventory/*.yaml` |
| `docs/key-requests/<family>.yaml` (new keys) | `messages/keys.yaml` |
| `docs/vocab-requests/<family>.md` | `DESIGN.md`, `sources/` |

Several agents work in parallel, so porting agents never edit the shared files; the
integration step merges the fragments (DESIGN.md 12.8, 13.12; each `docs/` directory has a
README with the format). A single contributor working alone may edit `CHANGELOG.md`,
`messages/keys.yaml` and `inventory/*.yaml` directly; the gate checks the result the same
way.

## 1. Pick the id and the file

`<authority>.<instrument>.<article>[.<qualifier>]`, lower case, dots and hyphens only.
The file name is the id; the directory is `catalogue/<authority>/<instrument>/`.

| Source | authority | instrument dir | Example id |
| --- | --- | --- | --- |
| Part-FCL, Part-MED | `easa` | `part-fcl`, `part-med` | `easa.part-fcl.fcl-140-a.lapl-a` |
| Part-SFCL, Part-BFCL | `easa` | `part-sfcl`, `part-bfcl` | `easa.part-sfcl.sfcl-160-b.tmg` |
| 14 CFR 61, 68 | `faa` | `14cfr61`, `14cfr68` | `faa.14cfr61.61-56.flight-review` |
| LuftPersV, LuftVZO | `de` | `luftpersv`, `luftvzo` | `de.luftpersv.45-2.three-axis` |
| DULV / DAeC rules | `de` | `dulv`, `daec` | `de.dulv.ul-towing` |
| NinerLog-only | `other` | `ninerlog` | `other.ninerlog.expiry-only` |

Article segment: the article number with dots and parentheses turned into hyphens
(`FCL.740.A(b)(1)` becomes `fcl-740-a`, `61.57(c)` becomes `61-57-c`). Add a qualifier when
one article yields several rules (`.sep`, `.bungee`, `.night`).

`authority` must equal the first id segment; `source_kind` is `regulation`,
`national_law`, `association` or `app_policy` (DESIGN.md decision 4 and 13.2):

| source_kind | `source` | stored under `sources/` |
| --- | --- | --- |
| `regulation`, `national_law` | `cite`, `url`, `file`, verbatim `quote` | yes, with an allowed origin (13.13) |
| `association` (DULV, DAeC) | `cite` naming the document; no `file`; preferably no `quote` (at most 300 characters) | never; the delegating statute goes under `related_sources` |
| `app_policy` (NinerLog fallbacks) | `cite` naming the policy only | never |

## 2. Quote the source verbatim

`source.quote` (a string or a list) must appear in `source.file` after collapsing
whitespace and dropping Markdown `*`. Copy the sentences with your editor from
`sources/<authority>/<article>.md`; never type them from memory. Leave out the italic
paragraph headings rather than retyping them. Quote every sentence the rule encodes, and
only those. Other provisions the rule relies on go in `related_sources` (same shape, also
checked). If the text is not in `sources/`, write `quote: "TODO: verify"` and say so in
`notes`; the gate accepts it but the rule is not done.

## 3. Write the rule

Every name must be declared in `vocabulary.yaml`. Skeleton:

```yaml
id: easa.part-sfcl.sfcl-160-a.sailplane
title: SPL recency, sailplanes
authority: easa
instrument: Part-SFCL
article: SFCL.160(a)
support: supported            # supported | partial | not_supported
source_kind: regulation
effective_from: null
effective_to: null
source: { cite: EASA SFCL.160(a), url: ..., file: sources/easa/sfcl-160.md, quote: [...] }
citations: [EASA SFCL.160(a)(1), EASA SFCL.160(a)(2)]   # optional; default [source.cite]
applies_to: { subject: rating, classes: [GLIDER], authorities: [EASA, LBA] }
window: { rolling_months: 24 }
filter: { ... }               # optional, inherited by every requirement
requirements: { ... }
stages: [ ... ]
restored_by: [ ... ]          # optional
ruleDescriptionKey: easa_spl
divergences: [ ... ]
notes: >-
  Why partial, what is approximated, which reading of an ambiguous sentence was taken.
```

### Subjects and `applies_to`

| subject | one evaluation per | selected by |
| --- | --- | --- |
| `rating` | rating | `classes`, `excludeClasses`, `ulKinds` (`none` = no kind), `typeRated` (the rating has a type designator, or not), licence `authorities`, `excludeAuthorities`, `licenceKinds`, `excludeLicenceKinds` |
| `passengers` | class (and ultralight kind) per authority; first licence wins; with `typeRated: true` one per class and type designator (subject `detail` = the designator) | as `rating` |
| `licence` | licence | `authorities`, `licenceKinds` |
| `privilege` | privilege | `privilegeKinds`, licence filters |
| `credential` | certificate | `credentialTypes` |
| `flight_review` | holder (first matching licence) | licence filters |
| `training` | holder, `programme: <name>` | licence filters optional |
| `type` | variant in `record.variants` | rating filters |
| `launch_method` | method on a rating: trained privilege or ever logged | rating filters, `launchMethods` |

`applies_to.holds` restricts to holders who hold something (see stage conditions).
Authorities match case-insensitively. Licence kinds are classified from the licence type by
the aliases in `vocabulary.yaml` (`licence_kinds`).

### Windows

Date-only, both ends inclusive; nothing after `asOf` ever counts.

| window | covers on asOf D | moving (gets validUntil) |
| --- | --- | --- |
| `rolling_days: n` | D - n days .. D | yes |
| `rolling_months: n` | D - n months (end-of-month clamped) .. D | yes |
| `calendar_months: n` | 1st day of the nth month before D's month .. D | yes |
| `before_expiry_months: n` | expiry - n months .. min(expiry, D) | no |
| `validity_period: true` | validFrom (else issued) .. min(expiry, D) | no |
| `since_issue: licence` / `subject` | issue date .. D | no |
| `lifetime: true` | everything .. D | no |

"Last 2 years" is `rolling_months: 24`. "Within the n calendar months preceding the month"
is `calendar_months: n`. A requirement without a window inherits its parent's, then the
rule's.

### Requirements

A leaf:

```yaml
- id: landings                 # unique in the rule; lower_snake
  metric: landings.total
  min: 12
  unit: landings               # must be one of the metric's units
  nameKey: requirement.landings
  remedyKey: remedy.fly_more   # optional; params missing/unit/method are filled in
  window: { ... }              # optional override
  filter: { ... }              # merged over inherited filters, key by key
  when: { holds: { ... } }     # optional: row exists only when this holds
  informational: true          # optional: shown, never affects status
  messages: { met: ..., unmet: ..., untracked: ... }   # optional per-row messageKey
```

Combinators: `all_of: [...]`, `any_of: [...]` (needs `id`, every branch needs `id`),
`n_of: { n: 2, of: [...] }` (needs `id`). A combinator may carry `window`, `filter`, `when`.

Filters merge key by key: a child's `classes` replaces the parent's, other keys are kept. A
child cannot remove an inherited key, so put filters that not every leaf shares on the
leaves (YAML anchors `&name` / `*name` are fine, see the SEP rule).

`$subject` in `classes`, `ulKinds`, `launchMethods`, `typeDesignators` or `variants`
stands for the subject's class, ultralight kind (or privilege detail), launch method, type
designator or variant.

Useful patterns:

| Regulation says | Encode as |
| --- | --- |
| "12 take-offs and 12 landings" | two leaves: `takeoffs.total` and `landings.total` |
| "3 take-offs and 3 landings" reported as one row | `takeoffs_and_landings` (the smaller sum) |
| "as PIC or flying dual or solo under supervision" | `minutes.picOrDualOrSpic` |
| "as PIC" (count flights) | `metric: flights`, `filter: { roles: [pic] }` |
| "a training flight of at least 1 hour with an instructor" | `longest_training_flight_minutes`, `min: 60` |
| "refresher training of at least 1 hour" (may be several flights) | `minutes.dual`, `min: 60` |
| "a proficiency check with an examiner" | `metric: events`, `filter: { eventKinds: [proficiency_check], classes: [$subject] }` |
| "an IR proficiency check" | add `eventRatings: [IR]` |
| "launches by winch" | `launches` with `launchMethods: [winch]` |
| "tows while accompanied" | `tows` with `flags: { accompanied: true }` |
| "10 route sectors" | `route_sectors` (FCL.010: cruise of at least 15 min) |
| "1 route sector with an examiner" | `route_sectors` with `flags: { examinerOnBoard: true }` |
| "not flown that variant within 2 years" | subject `type`, `flights` with `variants: [$subject]`, `rolling_months: 24` |
| Annex I (ultralight) hours credited to a class | `ulCredit: [{ class: SEP_LAND, ulKinds: [THREE_AXIS] }]` (add `minMtomKg`) |
| "when holding both SEP(land) and TMG, either class counts" | `classes: [$subject]`, `heldClassPools: [[SEP_LAND, TMG]]` |
| "at night, full stop" | `takeoffs.night` + `full_stop_night_landings` |
| "sole manipulator", "pilot flying", tailwheel | `soleManipulator: true`, `pilotFlying: true`, `tailwheel: true` |
| "in an FFS" allowed | `simulator: include` (and `fstdTypes: [FFS]`) |
| "single-engine types up to 3 175 kg" | `maxEngines: 1, maxMtomKg: 3175` (flight fields `engines`, `mtomKg`; absent is unknown) |
| "landings on a surface designated to require a mountain rating" | `mountain_landings` |
| "in an aircraft of the same type (if a type rating is required)" | two rules, `typeRated: false` (per class) and `typeRated: true` with `typeDesignators: [$subject]` (per type) |
| something NinerLog cannot count | `metric: not_recorded` (always `tracked: false`, `messages: { untracked: requirement.untracked }`); as a required row the tree can never be met, so it is usually `informational: true` or one alternative of an `any_of` (then "not recorded" stays unknown, never lapsed) |
| "2 of: 50 h instruction, refresher, assessment" | `n_of: { n: 2, of: [...] }` |
| a Go-only computation | `escape_hatch: <name>` on a leaf (declared in `vocabulary.yaml`, implemented in `engine/hatches`); explain in `notes` |

Unknown is not zero. Optional record fields (`vocabulary.yaml` `record_fields` with
`optional: true`) make items unknown when absent; filters with `absent: unknown` do too. A
leaf whose items are all unknown is `tracked: false` and never met; a tree that is neither
met nor definitely unmet is `undetermined`.

### Validity (derived expiry)

For certificates whose validity the regulation fixes (medicals, language proficiency,
instructor certificates):

```yaml
validity:
  from: issued                 # issued | valid_from (validFrom, else issued)
  periods:                     # first match wins; conditions use age at the anchor date
    - { when: { age_under: 40 }, months: 60, cap_at_age: 42 }
    - { when: { age_at_least: 40, age_under: 50 }, months: 24, cap_at_age: 51 }
    - { months: 12 }
    # end_of_month: true for "the end of the last day of the nth month after the month"
```

The effective expiry is the earlier of the derived and the recorded one. Without a date of
birth an age-dependent expiry is unknown: put a `{ missing: date_of_birth }` stage first.

### Stages

Top to bottom; the first whose `when` holds sets `status` and `messageKey`. The last stage
must be `when: always`. Give stages `id`s when two share a status (coverage tags name them).

| condition | true when |
| --- | --- |
| `always` | always |
| `all_met` | the tree is met (or restored by a `restored_by` event) |
| `undetermined` | missing input: neither met nor definitely unmet |
| `{ met: id }`, `{ unmet: id }` | that requirement row is met / tracked and unmet |
| `expired` | the subject's expiry is before asOf (valid through the expiry date) |
| `no_expiry` | no recorded or derivable expiry |
| `{ expires_within: { days: n } }` | not expired and at most n days left |
| `{ valid_until_within: { days: n } }` | the evaluation's validUntil (moving window, met on asOf) is at most n days away |
| `before_window` | the anchored rule window opens after asOf |
| `{ met_within: { calendar_months: n } }` | the tree was last met at most n months ago (also `rolling_months`, `rolling_days`) |
| `{ holds: { classes \| licenceKinds \| privileges \| credentials: [...], ulKinds, sameLicence, valid, every } }` | the holder holds it |
| `{ missing: date_of_birth }` | the input is absent |
| `{ all: [...] }`, `{ any: [...] }`, `{ not: ... }` | combinations |

Statuses: `current`, `expiring`, `expired`, `lapsed`, `unknown`, `not_applicable`, as in
`docs/CURRENCY_MESSAGES.md`. Params: `{ days: days_to_expiry }`, `{ date: expiry_date }`,
`{ date: window_opens_at }`, `{ date: valid_until }`, `{ days: days_to_valid_until }`,
`{ needed: { needed: <req id> } }`,
`{ date: { last_date: <req id> } }`. Every non-optional param of the key must be supplied.

`restored_by: [{ event: ipc, filter: {...} }]`: from the event date until the event would
leave the rule's (moving) window the tree counts as met.

### One subject, one rule (overlaps)

The gate fails when two supported or partial rules can select the same subject (same
subject kind with intersecting authorities, licence kinds, classes, ultralight kinds,
`typeRated`, privilege kinds, credential types, launch methods, programme and effective
period). Resolve it by narrowing `applies_to`, by `supersedes: [<rule id>]` on the more
specific rule (the engine then drops the superseded rule's evaluation of that subject), or,
when both rules state different conditions for the same subject, by a shared
`group: <name>` declared in `vocabulary.yaml` `rule_groups` with a description of how the
members combine.

### Message keys

Use keys from `messages/keys.yaml`; keep the API's keys where the API emits one for the same
statement (parity). Deprecated keys are rejected. If you need a key that is not there,
request it in `docs/key-requests/<family>.yaml` with `origin: catalogue`, `emitted: false`,
`documented: false` and a `notes` line; never rename or remove one.

### Divergences and the CHANGELOG

Encode the article, not the API (decision 6). For every deliberate difference from the
API's behaviour (the inventory's `suspected_divergence` entries are a starting list, to be
checked against the source), add:

```yaml
divergences:
  - id: takeoffs                # lower-kebab, unique in the rule
    api: Counts landings only.
    article: The experience route needs 12 take-offs and 12 landings.
    cite: EASA FCL.740.A(b)(1)(ii)(B)
```

and exactly one pilot-readable line (a fragment in `docs/changelog-fragments/<family>.md`,
merged into `CHANGELOG.md` under Unreleased) that ends in `` `<rule id>#<divergence id>` ``.
The gate fails without it, on a line naming a divergence that does not exist, and on a
divergence listed twice. Divergence ids are stable: never rename one once released.

### not_supported entries

Every article in scope gets an entry, even when the app has no data for it:

```yaml
id: easa.part-bfcl.bfcl-160.recency
title: BPL recency
authority: easa
instrument: Part-BFCL
article: BFCL.160
support: not_supported
source_kind: regulation
source: { cite: EASA BFCL.160, url: ..., file: sources/easa/bfcl-160.md, quote: [...] }
applies_to: { subject: licence, licenceKinds: [BPL] }
notes: Balloons are not in the app: no balloon class, no balloon flight data.
```

No requirements, stages or cases are needed; the gate checks the quote and the names.

## 4. Map the inventories

- `inventory/code-rules.yaml`: set `maps_to: <rule id>` (or a list) on every entry your rule
  replaces. Entries marked `scope: consumer` stay in the API and are not mapped.
- `inventory/articles-*.yaml`: add your rule id to the article's `maps_to` list. One article
  often maps to several rules; append, do not replace.

## 5. Write the cases

One file per case in `cases/<rule id>/<name>.yaml`; `name` equals the file name.

```yaml
rule: faa.14cfr61.61-57-c.instrument
name: window-day-before
description: What the case shows, in one sentence.
covers: [window:edge-out, stage:grace]      # documentation; checked against the evaluation
asOf: 2026-08-16
record:
  licences: [{ id: l-faa, authority: FAA, type: PRIVATE }]
  ratings: [{ id: r-ir, licenceId: l-faa, class: IR }]
  flights:
    - { date: 2026-01-31, class: SEP_LAND, minutes: { total: 120, pic: 120 }, approaches: 6, holds: 1, interceptAndTrack: true }
expect:
  - subject: { kind: rating, id: r-ir }
    status: expiring
    messageKey: rating.ir_lapsed_safety_pilot
    requirements:
      approaches: { current: 0, required: 6, met: false, remedyKey: remedy.fly_more, remedyParams: { missing: 6, unit: approaches } }
```

Only the fields you write are compared. `expect` must list every subject the rule
evaluates (an unexpected evaluation is a failure); `expect: []` asserts the rule evaluates
nothing, which is how you test `applies_to`. A requirement row can be asserted
`{ absent: true }`. Record fields you omit are zero, except optional fields, which are
unknown.

### Coverage tags

The gate derives the required tags from the rule and observes the tags each case actually
exercises; your `covers:` list must be a subset of what is observed.

| tag | required for | a case hits it when |
| --- | --- | --- |
| `stage:<id or status>` | every stage | that stage decides the status |
| `requirement:<id>:met` / `:unmet` | every leaf except `not_recorded` | the row is met / tracked and not met |
| `requirement:<id>:untracked` | every `not_recorded` leaf (instead of met/unmet) | the row is shown untracked |
| `any_of:<node>:<branch>` | every any_of branch that can be met (not informational, not `not_recorded`) | that branch is met and no sibling is |
| `n_of:<node>:met` / `:unmet` | every n_of | the n_of is met / not met |
| `window:edge-in` / `window:edge-out` | every bounded window (one window) | an item that passes the filters is dated on the window's first day / the day before |
| `window:<kind>=<n>:edge-in` / `:edge-out` | when a rule uses several windows | as above, per window, e.g. `window:before_expiry_months=3:edge-in` |
| `event:<kind>` | every `restored_by` event | the event restores the tree on asOf |
| `unknown:<metric>` | every leaf metric with optional input | a leaf with that metric ends `tracked: false` |
| `unknown:<filter>` | every filter with `absent: unknown` that a leaf uses | an item in a leaf's window is unknown because of that filter |
| `expiry:before` / `expiry:on` / `expiry:after` | rules that use the expiry (anchored windows, expiry conditions, validity) | asOf is before / on / after the expiry date |

Examples from the reference rules:

- `stage:met_expiring`: `cases/easa.part-fcl.fcl-740-a.sep/experience-met-expiring.yaml`
- `any_of:refresher:refresher_exemption` and `window:before_expiry_months=3:edge-out`:
  `proficiency-check-too-early.yaml`
- `window:before_expiry_months=12:edge-in` and `:edge-out`: `experience-window-edges.yaml`
- `expiry:on`: `valid-on-expiry-day.yaml`; `expiry:after`: `expired-day-after.yaml`
- `unknown:ulCredit`: `ultralight-credit.yaml` (an ultralight flight with no kind)
- `event:ipc`: `cases/faa.14cfr61.61-57-c.instrument/ipc-restores.yaml`
- `unknown:intercept_and_track`, `stage:tasks_not_logged`: `tracking-not-logged.yaml`
- `window:edge-in` / `window:edge-out`: `window-first-day.yaml`, `window-day-before.yaml`

Beyond the tags, write the cases a pilot would ask about: the boundary day, the day after,
each alternative on its own, what does not count (another class, a simulator, a kindless
ultralight), and `expect: []` for subjects the rule must not evaluate.

## 6. Run the gate for your rule

```bash
cd rules
go generate ./...                                    # adds gen/rule_<id>_test.go
go test ./gen/ -run 'TestRule_<id with _ for . and ->'
go run ./cmd/rulescheck -report -rule <rule id>      # schema, names, quote, cases, missing tags
go run ./cmd/rulescheck -report                      # whole report: inventory and article progress
```

A rule is done when `-rule <id>` prints `OK`: no errors, no case failures, no missing tags.
Before you finish, run `go vet ./... && go test ./...` and `go run ./cmd/rulescheck -report`.

## 7. When the vocabulary is not enough

Do not edit `vocabulary.yaml`, the schemas or the engine. First try harder: most "missing"
concepts are a filter on an existing metric, an `any` filter, a nested combinator, a
`holds` condition or an event kind. If it truly cannot be expressed:

1. Encode what can be expressed, set `support: partial`, and say in `notes` what is missing.
2. Append a request to `docs/vocab-requests/<family>.md`:

```markdown
## <rule id>: <short name of the missing entry>

- Article text (verbatim, with cite):
- What the rule needs to say:
- Proposed vocabulary entry (section, name, semantics, record input it reads):
- Why existing entries do not work:
- Cases that would exercise it:
```

The coordinator batches requests into a vocabulary change (vocabulary, schema, engine,
generator output and a case, as DESIGN.md section 5 requires).
