package models

import (
	"time"

	"github.com/google/uuid"
)

// WeGlide sync statuses.
const (
	WeGlideSyncOK      = "ok"
	WeGlideSyncPartial = "partial"
	WeGlideSyncFailed  = "failed"
)

// MaxWeGlideSyncErrorLen caps WeGlideLink.LastSyncError in runes.
const MaxWeGlideSyncErrorLen = 500

// WeGlideLink is a user's link to their WeGlide account.
type WeGlideLink struct {
	UserID uuid.UUID
	// APIKeyEncrypted is nonce || AES-256-GCM ciphertext of the API key.
	APIKeyEncrypted []byte
	WeGlideUserID   *string
	// LastSyncAt is when the last complete sync finished.
	LastSyncAt     *time.Time
	LastSyncStatus *string
	LastSyncError  *string
	RequestsToday  int
	// RequestsDay is the UTC date RequestsToday counts.
	RequestsDay *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RequestsUsedOn returns the requests counted on the UTC date of day.
func (l *WeGlideLink) RequestsUsedOn(day time.Time) int {
	if l == nil || l.RequestsDay == nil {
		return 0
	}
	if l.RequestsDay.UTC().Format("2006-01-02") != day.UTC().Format("2006-01-02") {
		return 0
	}
	return l.RequestsToday
}
