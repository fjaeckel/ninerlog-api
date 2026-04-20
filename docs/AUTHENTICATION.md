# Authentication & Authorization

NinerLog API uses JWT-based authentication with optional TOTP two-factor authentication, account lockout protection, and per-IP rate limiting.

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

**Errors:** `401` invalid credentials, `403` account disabled, `429` account locked (too many failed attempts).

Login deletes all prior refresh tokens for the user, enforcing a **single active session**.

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

## Protected Routes

All endpoints except the following require a valid access token in the `Authorization` header:

| Public Endpoint | Description |
|---|---|
| `POST /api/v1/auth/register` | Registration |
| `POST /api/v1/auth/login` | Login |
| `POST /api/v1/auth/refresh` | Token refresh |
| `POST /api/v1/auth/2fa/login` | 2FA login |
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
| Admin endpoints (`/admin/*`) | 30 requests / minute |

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
| `POST` | `/admin/users/{userId}/reset-2fa` | Force-reset 2FA |
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
