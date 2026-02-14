package models

import (
	"time"

	"github.com/google/uuid"
)

// ClassType represents the type of class rating
type ClassType string

const (
	ClassTypeSEPLand ClassType = "SEP_LAND"
	ClassTypeSEPSea  ClassType = "SEP_SEA"
	ClassTypeMEPLand ClassType = "MEP_LAND"
	ClassTypeMEPSea  ClassType = "MEP_SEA"
	ClassTypeSETLand ClassType = "SET_LAND"
	ClassTypeSETSea  ClassType = "SET_SEA"
	ClassTypeTMG     ClassType = "TMG"
	ClassTypeIR      ClassType = "IR"
	ClassTypeOther   ClassType = "OTHER"
)

// ValidClassTypes returns all valid class types
func ValidClassTypes() []ClassType {
	return []ClassType{
		ClassTypeSEPLand, ClassTypeSEPSea,
		ClassTypeMEPLand, ClassTypeMEPSea,
		ClassTypeSETLand, ClassTypeSETSea,
		ClassTypeTMG, ClassTypeIR, ClassTypeOther,
	}
}

// IsValidClassType checks if a class type is valid
func IsValidClassType(ct ClassType) bool {
	for _, valid := range ValidClassTypes() {
		if ct == valid {
			return true
		}
	}
	return false
}

// ClassRating represents a class rating attached to a license
type ClassRating struct {
	ID         uuid.UUID  `json:"id"`
	LicenseID  uuid.UUID  `json:"licenseId"`
	ClassType  ClassType  `json:"classType"`
	IssueDate  time.Time  `json:"issueDate"`
	ExpiryDate *time.Time `json:"expiryDate,omitempty"`
	Notes      *string    `json:"notes,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// IsExpired checks if the class rating has expired
func (cr *ClassRating) IsExpired() bool {
	if cr.ExpiryDate == nil {
		return false
	}
	return cr.ExpiryDate.Before(time.Now())
}

// IsExpiringSoon checks if the class rating expires within N days
func (cr *ClassRating) IsExpiringSoon(days int) bool {
	if cr.ExpiryDate == nil {
		return false
	}
	threshold := time.Now().AddDate(0, 0, days)
	return cr.ExpiryDate.Before(threshold) && !cr.IsExpired()
}
