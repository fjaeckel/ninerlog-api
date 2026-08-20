package models

import (
	"time"

	"github.com/google/uuid"
)

// SystemAnnouncement is an operator-authored message shown to every user until
// it expires (a nil ExpiresAt never expires). Severity is one of "info",
// "success", "warning", or "critical"; validation happens at the API boundary.
type SystemAnnouncement struct {
	ID        uuid.UUID
	Message   string
	Severity  string
	ExpiresAt *time.Time
	CreatedBy uuid.UUID
	CreatedAt time.Time
}
