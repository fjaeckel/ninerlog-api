package models

import (
	"time"

	"github.com/google/uuid"
)

// OIDCIdentity links an external OpenID Connect identity to a local user.
//
// (Issuer, Subject) is the identity's primary key at the provider and the only
// value lookups use; Email is informational and may change at any time.
type OIDCIdentity struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"userId"`
	Issuer      string     `json:"issuer"`
	Subject     string     `json:"subject"`
	Email       string     `json:"email"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
}

// OIDCLoginState is the server-side half of one authorization request: the
// nonce that binds the ID token to this login, and the PKCE verifier whose
// challenge was sent to the provider. Keyed by the SHA-256 of the state
// parameter, never the state itself.
type OIDCLoginState struct {
	StateHash []byte
	// BrowserHash is the SHA-256 of the value held in the browser's
	// short-lived OIDC cookie, binding this pending login to the browser that
	// started it.
	BrowserHash  []byte
	Nonce        string
	CodeVerifier string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// OIDCHandoffCode is a single-use, short-lived credential the callback hands
// to the browser so the SPA can fetch the real token pair over POST.
type OIDCHandoffCode struct {
	CodeHash  []byte
	UserID    uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
}
