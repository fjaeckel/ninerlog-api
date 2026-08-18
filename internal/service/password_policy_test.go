package service

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     error
	}{
		// Accepted
		{"all four classes", "SecurePass123!", nil},
		{"exactly minimum length", "Abcdefghij1!", nil},
		{"exactly maximum length", strings.Repeat("aB1!", 18), nil},
		{"space as the special character", "Secure Pass123", nil},
		{"unicode letters and symbols", "Fliegerübung1§", nil},
		{"digit and symbol at the front", "1!aBcdefghijk", nil},

		// Length
		{"empty", "", ErrPasswordTooShort},
		{"one below minimum", "Abcdefghi1!", ErrPasswordTooShort},
		{"one above maximum", strings.Repeat("aB1!", 18) + "x", ErrPasswordTooLong},
		{"multibyte pushes past the byte limit", strings.Repeat("ü", 36) + "aB1!", ErrPasswordTooLong},
		// Length is checked before complexity.
		{"short and weak reports too short", "abc", ErrPasswordTooShort},

		// Complexity — each class missing in turn, all at valid length
		{"no lowercase", "ABCDEFGHIJ1!", ErrPasswordTooWeak},
		{"no uppercase", "abcdefghij1!", ErrPasswordTooWeak},
		{"no digit", "Abcdefghijk!", ErrPasswordTooWeak},
		{"no special", "Abcdefghij12", ErrPasswordTooWeak},
		{"letters only", "abcdefghijkl", ErrPasswordTooWeak},
		{"digits only", "123456789012", ErrPasswordTooWeak},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if !errors.Is(err, tt.want) {
				t.Errorf("validatePassword(%q) = %v, want %v", tt.password, err, tt.want)
			}
		})
	}
}

// A password at exactly the bcrypt ceiling is accepted; one byte more is
// rejected.
func TestValidatePasswordLengthBoundaries(t *testing.T) {
	atMax := "aB1!" + strings.Repeat("x", PasswordMaxLength-4)
	if len(atMax) != PasswordMaxLength {
		t.Fatalf("fixture is %d bytes, want %d", len(atMax), PasswordMaxLength)
	}
	if err := validatePassword(atMax); err != nil {
		t.Errorf("password of exactly %d bytes rejected: %v", PasswordMaxLength, err)
	}
	if err := validatePassword(atMax + "x"); !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("password of %d bytes = %v, want ErrPasswordTooLong", PasswordMaxLength+1, err)
	}

	atMin := "aB1!" + strings.Repeat("x", PasswordMinLength-4)
	if err := validatePassword(atMin); err != nil {
		t.Errorf("password of exactly %d bytes rejected: %v", PasswordMinLength, err)
	}
	if err := validatePassword(atMin[:len(atMin)-1]); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("password of %d bytes = %v, want ErrPasswordTooShort", PasswordMinLength-1, err)
	}
}
