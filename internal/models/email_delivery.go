package models

import (
	"time"

	"github.com/google/uuid"
)

// EmailDeliveryEvent is one recorded send attempt. The table is append-only:
// it is the history of what the SMTP conversation said, not current state.
type EmailDeliveryEvent struct {
	ID uuid.UUID `json:"id"`
	// UserID is nil once the account has been removed, or when the address was
	// not tied to an account at the time of sending.
	UserID    *uuid.UUID `json:"userId,omitempty"`
	Recipient string     `json:"recipient"`
	EmailType string     `json:"emailType"`
	Status    string     `json:"status"`
	// SMTPCode is the server's reply code, nil when there was no conversation
	// (an unparseable address, a dropped connection).
	SMTPCode  *int      `json:"smtpCode,omitempty"`
	Detail    *string   `json:"detail,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// EmailSuppression is an address we have stopped mailing because it refused
// mail permanently. Only a recipient-level refusal creates one — see
// pkg/email.DeliveryStatus.
type EmailSuppression struct {
	Email          string    `json:"email"`
	Reason         string    `json:"reason"`
	SMTPCode       *int      `json:"smtpCode,omitempty"`
	Detail         *string   `json:"detail,omitempty"`
	FirstBouncedAt time.Time `json:"firstBouncedAt"`
	LastBouncedAt  time.Time `json:"lastBouncedAt"`
	BounceCount    int       `json:"bounceCount"`
}
