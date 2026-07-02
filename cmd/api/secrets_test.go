package main

import (
	"strings"
	"testing"
)

// A valid secret: 32+ chars and not a placeholder.
const (
	goodAccessSecret  = "9f2c1a4e7b8d0f3652a1c9e4b7d80f3a" // 32 chars
	goodRefreshSecret = "3a0f8d7b4e9c2a5636f30d8b7e4a1c2f" // 32 chars, different
)

func TestValidateJWTSecrets_Valid(t *testing.T) {
	if err := validateJWTSecrets(goodAccessSecret, goodRefreshSecret); err != nil {
		t.Fatalf("validateJWTSecrets() with valid secrets returned error: %v", err)
	}
}

func TestValidateJWTSecrets_EmptyAccess(t *testing.T) {
	err := validateJWTSecrets("", goodRefreshSecret)
	if err == nil {
		t.Fatal("expected error for empty JWT_SECRET, got nil")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Errorf("error should name JWT_SECRET, got: %v", err)
	}
}

func TestValidateJWTSecrets_EmptyRefresh(t *testing.T) {
	err := validateJWTSecrets(goodAccessSecret, "")
	if err == nil {
		t.Fatal("expected error for empty REFRESH_SECRET, got nil")
	}
	if !strings.Contains(err.Error(), "REFRESH_SECRET") {
		t.Errorf("error should name REFRESH_SECRET, got: %v", err)
	}
}

func TestValidateJWTSecrets_TooShort(t *testing.T) {
	short := strings.Repeat("a", minJWTSecretLength-1)
	if err := validateJWTSecrets(short, goodRefreshSecret); err == nil {
		t.Errorf("expected error for %d-char secret (min %d), got nil", len(short), minJWTSecretLength)
	}
}

func TestValidateJWTSecrets_MinLengthBoundary(t *testing.T) {
	exact := strings.Repeat("a", minJWTSecretLength)
	other := strings.Repeat("b", minJWTSecretLength)
	if err := validateJWTSecrets(exact, other); err != nil {
		t.Errorf("secret of exactly the minimum length should be accepted, got: %v", err)
	}
}

func TestValidateJWTSecrets_RejectsPlaceholders(t *testing.T) {
	// The exact placeholder strings that used to be the hardcoded fallbacks.
	err := validateJWTSecrets(
		"change-this-secret-key-in-production",
		"change-this-refresh-secret-in-production",
	)
	if err == nil {
		t.Fatal("expected error for shipped placeholder secrets, got nil")
	}
}

func TestValidateJWTSecrets_RejectsPlaceholderEvenWhenLongEnough(t *testing.T) {
	// A placeholder is >= 32 chars, so length alone would pass; the placeholder
	// check must still reject it.
	placeholder := "change-this-secret-key-in-production"
	if len(placeholder) < minJWTSecretLength {
		t.Fatalf("test precondition: placeholder should be >= %d chars", minJWTSecretLength)
	}
	if err := validateJWTSecret("JWT_SECRET", placeholder); err == nil {
		t.Error("placeholder secret should be rejected regardless of length")
	}
}

func TestValidateJWTSecrets_RejectsIdenticalSecrets(t *testing.T) {
	err := validateJWTSecrets(goodAccessSecret, goodAccessSecret)
	if err == nil {
		t.Fatal("expected error when access and refresh secrets are identical, got nil")
	}
	if !strings.Contains(err.Error(), "different") {
		t.Errorf("error should explain secrets must differ, got: %v", err)
	}
}
