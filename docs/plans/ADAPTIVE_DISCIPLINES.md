# Plan: adaptive disciplines ("toolkits")

**Status: proposed, not implemented.** This is a forward-looking plan. Where it describes
today's code, it cites `file:line` as audited on 2026-09-26; where it describes behaviour,
that behaviour does not exist yet. The binding acceptance criteria are the personas in
[../PERSONAS.md](../PERSONAS.md).

## Problem

NinerLog shows every pilot every feature. A club glider pilot sees off-block/on-block, IFR
approaches, multi-pilot, PICUS, examiner and relief time on every flight
(`ninerlog-frontend/src/components/flights/FlightForm.tsx:1097-1388`). An airline pilot sees
trikes and powered paragliders in every class picker. Nothing in the frontend reads the
pilot's licences or ratings. The only field that adapts is the launch method
(`FlightForm.tsx:442-447`), and it adapts to the selected aircraft using a substring match on
"glider".

Features only matter to the pilots who hold the rating, fly the aircraft, or are training
toward it. The app should know which pilot it is talking to.

## The idea

The server derives a **pilot profile**: for each discipline, the *evidence* (licences,
ratings, aircraft, recent flights), the pilot's *intent* (automatic, on, off, or training
toward it), and the resulting *status*. The frontend asks one question per element: "does
this serve an active discipline, or does the record already hold data for it?"

