package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestNotificationPreferencesDefaults(t *testing.T) {
	prefs := NotificationPreferences{}
	if prefs.EmailEnabled {
		t.Error("Default EmailEnabled should be false")
	}
	if len(prefs.EnabledCategories) != 0 {
		t.Error("Default EnabledCategories should be empty")
	}
	if prefs.CheckHour != 0 {
		t.Error("Default CheckHour should be 0")
	}
}

func TestNotificationPreferencesWithValues(t *testing.T) {
	userID := uuid.New()
	prefs := NotificationPreferences{
		ID:                uuid.New(),
		UserID:            userID,
		EmailEnabled:      true,
		EnabledCategories: AllNotificationCategories,
		WarningDays:       pq.Int64Array{30, 14, 7},
		CheckHour:         8,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if !prefs.EmailEnabled {
		t.Error("EmailEnabled should be true")
	}
	if len(prefs.WarningDays) != 3 {
		t.Errorf("WarningDays length = %d, want 3", len(prefs.WarningDays))
	}
	if prefs.UserID != userID {
		t.Errorf("UserID = %v, want %v", prefs.UserID, userID)
	}
	if prefs.CheckHour != 8 {
		t.Errorf("CheckHour = %d, want 8", prefs.CheckHour)
	}
	if len(prefs.EnabledCategories) != 10 {
		t.Errorf("EnabledCategories length = %d, want 10", len(prefs.EnabledCategories))
	}
}

func TestNotificationPreferencesIsCategoryEnabled(t *testing.T) {
	prefs := NotificationPreferences{
		EnabledCategories: pq.StringArray{
			string(NotifCategoryCredentialMedical),
			string(NotifCategoryRatingExpiry),
		},
	}

	if !prefs.IsCategoryEnabled(NotifCategoryCredentialMedical) {
		t.Error("credential_medical should be enabled")
	}
	if !prefs.IsCategoryEnabled(NotifCategoryRatingExpiry) {
		t.Error("rating_expiry should be enabled")
	}
	if prefs.IsCategoryEnabled(NotifCategoryCurrencyPassenger) {
		t.Error("currency_passenger should NOT be enabled")
	}
	if prefs.IsCategoryEnabled(NotifCategoryCurrencyNight) {
		t.Error("currency_night should NOT be enabled")
	}
}

func TestCredentialCategoryForType(t *testing.T) {
	tests := []struct {
		credType CredentialType
		want     NotificationCategory
	}{
		{CredentialTypeEASAClass1Medical, NotifCategoryCredentialMedical},
		{CredentialTypeEASAClass2Medical, NotifCategoryCredentialMedical},
		{CredentialTypeEASALAPLMedical, NotifCategoryCredentialMedical},
		{CredentialTypeFAAClass1Medical, NotifCategoryCredentialMedical},
		{CredentialTypeFAAClass2Medical, NotifCategoryCredentialMedical},
		{CredentialTypeFAAClass3Medical, NotifCategoryCredentialMedical},
		{CredentialTypeLangICAOLevel4, NotifCategoryCredentialLanguage},
		{CredentialTypeLangICAOLevel5, NotifCategoryCredentialLanguage},
		{CredentialTypeLangICAOLevel6, NotifCategoryCredentialLanguage},
		{CredentialTypeSecClearanceZUP, NotifCategoryCredentialSecurity},
		{CredentialTypeSecClearanceZUBB, NotifCategoryCredentialSecurity},
		{CredentialTypeOther, NotifCategoryCredentialOther},
	}

	for _, tt := range tests {
		got := CredentialCategoryForType(tt.credType)
		if got != tt.want {
			t.Errorf("CredentialCategoryForType(%s) = %s, want %s", tt.credType, got, tt.want)
		}
	}
}

func TestNotificationLogFields(t *testing.T) {
	userID := uuid.New()
	refID := uuid.New()
	refType := "credential"
	days := 30
	now := time.Now()
	subject := "Test subject"

	log := NotificationLog{
		ID:                  uuid.New(),
		UserID:              userID,
		NotificationType:    string(NotifCategoryCredentialMedical),
		ReferenceID:         &refID,
		ReferenceType:       &refType,
		DaysBeforeExpiry:    &days,
		ExpiryReferenceDate: &now,
		Subject:             &subject,
		SentAt:              time.Now(),
	}

	if log.NotificationType != string(NotifCategoryCredentialMedical) {
		t.Errorf("NotificationType = %s, want %s", log.NotificationType, NotifCategoryCredentialMedical)
	}
	if *log.ReferenceID != refID {
		t.Errorf("ReferenceID = %v, want %v", *log.ReferenceID, refID)
	}
	if *log.DaysBeforeExpiry != 30 {
		t.Errorf("DaysBeforeExpiry = %d, want 30", *log.DaysBeforeExpiry)
	}
	if log.ExpiryReferenceDate == nil {
		t.Error("ExpiryReferenceDate should not be nil")
	}
	if *log.Subject != "Test subject" {
		t.Errorf("Subject = %s, want 'Test subject'", *log.Subject)
	}
}

func TestNotificationLogOptionalFields(t *testing.T) {
	log := NotificationLog{
		ID:               uuid.New(),
		UserID:           uuid.New(),
		NotificationType: string(NotifCategoryCurrencyPassenger),
		SentAt:           time.Now(),
	}

	if log.ReferenceID != nil {
		t.Error("ReferenceID should be nil")
	}
	if log.ReferenceType != nil {
		t.Error("ReferenceType should be nil")
	}
	if log.DaysBeforeExpiry != nil {
		t.Error("DaysBeforeExpiry should be nil")
	}
	if log.ExpiryReferenceDate != nil {
		t.Error("ExpiryReferenceDate should be nil")
	}
	if log.Subject != nil {
		t.Error("Subject should be nil")
	}
}

func TestAllNotificationCategories(t *testing.T) {
	if len(AllNotificationCategories) != 10 {
		t.Errorf("AllNotificationCategories should have 10 entries, got %d", len(AllNotificationCategories))
	}

	// Verify all expected categories are present
	expected := []NotificationCategory{
		NotifCategoryCredentialMedical,
		NotifCategoryCredentialLanguage,
		NotifCategoryCredentialSecurity,
		NotifCategoryCredentialOther,
		NotifCategoryRatingExpiry,
		NotifCategoryCurrencyPassenger,
		NotifCategoryCurrencyNight,
		NotifCategoryCurrencyInstrument,
		NotifCategoryCurrencyFlightReview,
		NotifCategoryCurrencyRevalidation,
	}

	for _, cat := range expected {
		found := false
		for _, c := range AllNotificationCategories {
			if c == string(cat) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllNotificationCategories missing %s", cat)
		}
	}
}
