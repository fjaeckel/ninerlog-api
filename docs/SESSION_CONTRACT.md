# Session contract

**This document is binding on both `ninerlog-api` and `ninerlog-frontend`.** It is the single
source of truth for how a NinerLog session begins, survives, and ends. Any change to session
or token behaviour on either side changes this file first, in the same PR.

The rules exist because the two sides can only stay compatible if they agree on what a failed
request *means*. Most spurious logouts come from one side inventing a meaning the other never
promised.

---

## 1. Token architecture

| Token | Lifetime | Signed with | Carries |
|---|---|---|---|
| Access | 15 minutes | `JWT_SECRET` | `user_id`, `session_id` |
| Refresh | 7 days | `REFRESH_SECRET` | `user_id`, `session_id` |
| 2FA challenge | 5 minutes | `JWT_SECRET` | `user_id`, `sub: 2fa-challenge` |

Lifetimes are fixed in `cmd/api/main.go`; they are not configurable per deployment.

## 2. A session is not a token

A **session** is one signed-in device. It is identified by `session_id`, which is minted at
login and **preserved across every rotation**. The refresh token rotates; the session does not.

`refresh_tokens` rows are the chain of tokens belonging to a session. A session is *live* while
it holds at least one unrevoked, unexpired row.

## 3. Concurrent sessions are supported

- A user may hold up to `MAX_SESSIONS_PER_USER` (default **5**) live sessions at once.
- **Logging in never ends another session.** Signing in on a phone must not sign out a browser.
- Exceeding the cap evicts the **least recently used** session. It never rejects the new login.
- Sessions are listed and revoked through `/auth/sessions` (see §7).

> This replaced a single-active-session rule, under which every login deleted all of a user's
> refresh tokens. Do not reintroduce that behaviour, in any form, without changing this
> document.

## 4. Rotation has a grace window

Refreshing rotates the refresh token: the presented token is marked **superseded** and a new
pair is issued on the same session.

- A superseded token stays usable for `REFRESH_REUSE_GRACE` (default **30s**).
- Within the grace window, presenting it returns a new pair on the same session and does
  **not** disturb any pair already issued. Two clients racing a refresh both end up holding
  working tokens.
- After the grace window, presenting a superseded token is a **replay**: the entire session is
  revoked and the call returns 401.
- A token revoked *outright* — logout, password change, session revocation, admin disable —
  gets **no grace**. Signing out takes effect immediately.

The distinction is stored in two separate columns: `rotated_at` (superseded, grace applies) and
`revoked_at` (revoked, no grace). They must never be collapsed into one.

## 5. What each failure means to a client

This table is the heart of the contract. **Only 401 means the session is over.**

| Response to `POST /auth/refresh` | Meaning | The client must |
|---|---|---|
| `200` | Renewed | Store the new pair; continue |
| `401` | The session is genuinely gone | Clear credentials and send the user to sign in |
| `429` | Rate limited — the session is fine | Back off and retry; **keep credentials** |
| `5xx` | Server or proxy trouble — the session is fine | Back off and retry; **keep credentials** |
| Network error / offline / timeout | Nothing is known about the session | Back off and retry; **keep credentials** |

A backend restart, a deploy, a proxy 502, a flaky mobile connection and a rate limit are all
**transient**. A session must survive every one of them. Treating any non-401 as a logout is a
bug, not a safety measure.

## 6. Client obligations

Any NinerLog client — the React PWA, the mobile app, anything else — must:

1. **Persist both tokens** across restarts, in storage that survives the process.
2. **Clear credentials only on a 401 from `/auth/refresh`.** Never on 429, never on 5xx, never
   on a network error. (A 401 on an ordinary API call means "refresh first", not "log out".)
3. **Retry transient refresh failures with capped exponential backoff** rather than giving up
   or hammering the endpoint.
4. **Run at most one refresh at a time per client**, and share its result among all waiting
   callers.
5. **Coordinate across tabs and instances** of the same browser profile. When one tab rotates
   the token, the others adopt the new pair rather than presenting the old one. The server's
   grace window (§4) is a safety net for the races this cannot prevent — not a substitute for
   coordination.
6. **Refresh proactively** before expiry, and when returning to the foreground, but never so
   often that it trips the rate limit.

## 7. Session management endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/auth/sessions` | List live sessions, most recently used first, with `maxSessions` |
| `DELETE` | `/auth/sessions/{sessionId}` | Revoke one session (404 if unknown or another user's) |
| `DELETE` | `/auth/sessions` | Revoke every session except the caller's |

Each session reports `deviceLabel` (derived from the User-Agent), `ipAddress`, `createdAt`,
`lastUsedAt`, `expiresAt` and `current`. The label is established at login and survives
rotation: a refresh replaces it only when its own User-Agent names a recognisable device, so
renewing from a client that sends none never degrades the label to "Unknown device".

Revocation is always scoped to the authenticated user in SQL — a session ID alone never
authorises anything.

## 8. What still ends every session at once

These are deliberate and must stay:

- Password change and password reset.
- An administrator disabling the account.
- Account deletion.

## 9. Configuration

| Variable | Default | Effect |
|---|---|---|
| `MAX_SESSIONS_PER_USER` | `5` | Live sessions kept per user; the oldest is evicted beyond it |
| `REFRESH_REUSE_GRACE` | `30s` | How long a superseded refresh token stays usable |

An unparseable or non-positive value falls back to the default. Both are reported by
`GET /admin/config`.

## 10. Where this is implemented

| Concern | Location |
|---|---|
| Session policy, rotation, grace, replay detection | `internal/service/auth.go`, `internal/service/session.go` |
| Device labelling | `internal/service/devicelabel.go` |
| Session SQL | `internal/repository/postgres/refresh_token.go` |
| Endpoints | `internal/api/handlers/session.go` |
| Schema | `db/migrations/000063_add_refresh_token_sessions.up.sql` |
| Client-side refresh and cross-tab coordination | `ninerlog-frontend/src/api/client.ts` |
| Session UI | `ninerlog-frontend/src/pages/ProfilePage.tsx`, `src/hooks/useSessions.ts` |

Tests that hold these rules in place: `internal/service/session_test.go`,
`test/e2e/session_e2e_test.go`, and
`ninerlog-frontend/src/__tests__/api/refreshResilience.test.ts`.
