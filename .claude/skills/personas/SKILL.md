---
name: personas
description: Check a feature, field, currency rule, export, report or skill against NinerLog's binding personas (docs/PERSONAS.md) and the discipline relevance model. Use before designing any feature, before committing any change that alters what a pilot sees or what the API returns for them, when adding a currency rule, export column, import mapping or report metric, and when reviewing a PR. Especially for anything touching gliders, TMGs, ultralights, gyroplanes, IFR, multi-crew or instructors.
---

# Persona check

`docs/PERSONAS.md` is binding on this repo and `ninerlog-frontend`. Nine personas — six
gliding/UL pilots and three guards — each with jobs, "must see", "must fold away" and
numbered acceptance scenarios (L1, M3, A2 …). **A change is not done until you have run it
against them.**

## 1. Before you design

1. Read `docs/PERSONAS.md` sections 1–3. Do not work from memory; scenarios change.
2. Name the personas the change serves and the scenario IDs it closes. If none, write
   "persona-neutral" and why (a refactor, an ops change) — that is a legitimate answer.
3. Name the **disciplines** it serves (`AEROPLANE`, `TMG`, `SAILPLANE`, `ULTRALIGHT`,
   `GYROPLANE`, `HELICOPTER`, `IFR`, `MULTI_CREW`, `INSTRUCTOR`, `SIMULATOR`). Once the pilot
   profile exists (`docs/plans/ADAPTIVE_DISCIPLINES.md`, phase 1), every new field, report
   section, export column and currency card declares them.

## 2. Backend questions

Answer each in the PR description. "n/a" needs a reason.

| # | Question | Typical failure |
| --- | --- | --- |
| B1 | Does the data model let each served persona log this without inventing values? | A required `aircraftReg` for a powered paraglider (S2); required block times for a glider (L3, K3) |
| B2 | Does the currency rule count exactly what the regulation counts, for the right class/kind? | Cumulative dual minutes where the rule says one flight (K1, M2); kindless UL flights counted for every kind (S1) |
| B3 | Does a towed launch, a UL flight or a TMG flight leak into a rating it must not feed? | Towed launches in SEP revalidation (P3, N2) |
| B4 | Does the value survive **every** round trip: JSON export/import, CSV, PDF, club imports? | Launch method dropped by CSV/PDF/Vereinsflieger import (L2, L5) |
| B5 | Do auto-created aircraft (quick-add, import) get the class/kind the currency engine needs? | Imported gliders with no class count for nothing (L2, M3) |
| B6 | Is the status honest? `current` must mean the persona may legally fly. | FAA glider passenger rule reported as rating status |
| B7 | Does the change feed the discipline derivation (licence, rating, aircraft, flights)? If it adds a new signal, is `Derive` updated? | A new licence type the classifier never recognises |
| B8 | Is it unchanged for the guard personas G1–G3? | New required field on every flight |

Regulation claims in code or docs cite the article (e.g. `SFCL.160(b)(1)(iii)`,
`LuftPersV §45`) and belong in `docs/SAILPLANES.md` / `docs/DOMAIN.md`, per `aviation-domain`.

## 3. Tests the personas imply

- Each scenario a change closes gets a test that names it: `t.Run("L1 winch batch …")` in
  `test/e2e/`, or a table row in the unit test. Use the persona's own data (Lena's `D-1234`
  ASK 21, Mehmet's `D-MXYZ` C42) so the tests read as the scenario.
- A guard scenario (A1, A2, N2) is tested whenever the change touches a shared path.
- Never delete or weaken a scenario test to get green; follow `e2e-sync`.

## 4. Report

End your work (or review) with a persona line per affected persona:

```
Personas: L1 closed (e2e winch batch), L2 closed (Vereinsflieger S.-Art), K1 unaffected,
A1/A2 guard: no change to non-glider flights (e2e flight_e2e_test green).
```

For a full review, delegate to the `persona-reviewer` agent.
