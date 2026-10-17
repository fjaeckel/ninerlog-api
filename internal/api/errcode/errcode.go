// Package errcode holds the machine-readable `code` values of the Error
// envelope. Clients branch on these, never on the English `error` sentence,
// so a value here is a published contract: never rename or reuse one.
//
// Every 401 carries a code, and the codes split into two kinds a client must
// treat differently:
//
//   - SessionInvalid: the bearer token or its session was refused. Refresh
//     once and replay; a second refusal ends the session.
//   - Everything else: a credential presented in the request itself was
//     refused (a password, passkey, sign-in code, refresh token, 2FA code or a
//     third-party storage credential). The session, if any, is untouched, so
//     refreshing cannot help and the client must not sign the user out —
//     except InvalidRefreshToken, which means the session cannot be renewed.
package errcode

const (
	// SessionInvalid — missing, malformed, expired or revoked access token,
	// or the account behind it is disabled or deleted.
	SessionInvalid = "session_invalid"

	// InvalidCredentials — the user's own password or passkey was refused
	// (login, change password, change email, delete account, disable 2FA,
	// passkey login).
	InvalidCredentials = "invalid_credentials" // #nosec G101 -- a public error-code label sent to clients, not a credential.

	// InvalidRefreshToken — the refresh token is unknown, expired, revoked or
	// reused. The session cannot be renewed; sign in again.
	InvalidRefreshToken = "invalid_refresh_token"

	// InvalidSignInCode — the one-time OIDC hand-off code is unknown, used or
	// expired.
	InvalidSignInCode = "invalid_sign_in_code"

	// InvalidTwoFactorToken — the temporary 2FA challenge token from login is
	// invalid or expired; restart the login.
	InvalidTwoFactorToken = "invalid_two_factor_token"

	// TwoFactorRequired — the account has 2FA enabled and no code was given.
	TwoFactorRequired = "two_factor_required"

	// InvalidTwoFactorCode — the TOTP or recovery code was wrong.
	InvalidTwoFactorCode = "invalid_two_factor_code"

	// InvalidProviderCredentials — a cloud-backup provider (S3, WebDAV, …)
	// refused the credentials stored on the destination. Nothing about the
	// NinerLog session is wrong.
	InvalidProviderCredentials = "invalid_provider_credentials"
)
