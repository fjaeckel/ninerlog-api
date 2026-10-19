package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// countingRecorder counts send attempts and can suppress every recipient.
type countingRecorder struct {
	attempts   int
	suppressed bool
}

func (r *countingRecorder) RecordAttempt(context.Context, email.Attempt) { r.attempts++ }
func (r *countingRecorder) IsSuppressed(context.Context, string) bool    { return r.suppressed }

type dedupFixture struct {
	svc       *service.NotificationService
	notifRepo *mockNotificationRepo
	credRepo  *mockNotifCredentialRepo
	lister    *mockCustomLister
	recorder  *countingRecorder
	user      *models.User
	now       time.Time
}

func newDedupFixture(t *testing.T) *dedupFixture {
	t.Helper()
	f := &dedupFixture{
		notifRepo: newMockNotificationRepo(),
		credRepo:  newMockNotifCredentialRepo(),
		lister:    &mockCustomLister{},
		recorder:  &countingRecorder{},
		now:       time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC),
	}
	userRepo := newMockNotifUserRepo()
	f.user = &models.User{ID: uuid.New(), Email: "lena@example.com", Name: "Lena", PreferredLocale: "en"}
	userRepo.users[f.user.ID] = f.user
	prefs := &models.NotificationPreferences{
		UserID:            f.user.ID,
		EmailEnabled:      true,
		EnabledCategories: models.AllNotificationCategories,
		WarningDays:       pq.Int64Array{30, 14, 7},
	}
	f.notifRepo.allPrefs = []*models.NotificationPreferences{prefs}

	sender := email.NewSender(&email.SMTPConfig{})
	sender.SetDeliveryRecorder(f.recorder)
	f.svc = service.NewNotificationService(
		f.notifRepo, f.credRepo, newMockNotifFlightRepo(), newMockNotifLicenseRepo(),
		userRepo, sender, nil, f.lister,
	)
	f.svc.SetClock(func() time.Time { return f.now })
	return f
}

func (f *dedupFixture) customRule(status currency.Status, lapsedOn *time.Time) {
	if len(f.lister.items) == 0 {
		f.lister.items = []currency.CustomRuleWithStatus{ruleWithStatus("Club recency", true, true, status, nil)}
	}
	f.lister.items[0].Evaluation.Status = status
	f.lister.items[0].Evaluation.LapsedOn = lapsedOn
}

func TestNotifications_LapseNotifiedOncePerLapse(t *testing.T) {
	f := newDedupFixture(t)
	march := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	september := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

	steps := []struct {
		name     string
		status   currency.Status
		lapsedOn *time.Time
		wantSent int
	}{
		{"lapse is notified", currency.StatusExpired, &march, 1},
		{"same lapse on the next run is not notified", currency.StatusExpired, &march, 1},
		{"regained currency sends nothing", currency.StatusCurrent, nil, 1},
		{"a new lapse is notified again", currency.StatusExpired, &september, 2},
		{"the new lapse is not notified twice", currency.StatusExpired, &september, 2},
	}
	for _, step := range steps {
		f.customRule(step.status, step.lapsedOn)
		f.svc.TriggerCheck(context.Background())
		if got := len(f.notifRepo.sent()); got != step.wantSent {
			t.Fatalf("%s: sent = %d, want %d", step.name, got, step.wantSent)
		}
		if f.recorder.attempts != step.wantSent {
			t.Fatalf("%s: send attempts = %d, want %d", step.name, f.recorder.attempts, step.wantSent)
		}
	}
	for _, l := range f.notifRepo.sent() {
		if *l.DaysBeforeExpiry != 0 {
			t.Errorf("lapse notice logged with days %d, want 0", *l.DaysBeforeExpiry)
		}
	}
}

func TestNotifications_NeverCurrentNotifiedOnce(t *testing.T) {
	f := newDedupFixture(t)
	f.customRule(currency.StatusExpired, nil)
	f.svc.TriggerCheck(context.Background())
	f.svc.TriggerCheck(context.Background())
	sent := f.notifRepo.sent()
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(sent))
	}
	if !sent[0].ExpiryReferenceDate.Equal(models.NeverCurrentDate) {
		t.Errorf("concerned date = %v, want %v", sent[0].ExpiryReferenceDate, models.NeverCurrentDate)
	}
}

