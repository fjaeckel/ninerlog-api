# Authentication & Authorization

NinerLog API uses JWT-based authentication with optional TOTP two-factor authentication, account lockout protection, and per-IP rate limiting.

## Authentication modes

A deployment runs in exactly one of two modes. Everything else in this document
describes **local** mode, which is the default and what `ninerlog.com` runs.

| Mode | Enabled by | Credentials owned by |
|---|---|---|
| **local** | the default | NinerLog — passwords, optional TOTP, optional passkeys |
| **oidc** | `OIDC_ISSUER` is set | An external OpenID Connect provider, entirely |

Setting `OIDC_ISSUER` is a mode switch, not an additional sign-in method: it
disables password login, registration, email verification, password reset, TOTP
and passkeys, all of which then answer **503**. Leaving any of them reachable
would be a way around the provider's own MFA, lockout and offboarding policy.

Clients discover the mode from the public `GET /api/v1/auth/providers`. Token
architecture, refresh and logout below are identical in both modes — OIDC only
replaces how the first token pair is obtained.

Full setup, provider recipes and migration guidance: **[OIDC.md](./OIDC.md)**.

## Token Architecture

| Token | Lifetime | Purpose |
|---|---|---|
| Access token | 15 minutes | Authenticates API requests (`Authorization: Bearer <token>`) |
| Refresh token | 7 days | Obtains new access/refresh token pairs |
| 2FA challenge token | 5 minutes | Short-lived token issued when 2FA is required at login |

All tokens are signed with **HS256**. Access and 2FA tokens use `JWT_SECRET`; refresh tokens use `REFRESH_SECRET`.

**Claims:**

```json
{
  "user_id": "uuid",
  "jti": "unique-token-id",
  "exp": 1234567890,
  "iat": 1234567890
}
```

2FA challenge tokens additionally carry `"sub": "2fa-challenge"`.

---

## Authentication Flows

### Registration

```
POST /api/v1/auth/register
```

**Request:**
```json
{
  "email": "pilot@example.com",
  "password": "securepilotpass",
  "name": "Jane Doe"
}
```

**Validation:**
- Email — required, ≤255 characters, valid format
- Password — required, **≥12 characters**, ≤72 characters (bcrypt limit)
- Name — required

**201 Created:**
```json
{
  "accessToken": "eyJ...",
  "refreshToken": "eyJ...",
  "expiresIn": 900,
  "user": {
    "id": "uuid",
    "email": "pilot@example.com",
    "name": "Jane Doe",
    "twoFactorEnabled": false,
    "isAdmin": false
  }
}
```

**Errors:** `400` validation failure, `409` email already registered.

---

### Unverified account lifecycle

When SMTP is configured, registration creates the account with
`email_verified = false` and it cannot log in until the address is confirmed.
Without SMTP, registration marks the account verified immediately — there is no
way to ask for confirmation, so nothing below applies.

> **Never runs in OIDC mode.** With an identity provider in charge, local
> registration and email verification are switched off entirely, so
> `email_verified = false` no longer means "an abandoned signup". It means the
> provider did not assert `email_verified` for an account somebody may be using
> daily. Reaping on that signal would delete live accounts, so the service is
> not constructed at all when `OIDC_ISSUER` is set — no worker, no admin
> endpoint, and `UNVERIFIED_CLEANUP_ENABLED=true` cannot override it.

An account that never confirms is a dead account: it holds nothing and cannot be
used. Rather than accumulating forever, it is reminded once and then reaped.

```
signup ──24h──> reminder email ──30d──> account deleted
                (starts the clock)      (irreversible, cascades)
```

1. **Reminder.** A sweep (hourly by default) finds accounts unverified longer
   than `UNVERIFIED_REMINDER_AFTER`, mints a fresh verification token, and sends
   a reminder that states the deletion deadline outright.
