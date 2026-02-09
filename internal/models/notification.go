package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// NotificationPreferences holds a user's notification settings
type NotificationPreferences struct {
	ID                 uuid.UUID     `json:"id"`
	UserID             uuid.UUID     `json:"userId"`
	EmailEnabled       bool          `json:"emailEnabled"`
	CurrencyWarnings   bool          `json:"currencyWarnings"`
	CredentialWarnings bool          `json:"credentialWarnings"`
	WarningDays        pq.Int64Array `json:"warningDays"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

// NotificationLog tracks sent notifications to avoid duplicates
type NotificationLog struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"userId"`
	NotificationType string     `json:"notificationType"` // "currency_warning", "credential_warning"
	ReferenceID      *uuid.UUID `json:"referenceId,omitempty"`
	ReferenceType    *string    `json:"referenceType,omitempty"` // "license", "credential"
	DaysBeforeExpiry *int       `json:"daysBeforeExpiry,omitempty"`
	SentAt           time.Time  `json:"sentAt"`
}
