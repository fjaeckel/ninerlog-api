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

// dummyHash is a precomputed bcrypt hash consumed by DummyCompare; it never
// matches.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("ninerlog-timing-equalizer"), bcryptCost)

// DummyCompare performs a throwaway bcrypt comparison for the user-not-found
// login path.
func DummyCompare() {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte("ninerlog-timing-equalizer-x"))
}

// HashToken hashes a token using SHA-256 for storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
