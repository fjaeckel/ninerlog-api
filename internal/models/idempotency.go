package models

import (
	"time"

	"github.com/google/uuid"
)

// Idempotency record states. Enforced by a CHECK constraint on the
// idempotency_keys table.
const (
	// IdempotencyStateInProgress marks a key claimed by a request that has
	// not produced a response yet.
	IdempotencyStateInProgress = "in_progress"
	// IdempotencyStateCompleted marks a key whose request finished. The
	// captured response is replayed verbatim on retry.
	IdempotencyStateCompleted = "completed"
)

// IdempotencyRecord is one server-side replay record for a mutating request
// that carried an `Idempotency-Key` header, scoped to the user who sent it.
//
// CreatedAt is the claim timestamp and doubles as a fencing token: only the
// request that took the claim may finalize or release it, so a request that
// took over an abandoned claim cannot have its record clobbered by the
// original straggler.
type IdempotencyRecord struct {
	UserID uuid.UUID
	Key    string

	// RequestHash is the SHA-256 over method, path+query and (for
	// non-multipart requests) the request body.
	RequestHash []byte

	State string

	// ResponseStatus is nil while in progress, and stays nil on a completed
	// record whose response was too large to store.
	ResponseStatus      *int
	ResponseBody        []byte
	ResponseContentType string

	CreatedAt   time.Time
	CompletedAt *time.Time
	ExpiresAt   time.Time
}

// Replayable reports whether a stored record carries a response that can be
// returned verbatim to a retry.
func (r *IdempotencyRecord) Replayable() bool {
	return r.State == IdempotencyStateCompleted && r.ResponseStatus != nil
}
