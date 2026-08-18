package service

import "unicode"

// Local password policy, shared by every flow that sets a password:
// registration, password reset and password change.
const (
	// PasswordMinLength is the shortest accepted password.
	PasswordMinLength = 12
	// PasswordMaxLength is the longest accepted password (bcrypt's 72-byte
	// input limit).
	PasswordMaxLength = 72
)

// validatePassword enforces the local password policy: the length bounds
// above (measured in bytes) plus at least one lowercase letter, one uppercase
// letter, one digit and one special character (any rune that is neither a
// letter nor a number). Mirrored by the frontend in
// src/lib/passwordStrength.ts; keep the two in step.
func validatePassword(password string) error {
	if len(password) < PasswordMinLength {
		return ErrPasswordTooShort
	}
	if len(password) > PasswordMaxLength {
		return ErrPasswordTooLong
	}

	var hasLower, hasUpper, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r) || unicode.IsTitle(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case !unicode.IsLetter(r) && !unicode.IsNumber(r):
			hasSpecial = true
		}
	}

	if !hasLower || !hasUpper || !hasDigit || !hasSpecial {
		return ErrPasswordTooWeak
	}
	return nil
}
