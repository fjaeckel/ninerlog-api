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
	ClassTypeGlider  ClassType = "GLIDER"
	ClassTypeUL      ClassType = "ULTRALIGHT"
	ClassTypeGyro    ClassType = "GYROPLANE"
)

// ValidClassTypes returns all valid class types
func ValidClassTypes() []ClassType {
	return []ClassType{
		ClassTypeSEPLand, ClassTypeSEPSea,
		ClassTypeMEPLand, ClassTypeMEPSea,
		ClassTypeSETLand, ClassTypeSETSea,
		ClassTypeTMG, ClassTypeIR, ClassTypeOther,
		ClassTypeGlider, ClassTypeUL, ClassTypeGyro,
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
	ID        uuid.UUID `json:"id"`
	LicenseID uuid.UUID `json:"licenseId"`
	ClassType ClassType `json:"classType"`
	// ULKind is set only when ClassType is ULTRALIGHT.
	ULKind     *ULKind    `json:"ulKind,omitempty"`
	IssueDate  time.Time  `json:"issueDate"`
	ExpiryDate *time.Time `json:"expiryDate,omitempty"`
	Notes      *string    `json:"notes,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// NormalizeULKind clears ULKind on a non-ULTRALIGHT rating and returns
// ErrInvalidULKind for an unknown kind.
func (cr *ClassRating) NormalizeULKind() error {
	if cr.ClassType != ClassTypeUL {
		cr.ULKind = nil
		return nil
	}
	if cr.ULKind != nil && !IsValidRatingULKind(*cr.ULKind) {
		return ErrInvalidULKind
	}
	return nil
}

// IsExpired checks if the class rating has expired
func (cr *ClassRating) IsExpired() bool {
	return cr.IsExpiredAt(time.Now())
}

// IsExpiredAt reports whether the class rating has expired at now.
func (cr *ClassRating) IsExpiredAt(now time.Time) bool {
	if cr.ExpiryDate == nil {
		return false
	}
	return cr.ExpiryDate.Before(now)
}

// IsExpiringSoon checks if the class rating expires within N days
func (cr *ClassRating) IsExpiringSoon(days int) bool {
	return cr.IsExpiringSoonAt(time.Now(), days)
}

// IsExpiringSoonAt reports whether the class rating expires within days of now.
func (cr *ClassRating) IsExpiringSoonAt(now time.Time, days int) bool {
	if cr.ExpiryDate == nil {
		return false
	}
	threshold := now.AddDate(0, 0, days)
	return cr.ExpiryDate.Before(threshold) && !cr.IsExpiredAt(now)
}
