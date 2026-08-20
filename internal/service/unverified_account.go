package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/google/uuid"
)

// Defaults for the unverified-account lifecycle: one reminder a day after
// signup, and that reminder starts a 30-day deletion clock.
const (
	DefaultVerificationReminderAfter = 24 * time.Hour
	DefaultUnverifiedRetention       = 30 * 24 * time.Hour
	DefaultUnverifiedSweepInterval   = time.Hour

	// defaultReminderBatchSize bounds how many reminders one sweep sends.
	defaultReminderBatchSize = 200
)

// verificationTokenIssuer mints a fresh verification token for an account.
// Satisfied by *AuthService.
type verificationTokenIssuer interface {
	CreateEmailVerificationToken(ctx context.Context, userID uuid.UUID) (string, error)
}

// reminderSender is the part of *email.Sender the reaper needs.
type reminderSender interface {
	SendMessage(ctx context.Context, msg emailpkg.Message) error
	IsConfigured() bool
}

// UnverifiedAccountConfig is the lifecycle timing. Zero values fall back to the
// defaults above.
type UnverifiedAccountConfig struct {
	// ReminderAfter is how long after signup the follow-up email goes out.
	ReminderAfter time.Duration
	// Retention is how long after that reminder the account survives unverified.
	Retention time.Duration
	// SweepInterval is how often the worker looks for work.
	SweepInterval time.Duration
	// BatchSize bounds reminders per sweep.
	BatchSize int
}

func (c UnverifiedAccountConfig) withDefaults() UnverifiedAccountConfig {
	if c.ReminderAfter <= 0 {
		c.ReminderAfter = DefaultVerificationReminderAfter
	}
	if c.Retention <= 0 {
		c.Retention = DefaultUnverifiedRetention
	}
	if c.SweepInterval <= 0 {
		c.SweepInterval = DefaultUnverifiedSweepInterval
	}
	if c.BatchSize <= 0 {
		c.BatchSize = defaultReminderBatchSize
	}
	return c
}

// UnverifiedAccountService reminds, then reaps, accounts that never verified
// their email address. cmd/api/main.go starts the worker only when the email
// sender is configured, and the service re-checks on every sweep.
type UnverifiedAccountService struct {
	users      repository.UserRepository
	tokens     verificationTokenIssuer
	sender     reminderSender
	cfg        UnverifiedAccountConfig
	baseURLFn  func() string
	deletedFn  func(count int64)
	nowFn      func() time.Time
	sweepHooks chan struct{} // non-nil only in tests, signalled after each sweep
}

func NewUnverifiedAccountService(
	users repository.UserRepository,
	tokens verificationTokenIssuer,
	sender reminderSender,
	baseURLFn func() string,
	cfg UnverifiedAccountConfig,
) *UnverifiedAccountService {
	return &UnverifiedAccountService{
		users:     users,
		tokens:    tokens,
		sender:    sender,
		cfg:       cfg.withDefaults(),
		baseURLFn: baseURLFn,
		nowFn:     time.Now,
	}
}

// Config reports the effective lifecycle timing.
func (s *UnverifiedAccountService) Config() UnverifiedAccountConfig { return s.cfg }

// Start runs the sweep loop until ctx is cancelled.
func (s *UnverifiedAccountService) Start(ctx context.Context) {
	go func() {
		slog.Info("Unverified account reaper started",
			"reminderAfter", s.cfg.ReminderAfter.String(),
			"retention", s.cfg.Retention.String(),
			"interval", s.cfg.SweepInterval.String())

		ticker := time.NewTicker(s.cfg.SweepInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("Unverified account reaper stopped")
				return
			case <-ticker.C:
				s.Sweep(ctx)
			}
		}
	}()
}

// Sweep sends any due reminders and deletes any accounts past the retention
// horizon. Exported for the admin trigger and tests.
func (s *UnverifiedAccountService) Sweep(ctx context.Context) (reminded int, deleted int64) {
	// SMTP configuration is re-checked every sweep.
	if s.sender == nil || !s.sender.IsConfigured() {
		return 0, 0
	}

	reminded = s.sendDueReminders(ctx)
	deleted = s.deleteExpired(ctx)

	if s.sweepHooks != nil {
		select {
		case s.sweepHooks <- struct{}{}:
		default:
		}
	}
	return reminded, deleted
}

