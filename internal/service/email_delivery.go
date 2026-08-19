package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/google/uuid"
)

// maxDeliveryDetail bounds what is copied out of an SMTP reply into the log.
const maxDeliveryDetail = 1000

// DefaultDeliveryEventLimit / MaxDeliveryEventLimit bound the admin listing.
const (
	DefaultDeliveryEventLimit = 100
	MaxDeliveryEventLimit     = 500
)

// EmailDeliveryService records the outcome of every send and maintains the
// suppression list. It implements pkg/email.DeliveryRecorder.
type EmailDeliveryService struct {
	repo repository.EmailDeliveryRepository
}

func NewEmailDeliveryService(repo repository.EmailDeliveryRepository) *EmailDeliveryService {
	return &EmailDeliveryService{repo: repo}
}

// RecordAttempt appends the attempt to the delivery log and, for a permanent
// recipient-level refusal, adds the address to the suppression list. It
// swallows its own storage errors.
func (s *EmailDeliveryService) RecordAttempt(ctx context.Context, a emailpkg.Attempt) {
	if s == nil || s.repo == nil {
		return
	}

	// Record against a fresh, bounded context; the send path's context may
	// already be cancelled.
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	event := &models.EmailDeliveryEvent{
		Recipient: a.Recipient,
		EmailType: a.Type,
		Status:    string(a.Status),
	}
	if a.Code > 0 {
		code := a.Code
		event.SMTPCode = &code
	}
	if a.Detail != "" {
		detail := a.Detail
		if len(detail) > maxDeliveryDetail {
			detail = detail[:maxDeliveryDetail]
		}
		event.Detail = &detail
	}

	// Attribution is best-effort: an address with no account still gets a
	// logged event.
	if userID, err := s.repo.UserIDForEmail(recordCtx, a.Recipient); err == nil {
		event.UserID = &userID
	} else if !errors.Is(err, repository.ErrNotFound) {
		slog.Warn("Could not attribute email delivery event to a user", "recipient", a.Recipient, "error", err)
	}

	if err := s.repo.RecordEvent(recordCtx, event); err != nil {
		slog.Warn("Failed to record email delivery event",
			"recipient", a.Recipient, "status", string(a.Status), "error", err)
	}

	// Only a hard bounce earns a suppression.
	if a.Status == emailpkg.StatusHardBounce {
		var code *int
		if a.Code > 0 {
			c := a.Code
			code = &c
		}
		if err := s.repo.Suppress(recordCtx, a.Recipient, string(a.Status), code, a.Detail); err != nil {
			slog.Warn("Failed to suppress hard-bounced address", "recipient", a.Recipient, "error", err)
			return
		}
		slog.Warn("Email address suppressed after hard bounce",
			"recipient", a.Recipient, "smtpCode", a.Code)
		s.refreshSuppressionGauge(recordCtx)
	}
}

// IsSuppressed reports whether the address has hard-bounced before. It fails
// open: a lookup error returns false and the send proceeds.
func (s *EmailDeliveryService) IsSuppressed(ctx context.Context, recipient string) bool {
	if s == nil || s.repo == nil {
		return false
	}
	suppressed, err := s.repo.IsSuppressed(ctx, recipient)
	if err != nil {
		slog.Warn("Suppression lookup failed, sending anyway", "recipient", recipient, "error", err)
		return false
	}
	return suppressed
}

// ListEvents returns recent delivery events, newest first. An empty recipient
// means all recipients.
func (s *EmailDeliveryService) ListEvents(ctx context.Context, recipient string, limit int) ([]*models.EmailDeliveryEvent, error) {
	return s.repo.ListEvents(ctx, recipient, clampLimit(limit))
}

// ListSuppressions returns suppressed addresses, most recently bounced first.
func (s *EmailDeliveryService) ListSuppressions(ctx context.Context, limit int) ([]*models.EmailSuppression, error) {
	return s.repo.ListSuppressions(ctx, clampLimit(limit))
}

// CountSuppressions reports how many addresses are currently suppressed.
func (s *EmailDeliveryService) CountSuppressions(ctx context.Context) (int, error) {
	return s.repo.CountSuppressions(ctx)
}

// Unsuppress lifts a suppression; mail to the address is attempted again.
func (s *EmailDeliveryService) Unsuppress(ctx context.Context, email string) error {
	if err := s.repo.Unsuppress(ctx, email); err != nil {
		return err
	}
	slog.Info("Email suppression lifted", "recipient", email)
	s.refreshSuppressionGauge(ctx)
	return nil
}

// RefreshSuppressionGauge re-reads the suppression count into the metric.
// Called at startup.
func (s *EmailDeliveryService) RefreshSuppressionGauge(ctx context.Context) {
	s.refreshSuppressionGauge(ctx)
}

func (s *EmailDeliveryService) refreshSuppressionGauge(ctx context.Context) {
	count, err := s.repo.CountSuppressions(ctx)
	if err != nil {
		slog.Warn("Failed to count email suppressions", "error", err)
		return
	}
	emailpkg.EmailSuppressedAddresses.Set(float64(count))
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultDeliveryEventLimit
	}
	if limit > MaxDeliveryEventLimit {
		return MaxDeliveryEventLimit
	}
	return limit
}

// Compile-time assertion of pkg/email.DeliveryRecorder.
var _ emailpkg.DeliveryRecorder = (*EmailDeliveryService)(nil)

// UserIDForEmail is exposed for callers that want attribution outside the send
// path.
func (s *EmailDeliveryService) UserIDForEmail(ctx context.Context, email string) (uuid.UUID, error) {
	return s.repo.UserIDForEmail(ctx, email)
}
