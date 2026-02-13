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
	if prefs.CurrencyWarnings {
		t.Error("Default CurrencyWarnings should be false")
	}
	if prefs.CredentialWarnings {
		t.Error("Default CredentialWarnings should be false")
	}
}

func TestNotificationPreferencesWithValues(t *testing.T) {
	userID := uuid.New()
	prefs := NotificationPreferences{
		ID:                 uuid.New(),
		UserID:             userID,
		EmailEnabled:       true,
		CurrencyWarnings:   true,
		CredentialWarnings: true,
		WarningDays:        pq.Int64Array{30, 14, 7},
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
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
}

func TestNotificationLogFields(t *testing.T) {
	userID := uuid.New()
	refID := uuid.New()
	refType := "credential"
	days := 30

	log := NotificationLog{
		ID:               uuid.New(),
		UserID:           userID,
		NotificationType: "credential_warning",
		ReferenceID:      &refID,
		ReferenceType:    &refType,
		DaysBeforeExpiry: &days,
		SentAt:           time.Now(),
	}

	if log.NotificationType != "credential_warning" {
		t.Errorf("NotificationType = %s, want credential_warning", log.NotificationType)
	}
	if *log.ReferenceID != refID {
		t.Errorf("ReferenceID = %v, want %v", *log.ReferenceID, refID)
	}
	if *log.DaysBeforeExpiry != 30 {
		t.Errorf("DaysBeforeExpiry = %d, want 30", *log.DaysBeforeExpiry)
	}
}

func TestNotificationLogOptionalFields(t *testing.T) {
	log := NotificationLog{
		ID:               uuid.New(),
		UserID:           uuid.New(),
		NotificationType: "currency_warning",
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
}
