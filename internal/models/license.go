package models

import (
	"errors"
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
	return l.Validate() == nil
}

// Validate checks all required fields and returns a descriptive error
func (l *License) Validate() error {
	if l.UserID == uuid.Nil {
		return errors.New("user ID is required")
	}
	if l.RegulatoryAuthority == "" {
		return errors.New("regulatory authority is required")
	}
	if l.LicenseType == "" {
		return errors.New("license type is required")
	}
	if l.LicenseNumber == "" {
		return errors.New("license number is required")
	}
	if l.IssueDate.IsZero() {
		return errors.New("issue date is required")
	}
	if l.IssuingAuthority == "" {
		return errors.New("issuing authority is required")
	}
	return nil
}
