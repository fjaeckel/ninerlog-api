package models

import (
	"errors"
	"testing"
	"time"

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
	if err := a.Validate(); !errors.Is(err, ErrAircraftRegistrationRequired) {
		t.Errorf("Validate() = %v, want ErrAircraftRegistrationRequired", err)
	}
}

func TestAircraftValidate_MissingType(t *testing.T) {
	a := validAircraft()
	a.Type = ""
	if err := a.Validate(); !errors.Is(err, ErrAircraftTypeRequired) {
		t.Errorf("Validate() = %v, want ErrAircraftTypeRequired", err)
	}
}

func TestAircraftValidate_MissingMake(t *testing.T) {
	a := validAircraft()
	a.Make = ""
	if err := a.Validate(); !errors.Is(err, ErrAircraftMakeRequired) {
		t.Errorf("Validate() = %v, want ErrAircraftMakeRequired", err)
	}
}

func TestAircraftValidate_MissingModel(t *testing.T) {
	a := validAircraft()
	a.Model = ""
	if err := a.Validate(); !errors.Is(err, ErrAircraftModelRequired) {
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

func validityDate(s string) *time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestAircraftValidate_InvertedValidity(t *testing.T) {
	a := &Aircraft{Registration: "N123AB", Type: "C172", Make: "Cessna", Model: "172",
		ValidFrom: validityDate("2020-01-02"), ValidTo: validityDate("2020-01-01")}
	if err := a.Validate(); !errors.Is(err, ErrInvalidAircraftValidity) {
		t.Errorf("Validate() = %v, want ErrInvalidAircraftValidity", err)
	}
	a.ValidTo = validityDate("2020-01-02")
	if err := a.Validate(); err != nil {
		t.Errorf("single-day range: Validate() = %v, want nil", err)
	}
}

func TestAircraftCoversDate(t *testing.T) {
	a := &Aircraft{ValidFrom: validityDate("2020-01-01"), ValidTo: validityDate("2020-12-31")}
	tests := []struct {
		date string
		want bool
	}{
		{"2019-12-31", false},
		{"2020-01-01", true},
		{"2020-06-15", true},
		{"2020-12-31", true},
		{"2021-01-01", false},
	}
	for _, tt := range tests {
		if got := a.CoversDate(*validityDate(tt.date)); got != tt.want {
			t.Errorf("CoversDate(%s) = %v, want %v", tt.date, got, tt.want)
		}
	}
	if !(&Aircraft{}).CoversDate(*validityDate("1950-01-01")) {
		t.Error("open range must cover every date")
	}
}

func TestAircraftValidityOverlaps(t *testing.T) {
	ac := func(from, to string) *Aircraft {
		a := &Aircraft{}
		if from != "" {
			a.ValidFrom = validityDate(from)
		}
		if to != "" {
			a.ValidTo = validityDate(to)
		}
		return a
	}
	tests := []struct {
		name string
		a, b *Aircraft
		want bool
	}{
		{"both open", ac("", ""), ac("", ""), true},
		{"open vs bounded", ac("", ""), ac("2020-01-01", "2020-12-31"), true},
		{"consecutive", ac("", "2019-12-31"), ac("2020-01-01", ""), false},
		{"shared day", ac("", "2020-01-01"), ac("2020-01-01", ""), true},
		{"disjoint bounded", ac("2010-01-01", "2011-01-01"), ac("2015-01-01", "2016-01-01"), false},
		{"nested", ac("2010-01-01", "2020-01-01"), ac("2015-01-01", "2016-01-01"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.ValidityOverlaps(tt.b); got != tt.want {
				t.Errorf("a.ValidityOverlaps(b) = %v, want %v", got, tt.want)
			}
			if got := tt.b.ValidityOverlaps(tt.a); got != tt.want {
				t.Errorf("b.ValidityOverlaps(a) = %v, want %v", got, tt.want)
			}
		})
	}
}
