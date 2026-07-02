package main

import (
	"errors"
	"fmt"
)

// minJWTSecretLength is the minimum number of characters required for a JWT
// signing secret. 32 bytes (256 bits) matches the HMAC-SHA256 output size and
// is the smallest value that does not weaken the MAC.
const minJWTSecretLength = 32

// placeholderSecrets are example values that have shipped in the repository or
// documentation. They are public and must never be used to sign tokens.
var placeholderSecrets = map[string]struct{}{
	"change-this-secret-key-in-production":     {},
	"change-this-refresh-secret-in-production": {},
}

// validateJWTSecrets enforces that the access and refresh signing secrets are
// present, long enough, not a known public placeholder, and distinct from each
// other. It returns a non-nil error describing the first problem found so the
// caller can fail closed at startup instead of silently signing tokens with a
// guessable key.
func validateJWTSecrets(accessSecret, refreshSecret string) error {
	if err := validateJWTSecret("JWT_SECRET", accessSecret); err != nil {
		return err
	}
	if err := validateJWTSecret("REFRESH_SECRET", refreshSecret); err != nil {
		return err
	}
	if accessSecret == refreshSecret {
		return errors.New("JWT_SECRET and REFRESH_SECRET must be set to different values")
	}
	return nil
}

func validateJWTSecret(name, value string) error {
	if len(value) < minJWTSecretLength {
		return fmt.Errorf("%s must be set to a random value of at least %d characters", name, minJWTSecretLength)
	}
	if _, isPlaceholder := placeholderSecrets[value]; isPlaceholder {
		return fmt.Errorf("%s must not use the example placeholder value; generate a strong random secret", name)
	}
	return nil
}
