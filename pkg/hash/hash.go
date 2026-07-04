package hash

import (
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// HashPassword hashes a plain text password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ComparePassword compares a hashed password with a plain text password
func ComparePassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// dummyHash is a precomputed bcrypt hash used only to burn a comparable amount
// of CPU when authenticating a non-existent account, so the "unknown user" and
// "wrong password" paths take similar time. It is never expected to match.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("ninerlog-timing-equalizer"), bcryptCost)

// DummyCompare performs a throwaway bcrypt comparison. Call it on the
// user-not-found login path so an attacker cannot distinguish a missing account
// from a wrong password by response timing (user enumeration, CWE-204).
func DummyCompare() {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte("ninerlog-timing-equalizer-x"))
}

// HashToken hashes a token using SHA-256 for storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