func TestNotifications_FirstCheckSendsOnlyNearestThreshold(t *testing.T) {
	f := newDedupFixture(t)
	expiry := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	credID := uuid.New()
	f.credRepo.creds[credID] = &models.Credential{
		ID: credID, UserID: f.user.ID, CredentialType: models.CredentialTypeEASAClass2Medical, ExpiryDate: &expiry,
	}

	for day := 0; day < 3; day++ {
		f.now = time.Date(2026, 10, 19+day, 8, 0, 0, 0, time.UTC)
		f.svc.TriggerCheck(context.Background())
		sent := f.notifRepo.sent()
		if len(sent) != 1 || f.recorder.attempts != 1 {
			t.Fatalf("day %d: sent = %d, attempts = %d, want 1 and 1", day, len(sent), f.recorder.attempts)
		}
		if *sent[0].DaysBeforeExpiry != 7 {
			t.Errorf("day %d: sent threshold = %d, want 7", day, *sent[0].DaysBeforeExpiry)
		}
	}

	skipped := map[int]bool{}
	for _, l := range f.notifRepo.logs {
		if l.Skipped {
			skipped[*l.DaysBeforeExpiry] = true
		}
	}
	if !skipped[30] || !skipped[14] || len(skipped) != 2 {
		t.Errorf("skipped thresholds = %v, want 30 and 14", skipped)
	}
}

func TestNotifications_WarningOnExpiryDateButNotAfter(t *testing.T) {
	f := newDedupFixture(t)
	expiry := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	credID := uuid.New()
	f.credRepo.creds[credID] = &models.Credential{
		ID: credID, UserID: f.user.ID, CredentialType: models.CredentialTypeEASAClass2Medical, ExpiryDate: &expiry,
	}

	f.now = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	f.svc.TriggerCheck(context.Background())
	if got := len(f.notifRepo.logs); got != 0 {
		t.Fatalf("day after expiry: logged %d, want 0", got)
	}

	f.now = time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC)
	f.svc.TriggerCheck(context.Background())
	if got := len(f.notifRepo.sent()); got != 1 {
		t.Fatalf("on the expiry date: sent %d, want 1", got)
	}
}

func TestNotifications_SendFailureRetriedNextRun(t *testing.T) {
	f := newDedupFixture(t)
	march := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	f.customRule(currency.StatusExpired, &march)

	f.recorder.suppressed = true
	f.svc.TriggerCheck(context.Background())
	if len(f.notifRepo.logs) != 0 || f.notifRepo.released != 1 {
		t.Fatalf("after failed send: logs = %d, released = %d; want 0 and 1", len(f.notifRepo.logs), f.notifRepo.released)
	}

	f.recorder.suppressed = false
	f.svc.TriggerCheck(context.Background())
	if got := len(f.notifRepo.sent()); got != 1 {
		t.Fatalf("retry: sent = %d, want 1", got)
	}
}

func TestNotifications_ClaimErrorIsReturnedAndNothingSent(t *testing.T) {
	f := newDedupFixture(t)
	f.notifRepo.claimErr = errors.New("connection reset")

	err := f.svc.SendLapseNotice(context.Background(), f.user, models.NotifCategoryCurrencyPassenger, uuid.New(), nil)
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("err = %v, want the claim error", err)
	}
	if f.recorder.attempts != 0 {
		t.Errorf("send attempts = %d, want 0", f.recorder.attempts)
	}
}

func TestNotifications_ReleaseErrorIsReturned(t *testing.T) {
	f := newDedupFixture(t)
	f.recorder.suppressed = true
	f.notifRepo.releaseErr = errors.New("release failed")

	err := f.svc.SendLapseNotice(context.Background(), f.user, models.NotifCategoryCurrencyPassenger, uuid.New(), nil)
	if err == nil || !strings.Contains(err.Error(), "release failed") || !strings.Contains(err.Error(), "send") {
		t.Fatalf("err = %v, want both the send and the release error", err)
	}
}

