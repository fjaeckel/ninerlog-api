# 0001. Pilot currency rules as an audited catalogue with a closed vocabulary

- Status: Accepted
- Date: 2026-09-27
- Deciders: Frederic Jung
- Contract: [DESIGN.md](../../DESIGN.md) (sections 11 to 13 record the decisions taken while
  building it)

## Context

NinerLog tells pilots whether they may exercise a licence, rating or privilege on a date:
recency, revalidation, validity, passenger carriage, medical validity. Until now these rules
were hand-written Go in `ninerlog-api/internal/service/currency`, one evaluator per rule
family, with the regulation text living only in comments and in the heads of the people who
wrote them.

An audit of that code against the regulations (the inventory in `inventory/code-rules.yaml`
lists the 51 rules the API evaluates, most with a suspected divergence) found that
the rules had drifted from the articles they implement, and that nothing would have caught
it:

- the same concept was implemented differently per family: ratings and medicals expired on
  their expiry date, privileges and the flight review the day after; authorities were
  matched case-sensitively, so a licence entered as "easa" fell back to expiry-only tracking;
- requirements were simplified away: landings counted where the article says take-offs and
  landings (10 rules), any landing where it says full stop or sole manipulator or pilot
  flying, launches of any method where the article names one;
- rules applied to licences they do not govern: Part-FCL and Part-SFCL rules also evaluated
  ratings on DULV and DAeC ultralight licences (18 rules);
- whole routes of an article were missing (proficiency checks that revalidate on their own,
  FI(S) or IR credit, refresher routes), and some articles in scope were not evaluated at
  all;
- missing input was treated as zero, so a pilot whose logbook did not record something could
  be shown current.

The catalogue now records 180 such differences, each as a named divergence with the
article's wording and a pilot-readable CHANGELOG line. A second consumer is coming: the iOS
app must evaluate currency offline, and it must reach the same result as the API.

## Decision

1. **An independent Go module** (`github.com/fjaeckel/ninerlog-rules`) inside `rules/`, with
   no dependency on the API. The API will depend on it, never the reverse; it can be split
   out at any time.
2. **A YAML catalogue**, one file per rule, that quotes the article verbatim from an
   official consolidated text stored under `sources/` (the gate checks every quote), states
   what it applies to, its windows, requirements and stages, and lists every deliberate
   divergence from the API.
3. **A closed vocabulary** (`vocabulary.yaml`) of metrics, filters, windows, combinators,
   stage conditions, subjects and units. There is no expression language: a rule can only
   say what the vocabulary lets it say, so every rule is reviewable by someone who reads
   regulations rather than code, and every construct has one implementation to test.
   Adding to the vocabulary is a reviewed change with its own case.
4. **Golden cases**: each rule has cases (a record, a date, the expected evaluations) that
   are the executable specification, shared by every implementation.
5. **Generated code** for what is mechanical: constants for rule ids, keys and metrics, the
   metric dispatch the engine must implement (a missing metric does not compile), and one
   test per rule that runs its cases.
6. **Hand-written evaluators per language**: the Go engine now, a Swift engine for the iOS
   app later, each interpreting the same catalogue and passing the same cases.
7. **A coverage gate** (`rulescheck -strict`, in CI) that derives from each rule the tags its
   cases must exercise (every stage, every requirement met and unmet, every alternative on
   its own, both window edges, every unknown input, the expiry day) and fails on anything
   missing; that fails when two rules could evaluate the same subject (no double
   evaluation); when an article in scope or a rule the API evaluates has no catalogue entry;
   when a vocabulary entry is unused; when a stored source is not of an allowed origin; and
   when engine statement coverage is below 95 %.
8. **Date-only semantics**: windows are calendar dates, both ends inclusive, and anything
   with an expiry date is valid through that date and expired from the next day, uniformly.
9. **Unknown is not zero**: absent optional input makes a requirement untracked, never met;
   the result is `unknown` with a message that says what to record.
10. **Divergences are encoded as the article says**, not copied from the API; each is named
    (`<rule id>#<divergence id>`, stable) and has exactly one CHANGELOG line for pilots.

## Alternatives considered

- **Keep the Go rules and add tests.** Cheapest now, but the rules stay readable only by Go
  programmers, the regulation text stays outside the code, nothing ties a rule to the article
  it implements, and the iOS app would need a second hand port with no shared specification.
  This is how the drift happened.
