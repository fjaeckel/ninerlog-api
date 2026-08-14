package service

import "unicode"

// Local password policy, shared by every flow that sets a password:
// registration, password reset and password change.
const (
	// PasswordMinLength is the shortest accepted password.
	PasswordMinLength = 12
	// PasswordMaxLength is the longest accepted password. bcrypt silently
	// truncates input beyond 72 bytes, so anything longer would hash to the
	// same digest as its 72-byte prefix.
	PasswordMaxLength = 72
)

// validatePassword enforces the local password policy: the length bounds above
// plus at least one lowercase letter, one uppercase letter, one digit and one
// special character. A special character is any rune that is neither a letter
// nor a number, so punctuation, symbols and spaces all qualify.
//
// Length is measured in bytes, matching bcrypt's own limit — a multi-byte rune
// therefore counts for more than one character.
//
// The frontend mirrors these rules in src/lib/passwordStrength.ts to drive its
// strength meter; keep the two in step.
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
