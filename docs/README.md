# NinerLog API Documentation

Developer documentation for the NinerLog API backend. Start with the
**[Developer Guide](./DEVELOPER_GUIDE.md)**, which links everything together.

## Core guides

| Document | Covers |
| --- | --- |
| [DEVELOPER_GUIDE.md](./DEVELOPER_GUIDE.md) | Orientation, tech stack, core concepts, documentation map |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Layers, request lifecycle, package relationships, startup/wiring |
| [SQL_LAYERING.md](./SQL_LAYERING.md) | Where SQL may live, repository catalogue, deliberate exceptions |
| [DATA_MODEL.md](./DATA_MODEL.md) | Domain entities, relationships, DB schema & migrations |
| [DOMAIN.md](./DOMAIN.md) | Flight logging, time handling, auto-calculations, validation, currency engine |
| [API.md](./API.md) | HTTP surface, OpenAPI-first workflow, routing & security |
| [FEATURES.md](./FEATURES.md) | End-to-end catalogue of every product feature |
| [PACKAGES.md](./PACKAGES.md) | Per-package reference for `internal/` and `pkg/` |
| [DEVELOPMENT.md](./DEVELOPMENT.md) | Setup, build, test, codegen, conventions, CI |

## Topic deep-dives

| Document | Covers |
| --- | --- |
| [AIRCRAFT_REGISTRATIONS.md](./AIRCRAFT_REGISTRATIONS.md) | Canonical registration notation and the vendored nationality-mark table |
| [AUTHENTICATION.md](./AUTHENTICATION.md) | Tokens, 2FA, WebAuthn, lockout, rate limiting |
| [OIDC.md](./OIDC.md) | Optional single sign-on: configuration, provider recipes, provisioning, migration |
| [METRICS.md](./METRICS.md) | Prometheus metrics and observability |
| [PERFORMANCE.md](./PERFORMANCE.md) | Performance budgets, benchmarks, profiling |
| [RUNNING_TESTS.md](./RUNNING_TESTS.md) | Running unit/integration/e2e tests |
| [SAILPLANES.md](./SAILPLANES.md) | Sailplanes and TMGs: classes, launch methods, Part-SFCL recency |
| [SESSION_CONTRACT.md](./SESSION_CONTRACT.md) | Binding cross-repo session contract |
| [PERSONAS.md](./PERSONAS.md) | Binding personas and acceptance scenarios every feature is checked against |

## Plans

Forward-looking, not yet implemented. Code references are as audited on the date in each file.

| Document | Covers |
| --- | --- |
| [plans/ADAPTIVE_DISCIPLINES.md](./plans/ADAPTIVE_DISCIPLINES.md) | Pilot profile and "toolkits": showing features to the pilots whose ratings they serve |
| [plans/GLIDING_AND_ULTRALIGHT.md](./plans/GLIDING_AND_ULTRALIGHT.md) | Audit and hand-off work packages for glider, TMG and ultralight pilots |

## Keeping docs accurate

These documents are part of the codebase and must reflect reality. When you change
behaviour, update the relevant document(s) in the same pull request — see the mapping in
[DEVELOPMENT.md](./DEVELOPMENT.md#documentation) and the rule in
[`.github/copilot-instructions.md`](../.github/copilot-instructions.md#documentation-maintenance).
