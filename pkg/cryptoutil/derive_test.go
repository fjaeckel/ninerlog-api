package cryptoutil

import (
	"bytes"
	"testing"
)

func testMaster(t *testing.T) []byte {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return key
}

func TestDeriveKeyIsDeterministic(t *testing.T) {
	master := testMaster(t)

	first, err := DeriveKey(master, PurposeDocumentFile)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	second, err := DeriveKey(master, PurposeDocumentFile)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}

	// A restart must produce the same subkey, or every stored value becomes
	// unreadable the moment the process is restarted.
	if !bytes.Equal(first, second) {
		t.Fatal("same master and purpose produced different subkeys")
	}
	if len(first) != KeySize {
		t.Fatalf("subkey length = %d, want %d", len(first), KeySize)
	}
}

func TestDeriveKeySeparatesPurposes(t *testing.T) {
	master := testMaster(t)

	// Every purpose the server uses, checked pairwise: one key in the
	// environment stands behind all of them, and the whole point is that none
	// of them is the same key as another, or as the master.
	purposes := []string{PurposeTOTPSecrets, PurposeBackupCredentials, PurposeDocumentFile}
	seen := make(map[string]string, len(purposes))
	for _, purpose := range purposes {
		key, err := DeriveKey(master, purpose)
		if err != nil {
			t.Fatalf("DeriveKey(%s): %v", purpose, err)
		}
		if bytes.Equal(key, master) {
			t.Fatalf("%s: subkey equals the master key", purpose)
		}
		if other, clash := seen[string(key)]; clash {
			t.Fatalf("%s and %s derived the same key — domain separation is not happening", purpose, other)
		}
		seen[string(key)] = purpose
	}
}

func TestDeriveKeyDiffersPerMaster(t *testing.T) {
	a, err := DeriveKey(testMaster(t), PurposeDocumentFile)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	b, err := DeriveKey(testMaster(t), PurposeDocumentFile)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("different masters derived the same subkey")
	}
}

func TestDeriveKeyRejectsBadInput(t *testing.T) {
	master := testMaster(t)

	if _, err := DeriveKey(master[:16], PurposeDocumentFile); err != ErrInvalidKey {
		t.Fatalf("short master: err = %v, want ErrInvalidKey", err)
	}
	if _, err := DeriveKey(nil, PurposeDocumentFile); err != ErrInvalidKey {
		t.Fatalf("nil master: err = %v, want ErrInvalidKey", err)
	}
	if _, err := DeriveKey(master, ""); err == nil {
		t.Fatal("empty purpose was accepted")
	}
}

// A value sealed under one purpose must not open under another. This is the
// property the whole derivation scheme exists to provide: leaking the subkey
// that protects licence scans must not also unlock 2FA secrets.
func TestDerivedAEADsCannotOpenEachOthersData(t *testing.T) {
	master := testMaster(t)

	files, err := DeriveAEAD(master, PurposeDocumentFile)
	if err != nil {
		t.Fatalf("DeriveAEAD: %v", err)
	}
	totp, err := DeriveAEAD(master, PurposeTOTPSecrets)
	if err != nil {
		t.Fatalf("DeriveAEAD: %v", err)
	}

	ciphertext, nonce, err := files.Encrypt([]byte("a licence scan"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := totp.Decrypt(ciphertext, nonce); err != ErrInvalidCiphertext {
		t.Fatalf("cross-purpose decrypt: err = %v, want ErrInvalidCiphertext", err)
	}

	plaintext, err := files.Decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatalf("same-purpose decrypt: %v", err)
	}
	if string(plaintext) != "a licence scan" {
		t.Fatalf("round trip = %q", plaintext)
	}
}
