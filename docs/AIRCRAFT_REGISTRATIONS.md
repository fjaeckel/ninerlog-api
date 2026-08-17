# Aircraft registration normalisation

NinerLog stores aircraft registrations in the canonical notation of their state
of registry. `deabc`, `DE-ABC` and `D EABC` are all the same aeroplane and are
all stored as `D-EABC`; `N-12345` is stored as `N12345`.

This matters because the registration is the join key. Flights carry
`aircraft_reg` as a denormalised string, and the per-registration statistics and
90-day recency in `GET /aircraft/stats` group flights by it. Two spellings of
one aircraft mean two fleet entries, two rows of statistics, and recency that
counts half the landings.

The rules live in `pkg/registration`. This document covers why the table exists,
how it decides, and how to keep it current.

## Why a table and not a rule

ICAO Annex 7 says a registration is a **nationality mark** followed by a
**registration mark**, and that the two are separated by a hyphen when the
registration mark starts with a letter.

That rule alone is not enough to normalise with:

| Registration | Registration mark | Annex 7 rule says | Reality |
| --- | --- | --- | --- |
| `D-EABC` | `EABC` | hyphen | `D-EABC` ✓ |
| `N12345` | `12345` | no hyphen | `N12345` ✓ |
| `JA8089` | `8089` | no hyphen | `JA8089` ✓ |
| `HL7747` | `7747` | no hyphen | `HL7747` ✓ |
| `B-1234` | `1234` | no hyphen | `B-1234` ✗ |
| `RA-12345` | `12345` | no hyphen | `RA-12345` ✗ |
| `CU-T1234` | `T1234` | hyphen | `CU-T1234` ✓ |

China and Russia hyphenate a numeric registration mark; the United States,
Japan and South Korea do not. Whether a state hyphenates is a fact about the
state, so it is recorded per state in `pkg/registration/prefixes.go` — the
`NoHyphen` field — rather than computed from the string.

The second thing a rule cannot do is find where the nationality mark ends.
`DEABC` could split as `D`+`EABC` or `DE`+`ABC`; only the table knows that `D`
is a mark and `DE` is not.

## How normalisation decides

`registration.Normalize` (`pkg/registration/registration.go`):

1. Uppercase and trim; collapse internal whitespace.
2. Strip every separator — hyphens, spaces, dots, slashes — to get the *bare*
   form. `D-EABC`, `DE ABC` and `deabc` all reduce to `DEABC`.
3. Match the bare form against the table, **longest nationality mark first**,
   and require the remainder to match the shape of registration mark that state
   issues.
4. Re-emit with or without the hyphen according to the matched entry.

Step 3's shape check is what makes longest-first matching safe. Germany issues
four-digit registration marks to gliders, so `D-2345` is a German glider — but
Angola's mark is `D2`. Prefix-only matching would turn `D-2345` into
`D2-345` and file a German glider under Angola. Angola's registration marks are
three letters, `345` is not, so the shape check rejects `D2` and the match falls
through to `D`.

The same check keeps non-registrations out. Values that end up in a
registration field in practice — `SIM`, `FNPT2`, an aircraft type entered in
the wrong column like `B738` or `C172`, `N/A` — match no state's shape and are
**left alone**, uppercased and trimmed but not rewritten. Normalisation never
turns a value it does not understand into a different value.

`prefixes.go` explains, per entry, when a shape pattern is worth spelling out;
most entries do not need one.

## Where normalisation happens

At the service layer, so every write path is covered by one choke point:

| Path | Where |
| --- | --- |
| `POST /aircraft`, `PATCH /aircraft/{id}` | `AircraftService.CreateAircraft` / `UpdateAircraft` |
| `POST /flights`, `PATCH /flights/{id}`, flight sessions | `FlightService.CreateFlight` / `UpdateFlight` |
| CSV / ForeFlight / JSON-backup import | the services above, plus canonical keying in `internal/api/handlers/import.go` so one aircraft spelled two ways in one file yields one fleet entry |
| `POST /flights/recalculate` | the services above, plus a fleet pass — see below |

Two consequences worth knowing:

- **Canonicalising is not renaming.** When `UpdateAircraft` finds that the only
  change is the stored registration's spelling (`DEABC` → `D-EABC`), it repoints
  that aircraft's flights even when `renameFlights` is false. It is the same
  aircraft; leaving the flights behind would strand them in the statistics. A
  genuine rename (`D-EABC` → `D-EFGH`) keeps its existing opt-in behaviour.
- **Signed flights are not touched.** A flight carrying a completed signature is
  locked (`ErrFlightLocked`) and keeps its original spelling, exactly as it keeps
  every other field. Void the signature to normalise it.

