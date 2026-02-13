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

func TestAircraftValidate_ValidEngineTypes(t *testing.T) {
	validTypes := []EngineType{EngineTypePiston, EngineTypeTurboprop, EngineTypeJet, EngineTypeElectric}
	for _, et := range validTypes {
		a := validAircraft()
		a.EngineType = &et
		if err := a.Validate(); err != nil {
			t.Errorf("Validate() with engine type %s = %v, want nil", et, err)
		}
	}
}

func TestAircraftValidate_InvalidEngineType(t *testing.T) {
	a := validAircraft()
	invalid := EngineType("nuclear")
	a.EngineType = &invalid
	if err := a.Validate(); err != ErrAircraftInvalidEngineType {
		t.Errorf("Validate() = %v, want ErrAircraftInvalidEngineType", err)
	}
}

func TestAircraftValidate_NilEngineType(t *testing.T) {
	a := validAircraft()
	a.EngineType = nil
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for nil engine type", err)
	}
}
