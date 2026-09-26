package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type mockReminderLister struct {
	items []*models.AircraftReminder
}

func (m *mockReminderLister) ListAll(_ context.Context, _ uuid.UUID, _ *int) ([]*models.AircraftReminder, error) {
	return m.items, nil
}

func reminderDueIn(days int, kind models.AircraftReminderKind) *models.AircraftReminder {
	today := models.DateOnly(time.Now().UTC())
	return &models.AircraftReminder{
		ID: uuid.New(), AircraftRegistration: "D-MXYZ", Kind: kind, DueDate: today.AddDate(0, 0, days),
	}
}

func newReminderNotifService(t *testing.T, categories pq.StringArray, lister *mockReminderLister) (*service.NotificationService, *mockNotificationRepo) {
	t.Helper()
	notifRepo := newMockNotificationRepo()
	userRepo := newMockNotifUserRepo()
	userID := uuid.New()
	userRepo.users[userID] = &models.User{ID: userID, Email: "mehmet@test.com", Name: "Mehmet", PreferredLocale: "de"}
	prefs := &models.NotificationPreferences{
		UserID: userID, EmailEnabled: true, EnabledCategories: categories,
		WarningDays: pq.Int64Array{30, 14, 7},
	}
	notifRepo.allPrefs = []*models.NotificationPreferences{prefs}
	svc := service.NewNotificationService(notifRepo, newMockNotifCredentialRepo(), newMockNotifFlightRepo(),
		newMockNotifLicenseRepo(), userRepo, email.NewSender(&email.SMTPConfig{}), nil, nil)
	svc.SetAircraftReminderSource(lister)
	return svc, notifRepo
}

// reminderNotices returns the notices sent for reminder id, without the
// thresholds recorded as skipped.
func reminderNotices(repo *mockNotificationRepo, id uuid.UUID) []*models.NotificationLog {
	var out []*models.NotificationLog
	for _, l := range repo.logs {
		if l.NotificationType == string(models.NotifCategoryAircraftReminder) && l.ReferenceID != nil && *l.ReferenceID == id && !l.Skipped {
			out = append(out, l)
		}
	}
	return out
}

func TestAircraftReminderNotifications(t *testing.T) {
	dueSoon := reminderDueIn(10, models.ReminderKindAnnualInspection)
	overdue := reminderDueIn(-3, models.ReminderKindRescueSystemRepack)
	farOff := reminderDueIn(90, models.ReminderKindInsurance)
	lister := &mockReminderLister{items: []*models.AircraftReminder{dueSoon, overdue, farOff}}

	svc, repo := newReminderNotifService(t, models.AllNotificationCategories, lister)
	svc.TriggerCheck(context.Background())

	t.Run("M job 3: Jahresnachprüfung due soon notifies once at the nearest threshold", func(t *testing.T) {
		logs := reminderNotices(repo, dueSoon.ID)
		if len(logs) != 1 {
			t.Fatalf("got %d notices, want 1", len(logs))
		}
		if *logs[0].DaysBeforeExpiry != 14 {
			t.Errorf("threshold = %d, want 14", *logs[0].DaysBeforeExpiry)
		}
		if !logs[0].ExpiryReferenceDate.Equal(dueSoon.DueDate) {
			t.Error("dedup key must carry the due date")
		}
	})
	t.Run("overdue repack sends one lapse notice", func(t *testing.T) {
		logs := reminderNotices(repo, overdue.ID)
		if len(logs) != 1 || *logs[0].DaysBeforeExpiry != 0 {
			t.Fatalf("want one lapse notice, got %d", len(logs))
		}
	})
	t.Run("reminder outside every warning day is quiet", func(t *testing.T) {
		if n := len(reminderNotices(repo, farOff.ID)); n != 0 {
			t.Errorf("got %d notices, want 0", n)
		}
	})
	t.Run("a second check repeats nothing", func(t *testing.T) {
		svc.TriggerCheck(context.Background())
		if n := len(reminderNotices(repo, dueSoon.ID)); n != 1 {
			t.Errorf("due-soon: got %d notices, want 1", n)
		}
		if n := len(reminderNotices(repo, overdue.ID)); n != 1 {
			t.Errorf("overdue: got %d notices, want 1", n)
		}
	})
	t.Run("completing re-arms the notice for the new due date", func(t *testing.T) {
		overdue.DueDate = models.DateOnly(time.Now().UTC()).AddDate(0, 0, 5)
		svc.TriggerCheck(context.Background())
		logs := reminderNotices(repo, overdue.ID)
		if len(logs) != 2 || *logs[1].DaysBeforeExpiry != 7 {
			t.Errorf("want a fresh 7-day warning for the new due date, got %d notices", len(logs))
		}
	})
}

func TestAircraftReminderNotifications_CategoryOff(t *testing.T) {
	lister := &mockReminderLister{items: []*models.AircraftReminder{reminderDueIn(5, models.ReminderKindARC)}}
	svc, repo := newReminderNotifService(t, pq.StringArray{string(models.NotifCategoryCredentialMedical)}, lister)
	svc.TriggerCheck(context.Background())
	if len(repo.logs) != 0 {
		t.Errorf("category off: got %d notices, want 0", len(repo.logs))
	}
}