## Fixing existing data

`POST /flights/recalculate` is the migration path. It canonicalises the fleet
first, then recalculates every flight — which normalises `aircraft_reg` on the
way through. Its response reports:

- `aircraftNormalized` — fleet entries rewritten.
- `aircraftConflicts` — fleet entries that could **not** be rewritten because
  the canonical spelling is already held by another aircraft in the same fleet.
  That is one aircraft entered twice (`DEABC` and `D-EABC`). Merging them means
  choosing which entry's make, model and notes survive, so both are left in
  place and the count is reported instead of guessing. Resolve by deleting the
  redundant entry, or by `PATCH`ing it with `renameFlights: true` onto the
  surviving registration.

There is deliberately no data migration: normalisation touches user-visible
identifiers, and a user-triggered, reported operation is the honest way to apply
it.

## Keeping the table current

### Sources

1. **ICAO** — the authority for both facts this table records. Annex 7
   Standard 3.3 requires a state to select its nationality mark from the radio
   call-sign series the ITU allocated to it; Annex 7 also carries the hyphen
   convention. ICAO publishes the marks states have actually selected.
   <https://www.icao.int/nationality-marks>
2. **Wikipedia, *List of aircraft registration prefixes*** — the practical
   consolidation, and by far the easiest to diff.
   <https://en.wikipedia.org/wiki/List_of_aircraft_registration_prefixes>

Use (2) to notice a change and (1) to confirm it before editing.

#### What ITU Appendix 42 is and is not

The *Table of International Call Sign Series* (ITU Radio Regulations Appendix
42) is often cited as the source for registration prefixes. It is not — it is a
radiocommunication document and says nothing about aircraft. It is upstream of
ICAO's list only by the reference in Standard 3.3, and it allocates whole
blocks rather than aircraft marks:

| State | ITU call-sign allocation | Nationality mark |
| --- | --- | --- |
| Germany | `DAA`–`DRZ` | `D` |
| United States | `AAA`–`ALZ`, `KAA`–`KZZ`, `NAA`–`NZZ`, `WAA`–`WZZ` | `N` |

Which slice of its block a state uses for aircraft is the state's own choice,
and hyphenation does not appear in Appendix 42 at all. So Appendix 42 is a
cross-check that a mark falls inside its state's allocation — useful for
sanity-checking a surprising entry, useless as a source for what the mark is or
how it is written.
<https://www.itu.int/en/ITU-R/terrestrial/fmd/Pages/call_sign_series.aspx>

### Why vendored and not fetched

The airport database (`internal/airports`) is fetched at runtime and refreshed
on a timer because it changes continuously. Nationality marks do not: a change
means a new state, a state changing its mark, or a territory changing hands —
the order of once every few years. Fetching that at runtime would add a network
dependency, a refresh job, metrics, and an alert rule, and buy nothing. The
table is vendored, and `LastReviewed` at the top of `prefixes.go` records when
it was last checked.

Because nothing about the table is observable at runtime, `GET /admin/config`
reports `registrationPrefixCount` and `registrationPrefixesReviewed` — how many
marks are loaded and when they were last reviewed. That is the only way an
operator can tell whether registrations are being normalised against a current
ICAO list.

### The drift check

```bash
make prefix-check                         # diff the table against Wikipedia's list
make prefix-check SOURCE=<other url>      # or any other upstream
python3 scripts/check-registration-prefixes.py --file saved-list.txt
```

It reports marks present on one side but not the other. It is a **review aid,
not an auto-updater** — it looks for cells shaped like a nationality mark rather
than parsing one upstream's layout, because layouts change more often than the
data does, so it can over-report. Nothing edits the Go table but a human.

`.github/workflows/registration-prefixes.yml` runs it quarterly and on any PR
that touches `pkg/registration/`. A failure is a prompt to review, not a bug.

### Making a change

1. Run the drift check and confirm each difference against ICAO's published
   nationality marks — not ITU Appendix 42, which cannot answer either
   question (see above).
2. Edit `entries` in `pkg/registration/prefixes.go`, keeping it sorted by mark.
   Set `NoHyphen` only for a state that genuinely runs the two marks together.
   Add a `Suffix` pattern only if the new mark shadows a shorter one, or is
   itself one or two characters and would otherwise swallow non-registrations —
   the comment at the top of the file explains when.
3. Update `LastReviewed`.
4. `make test`. `TestEveryMarkIsReachable` will catch a new entry that a longer
   mark shadows into unreachability, and `TestTableIntegrity` catches duplicates
   and malformed rows. Add a case to `TestNormalize` for anything subtle.

Adding a mark only affects registrations that were previously unrecognised, so
it cannot change how an already-normalised registration is stored.