To the pilot, each discipline is a **toolkit**: "Glider toolkit", "Ultralight toolkit",
"IFR toolkit". Toolkits turn on by themselves when evidence appears, with a one-time
explanation that can be dismissed ("Glider toolkit turned on because you logged D-1234.
Keep it / Turn off"). Toolkits can also be turned on as a training goal before any evidence
exists: "I'm training for my SPL".

Three properties make this more than feature flags:

1. **Evidence is computed, never stored.** Only intent is stored, so nothing goes stale when
   the rules change, and nothing needs migrating.
2. **Two scopes.** The pilot scope sets defaults for the shell, dashboard, reports, pickers
   and onboarding. Inside a form, the selected aircraft decides. When Petra switches from her
   ASG 29E to the tow plane, the form becomes an SEP form. There is no global mode.
3. **Fold, never hide** (the invariants in [PERSONAS.md §1](../PERSONAS.md#1-the-relevance-principle)).
   Irrelevant elements collapse into a "More (n)" drawer. Elements that hold data always
   render. While loading, the app fails open. Every decision can say why.

## Disciplines and derivation

The enum is `AEROPLANE, TMG, SAILPLANE, ULTRALIGHT, GYROPLANE, HELICOPTER, IFR, MULTI_CREW,
INSTRUCTOR, SIMULATOR`. Clients must treat an unknown value as `active`, so the enum can grow.

Evidence strengths:
- **strong**: a licence or rating.
- **recent**: a matching non-simulator, non-passenger flight in the last 24 months, or an
  aircraft in the fleet.
- **dormant**: flights exist, but all are older than 24 months.

| Discipline | Strong | Recent | Training signal |
| --- | --- | --- | --- |
| `AEROPLANE` | SEP/MEP/SET rating; PPL(A), LAPL(A), CPL, ATPL, MPL, FAA Private/Commercial/Sport/Recreational | aircraft class SEP/MEP/SET; UL `THREE_AXIS` (via `models.PartFCLClass`) | dual-only flights, no rating |
| `TMG` | `TMG` rating (Part-FCL or SPL extension) | aircraft class `TMG`; UL `THREE_AXIS_MOTORGLIDER` | same |
| `SAILPLANE` | `GLIDER` rating; SPL, LAPL(S), FAA Glider | aircraft `GLIDER`; UL `SAILPLANE` | same |
| `ULTRALIGHT` | `ULTRALIGHT` rating; UL licence kind (DULV, DAeC, LBA UL) | aircraft `ULTRALIGHT` | same; carries `ulKinds[]` |
| `GYROPLANE` | `GYROPLANE` rating; GPL | aircraft `GYROPLANE`; UL `GYROPLANE` | same |
| `HELICOPTER` | `(H)` licence type | UL `HELICOPTER` | same |
| `IFR` | `IR` rating | IFR time or approaches logged | intent `goal` only |
| `MULTI_CREW` | ATPL, MPL | multi-pilot aircraft; SIC, multi-pilot or relief time | — |
| `INSTRUCTOR` | FI/CRI/FI(S)/examiner licence or rating text | dual-given or examiner time | — |
| `SIMULATOR` | — | FSTD sessions | — |

Status resolution, first match wins:

1. intent `off` → `off`
2. intent `on`, or strong or recent evidence → `active`
3. intent `goal`, or dual-only evidence with no matching rating → `training`
4. only dormant evidence → `dormant` (shown in totals and reports, folded in forms and pickers)
5. nothing → `off`, with no evidence

Licence text is classified by one function: `models.ClassifyLicence(type, authority)`.
Today that logic is spread across `internal/service/currency/easa.go:37,44`, `gpl.go:46` and
`faa.go:217,225`, with a copy in the frontend at `src/lib/ultralight.ts:5-13`. An
unrecognised licence type produces no evidence. It never produces negative evidence.

## API contract

```yaml
/users/me/pilot-profile:
  get:   { operationId: getPilotProfile,    responses: { '200': PilotProfile, '401' } }
  patch: { operationId: updatePilotProfile, requestBody: PilotProfileUpdate,
           responses: { '200': PilotProfile, '400', '401' } }

Discipline:        { type: string, enum: [AEROPLANE, TMG, SAILPLANE, ULTRALIGHT, GYROPLANE,
                     HELICOPTER, IFR, MULTI_CREW, INSTRUCTOR, SIMULATOR] }
DisciplineIntent:  { type: string, enum: [auto, "on", "off", goal] }
DisciplineStatus:  { type: string, enum: [active, training, dormant, "off"] }
DisciplineEvidence:
  required: [source, strength, ref]
  properties:
    source:   { enum: [LICENCE, RATING, AIRCRAFT, FLIGHTS, FLIGHTS_DUAL, FLIGHTS_INSTRUCTING] }
    strength: { enum: [strong, recent, dormant] }
    ref:      { type: string }            # "SPL 12345", "D-1234", "14 flights"
    refId:    { type: string, format: uuid, nullable: true }
    lastSeen: { type: string, format: date, nullable: true }
DisciplineState:
  required: [discipline, status, intent, evidence]
  properties:
    discipline, status, intent, evidence[], ulKinds[] (ULKind),
    acknowledgedAt: { type: string, format: date-time, nullable: true }
PilotProfile:
  required: [mode, disciplines, pendingAcknowledgement]
  properties:
    mode: { enum: [adaptive, everything] }
    disciplines: DisciplineState[]      # all values, stable order
    pendingAcknowledgement: Discipline[] # auto-activated, not yet acknowledged
PilotProfileUpdate:                      # partial, idempotent merge
  properties:
    mode, intents: { additionalProperties: DisciplineIntent }, acknowledge: Discipline[]
```

An unknown discipline key in `intents` or `acknowledge` returns 400.

## Backend placement

- Migration `pilot_profiles`: one row per user. Columns: `user_id` (PK, FK users ON DELETE
  CASCADE), `mode` with a CHECK, `disciplines JSONB` (`{"SAILPLANE":{"intent":"on",
  "acknowledgedAt":"…"}}`), and timestamps. This follows the single-row precedent of
  `notification_preferences`.
- `internal/models/licence_kind.go`: `LicenceKind`, `ClassifyLicence`.
- `internal/models/pilot_profile.go`: the enums, the settings and their validation.
- Repository `PilotProfileRepository {Get, Upsert}` and `DisciplineEvidenceSource`. The
  evidence comes from **one** aggregate query: flights `LEFT JOIN aircraft`, grouped by class
  and `ul_kind`, with `max(date)` and `FILTER` sums for dual, dual-given, examiner, IFR,
  multi-pilot, SIC, relief and simulator time.
- `internal/service/pilotprofile/derive.go`: `Derive(licences, ratings, fleet, flightAgg,
  settings, now) []DisciplineState`. This is a pure function that holds every rule from the
  table above. `service.go` holds Get/Update. It must not import Gin.
- Handler: `internal/api/handlers/pilot_profile.go`. Wiring: `cmd/api/main.go`.
- **Portability** (rule 6): add `PilotProfile` to `cloudbackup.Payload`, restore it on import,
  and add a `coverage_test.go` entry.
- **Admin** (rule 5): add `pilotProfiles` to `AdminStats`, holding the `everything`-mode count
  and override counts per discipline and intent.
- **Metrics:** none. There is no job, cache or limiter.
- **Server consumers** (later): currency reminders suppressed for disciplines set to `off`;
  PDF/CSV exports default their layout from active disciplines (see the gliding plan, WP-22).

## Frontend placement

```ts
// src/hooks/usePilotProfile.ts
usePilotProfile(); useUpdatePilotProfile();
useDisciplines(): { mode; isLoading; status(d); isRelevant(ds: Discipline[]); evidence(d) };
// loading / error / unknown discipline → 'active' (fail open)

// src/lib/relevance/registry.ts — the one list of adaptive elements
interface FeatureDef {
  id: string;                               // 'flight.launchMethod'
  kind: 'nav' | 'field' | 'section' | 'report' | 'dashboardCard' | 'column' | 'picker';
  serves: Discipline[] | 'all';
  aircraftMatch?: (ac: Aircraft) => boolean;  // aircraft scope wins inside forms
  hasData?: (ctx: RelevanceCtx) => boolean;   // invariant 2
  columnBoost?: number;                        // auto column mode only
}
useRelevance(id, ctx?): { visible; folded; reason? };

// src/components/relevance/
<Relevant id="flight.launchMethod" ctx={{ aircraft, record }}>…</Relevant>
<FoldDrawer />            // "More (3)" per form or page
<ToolkitToast />          // in Layout; reads pendingAcknowledgement
```

Other frontend changes:
- Add `['pilot-profile']` to `FLIGHT_DEPENDENT_QUERY_KEYS` and to the licence, rating and
  aircraft mutations.
- Add a `relevance` i18n namespace in en and de.
- Add a **"What I fly"** section to the Profile page: one chip per toolkit, showing status,
  the evidence behind it, an on/off/training control, and a "show everything" switch.
- Custom flight-table column mode keeps priority. Relevance only reorders auto mode.

## Phases

Each phase is one PR, handed to one implementer, with the contract above. Every phase runs the
`personas` skill and reports scenario IDs.

| Phase | Repo | Agent | Scope | Closes |
| --- | --- | --- | --- | --- |
| 0 | api | `endpoint-implementer` | `ClassifyLicence`; route the currency helpers through it. No behaviour change: every currency unit and e2e test stays green. Add table tests. | — |
| 1 | api | `endpoint-implementer` + `migration-author` | Spec, migration, `Derive`, service, handler, export/import, admin stat, docs (`DOMAIN.md` derivation section, `API.md`, `FEATURES.md`, `DATA_MODEL.md`). e2e `pilot_profile_e2e_test.go`: one case per persona (Lena → `SAILPLANE` active; Jonas → training; Karl → `TMG` active, `SAILPLANE` dormant; Mehmet → `ULTRALIGHT[THREE_AXIS]` active, `AEROPLANE` dormant; Mark → `AEROPLANE`/`IFR`/`MULTI_CREW`/`SIMULATOR`; Ruth → nothing). Plus an export/import round trip and cross-user isolation. | J1, N1, R2 (API side) |
| 2 | fe | implementer | Regenerate the client; hooks, registry (empty), `<Relevant>`, `<FoldDrawer>`, toast, "What I fly". **One screenshot fixture set per persona** in `scripts/screenshots/fixtures.mjs`, plus a `persona` option in `targets.mjs`. Vitest truth table for `useRelevance` (relevant / has data / everything / loading / unknown). | R2 |
| 3a | fe | implementer | FlightForm onto the registry: launch method (`SAILPLANE`; aircraft `GLIDER` or UL `SAILPLANE`; not TMG), IFR section (`IFR`), multi-pilot/PICUS/SPIC/relief (`MULTI_CREW`), examiner (`INSTRUCTOR`), every item with `hasData`. | L3, K1, S3, A1 |
| 3b | fe | implementer | AircraftForm flags, class pickers (relevant classes first, the rest under "More classes"), Dashboard order, Reports sections, nav (fold only, never remove), column boost. | A1, A2, S3 |
| 3c | fe | implementer | Onboarding step "What do you fly, and what are you training for?", which writes intents. | R1, J1 |
| 4 | api | implementer | Reminders respect `off`; export layout defaults. | L5 |

Screenshots before and after for every persona fixture are mandatory from phase 3a onward.
Mark's fixture is the regression guard, and its screenshot diff must be empty for A2.

## Risks

- **Hiding something a pilot needs.** Mitigated by the four invariants. A Playwright test
  asserts that a field holding a value always renders for every persona.
- **Free-text licences misclassified.** Only classifier hits count, the reason is always
  shown, and the pilot can override it.
- **Query cost.** One grouped query per GET, a TanStack `staleTime` of 5 minutes, and an index
  on `flights(user_id, date)` to check.
- **Display preferences are not portable today.** The `users` row is exempt from export, so
  `flightListColumns` does not survive a move to another installation. Separate follow-up.
- **No `HELICOPTER` or `BALLOON` aircraft class exists.** The enum must not promise more than
  the domain supports. Adding a class is its own domain change.