- **An expression language (e.g. CEL) in the rule files.** Flexible and portable in theory,
  but rules become small programs: reviewers must read code again, the gate cannot derive
  what to test from structure it does not understand, and a CEL implementation with the same
  semantics would be needed in Swift. The flexibility is exactly what let the Go rules drift.
- **Generate the evaluators from the catalogue** (YAML to Go and Swift source). One truth,
  but two code generators to maintain whose output is hard to debug and review, and every
  vocabulary change becomes a generator change in two languages. Generation is kept for the
  mechanical parts only (decision 5).
- **One evaluator compiled to WebAssembly** and embedded in the API and the iOS app. A single
  implementation, but a WebAssembly runtime inside the app, a large binary for Go's runtime
  (or TinyGo's limitations), awkward debugging on device, and no benefit for reviewers. The
  golden cases give the same guarantee (both engines agree) with native code on each side.

## Consequences

- The catalogue, not the Go code, is the reference for what NinerLog says about currency.
  Changing a rule means changing its YAML, its cases and, where the behaviour changes for
  pilots, the CHANGELOG.
- **Adopting it in the API** (a later change, behind the parity harness that lists the named
  divergences) means building the neutral record (`schema/record.schema.json`) from the
  database. New inputs the API must supply for the rules to be decidable instead of
  `unknown`: whether the pilot was sole manipulator (FAA 61.57) or pilot flying (EASA
  FCL.060(b)); full-stop landings and full-stop night landings; take-offs as well as
  landings; cruise time (route sectors) and intercept-and-track; what a tow flight towed and
  whether it was accompanied; the aircraft's engine count and maximum take-off mass
  (FCL.740.H(a)(2), ultralight credit) and landings on designated mountain surfaces
  (FCL.815); type designators on type ratings only (so class and type ratings can be told
  apart, FAA 61.57 per type); non-flight events (proficiency checks, skill and practical
  tests, refresher courses, assessments of competence, endorsements, flight reviews);
  aircraft variants; the holder's date of birth (derived medical validity); and `validFrom`
  dates. The API gains 121 catalogue message keys (`origin: catalogue`) that the frontend
  must translate, and several statuses change meaning for pilots (`lapsed` for rolling
  recency where the API said `expired`; `unknown` where input is missing).
- **The iOS offline engine** implements the same vocabulary in Swift, loads the same
  catalogue (bundled or downloaded with the app's data) and runs the same golden cases in its
  CI; a vocabulary change lands in both engines before a rule may use it.
- Porting and changing rules is slower than editing Go, by design: a rule is done only when
  the gate is green for it. Parallel porting is supported by fragment directories merged in an
  integration step (DESIGN.md 12.8, 13.12).
- The module carries verbatim regulation texts. Only origins that are safe to reuse are
  stored (US federal works, German official works, EU legal acts under Decision 2011/833/EU,
  each with its attribution); AMC/GM, association and ICAO documents never are, and the gate
  enforces it.

## Extraction procedure

When the module moves to its own repository:

```bash
# in ninerlog-api, on main
git subtree split --prefix=rules -b rules-split
git push git@github.com:fjaeckel/ninerlog-rules.git rules-split:main
```

Then, in the new repository: move `.github/workflows/rules.yml` to `.github/workflows/ci.yml`
without the `paths` filters and the `working-directory: rules` default, and tag a release.
In `ninerlog-api`: replace `rules/` by a `require github.com/fjaeckel/ninerlog-rules vX.Y.Z`
(a `replace` directive to a local checkout while both change together), delete `rules/` and
its workflow. No code under `rules/` imports or reads anything outside it, so the split needs
no code change (DESIGN.md section 1).

## Open items

- The DULV and DAeC association rules are encoded from the API's thresholds; their document
  titles and figures need checking against the current association documents by a person
  (marked `TODO: verify`). The documents are cited, never stored.
- The API adoption itself: the record builder, the parity harness over the named divergences,
  translations of the new keys.
- The Swift engine for the iOS app.
- Authority packs (`packs/`: class names, fly/no-fly gates, layout) are specified but empty.
- Declined at integration for now (DESIGN.md 13.5, 13.6): licence kinds for PPL(H), CPL(H)
  and ATPL(H) (MED.A.030 class 1 and class 2 for helicopter licences); a "multi-pilot or
  turbojet" aircraft property for FAA 61.58.
- Keeping `sources/` current: the consolidations were retrieved on 2026-09-27; a refresh
  procedure (and whether raw downloads are kept as provenance, on hold pending a decision)
  is not yet defined.
