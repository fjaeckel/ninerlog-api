package models

import (
	"testing"
)

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"InvalidTimeDistribution", ErrInvalidTimeDistribution, "PIC and dual time are mutually exclusive"},
		{"InvalidNightTime", ErrInvalidNightTime, "night time exceeds total time"},
		{"InvalidIFRTime", ErrInvalidIFRTime, "IFR time exceeds total time"},
		{"NegativeTime", ErrNegativeTime, "flight times cannot be negative"},
		{"NegativeLandings", ErrNegativeLandings, "landings cannot be negative"},
		{"AircraftRegistrationRequired", ErrAircraftRegistrationRequired, "aircraft registration is required"},
		{"AircraftTypeRequired", ErrAircraftTypeRequired, "aircraft type is required"},
		{"AircraftMakeRequired", ErrAircraftMakeRequired, "aircraft make is required"},
		{"AircraftModelRequired", ErrAircraftModelRequired, "aircraft model is required"},
		{"AircraftInvalidEngineType", ErrAircraftInvalidEngineType, "invalid engine type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.want {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.want)
			}
		})
	}
}

func TestErrorsAreDistinct(t *testing.T) {
	errs := []error{
		ErrInvalidTimeDistribution,
		ErrInvalidNightTime,
		ErrInvalidIFRTime,
		ErrNegativeTime,
		ErrNegativeLandings,
		ErrAircraftRegistrationRequired,
		ErrAircraftTypeRequired,
		ErrAircraftMakeRequired,
		ErrAircraftModelRequired,
		ErrAircraftInvalidEngineType,
	}
	seen := make(map[string]bool)
	for _, e := range errs {
		msg := e.Error()
		if seen[msg] {
			t.Errorf("Duplicate error message: %q", msg)
		}
		seen[msg] = true
	}
}
