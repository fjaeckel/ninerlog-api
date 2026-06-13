# NinerLog API Documentation

Developer documentation for the NinerLog API backend. Start with the
**[Developer Guide](./DEVELOPER_GUIDE.md)**, which links everything together.

## Core guides

| Document | Covers |
| --- | --- |
| [DEVELOPER_GUIDE.md](./DEVELOPER_GUIDE.md) | Orientation, tech stack, core concepts, documentation map |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Layers, request lifecycle, package relationships, startup/wiring |
| [DATA_MODEL.md](./DATA_MODEL.md) | Domain entities, relationships, DB schema & migrations |
| [DOMAIN.md](./DOMAIN.md) | Flight logging, time handling, auto-calculations, validation, currency engine |
| [API.md](./API.md) | HTTP surface, OpenAPI-first workflow, routing & security |
| [FEATURES.md](./FEATURES.md) | End-to-end catalogue of every product feature |
| [PACKAGES.md](./PACKAGES.md) | Per-package reference for `internal/` and `pkg/` |
| [DEVELOPMENT.md](./DEVELOPMENT.md) | Setup, build, test, codegen, conventions, CI |

## Topic deep-dives

| Document | Covers |
| --- | --- |
| [AUTHENTICATION.md](./AUTHENTICATION.md) | Tokens, 2FA, WebAuthn, lockout, rate limiting |
| [METRICS.md](./METRICS.md) | Prometheus metrics and observability |
| [PERFORMANCE.md](./PERFORMANCE.md) | Performance budgets, benchmarks, profiling |
| [RUNNING_TESTS.md](./RUNNING_TESTS.md) | Running unit/integration/e2e tests |

## Keeping docs accurate

These documents are part of the codebase and must reflect reality. When you change
behaviour, update the relevant document(s) in the same pull request — see the mapping in
[DEVELOPMENT.md](./DEVELOPMENT.md#documentation) and the rule in
[`.github/copilot-instructions.md`](../.github/copilot-instructions.md#documentation-maintenance).
