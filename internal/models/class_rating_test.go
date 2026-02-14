package models

import (
	"testing"
	"time"
)

func TestValidClassTypes(t *testing.T) {
	types := ValidClassTypes()
	if len(types) != 9 {
		t.Errorf("ValidClassTypes() count = %d, want 9", len(types))
	}
}

func TestIsValidClassType_Valid(t *testing.T) {
	validTypes := []ClassType{
		ClassTypeSEPLand, ClassTypeSEPSea, ClassTypeMEPLand, ClassTypeMEPSea,
		ClassTypeSETLand, ClassTypeSETSea, ClassTypeTMG, ClassTypeIR, ClassTypeOther,
	}
	for _, ct := range validTypes {
		if !IsValidClassType(ct) {
			t.Errorf("IsValidClassType(%s) = false, want true", ct)
		}
	}
}

func TestIsValidClassType_Invalid(t *testing.T) {
	if IsValidClassType("INVALID") {
		t.Error("IsValidClassType(INVALID) = true, want false")
	}
}

func TestClassRatingIsExpired_NoExpiry(t *testing.T) {
	cr := &ClassRating{ExpiryDate: nil}
	if cr.IsExpired() {
		t.Error("IsExpired() = true, want false for nil expiry")
	}
}

func TestClassRatingIsExpired_Future(t *testing.T) {
	future := time.Now().Add(365 * 24 * time.Hour)
	cr := &ClassRating{ExpiryDate: &future}
	if cr.IsExpired() {
		t.Error("IsExpired() = true, want false for future date")
	}
}

func TestClassRatingIsExpired_Past(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	cr := &ClassRating{ExpiryDate: &past}
	if !cr.IsExpired() {
		t.Error("IsExpired() = false, want true for past date")
	}
}

func TestClassRatingIsExpiringSoon(t *testing.T) {
	soon := time.Now().Add(15 * 24 * time.Hour)
	cr := &ClassRating{ExpiryDate: &soon}
	if !cr.IsExpiringSoon(30) {
		t.Error("IsExpiringSoon(30) = false, want true for 15 days out")
	}
	if cr.IsExpiringSoon(10) {
		t.Error("IsExpiringSoon(10) = true, want false for 15 days out")
	}
}

func TestClassRatingIsExpiringSoon_AlreadyExpired(t *testing.T) {
	past := time.Now().Add(-5 * 24 * time.Hour)
	cr := &ClassRating{ExpiryDate: &past}
	if cr.IsExpiringSoon(30) {
		t.Error("IsExpiringSoon() = true, want false for already expired")
	}
}

func TestClassTypeConstants(t *testing.T) {
	tests := []struct {
		ct   ClassType
		want string
	}{
		{ClassTypeSEPLand, "SEP_LAND"},
		{ClassTypeSEPSea, "SEP_SEA"},
		{ClassTypeMEPLand, "MEP_LAND"},
		{ClassTypeMEPSea, "MEP_SEA"},
		{ClassTypeSETLand, "SET_LAND"},
		{ClassTypeSETSea, "SET_SEA"},
		{ClassTypeTMG, "TMG"},
		{ClassTypeIR, "IR"},
		{ClassTypeOther, "OTHER"},
	}
	for _, tt := range tests {
		if string(tt.ct) != tt.want {
			t.Errorf("ClassType = %s, want %s", tt.ct, tt.want)
		}
	}
}
