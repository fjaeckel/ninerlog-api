package main

import (
	"errors"
	"fmt"
)

// minJWTSecretLength is the minimum number of characters required for a JWT
// signing secret.
const minJWTSecretLength = 32

// placeholderSecrets are known public example values, rejected as signing
// secrets.
var placeholderSecrets = map[string]struct{}{
	"change-this-secret-key-in-production":     {},
	"change-this-refresh-secret-in-production": {},
}

// validateJWTSecrets enforces that the access and refresh signing secrets are
// present, long enough, not a known public placeholder, and distinct from each
// other. It returns a non-nil error describing the first problem found.
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
