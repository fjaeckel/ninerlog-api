package cryptoutil

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func mustNewAEAD(t *testing.T) *AEAD {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	a, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	a := mustNewAEAD(t)

	plaintexts := [][]byte{
		[]byte(""),
		[]byte("hello"),
		bytes.Repeat([]byte("a"), 1024),
		[]byte(`{"access_key_id":"AKIA1234","secret_access_key":"verysecret"}`),
	}
	for _, pt := range plaintexts {
		ct, nonce, err := a.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		if len(nonce) != NonceSize {
			t.Fatalf("nonce size = %d, want %d", len(nonce), NonceSize)
		}
		got, err := a.Decrypt(ct, nonce)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatalf("round-trip mismatch: got %q want %q", got, pt)
		}
	}
}

func TestEncryptUsesFreshNonce(t *testing.T) {
	a := mustNewAEAD(t)
	pt := []byte("same plaintext")
	_, nonce1, err := a.Encrypt(pt)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, nonce2, err := a.Encrypt(pt)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(nonce1, nonce2) {
		t.Fatalf("nonce was reused between encryptions")
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	a := mustNewAEAD(t)
	ct, nonce, err := a.Encrypt([]byte("important"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Flip a bit in the middle of the ciphertext.
	bad := append([]byte(nil), ct...)
	bad[len(bad)/2] ^= 0x01
	if _, err := a.Decrypt(bad, nonce); err == nil {
		t.Fatalf("tampered ciphertext was accepted")
	}
}

func TestDecryptRejectsWrongNonce(t *testing.T) {
	a := mustNewAEAD(t)
	ct, _, err := a.Encrypt([]byte("plaintext"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	bad := make([]byte, NonceSize)
	if _, err := a.Decrypt(ct, bad); err == nil {
		t.Fatalf("wrong nonce was accepted")
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	a1 := mustNewAEAD(t)
	a2 := mustNewAEAD(t)
	ct, nonce, err := a1.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := a2.Decrypt(ct, nonce); err == nil {
		t.Fatalf("decrypt succeeded with a different key")
	}
}

func TestDecryptRejectsShortNonce(t *testing.T) {
	a := mustNewAEAD(t)
	ct, _, err := a.Encrypt([]byte("x"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := a.Decrypt(ct, []byte{0x01}); err != ErrInvalidCiphertext {
		t.Fatalf("expected ErrInvalidCiphertext, got %v", err)
	}
}

func TestNewRejectsWrongKeySize(t *testing.T) {
	for _, n := range []int{0, 1, 16, 31, 33, 64} {
		key := make([]byte, n)
		if _, err := New(key); err != ErrInvalidKey {
			t.Errorf("New(len=%d): expected ErrInvalidKey, got %v", n, err)
		}
	}
}

func TestNewFromBase64(t *testing.T) {
	key, _ := GenerateKey()
	encoded := base64.StdEncoding.EncodeToString(key)
	a, err := NewFromBase64(encoded)
	if err != nil {
		t.Fatalf("NewFromBase64: %v", err)
	}
	if a == nil {
		t.Fatalf("nil AEAD")
	}

	// Also accept URL-safe encoding.
	urlSafe := base64.URLEncoding.EncodeToString(key)
	if _, err := NewFromBase64(urlSafe); err != nil {
		t.Fatalf("NewFromBase64 (url): %v", err)
	}
}

func TestNewFromBase64Rejects(t *testing.T) {
	if _, err := NewFromBase64(""); err == nil {
		t.Fatalf("empty key was accepted")
	}
	if _, err := NewFromBase64("not-base64!"); err == nil {
		t.Fatalf("malformed base64 was accepted")
	}
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, err := NewFromBase64(short); err == nil {
		t.Fatalf("short key was accepted")
	}
}

func TestGenerateKeyBase64IsDecodable(t *testing.T) {
	encoded, err := GenerateKeyBase64()
	if err != nil {
		t.Fatalf("GenerateKeyBase64: %v", err)
	}
	if strings.TrimSpace(encoded) == "" {
		t.Fatalf("empty encoded key")
	}
	if _, err := NewFromBase64(encoded); err != nil {
		t.Fatalf("could not consume our own generated key: %v", err)
	}
}
