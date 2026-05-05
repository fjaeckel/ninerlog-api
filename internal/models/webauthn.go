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

// WebAuthnSession holds the transient state of an in-flight WebAuthn ceremony
// (registration or authentication) until the client returns its assertion.
type WebAuthnSession struct {
	ID          uuid.UUID
	UserID      *uuid.UUID // nil for usernameless / discoverable login
	Challenge   string
	SessionData []byte // serialized webauthn.SessionData
	Purpose     string // "registration" | "login"
	ExpiresAt   time.Time
	CreatedAt   time.Time
}
