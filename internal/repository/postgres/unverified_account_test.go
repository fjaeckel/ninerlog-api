package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

// TestUnverifiedQueries_CarryTheirSafetyGuards asserts the guard predicates on
// the reaper's SQL itself.
func TestUnverifiedQueries_CarryTheirSafetyGuards(t *testing.T) {
	requiredGuards := []struct {
		fragment string
		why      string
	}{
		{"email_verified = FALSE", "only unverified accounts are in scope"},
		{"last_login_at IS NULL", "an account that has signed in is in use, whatever its verified flag says"},
		{"oidc_identities", "provider-backed accounts never consume our verification link"},
	}

	t.Run("listing for reminders", func(t *testing.T) {
		db, mock, capturedSQL := captureSQL(t)
		mock.ExpectQuery(".*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "preferred_locale", "created_at"}))

		repo := NewUserRepository(db)
		if _, err := repo.ListUnverifiedForReminder(context.Background(), time.Now(), 10); err != nil {
			t.Fatalf("ListUnverifiedForReminder: %v", err)
		}
		captured := *capturedSQL

		for _, guard := range requiredGuards {
			if !regexp.MustCompile(regexp.QuoteMeta(guard.fragment)).MatchString(captured) {
				t.Errorf("listing query is missing %q — %s\n%s", guard.fragment, guard.why, captured)
			}
		}
		if !regexp.MustCompile("verification_reminder_sent_at IS NULL").MatchString(captured) {
			t.Errorf("listing query must skip already-reminded accounts:\n%s", captured)
		}
	})

	t.Run("deletion", func(t *testing.T) {
		db, mock, capturedSQL := captureSQL(t)
		mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 3))

		repo := NewUserRepository(db)
		deleted, err := repo.DeleteUnverifiedRemindedBefore(context.Background(), time.Now())
		if err != nil {
			t.Fatalf("DeleteUnverifiedRemindedBefore: %v", err)
		}
		if deleted != 3 {
			t.Errorf("deleted = %d, want 3", deleted)
		}

		captured := *capturedSQL
		for _, guard := range requiredGuards {
			if !regexp.MustCompile(regexp.QuoteMeta(guard.fragment)).MatchString(captured) {
				t.Errorf("delete query is missing %q — %s\n%s", guard.fragment, guard.why, captured)
			}
		}
		// The delete query requires that a reminder was sent.
		if !regexp.MustCompile("verification_reminder_sent_at IS NOT NULL").MatchString(captured) {
			t.Errorf("delete query must require that a reminder was actually sent:\n%s", captured)
		}
	})
}

func TestMarkVerificationReminderSent_OnlyStampsAnUnstampedAccount(t *testing.T) {
	db, mock, capturedSQL := captureSQL(t)
	mock.ExpectExec("UPDATE users").WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewUserRepository(db)
	if err := repo.MarkVerificationReminderSent(context.Background(), uuid.New(), time.Now()); err != nil {
		t.Fatalf("MarkVerificationReminderSent: %v", err)
	}

	// The stamp only writes a still-NULL column.
	if captured := *capturedSQL; !regexp.MustCompile("verification_reminder_sent_at IS NULL").MatchString(captured) {
		t.Errorf("the stamp must be guarded against overwriting an existing one:\n%s", captured)
	}
}

// captureSQL builds a sqlmock whose matcher records the SQL it is handed and
// accepts everything.
func captureSQL(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *string) {
	t.Helper()
	var captured string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(
		sqlmock.QueryMatcherFunc(func(_, actualSQL string) error {
			captured = actualSQL
			return nil
		}),
	))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, mock, &captured
}
