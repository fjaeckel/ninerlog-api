package email

import (
	"context"
	"fmt"
)

// DeliveryStatus is what one send attempt turned out to be. It is deliberately
// narrow: an SMTP client only ever learns about delivery during the SMTP
// conversation, so these are the only outcomes we can honestly claim.
type DeliveryStatus string

const (
	// StatusDelivered means the receiving server accepted the message. It is
	// not a promise that the message reached an inbox — a server that accepts
	// and then bounces asynchronously (which most do) is invisible from here.
	StatusDelivered DeliveryStatus = "delivered"

	// StatusHardBounce means the recipient was refused permanently: a 5xx reply
	// to RCPT TO or to the message data. The mailbox does not exist, or the
	// server will never take mail for it. Retrying the same address is futile.
	StatusHardBounce DeliveryStatus = "hard_bounce"

	// StatusSoftBounce is a temporary refusal of the recipient (4xx): greylisting,
	// a full mailbox, a server over quota. Worth retrying later.
	StatusSoftBounce DeliveryStatus = "soft_bounce"

	// StatusInvalidAddress means the address failed to parse, so nothing was
	// dialled. Permanent, but our judgement rather than the server's.
	StatusInvalidAddress DeliveryStatus = "invalid_address"

	// StatusSuppressed means we declined to send because the address is on the
	// suppression list from an earlier hard bounce.
	StatusSuppressed DeliveryStatus = "suppressed"

	// StatusRejected means the server took the recipient but refused the
	// message itself, after the data was sent — a size limit, a content or
	// reputation policy. It says nothing about whether the mailbox exists, so
	// it must never suppress the address.
	StatusRejected DeliveryStatus = "rejected"

	// StatusServerError covers every failure that is about our side or the
	// connection rather than the recipient: DNS, refused connections, TLS
	// failures, bad SMTP credentials. Keeping these distinct is the whole point
	// of classifying per command — a 535 "authentication failed" is a 5xx, and
	// treating it as a bounce would suppress every address the instant SMTP
	// credentials went stale.
	StatusServerError DeliveryStatus = "server_error"

	// StatusDryRun records a send skipped because SMTP is not configured.
	StatusDryRun DeliveryStatus = "dry_run"
)

// IsRecipientFailure reports whether the status blames the address rather than
// our infrastructure. Only these justify giving up on an address.
func (s DeliveryStatus) IsRecipientFailure() bool {
	switch s {
	case StatusHardBounce, StatusInvalidAddress, StatusSuppressed:
		return true
	default:
		return false
	}
}

// Message is one outbound email. Type is a short stable identifier for what the
// message was for ("verify_email", "verification_reminder", …) so the delivery
// log can be read by purpose.
type Message struct {
	To       string
	Subject  string
	HTMLBody string
	Type     string
}

// Message type identifiers used across the codebase. New senders should add a
// constant here rather than passing a literal, so the delivery log stays
// groupable.
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

// SendError carries the classification alongside the underlying failure so a
// caller can tell "this address is dead" from "our mail server is down" without
// re-parsing SMTP replies.
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

// Permanent reports whether retrying this address could ever succeed. A server
// error is not permanent — the same address will go through once the server or
// the credentials are fixed.
func (e *SendError) Permanent() bool { return e.Status.IsRecipientFailure() }

// Attempt is one row of the delivery log.
type Attempt struct {
	Recipient string
	Type      string
	Status    DeliveryStatus
	// Code is the SMTP reply code, 0 when there was no conversation.
	Code int
	// Detail is the server's reply text or our own reason, for an operator to
	// read. It is never shown to the recipient.
	Detail string
}

// DeliveryRecorder persists what happened to each send and answers whether an
// address has been given up on. It is an interface so pkg/email stays free of
// database dependencies; the implementation lives in internal/service.
//
// RecordAttempt returns nothing on purpose: failing to write an audit row must
// never turn a delivered email into a reported failure. Implementations log
// their own storage errors.
type DeliveryRecorder interface {
	RecordAttempt(ctx context.Context, a Attempt)
	// IsSuppressed reports whether the address has hard-bounced before. It is
	// consulted before dialling, so implementations should fail open (return
	// false) if they cannot answer — refusing to send because a lookup failed
	// would be worse than one wasted delivery attempt.
	IsSuppressed(ctx context.Context, recipient string) bool
}