func TestNotifications_ClaimConflictSkipsSend(t *testing.T) {
	f := newDedupFixture(t)
	refID := uuid.New()
	lapsedOn := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	days := 0
	held := lapsedOn
	f.notifRepo.logs = append(f.notifRepo.logs, &models.NotificationLog{
		ID: uuid.New(), UserID: f.user.ID, NotificationType: string(models.NotifCategoryCurrencyPassenger),
		ReferenceID: &refID, DaysBeforeExpiry: &days, ExpiryReferenceDate: &held,
	})

	if err := f.svc.SendLapseNotice(context.Background(), f.user, models.NotifCategoryCurrencyPassenger, refID, &lapsedOn); err != nil {
		t.Fatalf("SendLapseNotice: %v", err)
	}
	if f.recorder.attempts != 0 || len(f.notifRepo.logs) != 1 {
		t.Errorf("attempts = %d, logs = %d; want 0 and 1", f.recorder.attempts, len(f.notifRepo.logs))
	}
}

func TestNotifications_BackfilledRowCoversEarlierLapse(t *testing.T) {
	f := newDedupFixture(t)
	refID := uuid.New()
	days := 0
	sentOn := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	f.notifRepo.logs = append(f.notifRepo.logs, &models.NotificationLog{
		ID: uuid.New(), UserID: f.user.ID, NotificationType: string(models.NotifCategoryCurrencyPassenger),
		ReferenceID: &refID, DaysBeforeExpiry: &days, ExpiryReferenceDate: &sentOn,
	})

	sameLapse := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if err := f.svc.SendLapseNotice(context.Background(), f.user, models.NotifCategoryCurrencyPassenger, refID, &sameLapse); err != nil {
		t.Fatal(err)
	}
	if f.recorder.attempts != 0 {
		t.Fatalf("lapse before the backfilled date was re-sent")
	}

	laterLapse := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	if err := f.svc.SendLapseNotice(context.Background(), f.user, models.NotifCategoryCurrencyPassenger, refID, &laterLapse); err != nil {
		t.Fatal(err)
	}
	if f.recorder.attempts != 1 {
		t.Errorf("later lapse: attempts = %d, want 1", f.recorder.attempts)
	}
}

func TestCrossedThresholds(t *testing.T) {
	tests := []struct {
		name        string
		days        pq.Int64Array
		daysUntil   int
		wantNearest int
		wantFarther []int
		wantOK      bool
	}{
		{"five days out crosses all three", pq.Int64Array{30, 14, 7}, 5, 7, []int{14, 30}, true},
		{"unsorted with duplicates", pq.Int64Array{7, 30, 14, 30}, 10, 14, []int{30}, true},
		{"exactly on a threshold", pq.Int64Array{30, 14, 7}, 14, 14, []int{30}, true},
		{"expiry date itself", pq.Int64Array{30, 14, 7}, 0, 7, []int{14, 30}, true},
		{"beyond every threshold", pq.Int64Array{30, 14, 7}, 31, 0, nil, false},
		{"already expired", pq.Int64Array{30, 14, 7}, -1, 0, nil, false},
		{"no thresholds", nil, 3, 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nearest, farther, ok := service.CrossedThresholds(tt.days, tt.daysUntil)
			if ok != tt.wantOK || nearest != tt.wantNearest || len(farther) != len(tt.wantFarther) {
				t.Fatalf("got (%d, %v, %v), want (%d, %v, %v)", nearest, farther, ok, tt.wantNearest, tt.wantFarther, tt.wantOK)
			}
			for i := range farther {
				if farther[i] != tt.wantFarther[i] {
					t.Errorf("farther = %v, want %v", farther, tt.wantFarther)
				}
			}
		})
	}
}

func TestGetCheckInterval_NonPositiveFallsBack(t *testing.T) {
	for _, v := range []string{"0", "0s", "-5m"} {
		t.Setenv("NOTIFICATION_CHECK_INTERVAL", v)
		if d := service.GetCheckInterval(); d != time.Hour {
			t.Errorf("NOTIFICATION_CHECK_INTERVAL=%q: interval = %v, want 1h", v, d)
		}
	}
}

func TestStartBackgroundChecker_NonPositiveIntervalDoesNotPanic(t *testing.T) {
	notifRepo := newMockNotificationRepo()
	svc := newTestNotifService(notifRepo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.StartBackgroundChecker(ctx, 0)
	svc.StartBackgroundChecker(ctx, -time.Minute)
	time.Sleep(20 * time.Millisecond)
}
