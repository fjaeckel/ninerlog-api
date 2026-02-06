package models

import (
	"time"

	"github.com/google/uuid"
)

// LicenseType represents the type of pilot license
type LicenseType string

const (
	LicenseTypeEASAPPL  LicenseType = "EASA_PPL"
	LicenseTypeFAAPPL   LicenseType = "FAA_PPL"
	LicenseTypeEASASPL  LicenseType = "EASA_SPL"
	LicenseTypeFAASport LicenseType = "FAA_SPORT"
	LicenseTypeEASACPL  LicenseType = "EASA_CPL"
	LicenseTypeFAACPL   LicenseType = "FAA_CPL"
	LicenseTypeEASAATPL LicenseType = "EASA_ATPL"
	LicenseTypeFAAATPL  LicenseType = "FAA_ATPL"
	LicenseTypeEASAIR   LicenseType = "EASA_IR"
	LicenseTypeFAAIR    LicenseType = "FAA_IR"
)

// License represents a pilot license
type License struct {
	ID               uuid.UUID   `json:"id"`
	UserID           uuid.UUID   `json:"userId"`
	LicenseType      LicenseType `json:"licenseType" binding:"required"`
	LicenseNumber    string      `json:"licenseNumber" binding:"required"`
	IssueDate        time.Time   `json:"issueDate" binding:"required"`
	ExpiryDate       *time.Time  `json:"expiryDate,omitempty"`
	IssuingAuthority string      `json:"issuingAuthority" binding:"required"`
	IsActive         bool        `json:"isActive"`
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

// IsValid checks if all required fields are set
func (l *License) IsValid() bool {
	return l.UserID != uuid.Nil &&
		l.LicenseType != "" &&
		l.LicenseNumber != "" &&
		!l.IssueDate.IsZero() &&
		l.IssuingAuthority != ""
}

// IsExpired checks if the license has expired
func (l *License) IsExpired() bool {
	if l.ExpiryDate == nil {
		return false
	}
	return l.ExpiryDate.Before(time.Now())
}
