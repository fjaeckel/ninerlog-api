package models

import (
	"time"

	"github.com/google/uuid"
)

type CredentialType string

const (
	CredentialTypeEASAClass1Medical CredentialType = "EASA_CLASS1_MEDICAL"
	CredentialTypeEASAClass2Medical CredentialType = "EASA_CLASS2_MEDICAL"
	CredentialTypeEASALAPLMedical   CredentialType = "EASA_LAPL_MEDICAL"
	CredentialTypeFAAClass1Medical  CredentialType = "FAA_CLASS1_MEDICAL"
	CredentialTypeFAAClass2Medical  CredentialType = "FAA_CLASS2_MEDICAL"
	CredentialTypeFAAClass3Medical  CredentialType = "FAA_CLASS3_MEDICAL"
	CredentialTypeLangICAOLevel4    CredentialType = "LANG_ICAO_LEVEL4"
	CredentialTypeLangICAOLevel5    CredentialType = "LANG_ICAO_LEVEL5"
	CredentialTypeLangICAOLevel6    CredentialType = "LANG_ICAO_LEVEL6"
	CredentialTypeSecClearanceZUP   CredentialType = "SEC_CLEARANCE_ZUP"
	CredentialTypeSecClearanceZUBB  CredentialType = "SEC_CLEARANCE_ZUBB"
	CredentialTypeOther             CredentialType = "OTHER"
)

// Credential represents a pilot credential (medical, language proficiency, security clearance)
type Credential struct {
	ID               uuid.UUID      `json:"id"`
	UserID           uuid.UUID      `json:"userId"`
	CredentialType   CredentialType `json:"credentialType"`
	CredentialNumber *string        `json:"credentialNumber,omitempty"`
	IssueDate        time.Time      `json:"issueDate"`
	ExpiryDate       *time.Time     `json:"expiryDate,omitempty"`
	IssuingAuthority string         `json:"issuingAuthority"`
	Notes            *string        `json:"notes,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// IsExpired checks if the credential has expired
func (c *Credential) IsExpired() bool {
	if c.ExpiryDate == nil {
		return false
	}
	return c.ExpiryDate.Before(time.Now())
}

// IsExpiringSoon checks if the credential expires within the given number of days
func (c *Credential) IsExpiringSoon(days int) bool {
	if c.ExpiryDate == nil {
		return false
	}
	threshold := time.Now().AddDate(0, 0, days)
	return c.ExpiryDate.Before(threshold) && !c.IsExpired()
}
