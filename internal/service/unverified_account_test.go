package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/google/uuid"
)

// stubTokenIssuer stands in for AuthService's token minting.
type stubTokenIssuer struct {
	err    error
	issued []uuid.UUID
}

func (s *stubTokenIssuer) CreateEmailVerificationToken(_ context.Context, userID uuid.UUID) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.issued = append(s.issued, userID)
	return "token-" + userID.String(), nil
}

// stubSender records what the reaper tried to send and can fail on demand.
type stubSender struct {
	configured bool
	sent       []emailpkg.Message
	// failWith is returned for every send when set.
	failWith error
}

func (s *stubSender) SendMessage(_ context.Context, msg emailpkg.Message) error {
	if s.failWith != nil {
		return s.failWith
	}
	s.sent = append(s.sent, msg)
	return nil
}

func (s *stubSender) IsConfigured() bool { return s.configured }

// reaperFixture wires a service over the shared mock user repo, with time
// pinned so the test controls the clock rather than sleeping.
type reaperFixture struct {
	svc    *service.UnverifiedAccountService
	users  *mockUserRepo
	tokens *stubTokenIssuer
	sender *stubSender
}

func newReaperFixture(t *testing.T, cfg service.UnverifiedAccountConfig) *reaperFixture {
	t.Helper()
	users := newMockUserRepo()
	tokens := &stubTokenIssuer{}
	sender := &stubSender{configured: true}
	svc := service.NewUnverifiedAccountService(
		users, tokens, sender,
		func() string { return "https://logbook.test" },
		cfg,
	)
	return &reaperFixture{svc: svc, users: users, tokens: tokens, sender: sender}
}

// addUser inserts an account directly, bypassing the mock's auto-verify.
func (f *reaperFixture) addUser(email string, createdAt time.Time, verified bool) *models.User {
	u := &models.User{
		ID:              uuid.New(),
		Email:           email,
		Name:            "Test Pilot",
		EmailVerified:   verified,
		PreferredLocale: "en",
		CreatedAt:       createdAt,
	}
	f.users.users[email] = u
	return u
}

func TestSweep_RemindsOnlyAccountsPastTheReminderDelay(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{
		ReminderAfter: 24 * time.Hour,
		Retention:     30 * 24 * time.Hour,
	})

	now := time.Now()
	old := f.addUser("stale@example.test", now.Add(-48*time.Hour), false)
	f.addUser("fresh@example.test", now.Add(-1*time.Hour), false)

	reminded, deleted := f.svc.Sweep(context.Background())

	if reminded != 1 {
		t.Fatalf("reminded = %d, want 1", reminded)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0 — nothing has been reminded long enough", deleted)
	}
	if len(f.sender.sent) != 1 || string(f.sender.sent[0].To) != "stale@example.test" {
		t.Fatalf("unexpected recipients: %+v", f.sender.sent)
	}
	if f.sender.sent[0].Type != emailpkg.TypeVerificationReminder {
		t.Errorf("message type = %q", f.sender.sent[0].Type)
	}
	if _, ok := f.users.reminders[old.ID]; !ok {
		t.Error("the reminded account should carry a reminder stamp — it starts the deletion clock")
	}
}

func TestSweep_RemindsEachAccountOnlyOnce(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{ReminderAfter: 24 * time.Hour})
	f.addUser("stale@example.test", time.Now().Add(-48*time.Hour), false)

	f.svc.Sweep(context.Background())
	if reminded, _ := f.svc.Sweep(context.Background()); reminded != 0 {
		t.Fatalf("second sweep reminded %d accounts, want 0", reminded)
	}
	if len(f.sender.sent) != 1 {
		t.Fatalf("sent %d reminders, want 1", len(f.sender.sent))
	}
}

func TestSweep_SkipsVerifiedLoggedInAndOIDCAccounts(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{ReminderAfter: time.Hour})
	longAgo := time.Now().Add(-90 * 24 * time.Hour)

	f.addUser("verified@example.test", longAgo, true)

	// Unverified but actively signing in.
	loggedIn := f.addUser("active@example.test", longAgo, false)
	lastLogin := time.Now().Add(-time.Hour)
	loggedIn.LastLoginAt = &lastLogin

	linked := f.addUser("sso@example.test", longAgo, false)
	f.users.oidcLinked[linked.ID] = true

	reminded, _ := f.svc.Sweep(context.Background())
	if reminded != 0 {
		t.Fatalf("reminded = %d, want 0 — none of these are dead signups", reminded)
	}
	if len(f.sender.sent) != 0 {
		t.Fatalf("unexpected sends: %+v", f.sender.sent)
	}
}

