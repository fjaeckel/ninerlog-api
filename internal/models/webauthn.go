package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// WebAuthnCredential represents a passkey / WebAuthn credential registered by a user.
type WebAuthnCredential struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	CredentialID    []byte // raw credential id from the authenticator
	PublicKey       []byte // CBOR-encoded COSE public key
	AttestationType string
	AAGUID          []byte
	SignCount       uint32
	Transports      pq.StringArray
	Label           *string
	UserPresent     bool
	UserVerified    bool
	BackupEligible  bool
	BackupState     bool
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}

// WebAuthn ceremony kinds. Stored in WebAuthnSession.Ceremony and enforced by
// a CHECK constraint on the webauthn_sessions table.
const (
	WebAuthnCeremonyRegistration = "registration"
	WebAuthnCeremonyLogin        = "login"
)

// WebAuthnSession holds the transient state of an in-flight WebAuthn ceremony
// (registration or authentication) until the client returns its assertion.
//
// The raw handle that identifies a session is never stored: IDHash holds its
// SHA-256, so a database dump does not yield usable ceremony state.
type WebAuthnSession struct {
	IDHash    []byte     // sha256 of the raw handle issued to the client
	UserID    *uuid.UUID // nil for usernameless / discoverable login
	Ceremony  string     // WebAuthnCeremonyRegistration | WebAuthnCeremonyLogin
	Data      []byte     // serialized webauthn.SessionData
	ExpiresAt time.Time
	CreatedAt time.Time
}
