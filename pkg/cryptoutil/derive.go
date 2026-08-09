package cryptoutil

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
)

// Purposes for DeriveKey. Each names one place encrypted data is stored, and
// each string is baked into stored ciphertext forever: changing one makes every
// value sealed under the old label unreadable. Add new purposes; never edit an
// existing one. The trailing version lets a purpose be re-keyed deliberately —
// bump it and the old data becomes undecryptable, which is a migration, not a
// rename.
// These are labels, not key material. They are public by design — the same
// three strings ship in every build — and knowing one buys nothing without the
// master key it is combined with. gosec's G101 flags them anyway, on the name
// alone.
const (
	PurposeTOTPSecrets       = "ninerlog/totp-secrets/v1"       // #nosec G101 -- an HKDF label naming what a subkey is for, not a secret
	PurposeBackupCredentials = "ninerlog/backup-credentials/v1" // #nosec G101 -- as above
	PurposeDocumentFile      = "ninerlog/document-files/v1"
)

// DeriveKey returns a 32-byte subkey for one purpose, derived from a master key
// with HKDF-SHA256.
//
// The point is domain separation. One operator-facing secret (ENCRYPTION_KEY)
// stands behind several independent uses, but no two uses ever hold the same
// key bytes: recovering the subkey that protects licence scans tells an
// attacker nothing about the one protecting 2FA secrets, and neither reveals
// the master. That matters more here than it would with a single-purpose key,
// because these subkeys have very different lifetimes and blast radii.
//
// No salt is used. HKDF's salt strengthens extraction from a *low-entropy*
// secret; the master is 32 bytes of enforced-length key material, and a random
// salt would have to be stored alongside every ciphertext to be reproducible.
// The purpose string is passed as HKDF's info parameter, which is exactly what
// it is for.
func DeriveKey(master []byte, purpose string) ([]byte, error) {
	if len(master) != KeySize {
		return nil, ErrInvalidKey
	}
	if purpose == "" {
		return nil, fmt.Errorf("cryptoutil: derive purpose must not be empty")
	}
	key, err := hkdf.Key(sha256.New, master, nil, purpose, KeySize)
	if err != nil {
		return nil, fmt.Errorf("cryptoutil: hkdf: %w", err)
	}
	return key, nil
}

// DeriveAEAD is DeriveKey followed by New — the usual way a caller obtains the
// cipher for its own purpose without ever handling the master key's bytes.
func DeriveAEAD(master []byte, purpose string) (*AEAD, error) {
	key, err := DeriveKey(master, purpose)
	if err != nil {
		return nil, err
	}
	return New(key)
}