func TestSweep_DeletesOnlyAfterTheRetentionWindowFromTheReminder(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{
		ReminderAfter: 24 * time.Hour,
		Retention:     30 * 24 * time.Hour,
	})

	user := f.addUser("stale@example.test", time.Now().Add(-100*24*time.Hour), false)

	// First sweep sends the reminder and starts the clock.
	f.svc.Sweep(context.Background())

	// One day short of the window: the account survives. The clock runs from
	// the reminder, not signup.
	f.users.reminders[user.ID] = time.Now().Add(-29 * 24 * time.Hour)
	if _, deleted := f.svc.Sweep(context.Background()); deleted != 0 {
		t.Fatalf("deleted %d accounts before the window elapsed", deleted)
	}
	if _, ok := f.users.users["stale@example.test"]; !ok {
		t.Fatal("account was deleted too early")
	}

	// One day past it: reaped.
	f.users.reminders[user.ID] = time.Now().Add(-31 * 24 * time.Hour)
	if _, deleted := f.svc.Sweep(context.Background()); deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, ok := f.users.users["stale@example.test"]; ok {
		t.Fatal("account should have been reaped")
	}
}

func TestSweep_VerifyingBeforeTheDeadlineSavesTheAccount(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{
		ReminderAfter: 24 * time.Hour,
		Retention:     30 * 24 * time.Hour,
	})
	user := f.addUser("late@example.test", time.Now().Add(-48*time.Hour), false)

	f.svc.Sweep(context.Background())

	// The user clicks the link on day 29; the stamp stays, but the flag flips.
	user.EmailVerified = true
	f.users.reminders[user.ID] = time.Now().Add(-31 * 24 * time.Hour)

	if _, deleted := f.svc.Sweep(context.Background()); deleted != 0 {
		t.Fatalf("deleted = %d — a verified account must never be reaped", deleted)
	}
	if _, ok := f.users.users["late@example.test"]; !ok {
		t.Fatal("verified account was deleted")
	}
}

func TestSweep_TransientSendFailureDefersTheClock(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{ReminderAfter: time.Hour})
	user := f.addUser("stale@example.test", time.Now().Add(-48*time.Hour), false)

	// A transient send failure must not start the deletion clock.
	f.sender.failWith = &emailpkg.SendError{
		Status: emailpkg.StatusServerError,
		Err:    errors.New("connection refused"),
	}

	if reminded, _ := f.svc.Sweep(context.Background()); reminded != 0 {
		t.Fatalf("reminded = %d, want 0", reminded)
	}
	if _, stamped := f.users.reminders[user.ID]; stamped {
		t.Fatal("an SMTP outage must not start the deletion clock")
	}

	// Once mail works again the account is picked up and the clock starts.
	f.sender.failWith = nil
	if reminded, _ := f.svc.Sweep(context.Background()); reminded != 1 {
		t.Fatalf("reminded = %d after recovery, want 1", reminded)
	}
	if _, stamped := f.users.reminders[user.ID]; !stamped {
		t.Fatal("reminder should be stamped once it actually went out")
	}
}

func TestSweep_HardBounceStartsTheClockWithoutFurtherMail(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{
		ReminderAfter: time.Hour,
		Retention:     30 * 24 * time.Hour,
	})
	user := f.addUser("nosuchbox@example.test", time.Now().Add(-48*time.Hour), false)

	// A hard bounce starts the clock; the account still gets the full
	// retention window before deletion.
	f.sender.failWith = &emailpkg.SendError{
		Status: emailpkg.StatusHardBounce,
		Code:   550,
		Err:    errors.New("no such user"),
	}

	if _, deleted := f.svc.Sweep(context.Background()); deleted != 0 {
		t.Fatalf("a hard bounce must not shorten the window; deleted %d", deleted)
	}
	stamp, stamped := f.users.reminders[user.ID]
	if !stamped {
		t.Fatal("an undeliverable reminder must still start the clock")
	}

	// Still inside the window.
	f.users.reminders[user.ID] = stamp.Add(-29 * 24 * time.Hour)
	if _, deleted := f.svc.Sweep(context.Background()); deleted != 0 {
		t.Fatalf("deleted = %d before the window elapsed", deleted)
	}

	// Past it.
	f.users.reminders[user.ID] = time.Now().Add(-31 * 24 * time.Hour)
	if _, deleted := f.svc.Sweep(context.Background()); deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
}

func TestSweep_DoesNothingWithoutSMTP(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{ReminderAfter: time.Hour})
	f.sender.configured = false

	user := f.addUser("stale@example.test", time.Now().Add(-90*24*time.Hour), false)
	f.users.reminders[user.ID] = time.Now().Add(-90 * 24 * time.Hour)

	reminded, deleted := f.svc.Sweep(context.Background())
	if reminded != 0 || deleted != 0 {
		t.Fatalf("reminded = %d deleted = %d, want 0/0", reminded, deleted)
	}
	if _, ok := f.users.users["stale@example.test"]; !ok {
		t.Fatal("nothing may be reaped when verification is not enforced")
	}
}

func TestSweep_TokenMintingFailureSkipsTheAccount(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{ReminderAfter: time.Hour})
	user := f.addUser("stale@example.test", time.Now().Add(-48*time.Hour), false)
	f.tokens.err = errors.New("database unavailable")

	if reminded, _ := f.svc.Sweep(context.Background()); reminded != 0 {
		t.Fatal("no reminder should be counted when the token could not be minted")
	}
	if len(f.sender.sent) != 0 {
		t.Fatal("a reminder without a working link must not be sent")
	}
	if _, stamped := f.users.reminders[user.ID]; stamped {
		t.Fatal("the clock must not start when nothing was sent")
	}
}

