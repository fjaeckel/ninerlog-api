package models

import (
	"strings"
	"testing"
)

func TestValidateStringLength(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		max     int
		wantErr bool
	}{
		{"within limit", "name", "hello", 10, false},
		{"at limit", "name", "1234567890", 10, false},
		{"over limit", "name", "12345678901", 10, true},
		{"empty string", "name", "", 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStringLength(tt.field, tt.value, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStringLength(%q, %q, %d) error = %v, wantErr %v", tt.field, tt.value, tt.max, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOptionalStringLength(t *testing.T) {
	short := "hi"
	long := strings.Repeat("x", 300)

	if err := ValidateOptionalStringLength("f", nil, 10); err != nil {
		t.Errorf("nil value should pass: %v", err)
	}
	if err := ValidateOptionalStringLength("f", &short, 10); err != nil {
		t.Errorf("short value should pass: %v", err)
	}
	if err := ValidateOptionalStringLength("f", &long, 10); err == nil {
		t.Error("long value should fail")
	}
}

func TestValidateFlightTextFields(t *testing.T) {
	longRemarks := strings.Repeat("x", 2001)
	shortRemarks := "Normal remarks"

	// Valid flight
	f := &Flight{AircraftReg: "D-EABC", AircraftType: "C172", Remarks: &shortRemarks}
	if err := ValidateFlightTextFields(f); err != nil {
		t.Errorf("valid flight should pass: %v", err)
	}

	// Remarks too long
	f.Remarks = &longRemarks
	if err := ValidateFlightTextFields(f); err == nil {
		t.Error("remarks over 2000 chars should fail")
	}

	// Aircraft reg too long
	f.Remarks = &shortRemarks
	f.AircraftReg = strings.Repeat("X", 21)
	if err := ValidateFlightTextFields(f); err == nil {
		t.Error("aircraftReg over 20 chars should fail")
	}
}

func TestValidateAircraftTextFields(t *testing.T) {
	a := &Aircraft{Registration: "D-EABC", Type: "C172", Make: "Cessna", Model: "172S"}
	if err := ValidateAircraftTextFields(a); err != nil {
		t.Errorf("valid aircraft should pass: %v", err)
	}

	a.Registration = strings.Repeat("X", 21)
	if err := ValidateAircraftTextFields(a); err == nil {
		t.Error("registration over 20 chars should fail")
	}
}

func TestValidateContactTextFields(t *testing.T) {
	ct := &Contact{Name: "John Doe"}
	if err := ValidateContactTextFields(ct); err != nil {
		t.Errorf("valid contact should pass: %v", err)
	}

	ct.Name = strings.Repeat("X", 101)
	if err := ValidateContactTextFields(ct); err == nil {
		t.Error("name over 100 chars should fail")
	}
}

func TestValidateLicenseTextFields(t *testing.T) {
	l := &License{
		RegulatoryAuthority: "EASA",
		LicenseType:         "PPL(A)",
		LicenseNumber:       "DE.FCL.12345",
		IssuingAuthority:    "LBA",
	}
	if err := ValidateLicenseTextFields(l); err != nil {
		t.Errorf("valid license should pass: %v", err)
	}

	l.RegulatoryAuthority = strings.Repeat("X", 31)
	if err := ValidateLicenseTextFields(l); err == nil {
		t.Error("authority over 30 chars should fail")
	}
}

func TestValidateCredentialTextFields(t *testing.T) {
	cr := &Credential{
		CredentialType:   "EASA_CLASS_2_MEDICAL",
		IssuingAuthority: "LBA",
	}
	if err := ValidateCredentialTextFields(cr); err != nil {
		t.Errorf("valid credential should pass: %v", err)
	}

	longNotes := strings.Repeat("x", 1001)
	cr.Notes = &longNotes
	if err := ValidateCredentialTextFields(cr); err == nil {
		t.Error("notes over 1000 chars should fail")
	}
}
