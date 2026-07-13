package models

import (
	"time"

	"github.com/google/uuid"
)

// SignatureMethod describes how a FlightSignature was (or will be) captured.
type SignatureMethod string

const (
	// SignatureMethodLive is captured in person on the flight owner's own
	// device while the instructor is physically present. Completes
	// synchronously; no token involved.
	SignatureMethodLive SignatureMethod = "live"
	// SignatureMethodDeferred is a token-based flow: the owner creates a
	// request, and the instructor completes it later via the public
	// /sign/{token} link, delivered by email and/or a shareable link/QR.
	SignatureMethodDeferred SignatureMethod = "deferred"
)

// SignatureStatus is the lifecycle state of a FlightSignature row.
type SignatureStatus string

const (
	// SignatureStatusPending is a deferred request awaiting completion.
	SignatureStatusPending SignatureStatus = "pending"
	// SignatureStatusCompleted means the signature was captured and (for
	// deferred flows) the token has been consumed. The flight is locked.
	SignatureStatusCompleted SignatureStatus = "completed"
	// SignatureStatusRevoked means the owner cancelled a pending request
	// before anyone signed it.
	SignatureStatusRevoked SignatureStatus = "revoked"
	// SignatureStatusVoided means a completed signature was later
	// invalidated by the owner (with a reason), unlocking the flight again.
	SignatureStatusVoided SignatureStatus = "voided"
	// SignatureStatusExpired means a pending request's token lapsed before
	// anyone signed it.
	SignatureStatusExpired SignatureStatus = "expired"
)

// FlightSignature is an instructor sign-off request/record for a single
// flight log entry. See db/migrations/000042_create_flight_signatures for
// the full column rationale.
type FlightSignature struct {
	ID       uuid.UUID
	FlightID uuid.UUID
	UserID   uuid.UUID

	Method SignatureMethod
	Status SignatureStatus

	// Deferred token fields; nil for SignatureMethodLive.
	TokenHash      *string
	TokenExpiresAt *time.Time

	InstructorEmail *string
	EmailSentAt     *time.Time
	EmailSendCount  int

	ContactID *uuid.UUID

	// Captured at completion time.
	InstructorName         *string
	InstructorCredentialNo *string
	SignatureImage         []byte
	SignedAt               *time.Time
	SignerIP               *string
	SignerUserAgent        *string

	VoidedAt     *time.Time
	VoidedReason *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MaxSignatureImageBytes bounds the size of an uploaded signature image
// (decoded PNG bytes) to keep the flight_signatures table well-behaved.
const MaxSignatureImageBytes = 500 * 1024

// MinSignatureRequestExpiryHours / MaxSignatureRequestExpiryHours clamp the
// caller-supplied expiry window for a deferred signature request.
const (
	MinSignatureRequestExpiryHours     = 1
	MaxSignatureRequestExpiryHours     = 24 * 30 // 30 days
	DefaultSignatureRequestExpiryHours = 24 * 7  // 7 days
)