2. **Retention.** The reminder timestamp — not the signup date — starts the
   clock. `UNVERIFIED_ACCOUNT_RETENTION` after it, the account is deleted along
   with everything it owns via `ON DELETE CASCADE`.
3. **Confirming at any point** during the window flips `email_verified` and
   removes the account from scope permanently.

**Accounts that are never reaped.** The per-account guards below are enforced in
SQL at both the listing and the deletion so neither can drift from the other.
They are a second line of defence behind the OIDC-mode refusal above: a
deployment that ran OIDC and later switched back to local auth still holds
provider-provisioned accounts, and those must not become reapable just because
the mode changed.

| Excluded | Why |
| --- | --- |
| `email_verified = true` | Nothing to chase |
| `last_login_at IS NOT NULL` | The account is in use, whatever the flag says |
| Linked to an OIDC identity | Authenticates through its provider; our verification link is irrelevant to it |


**Send failures.** The stamp only goes on when the outcome is final:

- **Delivered** → clock starts.
- **Permanently undeliverable** (mailbox does not exist, or the address is
  already suppressed) → clock starts anyway. No later attempt would do better,
  and without the stamp the account would never be reaped at all. The full
  retention window still applies — a hard bounce does not shorten it.
- **Transient failure** (our SMTP server is down, greylisting) → nothing is
  stamped, and the account is retried on the next sweep. An outage must never
  start deletion clocks.

**Configuration** (all optional):

| Variable | Default | Meaning |
| --- | --- | --- |
| `UNVERIFIED_CLEANUP_ENABLED` | `true` | Set to `false` to keep dead rows rather than risk deleting one. Cannot switch the feature **on** — SMTP and non-OIDC mode are both prerequisites |
| `UNVERIFIED_REMINDER_AFTER` | `24h` | Delay from signup to the reminder |
| `UNVERIFIED_ACCOUNT_RETENTION` | `720h` (30d) | Grace period from the reminder to deletion |
| `UNVERIFIED_CLEANUP_INTERVAL` | `1h` | How often the sweep runs |

An unparseable or non-positive value falls back to the default rather than
disabling a step — a typo must not quietly reap accounts on a schedule nobody
chose.

`GET /api/v1/admin/config` reports `unverifiedCleanupEnabled` and, when it is
off, `unverifiedCleanupDisabledReason` (`oidc_mode`, `smtp_not_configured`, or
`disabled_by_configuration`), so a disabled reaper is explained rather than
merely absent.

`POST /api/v1/admin/maintenance/cleanup-unverified` runs a sweep on demand, and
answers `503` on a deployment where the reaper does not run.
`GET /api/v1/admin/users` reports `verificationReminderSentAt`,
`scheduledDeletionAt`, and `emailSuppressed` for accounts still in the window.

### Email deliverability

An SMTP client learns about delivery only during the SMTP conversation, so that
is what is recorded. Every send writes a row to `email_delivery_events` with the
outcome and the server's reply code, and a permanent recipient-level refusal
adds the address to `email_suppressions`, after which nothing is sent to it.

The classification depends on **which command** was refused, so the send path
runs the conversation step by step instead of calling `smtp.SendMail`:

| Outcome | Cause | Suppresses the address? |
| --- | --- | --- |
| `delivered` | Message accepted | — |
| `hard_bounce` | 5xx to `RCPT TO` | **Yes** |
| `soft_bounce` | 4xx to `RCPT TO`, or 4xx after the body | No |
| `rejected` | 5xx after the message body — size, content, reputation | No |
| `server_error` | Dial, TLS, `AUTH`, or `MAIL FROM` failure | No |
| `invalid_address` | Failed to parse; never dialled | Yes |
| `suppressed` | Refused before dialling | Yes (already) |
| `dry_run` | SMTP not configured | — |

A 5xx from `AUTH` and a 5xx from `RCPT TO` are both permanent replies. Treating
them alike would mean one stale SMTP password suppressed every address the
system mails, which is why authentication and envelope failures can never be
bounces.

