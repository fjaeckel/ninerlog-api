package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// NotificationCategory represents a granular notification category
type NotificationCategory string

const (
	NotifCategoryCredentialMedical    NotificationCategory = "credential_medical"
	NotifCategoryCredentialLanguage   NotificationCategory = "credential_language"
	NotifCategoryCredentialSecurity   NotificationCategory = "credential_security"
	NotifCategoryCredentialOther      NotificationCategory = "credential_other"
	NotifCategoryRatingExpiry         NotificationCategory = "rating_expiry"
	NotifCategoryCurrencyPassenger    NotificationCategory = "currency_passenger"
	NotifCategoryCurrencyNight        NotificationCategory = "currency_night"
	NotifCategoryCurrencyInstrument   NotificationCategory = "currency_instrument"
	NotifCategoryCurrencyFlightReview NotificationCategory = "currency_flight_review"
	NotifCategoryCurrencyRevalidation NotificationCategory = "currency_revalidation"
)

// AllNotificationCategories is the default set of all categories (all enabled)
var AllNotificationCategories = pq.StringArray{
	string(NotifCategoryCredentialMedical),
	string(NotifCategoryCredentialLanguage),
	string(NotifCategoryCredentialSecurity),
	string(NotifCategoryCredentialOther),
	string(NotifCategoryRatingExpiry),
	string(NotifCategoryCurrencyPassenger),
	string(NotifCategoryCurrencyNight),
	string(NotifCategoryCurrencyInstrument),
	string(NotifCategoryCurrencyFlightReview),
	string(NotifCategoryCurrencyRevalidation),
}

// NotificationPreferences holds a user's notification settings
type NotificationPreferences struct {
	ID                uuid.UUID      `json:"id"`
	UserID            uuid.UUID      `json:"userId"`
	EmailEnabled      bool           `json:"emailEnabled"`
	EnabledCategories pq.StringArray `json:"enabledCategories"`
	WarningDays       pq.Int64Array  `json:"warningDays"`
	CheckHour         int            `json:"checkHour"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

// IsCategoryEnabled checks if a specific notification category is enabled
func (p *NotificationPreferences) IsCategoryEnabled(category NotificationCategory) bool {
	for _, c := range p.EnabledCategories {
		if c == string(category) {
			return true
		}
	}
	return false
}

// NotificationLog tracks sent notifications to avoid duplicates
type NotificationLog struct {
	ID                  uuid.UUID  `json:"id"`
	UserID              uuid.UUID  `json:"userId"`
	NotificationType    string     `json:"notificationType"`
	ReferenceID         *uuid.UUID `json:"referenceId,omitempty"`
	ReferenceType       *string    `json:"referenceType,omitempty"`
	DaysBeforeExpiry    *int       `json:"daysBeforeExpiry,omitempty"`
	ExpiryReferenceDate *time.Time `json:"expiryReferenceDate,omitempty"`
	Subject             *string    `json:"subject,omitempty"`
	SentAt              time.Time  `json:"sentAt"`
}

// CredentialCategoryForType maps a credential type to a notification category
func CredentialCategoryForType(credType CredentialType) NotificationCategory {
	switch credType {
	case CredentialTypeEASAClass1Medical, CredentialTypeEASAClass2Medical, CredentialTypeEASALAPLMedical,
		CredentialTypeFAAClass1Medical, CredentialTypeFAAClass2Medical, CredentialTypeFAAClass3Medical:
		return NotifCategoryCredentialMedical
	case CredentialTypeLangICAOLevel4, CredentialTypeLangICAOLevel5, CredentialTypeLangICAOLevel6:
		return NotifCategoryCredentialLanguage
	case CredentialTypeSecClearanceZUP, CredentialTypeSecClearanceZUBB:
		return NotifCategoryCredentialSecurity
	default:
		return NotifCategoryCredentialOther
	}
}
