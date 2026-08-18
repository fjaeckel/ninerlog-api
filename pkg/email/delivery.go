package email

import (
	"context"
	"fmt"
)

// DeliveryStatus classifies the outcome of one send attempt.
type DeliveryStatus string

const (
	// StatusDelivered means the receiving server accepted the message;
	// asynchronous bounces are not visible here.
	StatusDelivered DeliveryStatus = "delivered"

	// StatusHardBounce means the recipient was refused permanently: a 5xx reply
	// to RCPT TO or to the message data.
	StatusHardBounce DeliveryStatus = "hard_bounce"

	// StatusSoftBounce is a temporary refusal of the recipient (4xx): greylisting,
	// a full mailbox, a server over quota.
	StatusSoftBounce DeliveryStatus = "soft_bounce"

	// StatusInvalidAddress means the address failed to parse; nothing was
	// dialled.
	StatusInvalidAddress DeliveryStatus = "invalid_address"

	// StatusSuppressed means the send was skipped: the address is on the
	// suppression list from an earlier hard bounce.
	StatusSuppressed DeliveryStatus = "suppressed"

	// StatusRejected means the server took the recipient but refused the
	// message itself (size limit, content or reputation policy); it never
	// suppresses the address.
	StatusRejected DeliveryStatus = "rejected"

	// StatusServerError covers failures of our side or the connection rather
	// than the recipient: DNS, refused connections, TLS failures, bad SMTP
	// credentials.
	StatusServerError DeliveryStatus = "server_error"

	// StatusDryRun records a send skipped while SMTP is not configured.
	StatusDryRun DeliveryStatus = "dry_run"
)

// IsRecipientFailure reports whether the status blames the address rather than
// the sending infrastructure.
func (s DeliveryStatus) IsRecipientFailure() bool {
	switch s {
	case StatusHardBounce, StatusInvalidAddress, StatusSuppressed:
		return true
	default:
		return false
	}
}

// Message is one outbound email. Type is a short stable identifier for the
// message's purpose ("verify_email", "verification_reminder", …).
type Message struct {
	To       string
	Subject  string
	HTMLBody string
	Type     string
}

// Message type identifiers used across the codebase.
const (
	TypeVerifyEmail          = "verify_email"
	TypeVerificationReminder = "verification_reminder"
	TypePasswordReset        = "password_reset"
	TypePasswordChanged      = "password_changed"
	TypeTwoFactorReset       = "two_factor_reset"
	TypeNotification         = "notification"
	TypeSignatureRequest     = "signature_request"
	TypeSignatureCompleted   = "signature_completed"
	TypeAdminTest            = "admin_test"
)

// SendError carries the delivery classification alongside the underlying
// failure.
type SendError struct {
	Status DeliveryStatus
	// Code is the SMTP reply code when the server gave one, 0 otherwise.
	Code int
	Err  error
}

func (e *SendError) Error() string {
	if e.Code > 0 {
		return fmt.Sprintf("email send failed (%s, smtp %d): %v", e.Status, e.Code, e.Err)
	}
	return fmt.Sprintf("email send failed (%s): %v", e.Status, e.Err)
}

func (e *SendError) Unwrap() error { return e.Err }

// Permanent reports whether the failure blames the address itself.
func (e *SendError) Permanent() bool { return e.Status.IsRecipientFailure() }

// Attempt is one row of the delivery log.
type Attempt struct {
	Recipient string
	Type      string
	Status    DeliveryStatus
	// Code is the SMTP reply code, 0 when there was no conversation.
	Code int
	// Detail is the server's reply text or a local classification note; never
	// shown to the recipient.
	Detail string
}

// DeliveryRecorder persists what happened to each send and answers whether an
// address has been given up on. The implementation lives in internal/service.
//
// RecordAttempt returns nothing; implementations log their own storage errors.
type DeliveryRecorder interface {
	RecordAttempt(ctx context.Context, a Attempt)
	// IsSuppressed reports whether the address has hard-bounced before.
	// Implementations fail open (return false) when they cannot answer.
	IsSuppressed(ctx context.Context, recipient string) bool
}