**Not covered:** asynchronous bounces. Most servers accept a message and bounce
it later via a DSN to the `Return-Path` address. Catching those needs a mailbox
polled over IMAP (with VERP addressing) or an ESP with webhooks; neither exists
here, so `delivered` means "the receiving server accepted it", not "it reached
an inbox".

Administrators can inspect the log at `GET /api/v1/admin/email/deliveries`, list
suppressions at `GET /api/v1/admin/email/suppressions`, and lift one with
`DELETE /api/v1/admin/email/suppressions/{email}` when a mailbox is known to
work again.

---

### Login

```
POST /api/v1/auth/login
```

**Request:**
```json
{
  "email": "pilot@example.com",
  "password": "securepilotpass"
}
```

**200 OK** (no 2FA):
```json
{
  "accessToken": "eyJ...",
  "refreshToken": "eyJ...",
  "expiresIn": 900,
  "user": { ... }
}
```

**200 OK** (2FA enabled — see [Two-Factor Authentication](#two-factor-authentication-totp)):
```json
{
  "requiresTwoFactor": true,
  "twoFactorToken": "eyJ..."
}
```

In the OpenAPI spec the 200 is a `oneOf` over `AuthResponse` and
`TwoFactorLoginRequired`. The two are mutually exclusive — `requiresTwoFactor`
appears only on the challenge, and the token fields only on a completed login —
so a generated client can switch on it without probing.

**Errors:** `401` invalid credentials, `403` account disabled, `429` account locked (too many failed attempts).

Login deletes all prior refresh tokens for the user, enforcing a **single active session**.

**Last login** (`users.last_login_at`, exposed as `lastLoginAt` in the admin user
list) is stamped by every path that actually hands an account a session:
password login, the 2FA second factor, a passkey assertion, the OIDC handoff
exchange, and the sign-up verification link — following that link signs the new
account in, so it is that account's first login and shows up as such. For a 2FA
account the password step alone does not stamp it; `POST /auth/2fa/login` does,
once the second factor passes. A token refresh never stamps it: it renews a
session an earlier login established.

---

### Token Refresh

```
POST /api/v1/auth/refresh
```

**Request:**
```json
{
  "refreshToken": "eyJ..."
}
```

**200 OK:**
```json
{
  "accessToken": "eyJ...",
  "refreshToken": "eyJ...",
  "expiresIn": 900
}
```

The old refresh token is **immediately revoked** (one-time use / rotation).

**Errors:** `401` invalid or expired refresh token.

---

### Password Change

```
POST /api/v1/auth/change-password
```
Requires authentication.

**Request:**
```json
{
  "currentPassword": "oldpassword",
  "newPassword": "newsecurepassword"
}
```

**204 No Content** on success. All refresh tokens are revoked, forcing re-login on all devices.

**Errors:** `401` wrong current password, `400` new password doesn't meet requirements.

---

### Password Reset

```
POST /api/v1/auth/password-reset-request
POST /api/v1/auth/password-reset
```
Public. The request endpoint mails a single-use token, valid **1 hour**, to the address on
file, and always answers `204` so it cannot be used to probe which addresses exist.

**Reset request:**
```json
{
  "token": "token-from-the-email",
  "newPassword": "newsecurepassword",
  "twoFactorCode": "123456"
}
```

**204 No Content** on success. The password is replaced, all refresh tokens are revoked, and
a **"Your password was changed"** notice is mailed to the account address — the signal that
makes an unauthorised reset visible to the owner.

**A reset does not disable 2FA.** It used to, which made control of the mailbox sufficient
for a full account takeover: the same inbox that receives the reset link also received the
only thing standing between an attacker and the account. Instead, an account with 2FA
enabled must prove the second factor here as well:

- `twoFactorCode` accepts a **TOTP code or an unused recovery code**. Recovery codes are the
  self-service path when the authenticator is lost, so no administrator is needed in the
  normal case.
- The reset token is consumed **only on success**, so a missing or wrong code can be retried
  with the same link.
- Wrong codes count toward the shared [account lockout](#account-lockout), which bounds
  guessing.
- Only a user who has lost the authenticator *and* every recovery code needs an admin to
  clear the enrolment via `POST /admin/users/{userId}/reset-2fa`.

Clients should submit token and password first and prompt for the code after a
`two_factor_required` response.

**Errors:**

| Status | `code` | Meaning |
|---|---|---|
| `400` | — | Invalid, expired, or already-used token; password fails the length rules |
| `401` | `two_factor_required` | Account has 2FA enabled and no code was supplied |
| `401` | `invalid_two_factor_code` | The TOTP or recovery code was wrong |
| `429` | — | Too many failed attempts — the account is temporarily locked |

---

### Account Deletion

```
DELETE /api/v1/users/me
```
Requires authentication.

**Request:**
```json
{
  "password": "currentpassword"
}
```

**204 No Content** on success. Cascades to all user data (flights, licenses, etc.).

---

## Two-Factor Authentication (TOTP)

TOTP seeds are **AES-256-GCM encrypted at rest** under a subkey of `ENCRYPTION_KEY`
(`cryptoutil.PurposeTOTPSecrets`), stored with an `enc:v1:` marker. There is no
unencrypted form: a seed is a bearer credential for someone's second factor, so a value
arriving without the marker is refused rather than read as a legacy plaintext secret.
Migration 61 cleared the enrolments written before this was mandatory — see
[UPGRADING.md](./UPGRADING.md).

### Setup

```
POST /api/v1/auth/2fa/setup
```
Requires authentication. Returns `409` if 2FA is already enabled.

**200 OK:**
```json
{
  "secret": "BASE32SECRET",
  "qrUri": "otpauth://totp/NinerLog:pilot@example.com?..."
}
```

Display the QR code or secret for the user to add to their authenticator app. 2FA is **not yet active** — the user must verify a code first.

### Verify & Enable

```
POST /api/v1/auth/2fa/verify
```
Requires authentication.

**Request:**
```json
{ "code": "123456" }
```

**200 OK:**
```json
{
  "recoveryCodes": [
    "abc12-def34",
    "ghij5-klmn6",
    "..."
  ]
}
```

Returns **8 one-time recovery codes**. These should be stored securely by the user — they cannot be retrieved again and are bcrypt-hashed at rest.

### Login with 2FA

```
POST /api/v1/auth/2fa/login
```
Public endpoint. Called after a login returns `requiresTwoFactor: true`.

**Request (TOTP code):**
```json
{
  "twoFactorToken": "eyJ...",
  "code": "123456"
}
```

**Request (recovery code):**
```json
{
  "twoFactorToken": "eyJ...",
  "code": "abc12-def34"
}
```

Recovery codes are single-use — consumed upon successful verification.

**200 OK:** Full `AuthResponse` with access and refresh tokens.

### Disable 2FA

```
POST /api/v1/auth/2fa/disable
```
Requires authentication.

**Request:**
```json
{ "password": "currentpassword" }
```

**204 No Content.** Clears the TOTP secret and all recovery codes.

---

## Passkeys (WebAuthn)

Disabled unless `WEBAUTHN_RP_ID` is set; every endpoint returns **503** otherwise.
A successful passkey login counts as two-factor and skips the 2FA challenge.

Each ceremony is two requests — `options` (begin) then `verify` (finish) — bound
together by a **single-use ceremony handle** returned as `sessionId`.

### Ceremony state

Ceremony state lives in the `webauthn_sessions` table, not in process memory, so
`options` and `verify` may be served by different instances and a ceremony
survives a restart of the process that started it.

```
options:  handle  = 16 bytes from crypto/rand, base64url (no padding)
          id_hash = sha256(handle)
          INSERT (id_hash, user_id, ceremony, data, expires_at = now() + TTL)
          → handle returned to the client as sessionId

verify:   DELETE FROM webauthn_sessions
           WHERE id_hash = sha256(sessionId)
             AND ceremony = $ceremony
             AND expires_at > now()
          RETURNING user_id, data
          0 rows → reject   1 row → verify the attestation/assertion
```

Properties this buys:

- **Exactly-once.** `DELETE ... RETURNING` consumes the row in one statement, so
  two racing `verify` calls cannot both proceed — there is no read-then-delete
  window, across replicas or within one process.
- **Unusable when expired.** The `expires_at > now()` predicate is checked on
  every read, so a stalled cleanup job can never make a stale challenge usable.
- **Nothing usable at rest.** Only `sha256(handle)` is stored; a database dump or
  read-only SQL injection yields no ceremony state. The raw handle is never logged.
- **Ceremony-bound.** A registration handle presented to `login/verify` is
  rejected, and vice versa.
- **User-scoped registration.** `register/verify` additionally requires the
  session's `user_id` to match the authenticated caller, so a stolen registration
  handle cannot attach a credential to a different account.

Expired, already-consumed, wrong-ceremony, wrong-user and never-issued handles all
return the **same** 400 — the failure modes are deliberately indistinguishable.

### Concurrent ceremonies

Sessions are keyed by handle rather than by user, so one user may hold several
ceremonies open at once (a registration on a laptop and a login on a phone) and
complete them in any order.

Open ceremonies per user are capped at `WEBAUTHN_MAX_OPEN_CEREMONIES`. Exceeding
the cap evicts the **oldest**, never rejects the newest, so a user who abandoned
earlier attempts is never locked out of the one they are making now. Discoverable
(usernameless) login has no user at `options` time, so its rows are not covered by
the cap — they are bounded by the `auth` rate limit, the TTL, and the cleanup tick.

### Endpoints

| Endpoint | Auth | Notes |
|---|---|---|
| `POST /api/v1/auth/webauthn/register/options` | Bearer | Returns `sessionId` + `publicKey` |
| `POST /api/v1/auth/webauthn/register/verify` | Bearer | Takes `sessionId`, `response`, optional `label` |
| `POST /api/v1/auth/webauthn/login/options` | Public | Omit `email` for discoverable login |
| `POST /api/v1/auth/webauthn/login/verify` | Public | Takes `sessionId`, `response` → token pair |
| `GET /api/v1/auth/webauthn/credentials` | Bearer | List registered passkeys |
| `DELETE /api/v1/auth/webauthn/credentials/{id}` | Bearer | Revoke a passkey |

Clients must hold `sessionId` in memory for the duration of the ceremony and send
it back with `verify`. It is single-use and short-lived, so it must **not** be put
in `localStorage` or `sessionStorage`.

### Configuration

| Variable | Default | Notes |
|---|---|---|
| `WEBAUTHN_RP_ID` | — | Relying Party ID; unset disables passkeys entirely |
| `WEBAUTHN_RP_NAME` | `NinerLog` | Display name shown by the authenticator |
| `WEBAUTHN_RP_ORIGINS` | `CORS_ORIGIN` | Comma-separated allowed origins |
| `WEBAUTHN_SESSION_TTL` | `5m` | Ceremony lifetime; also used as the client-side timeout, so a stored challenge never outlives the browser prompt |
| `WEBAUTHN_MAX_OPEN_CEREMONIES` | `10` | Per-user cap, oldest evicted |

A background tick deletes expired rows every 5 minutes. That is hygiene only —
correctness rests on the `expires_at` predicate above, not on the sweep running.

---

## Protected Routes

All endpoints except the following require a valid access token in the `Authorization` header:

| Public Endpoint | Description |
|---|---|
| `POST /api/v1/auth/register` | Registration |
| `POST /api/v1/auth/login` | Login |
| `POST /api/v1/auth/refresh` | Token refresh |
| `POST /api/v1/auth/2fa/login` | 2FA login |
| `GET /api/v1/auth/providers` | Which sign-in methods this server offers |
| `GET /api/v1/auth/oidc/authorize` | Start an OIDC login (503 in local mode) |
| `GET /api/v1/auth/oidc/callback` | OIDC provider redirect target (503 in local mode) |
| `POST /api/v1/auth/oidc/exchange` | Redeem the OIDC handoff code (503 in local mode) |
| `GET /api/v1/airports/search` | Airport search |
| `GET /api/v1/airports/:icaoCode` | Airport lookup |

**Header format:**
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

**401 responses:**
- `{"error": "Authentication required"}` — missing or malformed header
- `{"error": "Invalid or expired token"}` — token validation failed

---

## Rate Limiting

In-memory per-IP rate limiting (respects `X-Real-IP` / `X-Forwarded-For` from trusted proxies).

| Endpoint Group | Limit |
|---|---|
| Auth endpoints (`/auth/*`) | 10 requests / minute |
| OIDC login + `/auth/providers` | 60 requests / minute |
| Admin endpoints (`/admin/*`) | 30 requests / minute |

OIDC gets its own budget because one sign-in costs three requests (authorize,
callback, exchange), which would exhaust the 10/min auth bucket after three
logins from a single NAT address.

**429 Too Many Requests:**
```json
{ "error": "Too many requests, please try again later" }
```

Set `DISABLE_RATE_LIMIT=true` to disable (testing only).

---

## Account Lockout

Protection against brute-force login attacks:

- **Threshold:** 5 consecutive failed login attempts
- **Lock duration:** 15 minutes
- **Reset:** successful login resets the counter to 0

The counter is shared across every place a credential is checked — password login, the 2FA
challenge, and the two-factor step of a [password reset](#password-reset) — so an attacker
cannot get extra guesses by switching endpoints.

Locked accounts receive `429` with `"Account temporarily locked..."`. An admin can unlock accounts via `POST /api/v1/admin/users/{userId}/unlock`.

---

## Admin Access

Admin is determined by the `ADMIN_EMAIL` environment variable — the user whose email matches (case-insensitive) gets admin privileges.

Admin endpoints require both a valid access token and admin status. Non-admin users receive `403 "Admin access required"`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/admin/stats` | System statistics |
| `GET` | `/admin/users` | List users (search, pagination) |
| `POST` | `/admin/users/{userId}/disable` | Disable account |
| `POST` | `/admin/users/{userId}/enable` | Enable account |
| `POST` | `/admin/users/{userId}/unlock` | Unlock locked account |
| `POST` | `/admin/users/{userId}/reset-2fa` | Force-reset 2FA (last resort — authenticator *and* all recovery codes lost). Clears the secret and recovery codes, and mails the user a notice that their second factor was removed |
| `GET` | `/admin/audit-log` | View audit log |
| `GET` | `/admin/config` | View runtime config |
| `POST` | `/admin/maintenance/cleanup-tokens` | Purge expired tokens |
| `POST` | `/admin/maintenance/smtp-test` | Send test email |
| `POST` | `/admin/maintenance/trigger-notifications` | Trigger notification check |
| `POST` | `/admin/announcements` | Create announcement |
| `DELETE` | `/admin/announcements/{id}` | Delete announcement |

All admin actions are recorded in the `admin_audit_log` table.

---

## Security Hardening

**Password storage:** bcrypt, cost factor 12.

**Token storage:** Refresh tokens and password-reset tokens are stored as SHA-256 hashes — the plain tokens only exist client-side.

**Security headers** (all responses):
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Strict-Transport-Security: max-age=31536000; includeSubDomains`

**HTTP server timeouts:**
- Read: 30s, Write: 60s, Header read: 10s, Idle: 120s
- Max header size: 1 MB, Max multipart: 10 MB
