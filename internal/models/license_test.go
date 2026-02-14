package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func validLicense() *License {
	return &License{
		UserID:              uuid.New(),
		RegulatoryAuthority: "EASA",
		LicenseType:         "EASA_PPL",
		LicenseNumber:       "PPL-123456",
		IssueDate:           time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC),
		IssuingAuthority:    "EASA",
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

func TestLicenseIsValid_EmptyRegulatoryAuthority(t *testing.T) {
	l := validLicense()
	l.RegulatoryAuthority = ""
	if l.IsValid() {
		t.Error("IsValid() = true, want false for empty RegulatoryAuthority")
	}
}