// sendDueReminders mails everyone whose account has been unverified for longer
// than ReminderAfter and who has not been reminded yet.
func (s *UnverifiedAccountService) sendDueReminders(ctx context.Context) int {
	now := s.nowFn()
	users, err := s.users.ListUnverifiedForReminder(ctx, now.Add(-s.cfg.ReminderAfter), s.cfg.BatchSize)
	if err != nil {
		slog.Warn("Could not list unverified accounts for reminder", "error", err)
		return 0
	}

	sent := 0
	for _, user := range users {
		if ctx.Err() != nil {
			return sent
		}

		token, err := s.tokens.CreateEmailVerificationToken(ctx, user.ID)
		if err != nil {
			slog.Warn("Could not mint verification token for reminder", "userId", user.ID, "error", err)
			UnverifiedRemindersTotal.WithLabelValues("error").Inc()
			continue
		}

		tmpl := emailpkg.Templates(user.PreferredLocale)
		subject, body := tmpl.VerificationReminder(emailpkg.VerificationReminderParams{
			UserName:      user.Name,
			Link:          fmt.Sprintf("%s/verify-email?token=%s", s.baseURL(), token),
			DeletionDays:  int(s.cfg.Retention.Hours() / 24),
			LinkValidDays: int(emailVerificationTokenLifetime.Hours() / 24),
		})

		sendErr := s.sender.SendMessage(ctx, emailpkg.Message{
			To:       user.Email,
			Subject:  subject,
			HTMLBody: body,
			Type:     emailpkg.TypeVerificationReminder,
		})

		// The stamp starts the deletion clock; it is set on delivery and on a
		// permanent refusal, never on a transient failure.
		var sendError *emailpkg.SendError
		switch {
		case sendErr == nil:
			UnverifiedRemindersTotal.WithLabelValues("sent").Inc()
		case errors.As(sendErr, &sendError) && sendError.Permanent():
			slog.Info("Verification reminder undeliverable; starting deletion clock anyway",
				"userId", user.ID, "status", string(sendError.Status))
			UnverifiedRemindersTotal.WithLabelValues("undeliverable").Inc()
		default:
			slog.Warn("Verification reminder deferred after a transient failure",
				"userId", user.ID, "error", sendErr)
			UnverifiedRemindersTotal.WithLabelValues("deferred").Inc()
			continue
		}

		if err := s.users.MarkVerificationReminderSent(ctx, user.ID, now); err != nil {
			// ErrNotFound: the account verified or was removed between the
			// listing and now.
			if !errors.Is(err, repository.ErrNotFound) {
				slog.Warn("Could not stamp verification reminder", "userId", user.ID, "error", err)
			}
			continue
		}
		sent++
	}

	if sent > 0 {
		slog.Info("Verification reminders sent", "count", sent)
	}
	return sent
}

// deleteExpired removes accounts whose reminder is older than the retention
// window and which are still unverified.
func (s *UnverifiedAccountService) deleteExpired(ctx context.Context) int64 {
	horizon := s.nowFn().Add(-s.cfg.Retention)
	deleted, err := s.users.DeleteUnverifiedRemindedBefore(ctx, horizon)
	if err != nil {
		slog.Warn("Unverified account cleanup failed", "error", err)
		return 0
	}
	if deleted > 0 {
		slog.Info("Unverified accounts deleted",
			"count", deleted, "unverifiedSince", horizon.Format(time.RFC3339))
		UnverifiedAccountsDeletedTotal.Add(float64(deleted))
		if s.deletedFn != nil {
			s.deletedFn(deleted)
		}
	}
	return deleted
}

func (s *UnverifiedAccountService) baseURL() string {
	if s.baseURLFn == nil {
		return ""
	}
	return s.baseURLFn()
}

// LoadUnverifiedAccountConfig reads the lifecycle timing from the
// environment. Every value is optional; an unparseable or non-positive
// setting falls back to the default.
func LoadUnverifiedAccountConfig() UnverifiedAccountConfig {
	return UnverifiedAccountConfig{
		ReminderAfter: durationFromEnv("UNVERIFIED_REMINDER_AFTER", DefaultVerificationReminderAfter),
		Retention:     durationFromEnv("UNVERIFIED_ACCOUNT_RETENTION", DefaultUnverifiedRetention),
		SweepInterval: durationFromEnv("UNVERIFIED_CLEANUP_INTERVAL", DefaultUnverifiedSweepInterval),
	}
}

// Reasons the reaper is not running, reported to administrators.
const (
	// CleanupDisabledByOIDC: OIDC mode refuses the feature outright; it is
	// not configurable.
	CleanupDisabledByOIDC = "oidc_mode"
	// CleanupDisabledNoSMTP: without SMTP, registration marks accounts
	// verified on the spot.
	CleanupDisabledNoSMTP = "smtp_not_configured"
	// CleanupDisabledByConfig: UNVERIFIED_CLEANUP_ENABLED=false.
	CleanupDisabledByConfig = "disabled_by_configuration"
)

// UnverifiedCleanupDisabledReason reports why the reaper must not run, or ""
// if it may.
func UnverifiedCleanupDisabledReason(smtpConfigured, oidcEnabled bool) string {
	switch {
	case oidcEnabled:
		return CleanupDisabledByOIDC
	case !smtpConfigured:
		return CleanupDisabledNoSMTP
	case os.Getenv("UNVERIFIED_CLEANUP_ENABLED") == "false":
		return CleanupDisabledByConfig
	default:
		return ""
	}
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		slog.Warn("Ignoring invalid duration setting, using default",
			"variable", key, "value", raw, "default", fallback.String())
		return fallback
	}
	return parsed
}
