# E2E Test Regressions

Issues discovered by the e2e test suite running against the real API.
Fixed regressions are moved to the "Resolved" section below.

## Resolved (Phase 6b¾)

All regressions below were fixed in the Phase 6b¾ implementation.

### ~~1. No password length validation on registration~~ — FIXED
- Backend: `AuthService.Register()` now validates password 12-72 chars
- Frontend: RegisterPage and ProfilePage both enforce 12-char minimum

### ~~2. No required field validation on registration~~ — FIXED
- Backend: `AuthService.Register()` validates email, password, name are non-empty

### ~~3. Empty name accepted on registration~~ — FIXED
- Backend: `AuthService.Register()` trims and validates name

### ~~4. Email not normalized to lowercase~~ — FIXED
- Backend: Email normalized via `strings.ToLower()` in Register, Login, RequestPasswordReset, UpdateUser

### ~~5. License creation with missing fields returns 500~~ — FIXED
- Backend: `License.Validate()` returns descriptive errors, handler maps `ErrInvalidLicense` → 400
- Frontend: LicenseForm now shows error banner instead of silent `console.error`

### ~~6. DELETE /users/me/data returns 200 instead of 204~~ — NOT A BUG
- OpenAPI spec intentionally declares `200` with JSON response body. Closed.

### ~~7. Very long email (>255 chars) causes 500~~ — FIXED
- Backend: `AuthService.Register()` validates `len(email) <= 255`

### ~~8. Very long password (>72 bytes) causes 500~~ — FIXED
- Backend: `AuthService.Register()` validates `len(password) <= 72` (bcrypt limit)

### ~~9. Credential accepts expiry date before issue date~~ — FIXED
- Backend: `CredentialService.CreateCredential()` validates expiry > issue date
- Frontend: CredentialForm Zod schema adds `.refine()` for date ordering

### ~~10. Email update to existing email causes 500~~ — FIXED
- Backend: `AuthService.UpdateUser()` catches `ErrDuplicateEmail` → `ErrUserAlreadyExists`
- Handler maps to 409 Conflict
- Frontend: ProfilePage shows specific message for 409
