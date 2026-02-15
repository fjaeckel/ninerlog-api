package models

import (
	"testing"

	"github.com/google/uuid"
)

func validAircraft() *Aircraft {
	return &Aircraft{
		ID:           uuid.New(),
		UserID:       uuid.New(),
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172 Skyhawk",
		IsActive:     true,
	}
}

func TestAircraftValidate_Valid(t *testing.T) {
	a := validAircraft()
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestAircraftValidate_MissingRegistration(t *testing.T) {
	a := validAircraft()
	a.Registration = ""
	if err := a.Validate(); err != ErrAircraftRegistrationRequired {
		t.Errorf("Validate() = %v, want ErrAircraftRegistrationRequired", err)
	}
}

func TestAircraftValidate_MissingType(t *testing.T) {
	a := validAircraft()
	a.Type = ""
	if err := a.Validate(); err != ErrAircraftTypeRequired {
		t.Errorf("Validate() = %v, want ErrAircraftTypeRequired", err)
	}
}

func TestAircraftValidate_MissingMake(t *testing.T) {
	a := validAircraft()
	a.Make = ""
	if err := a.Validate(); err != ErrAircraftMakeRequired {
		t.Errorf("Validate() = %v, want ErrAircraftMakeRequired", err)
	}
}

func TestAircraftValidate_MissingModel(t *testing.T) {
	a := validAircraft()
	a.Model = ""
	if err := a.Validate(); err != ErrAircraftModelRequired {
		t.Errorf("Validate() = %v, want ErrAircraftModelRequired", err)
	}
}

func TestAircraftValidate_AircraftClass(t *testing.T) {
	a := validAircraft()
	sep := "SEP_LAND"
	a.AircraftClass = &sep
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for valid aircraft class", err)
	}
}

func TestAircraftValidate_CustomAircraftClass(t *testing.T) {
	a := validAircraft()
	custom := "ULTRALIGHT"
	a.AircraftClass = &custom
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for custom aircraft class", err)
	}
}

func TestAircraftValidate_NilAircraftClass(t *testing.T) {
	a := validAircraft()
	a.AircraftClass = nil
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for nil aircraft class", err)
	}
}
