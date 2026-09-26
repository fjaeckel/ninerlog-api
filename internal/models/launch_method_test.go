package models

import (
	"errors"
	"testing"
)

func derefOr(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}

func TestValidateFlightTextFields_LaunchMethod(t *testing.T) {
	tests := []struct {
		name    string
		method  *string
		wantErr bool
	}{
		{"nil", nil, false},
		{"empty", strp(""), false},
		{"winch", strp("winch"), false},
		{"aerotow", strp("aerotow"), false},
		{"self-launch", strp("self-launch"), false},
		{"car", strp("car"), false},
		{"bungee", strp("bungee"), false},
		{"unknown value", strp("catapult"), true},
		{"vereinsflieger code", strp("W"), true},
		{"wrong case", strp("Winch"), true},
		{"within the length bound", strp("self-launch-winch"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Flight{AircraftReg: "D-1234", AircraftType: "ASK21", LaunchMethod: tt.method}
			err := ValidateFlightTextFields(f)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidLaunchMethod) {
					t.Errorf("err = %v, want ErrInvalidLaunchMethod", err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNormalizeLaunchMethod(t *testing.T) {
	tests := []struct {
		name string
		in   *string
		want *string
	}{
		{"nil stays nil", nil, nil},
		{"blank clears", strp("  "), nil},
		{"lower-cases and trims", strp(" Winch "), strp("winch")},
		{"valid unchanged", strp("aerotow"), strp("aerotow")},
		{"unknown kept for validation", strp("Catapult"), strp("catapult")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Flight{LaunchMethod: tt.in}
			NormalizeLaunchMethod(f)
			if derefOr(f.LaunchMethod, "<nil>") != derefOr(tt.want, "<nil>") {
				t.Errorf("LaunchMethod = %s, want %s", derefOr(f.LaunchMethod, "<nil>"), derefOr(tt.want, "<nil>"))
			}
		})
	}
}

func TestIsTowedLaunch(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"winch", true},
		{"aerotow", true},
		{"car", true},
		{"bungee", true},
		{"self-launch", false},
		{"", false},
		{"catapult", false},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := IsTowedLaunch(tt.method); got != tt.want {
				t.Errorf("IsTowedLaunch(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestLaunchMethodApplies(t *testing.T) {
	kind := func(k ULKind) *ULKind { return &k }
	tests := []struct {
		name  string
		class *string
		kind  *ULKind
		want  bool
	}{
		{"no class", nil, nil, true},
		{"blank class", strp(""), nil, true},
		{"L2 glider", strp("GLIDER"), nil, true},
		{"glider lower case", strp(" glider "), nil, true},
		{"TMG", strp("TMG"), nil, true},
		{"UL without kind", strp("ULTRALIGHT"), nil, true},
		{"UL sailplane", strp("ULTRALIGHT"), kind(ULKindSailplane), true},
		{"UL motorglider", strp("ULTRALIGHT"), kind(ULKindThreeAxisMotorglider), true},
		{"UL three-axis", strp("ULTRALIGHT"), kind(ULKindThreeAxis), false},
		{"UL trike", strp("ULTRALIGHT"), kind(ULKindWeightShift), false},
		{"A2 SEP land", strp("SEP_LAND"), nil, false},
		{"SEP sea", strp("SEP_SEA"), nil, false},
		{"MEP land", strp("MEP_LAND"), nil, false},
		{"SET land", strp("SET_LAND"), nil, false},
		{"gyroplane", strp("GYROPLANE"), nil, false},
		{"other", strp("OTHER"), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LaunchMethodApplies(tt.class, tt.kind); got != tt.want {
				t.Errorf("LaunchMethodApplies = %v, want %v", got, tt.want)
			}
		})
	}
}
