package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// mockCustomLister returns a fixed set of custom rules with evaluations.
type mockCustomLister struct {
	items []currency.CustomRuleWithStatus
}

func (m *mockCustomLister) List(_ context.Context, _ uuid.UUID) ([]currency.CustomRuleWithStatus, error) {
	return m.items, nil
}

func ruleWithStatus(name string, enabled, notify bool, status currency.Status, expiresOn *string) currency.CustomRuleWithStatus {
	return currency.CustomRuleWithStatus{
		Rule: &models.CustomCurrencyRule{
			ID: uuid.New(), Name: name, Enabled: enabled, Notify: notify,
			Definition: models.CustomCurrencyRuleBody{Window: models.CurrencyWindow{Amount: 90, Unit: "days"}},
		},
		Evaluation: currency.CustomCurrencyResult{Status: status, WindowLabel: "last 90 days", ExpiresOn: expiresOn},
	}
}

func TestCustomCurrencyNotifications(t *testing.T) {
	soon := time.Now().AddDate(0, 0, 5).Format("2006-01-02")

	lister := &mockCustomLister{items: []currency.CustomRuleWithStatus{
		ruleWithStatus("Notify expiring", true, true, currency.StatusExpiring, &soon),
		ruleWithStatus("Notify expired", true, true, currency.StatusExpired, nil),
		ruleWithStatus("Opted out", true, false, currency.StatusExpired, nil),     // notify=false → skip
		ruleWithStatus("Paused", false, true, currency.StatusUnknown, nil),        // disabled → skip
		ruleWithStatus("Notify current", true, true, currency.StatusCurrent, nil), // current → nothing to warn
	}}

	notifRepo := newMockNotificationRepo()
	userRepo := newMockNotifUserRepo()
	userID := uuid.New()
	userRepo.users[userID] = &models.User{ID: userID, Email: "pilot@test.com", Name: "Pilot", PreferredLocale: "en"}

	prefs := &models.NotificationPreferences{
		UserID:       userID,
		EmailEnabled: true,
		WarningDays:  pq.Int64Array{30, 14, 7, 1},
		CheckHour:    time.Now().UTC().Hour(),
	}
	notifRepo.allPrefs = []*models.NotificationPreferences{prefs}
	notifRepo.prefs[userID] = prefs

	svc := service.NewNotificationService(
		notifRepo,
		newMockNotifCredentialRepo(),
		newMockNotifFlightRepo(),
		newMockNotifLicenseRepo(),
		userRepo,
		email.NewSender(&email.SMTPConfig{}), // unconfigured → dry-run, still logs
		nil,                                  // regulatory currency service (unused here)
		lister,
	)

	svc.TriggerCheck(context.Background())

	// Collect custom-currency logs by referenced rule name.
	logged := map[string]bool{}
	for _, l := range notifRepo.logs {
		if l.NotificationType == string(models.NotifCategoryCustomCurrency) {
			for _, it := range lister.items {
				if l.ReferenceID != nil && *l.ReferenceID == it.Rule.ID {
					logged[it.Rule.Name] = true
				}
			}
		}
	}

	if !logged["Notify expiring"] {
		t.Error("expected a notification for the expiring opted-in rule")
	}
	if !logged["Notify expired"] {
		t.Error("expected a notification for the lapsed opted-in rule")
	}
	if logged["Opted out"] {
		t.Error("must not notify for a rule with notify=false")
	}
	if logged["Paused"] {
		t.Error("must not notify for a paused rule")
	}
	if logged["Notify current"] {
		t.Error("must not notify for a current rule")
	}
}

func TestCustomCurrencyNotifications_RespectsEmailDisabled(t *testing.T) {
	soon := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	lister := &mockCustomLister{items: []currency.CustomRuleWithStatus{
		ruleWithStatus("Notify expiring", true, true, currency.StatusExpiring, &soon),
	}}
	notifRepo := newMockNotificationRepo()
	userRepo := newMockNotifUserRepo()
	userID := uuid.New()
	userRepo.users[userID] = &models.User{ID: userID, Email: "pilot@test.com", Name: "Pilot", PreferredLocale: "en"}
	prefs := &models.NotificationPreferences{UserID: userID, EmailEnabled: false, WarningDays: pq.Int64Array{30}}
	notifRepo.allPrefs = []*models.NotificationPreferences{prefs}

	svc := service.NewNotificationService(
		notifRepo, newMockNotifCredentialRepo(), newMockNotifFlightRepo(),
		newMockNotifLicenseRepo(), userRepo, email.NewSender(&email.SMTPConfig{}), nil, lister,
	)
	svc.TriggerCheck(context.Background())

	if len(notifRepo.logs) != 0 {
		t.Errorf("email disabled: expected no notifications, got %d", len(notifRepo.logs))
	}
}
