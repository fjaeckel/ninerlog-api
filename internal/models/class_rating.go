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

// IsExpired reports whether the class rating's expiry date, its last valid day,
// has passed (UTC calendar dates).
func (cr *ClassRating) IsExpired() bool {
	return cr.IsExpiredAt(time.Now())
}

// IsExpiredAt is IsExpired as of now.
func (cr *ClassRating) IsExpiredAt(now time.Time) bool {
	return ExpiredAt(cr.ExpiryDate, now)
}

// IsExpiringSoon reports whether the class rating is unexpired and its expiry date
// is at most `days` calendar days away.
func (cr *ClassRating) IsExpiringSoon(days int) bool {
	return cr.IsExpiringSoonAt(time.Now(), days)
}

// IsExpiringSoonAt is IsExpiringSoon as of now.
func (cr *ClassRating) IsExpiringSoonAt(now time.Time, days int) bool {
	return ExpiringWithin(cr.ExpiryDate, now, days)
}
