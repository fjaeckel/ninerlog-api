package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func validLicense() *License {
	return &License{
		UserID:           uuid.New(),
		LicenseType:      LicenseTypeEASAPPL,
		LicenseNumber:    "PPL-123456",
		IssueDate:        time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC),
		IssuingAuthority: "EASA",
		IsActive:         true,
	}
}

func TestLicenseIsValid_Valid(t *testing.T) {
	l := validLicense()
	if !l.IsValid() {
		t.Error("IsValid() = false, want true")
	}
}

func TestLicenseIsValid_NilUserID(t *testing.T) {
	l := validLicense()
	l.UserID = uuid.Nil
	if l.IsValid() {
		t.Error("IsValid() = true, want false for nil UserID")
	}
}

func TestLicenseIsValid_EmptyLicenseType(t *testing.T) {
	l := validLicense()
	l.LicenseType = ""
	if l.IsValid() {
		t.Error("IsValid() = true, want false for empty LicenseType")
	}
}

func TestLicenseIsValid_EmptyLicenseNumber(t *testing.T) {
	l := validLicense()
	l.LicenseNumber = ""
	if l.IsValid() {
		t.Error("IsValid() = true, want false for empty LicenseNumber")
	}
}

func TestLicenseIsValid_ZeroIssueDate(t *testing.T) {
	l := validLicense()
	l.IssueDate = time.Time{}
	if l.IsValid() {
		t.Error("IsValid() = true, want false for zero IssueDate")
	}
}

func TestLicenseIsValid_EmptyIssuingAuthority(t *testing.T) {
	l := validLicense()
	l.IssuingAuthority = ""
	if l.IsValid() {
		t.Error("IsValid() = true, want false for empty IssuingAuthority")
	}
}

func TestLicenseIsExpired_NoExpiryDate(t *testing.T) {
	l := validLicense()
	if l.IsExpired() {
		t.Error("IsExpired() = true, want false for nil ExpiryDate")
	}
}

func TestLicenseIsExpired_FutureDate(t *testing.T) {
	l := validLicense()
	future := time.Now().Add(365 * 24 * time.Hour)
	l.ExpiryDate = &future
	if l.IsExpired() {
		t.Error("IsExpired() = true, want false for future date")
	}
}

func TestLicenseIsExpired_PastDate(t *testing.T) {
	l := validLicense()
	past := time.Now().Add(-24 * time.Hour)
	l.ExpiryDate = &past
	if !l.IsExpired() {
		t.Error("IsExpired() = false, want true for past date")
	}
}

func TestLicenseTypes(t *testing.T) {
	types := []LicenseType{
		LicenseTypeEASAPPL, LicenseTypeFAAPPL,
		LicenseTypeEASASPL, LicenseTypeFAASport,
		LicenseTypeEASACPL, LicenseTypeFAACPL,
		LicenseTypeEASAATPL, LicenseTypeFAAATPL,
		LicenseTypeEASAIR, LicenseTypeFAAIR,
	}
	for _, lt := range types {
		if lt == "" {
			t.Error("License type constant is empty")
		}
	}
}
