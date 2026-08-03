# OIDC single sign-on

NinerLog can hand authentication to an external OpenID Connect provider —
Authentik, Keycloak, Authelia, Zitadel, Entra ID, Okta, Google, and anything
else that implements OIDC discovery.

This is **optional and off by default**. Setting `OIDC_ISSUER` is what turns it
on, and it is a *mode switch*, not an extra sign-in button.

> **In one sentence:** with `OIDC_ISSUER` set, the identity provider owns every
> account — NinerLog stops accepting passwords, stops letting anyone register,
> and stops offering TOTP and passkeys.

---

## Contents

- [Why it is a mode, not an option](#why-it-is-a-mode-not-an-option)
- [What changes when OIDC is on](#what-changes-when-oidc-is-on)
- [Quick start](#quick-start)
- [Configuration reference](#configuration-reference)
- [Provider recipes](#provider-recipes)
- [How the login flow works](#how-the-login-flow-works)
- [Account provisioning](#account-provisioning)
- [Migrating an existing deployment](#migrating-an-existing-deployment)
- [Administrators](#administrators)
- [Frontend integration](#frontend-integration)
- [Troubleshooting](#troubleshooting)
- [Security properties](#security-properties)

---

## Why it is a mode, not an option

Running OIDC *alongside* local passwords would mean every account has a second
door the identity provider cannot see. Your provider might require hardware MFA,
disable a leaving employee, or enforce a password policy — and none of that
would matter if the same person can still sign in with a NinerLog password they
set two years ago, or a passkey they registered on a personal phone.

So NinerLog runs in exactly one authentication mode:

| Mode | Enabled by | Who owns accounts |
| --- | --- | --- |
| **local** (default) | nothing — this is the default | NinerLog: passwords, optional TOTP, optional passkeys |
| **oidc** | `OIDC_ISSUER` is set | The identity provider, entirely |

`ninerlog.com` runs in local mode. Nothing in this document affects a
deployment that leaves `OIDC_ISSUER` unset.

---

## What changes when OIDC is on

**Switched off.** These all answer `503 Service Unavailable` with
`{"error": "Local authentication is disabled on this server (OIDC mode)"}`:

| Endpoint | |
| --- | --- |
| `POST /api/v1/auth/register` | no self-service sign-up |
| `POST /api/v1/auth/login` | no password login |
| `POST /api/v1/auth/verify-email`, `.../resend` | the provider vouches for the address |
| `POST /api/v1/auth/password-reset-request`, `/auth/password-reset` | password resets live at the provider |
| `POST /api/v1/auth/change-password` | there is no password to change |
| `POST /api/v1/auth/2fa/*` | second factors are the provider's job |
| `POST /api/v1/auth/webauthn/*` | passkeys are a local credential |
| `POST /api/v1/admin/users/{id}/reset-2fa` | nothing to reset |

**Switched on.**

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v1/auth/providers` | Public capability probe — which mode is this server in |
| `GET /api/v1/auth/oidc/authorize` | Browser redirect that starts a login |
| `GET /api/v1/auth/oidc/callback` | Where the provider sends the browser back |
| `POST /api/v1/auth/oidc/exchange` | Swaps the one-time handoff code for tokens |

**Unchanged.** Everything else: `POST /auth/refresh`, `POST /auth/logout`, the
whole logbook API, admin endpoints, exports, backups, instructor signing links.
Access and refresh tokens work exactly as they do in local mode — OIDC only
replaces how the *first* token is obtained.

**Changed in shape.**

- `PATCH /api/v1/users/me` refuses `name` and `email` with `403` — both come
  from the ID token and are re-applied on every login. Display preferences
  (time format, date format, columns, …) stay editable.
- `DELETE /api/v1/users/me` takes `confirmEmail` (the account's own address,
  typed out) instead of `password`.

---

## Quick start

Register NinerLog as a **confidential** client at your provider, with the
authorization-code grant and this exact redirect URI:

```
https://<your-api-host>/api/v1/auth/oidc/callback
```

Then set five variables:

```bash
OIDC_ISSUER=https://id.example.com/application/o/ninerlog/
OIDC_CLIENT_ID=ninerlog
OIDC_CLIENT_SECRET=<from your provider>
OIDC_REDIRECT_URL=https://api.ninerlog.example/api/v1/auth/oidc/callback
OIDC_POST_LOGIN_REDIRECT=https://ninerlog.example/auth/callback
```

Restart. The startup log confirms the mode:

```
level=INFO msg="OIDC mode enabled — local passwords, registration, 2FA and passkeys are disabled"
  issuer=https://id.example.com/application/o/ninerlog/ provider_name="Single sign-on" scopes="openid profile email"
```

A misconfiguration is fatal at startup rather than silent: if `OIDC_ISSUER` is
set and any other required value is missing or malformed, the process exits with
the offending variable named. Half-configured is the one state that would leave
you unable to sign in at all.

The provider does **not** need to be reachable when NinerLog starts. Discovery
happens on the first login attempt and is retried, so booting NinerLog and your
identity provider in the same compose stack works regardless of order.

---

## Configuration reference

### Required

| Variable | Description |
| --- | --- |
| `OIDC_ISSUER` | Issuer URL. Its presence enables OIDC mode. Must be the exact value the provider puts in the `iss` claim — usually the base URL whose `/.well-known/openid-configuration` resolves. No query string or fragment. |
| `OIDC_CLIENT_ID` | Client ID from the provider. |
| `OIDC_CLIENT_SECRET` | Client secret. NinerLog is a confidential client; the secret never reaches the browser. |
| `OIDC_REDIRECT_URL` | This API's own callback, absolute: `https://<api-host>/api/v1/auth/oidc/callback`. Must match the provider's registration byte for byte. |
| `OIDC_POST_LOGIN_REDIRECT` | Where the browser lands after login — a frontend URL. Fixed here and never taken from the request, so the callback cannot be abused as an open redirect. |

### Optional

| Variable | Default | Description |
| --- | --- | --- |
| `OIDC_PROVIDER_NAME` | `Single sign-on` | Label on the sign-in button. Set it to your provider's name ("Authentik", "Company SSO"). |
| `OIDC_SCOPES` | `openid profile email` | Comma- or space-separated. `openid` is added if you leave it out. Add scopes only if your provider needs them to release the email claim. |
| `OIDC_NAME_CLAIM` | `name` | Claim used as the pilot's display name. Falls back to `name`, then `preferred_username`, then the local part of the address. |
| `OIDC_LINK_BY_VERIFIED_EMAIL` | `false` | Let a first OIDC login adopt an existing local account with the same address. See [Migrating](#migrating-an-existing-deployment) — read that before switching it on. |
| `OIDC_TRUST_EMAIL_VERIFIED` | `false` | Treat provisioned addresses as verified even when the ID token carries no `email_verified` claim. Needed for providers that omit it, and **required for `ADMIN_EMAIL` to work** with such a provider. |
| `OIDC_LOGIN_STATE_TTL` | `10m` | How long a started login stays completable — the window in which the user types their password and passes MFA at the provider. |
| `OIDC_HANDOFF_TTL` | `60s` | How long the code in the post-login redirect stays redeemable. One automatic request; seconds are enough. |

Booleans accept `true`, `1`, `yes`, `on` (case-insensitive). Anything else,
including a typo, is `false` — a security-relevant switch never turns itself on
by accident. Durations are Go duration strings (`90s`, `10m`, `2h`); an
unparseable or non-positive value is a startup error, not a silent fallback.

### Variables that keep working

`ADMIN_EMAIL`, `CORS_ORIGIN`, `FRONTEND_URL`, SMTP settings (still used for
notifications and instructor signing emails), `BACKUP_CREDENTIALS_KEY`,
`METRICS_ENABLED`, and the rate-limit knobs are all unaffected.

`WEBAUTHN_RP_ID`, `WEBAUTHN_*` and `TOTP_ENCRYPTION_KEY` are ignored in OIDC
mode; the subsystems they configure are not started.

---

## Provider recipes

Values below are the ones that differ per provider. All of them additionally
need `OIDC_REDIRECT_URL` and `OIDC_POST_LOGIN_REDIRECT` as in the quick start.

### Authentik

Create an OAuth2/OpenID Provider plus an Application. Redirect URI must be an
exact match, not a regex.

```bash
OIDC_ISSUER=https://auth.example.com/application/o/ninerlog/
OIDC_CLIENT_ID=<Client ID>
OIDC_CLIENT_SECRET=<Client Secret>
OIDC_PROVIDER_NAME=Authentik
```

Authentik emits `email_verified`, so no extra flags are needed.

### Keycloak

```bash
OIDC_ISSUER=https://kc.example.com/realms/ninerlog
OIDC_CLIENT_ID=ninerlog
OIDC_CLIENT_SECRET=<from the Credentials tab>
OIDC_PROVIDER_NAME=Keycloak
```

Set the client to *Client authentication: On* (confidential) with the standard
flow enabled. Keycloak's `email_verified` mirrors the user's verified flag in
the realm — users created by an admin without verification will land in NinerLog
unverified, which matters only for `ADMIN_EMAIL`.

### Authelia

```bash
OIDC_ISSUER=https://auth.example.com
OIDC_CLIENT_ID=ninerlog
OIDC_CLIENT_SECRET=<the plaintext of the configured hash>
OIDC_PROVIDER_NAME=Authelia
OIDC_TRUST_EMAIL_VERIFIED=true
```

Authelia does not emit `email_verified`; addresses come from your user store and
are as trustworthy as that store. `OIDC_TRUST_EMAIL_VERIFIED=true` is the usual
setting here.

### Zitadel

```bash
OIDC_ISSUER=https://your-instance.zitadel.cloud
OIDC_CLIENT_ID=<Client ID>
OIDC_CLIENT_SECRET=<Client Secret>
OIDC_PROVIDER_NAME=Zitadel
```

Choose application type *Web* with *Code* grant and PKCE — NinerLog always sends
a PKCE challenge.

### Microsoft Entra ID

```bash
OIDC_ISSUER=https://login.microsoftonline.com/<tenant-id>/v2.0
OIDC_CLIENT_ID=<Application (client) ID>
OIDC_CLIENT_SECRET=<client secret value>
OIDC_PROVIDER_NAME=Microsoft
OIDC_TRUST_EMAIL_VERIFIED=true
```

Entra does not send `email_verified`. It also does not always send `email`:
add the `email` optional claim to the ID token in *Token configuration*, or
logins fail with `email_missing`.

### Google Workspace

```bash
OIDC_ISSUER=https://accounts.google.com
OIDC_CLIENT_ID=<...>.apps.googleusercontent.com
OIDC_CLIENT_SECRET=<...>
OIDC_PROVIDER_NAME=Google
```

Google emits `email_verified`. Restrict which accounts may sign in at Google —
NinerLog provisions anyone the provider lets through.

> **Any provider is a full-trust component.** Whoever your provider lets in gets
> a NinerLog logbook. Restrict the application to the right group or realm there;
> NinerLog has no allow-list of its own.

---

## How the login flow works

```
browser                    NinerLog API                     provider
   │                            │                               │
   │ GET /auth/oidc/authorize   │                               │
   ├───────────────────────────►│  mint state + nonce + PKCE    │
   │                            │  store hash(state), set cookie│
   │◄── 302 ────────────────────┤                               │
   │                            │                               │
   ├────────────── authorization request ─────────────────────► │
   │                            │            user authenticates │
   │◄───────────── 302 back with ?code&state ───────────────────┤
   │                            │                               │
   │ GET /auth/oidc/callback    │                               │
   ├───────────────────────────►│  consume state (once)         │
   │                            │  check cookie binding         │
   │                            ├── POST /token (+ verifier) ──►│
   │                            │◄────────── id_token ──────────┤
   │                            │  verify sig/iss/aud/exp/nonce │
   │                            │  provision or update the user │
   │◄── 302 ?oidc_code=… ───────┤  mint one-time handoff code   │
   │                            │                               │
   │ POST /auth/oidc/exchange   │                               │
   ├───────────────────────────►│  consume code (once)          │
   │◄── access + refresh token ─┤                               │
```

The final hop exists so that access and refresh tokens never appear in a URL,
where they would end up in browser history, the `Referer` header, and every
reverse-proxy access log. The redirect carries a code that is single-use and
expires in a minute; the tokens themselves come back over `POST`.

State, nonce and PKCE verifier each defend a different step and are all
independent 256-bit random values:

- **state** — proves the callback belongs to a login this server started, and is
  consumed with `DELETE … RETURNING` so a replayed callback URL fails.
- **browser cookie** — `ninerlog_oidc_state`, `HttpOnly`, `SameSite=Lax`, scoped
  to `/api/v1/auth/oidc`. Binds the login to the browser that began it, so an
  attacker cannot complete their own authorization inside a victim's browser and
  silently sign them into the attacker's account (login CSRF).
- **nonce** — embedded in the ID token, proves it was minted for *this* login.
- **PKCE verifier** — proves the code is redeemed by the party that requested it.

---

## Account provisioning

Accounts are created on first login and are keyed by `(issuer, sub)` — never by
email. Email addresses change; `sub` does not, and matching on the address would
let anyone able to set their own email at the provider claim someone else's
logbook.

On **first** login NinerLog creates a user with:

| Field | From |
| --- | --- |
| `email` | the `email` claim (required — login fails with `email_missing` without it) |
| `name` | `OIDC_NAME_CLAIM`, else `name`, else `preferred_username`, else the address's local part |
| `email_verified` | the `email_verified` claim, or `true` when `OIDC_TRUST_EMAIL_VERIFIED=true` |
| password | none — the hash is empty and can never validate |

On **every subsequent** login, `email`, `name` and `email_verified` are
re-applied from the ID token. A rename or address change at the provider lands
in NinerLog at the user's next sign-in. (If the new address is already taken by
a different local account, the change is skipped and logged rather than failing
the login.)

Accounts an administrator has disabled (`POST /admin/users/{id}/disable`) are
refused, both while provisioning and again when the handoff code is redeemed, so
disabling takes effect even mid-login.

**Deprovisioning is not automatic.** Removing a user at the provider stops them
signing in; it does not delete their NinerLog logbook. Delete the account
explicitly via `DELETE /api/v1/admin/users/{id}` if that is what you want.

---

## Migrating an existing deployment

Turning on OIDC where local accounts already exist needs one decision: whether
an incoming OIDC identity may **adopt** a pre-existing account with the same
address.

By default it may not. A first login for `pilot@example.com` creates a *new,
empty* logbook even when a local account with that address exists — and if the
address is taken, the login fails with `email_conflict` rather than silently
splitting the user's data. That is the safe default: with a provider where users
choose their own email address, adoption would be an account-takeover path.

To migrate real accounts, set:

```bash
OIDC_LINK_BY_VERIFIED_EMAIL=true
```

A first OIDC login then adopts the existing account when **both** hold:

1. the operator has opted in with this flag, and
2. the ID token asserts `email_verified: true` for the address.

Only enable this if you trust your provider's address verification — that is, if
users cannot set an arbitrary email on their own profile there. Once every user
has signed in once, the identity link is recorded permanently and you can set
the flag back to `false`.

Recommended sequence:

1. Announce a cutover window; local logins stop working the moment you deploy.
2. Ensure every user's provider address matches their NinerLog address exactly.
3. Deploy with `OIDC_LINK_BY_VERIFIED_EMAIL=true`.
4. Have everyone sign in once.
5. Optionally set the flag back to `false`.

Old password hashes, TOTP secrets and passkeys stay in the database untouched
and unusable — every endpoint that could use them is closed. If you ever switch
back to local mode, those credentials work again, and OIDC-provisioned accounts
(which have no password) simply cannot log in until you give them one.

---

## Administrators

Admin rights still come from `ADMIN_EMAIL`, matched against the account's email —
which in OIDC mode is the address from the ID token.

Admin status additionally requires the address to be **verified**. With a
provider that emits `email_verified: true` this is automatic. With one that
omits the claim you must set `OIDC_TRUST_EMAIL_VERIFIED=true`, otherwise the
admin account is provisioned unverified and `ADMIN_EMAIL` has no effect.

`GET /api/v1/admin/config` reports `authMode` and `oidcIssuer` so you can
confirm which mode a running instance is in. The client secret is never exposed
there or anywhere else in the API.

---

## Frontend integration

The client discovers the mode before showing any sign-in UI:

```http
GET /api/v1/auth/providers
```

```json
{
  "mode": "oidc",
  "passwordLoginEnabled": false,
  "registrationEnabled": false,
  "twoFactorEnabled": false,
  "webauthnEnabled": false,
  "oidc": { "enabled": true, "name": "Authentik", "authorizeUrl": "/api/v1/auth/oidc/authorize" }
}
```

Render a single "Sign in with {name}" button that **navigates** to
`authorizeUrl` — a top-level navigation, not `fetch`. The cookie and the
provider redirect both require a real browser navigation.

At `OIDC_POST_LOGIN_REDIRECT`, read the query string:

- `?oidc_code=…` → `POST /api/v1/auth/oidc/exchange` with `{"code": "…"}`, store
  the returned token pair exactly as after a password login, and clear the query
  parameter from the URL.
- `?oidc_error=…` → show a sign-in failure. Codes: `provider_error`,
  `provider_unavailable`, `invalid_state`, `email_missing`, `email_conflict`,
  `account_disabled`, `login_failed`. They are deliberately coarse; the detail is
  in the server log and in the `auth_oidc_login_attempts_total` metric.

Logout is unchanged (`POST /auth/logout` revokes the refresh token). It ends the
NinerLog session only — the provider session is untouched, so a user who clicks
"sign in" again may be signed straight back in without a prompt. RP-initiated
logout at the provider is not implemented.

---

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Startup exits: `OIDC_CLIENT_ID is required…` | Issuer set, rest missing | Set every required variable |
| `503` on the OIDC endpoints | `OIDC_ISSUER` unset — the server is in local mode | Check the startup log line |
| `503` "The identity provider is currently unreachable" | Discovery failed | Check the issuer URL resolves from *inside* the container; try `curl $OIDC_ISSUER/.well-known/openid-configuration` |
| `?oidc_error=invalid_state` | Expired login, a replayed callback URL, or the cookie was dropped | Retry from the sign-in button. If persistent: check the cookie survives your proxy, and that `OIDC_REDIRECT_URL` is https when the site is https |
| `?oidc_error=email_missing` | No usable `email` claim | Add the `email` scope, or configure the claim at the provider (see the Entra recipe) |
| `?oidc_error=email_conflict` | The address belongs to an existing local account | See [Migrating](#migrating-an-existing-deployment) |
| `?oidc_error=account_disabled` | An admin disabled the account | `POST /admin/users/{id}/enable` |
| `?oidc_error=login_failed` after entering credentials | ID token failed verification, or the code exchange was rejected | Check the server log — signature, issuer, audience and nonce failures are logged with the reason |
| Provider reports `redirect_uri_mismatch` | `OIDC_REDIRECT_URL` differs from the registration | They must match exactly, including scheme, port and trailing path |
| Admin rights not granted | Address provisioned unverified | Set `OIDC_TRUST_EMAIL_VERIFIED=true`, or make the provider emit `email_verified` |

Metrics: `auth_oidc_login_attempts_total{result}` counts each step —
`authorize`, `callback_success`, `success`, and every failure reason. It is the
fastest way to see whether failures are happening at the provider, at
verification, or at the exchange.

---

## Security properties

What the implementation guarantees, and where to verify each claim:

| Property | Where |
| --- | --- |
| Authorization code flow with PKCE (S256) on every login | `internal/service/oidc.go` — `BeginLogin` |
| ID token signature, issuer, audience and expiry verified against the provider's JWKS | `CompleteCallback` via `go-oidc`'s verifier |
| Nonce compared in constant time; a token minted for another login is rejected | `CompleteCallback` |
| `state` consumed exactly once (`DELETE … RETURNING`), expiry enforced on read | `internal/repository/postgres/oidc.go` |
| State bound to the originating browser by an HttpOnly, SameSite=Lax cookie (login CSRF) | `internal/api/handlers/oidc.go` |
| Only SHA-256 hashes of state, browser token and handoff code are stored — a database dump yields nothing replayable | migration `000052` |
| Tokens never travel in a URL; the redirect carries a single-use 60-second code | `OIDCCallback` → `ExchangeOidcCode` |
| Redirect target comes from configuration, never from the request — no open redirect | `redirectOIDC` |
| Identity matched on `(issuer, sub)`, never on email | `provisionUser` |
| Adopting an existing local account requires an operator opt-in *and* a verified-email assertion | `provisionUser` |
| Disabled accounts refused at provisioning and again at code redemption | `provisionUser`, `ExchangeHandoff` |
| OIDC accounts have an empty password hash, which bcrypt can never match; local login also refuses them explicitly, in constant time | `AuthService.Login` |
| Client secret never returned by any endpoint, including `/admin/config` | `GetAuthProviders`, `GetAdminConfig` |
| Browser-facing errors are coarse codes; detail stays in server logs | `oidcErrorCode` |

Test coverage for the flow, including the negative cases (wrong signing key,
wrong audience, expired token, replayed state, foreign browser, replayed handoff
code), lives in `internal/service/oidc_test.go` against a fake provider. The
"every local endpoint is closed" rule is locked in by
`internal/api/handlers/oidc_mode_test.go`.

---

See also: [AUTHENTICATION.md](./AUTHENTICATION.md) for the token architecture
that OIDC feeds into, and [API.md](./API.md) for the endpoint reference.
