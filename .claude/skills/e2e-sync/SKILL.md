---
name: e2e-sync
description: Keep test/e2e/ in step with API behaviour changes. Use whenever you change a status code, validation bound, required field, auth requirement or response shape — and whenever an e2e test goes red, to decide whether the test is stale or the code regressed. Complements docs-sync; e2e tests are a first-class deliverable and ship in the same PR.
---

# Keeping e2e tests accurate

`test/e2e/` is the only tier that exercises the real HTTP contract against a real database.
A behaviour change is **not complete** until the e2e tests that cover it are updated in the
same PR — the same rule `docs-sync` applies to `docs/`.

## The failure mode this prevents

When the API changes deliberately and its e2e test is left behind, the test asserts a contract
that no longer exists. The damage is not the red build, which is loud and gets fixed. It is that
**the behaviour the test existed to guard silently stops being exercised** — and a long-red test
teaches the team to skim past it.

Three real cases, all found at once and fixed in #150:

| Test | What happened |
| --- | --- |
| `too_long_default_is_rejected` | Asserted `"TOOLONG"` (7 chars) → 400. #116 widened `defaultDepartureIcao` from 4 to 100 chars for off-airport sites, so the test was asserting that a *valid* value is rejected. |
| `TestUserProfile/update_email` | Predated the `currentPassword` requirement on email change. |
| `TestEmailUpdateDuplicate` | Same cause — and it is the *only* coverage of the duplicate-address 409, which therefore had not run since the requirement landed. |

Note the shape of the third one: the test still failed, so nothing looked green, but the
assertion it was written for had not executed in months.

## When a change requires an e2e update

Any of these, whether or not a test is currently red:

- A status code changes, or a new rejection is added ahead of an existing one.
- A validation bound moves (length, range, enum, format).
- A field becomes required, optional, or newly required *conditionally* (as `currentPassword` is).
- An auth, ownership, or rate-limit rule changes.
- A response gains, loses, or renames a field.
- A new endpoint lands — it needs coverage, not just a spec entry.

## Which file

Tests are named after the area they cover; `ls test/e2e/` is usually enough. The
non-obvious ones:

| Changed | Look in |
| --- | --- |
| Cross-cutting oddities, duplicate/conflict handling, empty-state exports | `edge_cases_e2e_test.go` |
| Auth, tokens, password reset, 2FA, passkeys | `auth_`, `password_reset_`, `twofactor_`, `webauthn_` |
| Authz boundaries, injection, enumeration | `security_`, `pentest_owasp_`, `bruteforce_` |
| Email bodies and delivery (via MailPit) | `notification_content_`, `notification_` |
| Regressions with a written-up cause | `REGRESSIONS.md` |

Shared helpers (`NewE2EClient`, `registerAndLogin`, `requireStatus`, `assertStatus`,
`uniqueEmail`) live in `framework_test.go`. Prefer them over hand-rolled requests.

## When an e2e test goes red

Decide which of two things you are looking at, and **write down which one in the PR**:

1. **The code regressed** — fix the code. Never adjust the assertion to match the new
   behaviour. That is the "weaken a test to hide a regression" case CLAUDE.md rule 3 forbids;
   if you cannot fix it now, file a GitHub issue.
2. **The test is stale** — the change was deliberate and the test encodes the old contract.
   Update it, and cite the PR or commit that changed the behaviour in the test comment or the
   commit body, so the next reader can tell case 2 from case 1 without re-deriving it.

If you cannot tell which, check whether the behaviour is intentional at its source: the OpenAPI
spec, the handler, and `git log -L` on the validation line. In #150 a single
`git log -L 70,70:internal/models/validation.go` was decisive.

## The trap: green for the wrong reason

A stale test frequently fails at a *different, earlier* gate than the one it was written to
check — `TestEmailUpdateDuplicate` was rejected with 400 for a missing password and never
reached its 409. Two habits prevent it:

- **Assert the specific failure, not just the status class.** A 400 from the wrong validator
  is not the 400 you meant. Check the error body when several rejections share a code.
- **When you relax a bound, add the positive case too.** Asserting only that 101 chars is
  rejected would let a regression to the old 4-char limit keep the suite green. Pair it with a
  case that must be *accepted* — `"Hausen am Albis"` — so both walls of the bound are pinned.

Same logic for a new precondition: cover the guard itself (absent → 400, wrong → 401) alongside
the happy path, or the guard is untested.

## Before committing

```bash
bash scripts/run-e2e-tests.sh                     # full stack, all tests
bash scripts/run-e2e-tests.sh TestUserProfile -k  # one test, keep the stack up
```

`-k` leaves the environment running; re-run against it without a rebuild:

```bash
E2E_API_URL=http://localhost:3333 go test -tags=e2e -count=1 -run TestUserProfile ./test/e2e/...
```

Two things to know about the runner: it sets `set -e`, so a failing suite aborts before its
own "Some e2e tests failed" branch and API-log dump — read `docker compose -f
docker-compose.e2e.yaml logs api` yourself. And `test/e2e/` is behind `//go:build e2e`, so a
plain `go test ./...` compiles none of it; a syntax error there will not surface until the
tagged run. See `.claude/skills/testing/SKILL.md` for the other tiers.