func TestSweep_ReminderCarriesAWorkingLinkAndTheDeadline(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{
		ReminderAfter: time.Hour,
		Retention:     30 * 24 * time.Hour,
	})
	user := f.addUser("stale@example.test", time.Now().Add(-48*time.Hour), false)

	f.svc.Sweep(context.Background())

	if len(f.sender.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(f.sender.sent))
	}
	body := f.sender.sent[0].HTMLBody
	wantLink := "https://logbook.test/verify-email?token=token-" + user.ID.String()
	if !strings.Contains(body, wantLink) {
		t.Errorf("reminder body is missing the verification link %q:\n%s", wantLink, body)
	}
	// The reminder must state the deletion deadline.
	if !strings.Contains(body, "30 days") {
		t.Errorf("reminder must state the deletion deadline:\n%s", body)
	}
}

func TestSweep_GermanAccountsGetTheGermanReminder(t *testing.T) {
	f := newReaperFixture(t, service.UnverifiedAccountConfig{ReminderAfter: time.Hour})
	user := f.addUser("pilot@example.de", time.Now().Add(-48*time.Hour), false)
	user.PreferredLocale = "de"

	f.svc.Sweep(context.Background())

	if len(f.sender.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(f.sender.sent))
	}
	if !strings.Contains(f.sender.sent[0].Subject, "bestätigen") {
		t.Errorf("expected the German reminder, got subject %q", f.sender.sent[0].Subject)
	}
	if !strings.Contains(f.sender.sent[0].HTMLBody, "30 Tagen") {
		t.Errorf("German reminder must state the deadline:\n%s", f.sender.sent[0].HTMLBody)
	}
}

func TestUnverifiedAccountConfig_DefaultsAreApplied(t *testing.T) {
	svc := service.NewUnverifiedAccountService(nil, nil, nil, nil, service.UnverifiedAccountConfig{})
	cfg := svc.Config()

	if cfg.ReminderAfter != service.DefaultVerificationReminderAfter {
		t.Errorf("ReminderAfter = %v, want %v", cfg.ReminderAfter, service.DefaultVerificationReminderAfter)
	}
	if cfg.Retention != service.DefaultUnverifiedRetention {
		t.Errorf("Retention = %v, want %v", cfg.Retention, service.DefaultUnverifiedRetention)
	}
	if cfg.SweepInterval != service.DefaultUnverifiedSweepInterval {
		t.Errorf("SweepInterval = %v, want %v", cfg.SweepInterval, service.DefaultUnverifiedSweepInterval)
	}
}

func TestLoadUnverifiedAccountConfig_InvalidValuesFallBackToDefaults(t *testing.T) {
	// Unparseable or negative values fall back to the defaults.
	t.Setenv("UNVERIFIED_REMINDER_AFTER", "not-a-duration")
	t.Setenv("UNVERIFIED_ACCOUNT_RETENTION", "-5h")
	t.Setenv("UNVERIFIED_CLEANUP_INTERVAL", "30m")

	cfg := service.LoadUnverifiedAccountConfig()
	if cfg.ReminderAfter != service.DefaultVerificationReminderAfter {
		t.Errorf("ReminderAfter = %v, want the default", cfg.ReminderAfter)
	}
	if cfg.Retention != service.DefaultUnverifiedRetention {
		t.Errorf("Retention = %v, want the default", cfg.Retention)
	}
	if cfg.SweepInterval != 30*time.Minute {
		t.Errorf("SweepInterval = %v, want 30m", cfg.SweepInterval)
	}
}

func TestUnverifiedCleanupDisabledReason(t *testing.T) {
	tests := []struct {
		name           string
		smtpConfigured bool
		oidcEnabled    bool
		disableEnv     string
		want           string
	}{
		{
			name:           "runs with SMTP, local auth, and no override",
			smtpConfigured: true,
			want:           "",
		},
		{
			name:           "refused in OIDC mode even with SMTP configured",
			smtpConfigured: true,
			oidcEnabled:    true,
			want:           service.CleanupDisabledByOIDC,
		},
		{
			// OIDC outranks configuration.
			name:           "OIDC outranks an explicit enable",
			smtpConfigured: true,
			oidcEnabled:    true,
			disableEnv:     "true",
			want:           service.CleanupDisabledByOIDC,
		},
		{
			name: "refused without SMTP, because nothing was ever asked to verify",
			want: service.CleanupDisabledNoSMTP,
		},
		{
			name:           "operator can switch it off",
			smtpConfigured: true,
			disableEnv:     "false",
			want:           service.CleanupDisabledByConfig,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disableEnv != "" {
				t.Setenv("UNVERIFIED_CLEANUP_ENABLED", tc.disableEnv)
			}
			got := service.UnverifiedCleanupDisabledReason(tc.smtpConfigured, tc.oidcEnabled)
			if got != tc.want {
				t.Errorf("UnverifiedCleanupDisabledReason(smtp=%v, oidc=%v) = %q, want %q",
					tc.smtpConfigured, tc.oidcEnabled, got, tc.want)
			}
		})
	}
}
