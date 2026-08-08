---
name: security-audit
description: Security-review one feature or change in this repo — auth/ownership, injection, crypto and secrets, SSRF and external I/O, resource limits. Use when auditing a feature before or after implementation, reviewing a branch for security impact, or when asked for a security review. Also defines where findings may and may not be written.
---

# Security auditing a feature

Audit **one feature at a time**. A whole-repo sweep produces a wall of text nobody acts on; a
feature-scoped audit produces findings tied to code someone owns right now.

## Rule 0 — findings never enter the repository

Vulnerability details are attacker instructions. A findings document committed to a branch is a
disclosure, and force-pushing it away later does not reliably erase it (the commit stays
reachable by SHA on GitHub until garbage collection).

**Never** `git add`, commit, or push:

- audit reports, findings lists, or vulnerability write-ups
- commit messages, PR titles/bodies, or branch names that describe an unfixed weakness
- test cases or fixtures that encode a working exploit against current code
- public GitHub issues describing an unfixed vulnerability

**Do** instead:

- Write the report to `security-audits/` — gitignored, stays on the machine.
- Deliver the summary **in chat** to the person who asked.
- Track real issues through the private channel in `SECURITY.md` (hej@ninerlog.com) or a
  **GitHub Security Advisory** (private by default) — never a public issue.
- When a fix is pushed, write the commit message about the *behaviour* ("validate host key
  fingerprint before connecting"), not the *hole* ("fixes MITM allowing credential theft").

Fixes are pushed. Findings are not.

## Scope the feature first

Before reviewing, list the feature's actual surface — do not guess from the name:

1. **Entry points** — which operations in `api-spec/openapi.yaml`, plus manually-registered
   routes in `cmd/api/main.go` (reports and flight utilities are **not** in the spec).
2. **Is it public?** Check the public-path allow-list in `cmd/api/main.go` against
   `internal/api/middleware/auth.go`. A public path gets far more scrutiny.
3. **Layers touched** — handler → service → repository, plus any migration, background worker,
   or external provider.
4. **Data sensitivity** — does it read/write credentials, TOTP secrets, tokens, other users'
   rows, or PII?

## The per-layer checklist

### Handler (`internal/api/handlers/`)

- `userID` comes **only** from `getUserIDFromContext` — never from the body, query, or a path
  param. A user-supplied ID that selects whose data is touched is an IDOR.
- Admin operations go through `h.isAdminUser` / `requireAdmin` (which requires `EmailVerified`),
  not an ad-hoc email comparison.
- Errors returned to the client are generic. No `err.Error()` pass-through of bind/SQL/provider
  errors.
- New route registered manually? Confirm it is inside the authenticated group and covered by a
  rate-limit bucket.

### Service (`internal/service/`)

- **Ownership check on every path** — load by ID, compare `resource.UserID != userID`, return the
  sentinel error. Check `update` and `delete` too, not just `get`; that is where it gets missed.
- Nested/child resources: verify the **parent** is owned, not just the child.
- Bulk and batch operations: ownership must be enforced per item, not once for the first one.
- State changes that matter (disable, revoke, password/email change) revoke refresh tokens.
- Multi-step read-check-write on security state must be atomic — a conditional `UPDATE` with a
  `rows == 0` branch, or a transaction. A check in Go followed by a separate write is a race.

### Repository (`internal/repository/postgres/`)

- Every user value is a `$N` bind parameter. `fmt.Sprintf` may build **placeholder positions and
  hardcoded identifiers only** — never interpolate a value.
- Dynamic `ORDER BY` / column names resolve through a hardcoded switch or allowlist map with a
  safe default.
- `LIKE` patterns escape `%`, `_`, `\` — reuse `escapeLike` in `internal/flightsearch/sql.go`.
- New queries belong in the repository layer. Raw SQL in a handler is where the next injection
  will come from.

### External I/O and files

- Any outbound connection to a **user-supplied** host or URL must dial through
  `internal/service/cloudbackup/netguard` (it checks the post-DNS resolved IP, closing the
  DNS-rebind window). No exceptions — this is the SSRF boundary.
- No `InsecureSkipVerify`; no `ssh.InsecureIgnoreHostKey` outside an explicit, warned, opt-in
  config flag.
- File paths: never join user-controlled names into a filesystem or remote path. Backup filenames
  are machine-generated from a timestamp — keep it that way.
- CSV export cells go through `neutralizeCSVCell` (`internal/api/handlers/export.go`) — formula
  injection is defended centrally, so add columns via the existing writer, not a bypass.
- Uploads/imports need a size cap, a row cap, and a bound on what is held in memory.

### Crypto, secrets, config

- Randomness for anything security-bearing: `crypto/rand`, **and check the error return**.
- Encryption at rest: `pkg/cryptoutil` (AES-256-GCM, fresh nonce per encrypt). Do not hand-roll.
  A new use derives its own subkey from `ENCRYPTION_KEY` (`DeriveKey`, HKDF) rather than
  reusing another purpose's key or adding a third secret. Ciphertext in a table column binds
  to its row with `EncryptWithAAD` so a blob cannot be moved between rows or owners.
- Tokens are stored as SHA-256 hashes, are single-use, and expire. Passwords use `pkg/hash`
  (bcrypt cost 12).
- New config secret? Validate it at startup and **fail closed** (`cmd/api/secrets.go` is the
  pattern). A `slog.Warn` and booting anyway means production runs insecure.
- Nothing secret reaches the logger — not DSNs, tokens, credentials, or TOTP seeds.

## Verify before reporting

A finding you have not confirmed in the source is noise, and noise is what makes real findings
get ignored.

- Read the actual code and cite `file:line`. Do not report from a grep hit alone.
- State a concrete exploit: who the attacker is, what they send, what they get.
- Check whether an existing control already blocks it (auth middleware, netguard, ownership
  check, bind parameter) before calling it exploitable.
- If it depends on deployment config, say so and mark it conditional.

## Severity

| Severity | Meaning |
| --- | --- |
| Critical | Unauthenticated remote compromise, or any user reading/writing another user's data |
| High | Authenticated privilege escalation, credential/secret exposure, auth bypass |
| Medium | Real exploit needing a precondition (a deployment default, a race, a specific IdP) |
| Low | Defense-in-depth gap, info disclosure, self-harm-only, or a latent/dead-code weakness |

Distinguish a **code vulnerability** from an **insecure default**. Both matter, but they are
fixed by different people in different places — say which one you found.

## Report format

Deliver in chat; write the long form to `security-audits/<feature>-<date>.md` if one is wanted.

```
## <feature> — security review

Verdict: <one line — is this safe to ship?>

### Findings
<severity> — <one-line title>
  file:line
  Exploit: <concrete scenario>
  Fix: <specific change>

### Verified working
<controls you actually checked and found correct — so the reader knows they were not skipped>
```

Always include "verified working". An audit that only lists problems gives no signal about
coverage, and the next reviewer re-does the work you already did.
