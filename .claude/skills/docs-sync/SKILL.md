---
name: docs-sync
description: Which document to update for a given behaviour change in this repo. Use at the end of any change that alters endpoints, domain rules, schema, packages, wiring, or features — docs/ is a first-class deliverable and must ship in the same PR.
---

# Keeping docs accurate

Documentation in `docs/` is part of the codebase. A change is **not complete** until its
documentation matches reality. Update in the same PR:

| Changed | Update |
| --- | --- |
| HTTP contract / endpoints | `api-spec/openapi.yaml`, `docs/API.md`, `docs/FEATURES.md` |
| Domain rules (flights, currency, validation, time handling) | `docs/DOMAIN.md` |
| Entities, schema, migrations | `docs/DATA_MODEL.md` |
| Packages, responsibilities, startup wiring | `docs/PACKAGES.md`, `docs/ARCHITECTURE.md` |
| A product feature added/removed/changed | `docs/FEATURES.md` |
| Auth, metrics, performance, test tooling | `docs/AUTHENTICATION.md`, `docs/METRICS.md`, `docs/PERFORMANCE.md`, `docs/RUNNING_TESTS.md` |
| Developer workflow / commands | `docs/DEVELOPMENT.md`, `CONTRIBUTING.md`, `README.md` |

Guidelines:

- **Never leave documentation describing behaviour that no longer exists.** If you cannot fully
  update a doc, say so explicitly in the PR description.
- Do not hand-edit generated artefacts referenced by docs (`internal/api/generated/`) —
  regenerate them.
- `docs/DEVELOPER_GUIDE.md` is the entry point and documentation map; keep its links valid.
- Prose style in this repo: no emoji in docs, scripts, or tooling output.
- `.github/copilot-instructions.md` is stale and contradicts the codebase in places
  (TypeScript/Prisma/sqlc examples). Do not treat it as a source of truth; if you touch it,
  correct it toward `docs/`.
