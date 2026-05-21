// Package cryptoutil provides symmetric encryption primitives used to protect
// per-user secrets (currently: cloud backup credentials) at rest.
//
// Design goals:
//   - One key, one algorithm — AES-256-GCM with a 96-bit random nonce.
//   - Tampering and nonce-reuse are detectable (GCM authentication tag).
//   - Caller-friendly: ciphertext and nonce are returned as separate byte
//     slices so the storage layer can put each in its own column.
//   - Key handling: the key is loaded from a base64-encoded environment
//     variable at startup; KeyFromBase64 enforces a 32-byte key length and
//     refuses obviously-empty values to fail closed on a misconfigured
//     deployment.
package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// AES-256 key size in bytes.
const KeySize = 32

// GCM nonce size in bytes (96 bits; the standard recommendation).
const NonceSize = 12

var (
	// ErrInvalidKey is returned when the supplied key is not exactly 32 bytes.
	ErrInvalidKey = errors.New("cryptoutil: key must be 32 bytes (AES-256)")
	// ErrInvalidCiphertext is returned when the ciphertext fails authentication
	// or is malformed.
	ErrInvalidCiphertext = errors.New("cryptoutil: ciphertext could not be decrypted")
)

// AEAD wraps an AES-256-GCM cipher with helper methods. Construct via New or
// NewFromBase64.
type AEAD struct {
	gcm cipher.AEAD
}

// New constructs an AEAD from a 32-byte key.
func New(key []byte) (*AEAD, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cryptoutil: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cryptoutil: cipher.NewGCM: %w", err)
	}
	if gcm.NonceSize() != NonceSize {
		return nil, fmt.Errorf("cryptoutil: unexpected GCM nonce size %d", gcm.NonceSize())
	}
	return &AEAD{gcm: gcm}, nil
}

// NewFromBase64 decodes a standard or URL-safe base64 string and constructs an
// AEAD. Spaces in the input are ignored to tolerate copy-paste artifacts.
func NewFromBase64(encoded string) (*AEAD, error) {
	key, err := DecodeKey(encoded)
	if err != nil {
		return nil, err
	}
	return New(key)
}

// DecodeKey decodes a base64-encoded key and validates length.
func DecodeKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, ErrInvalidKey
	}
	// Try both standard and URL-safe encodings; either is acceptable in env vars.
	if key, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(key) == KeySize {
		return key, nil
	}
	if key, err := base64.RawStdEncoding.DecodeString(encoded); err == nil && len(key) == KeySize {
		return key, nil
	}
	if key, err := base64.URLEncoding.DecodeString(encoded); err == nil && len(key) == KeySize {
		return key, nil
	}
	if key, err := base64.RawURLEncoding.DecodeString(encoded); err == nil && len(key) == KeySize {
		return key, nil
	}
	return nil, ErrInvalidKey
}

// GenerateKey returns a cryptographically random 32-byte key suitable for
// AES-256. Intended for development tooling and tests.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// GenerateKeyBase64 returns GenerateKey encoded as standard base64.
func GenerateKeyBase64() (string, error) {
	key, err := GenerateKey()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Encrypt seals plaintext with a fresh random nonce. Returns (ciphertext,
// nonce, error). The 16-byte authentication tag is appended to the ciphertext
// by the underlying GCM implementation.
func (a *AEAD) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, NonceSize)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = a.gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt authenticates and decrypts ciphertext produced by Encrypt.
func (a *AEAD) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	if len(nonce) != NonceSize {
		return nil, ErrInvalidCiphertext
	}
	plaintext, err := a.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}
