package models

import (
	"time"

	"github.com/google/uuid"
)

// License represents a pilot license
type License struct {
	ID                      uuid.UUID `json:"id"`
	UserID                  uuid.UUID `json:"userId"`
	RegulatoryAuthority     string    `json:"regulatoryAuthority" binding:"required"`
	LicenseType             string    `json:"licenseType" binding:"required"`
	LicenseNumber           string    `json:"licenseNumber" binding:"required"`
	IssueDate               time.Time `json:"issueDate" binding:"required"`
	IssuingAuthority        string    `json:"issuingAuthority" binding:"required"`
	RequiresSeparateLogbook bool      `json:"requiresSeparateLogbook"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

// IsValid checks if all required fields are set
func (l *License) IsValid() bool {
	return l.UserID != uuid.Nil &&
		l.RegulatoryAuthority != "" &&
		l.LicenseType != "" &&
		l.LicenseNumber != "" &&
		!l.IssueDate.IsZero() &&
		l.IssuingAuthority != ""
}
