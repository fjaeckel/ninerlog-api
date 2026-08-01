package models

import (
	"reflect"
	"testing"
)

func TestNormalizeFlightListColumns(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "nil becomes an empty list, not nil",
			input: nil,
			want:  []string{},
		},
		{
			name:  "empty stays empty — in custom mode that means no optional columns",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "unknown keys are dropped",
			input: []string{"picTime", "totalTime", "definitelyNotAColumn"},
			want:  []string{"picTime"},
		},
		{
			name:  "duplicates collapse",
			input: []string{"nightTime", "nightTime", "nightTime"},
			want:  []string{"nightTime"},
		},
		{
			name:  "result is in canonical display order, not input order",
			input: []string{"remarks", "picTime", "offOnBlock", "landings"},
			want:  []string{"offOnBlock", "picTime", "landings", "remarks"},
		},
		{
			name:  "every known column survives",
			input: FlightListColumns,
			want:  FlightListColumns,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeFlightListColumns(tt.input)
			if got == nil {
				t.Fatal("NormalizeFlightListColumns() = nil, want a non-nil slice (the column is NOT NULL)")
			}
			if !reflect.DeepEqual([]string(got), tt.want) {
				t.Errorf("NormalizeFlightListColumns() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeFlightListColumnMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{FlightListColumnModeCustom, FlightListColumnModeCustom},
		{FlightListColumnModeAuto, FlightListColumnModeAuto},
		{"", FlightListColumnModeAuto},
		{"CUSTOM", FlightListColumnModeAuto},
		{"nonsense", FlightListColumnModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeFlightListColumnMode(tt.input); got != tt.want {
				t.Errorf("NormalizeFlightListColumnMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// The frontend and the OpenAPI enum both key off these strings, and the time
// columns' order decides which ones survive on a narrow screen. Freeze it so a
// reordering has to be a deliberate edit here as well.
func TestFlightListColumnsIsStable(t *testing.T) {
	want := []string{
		"offOnBlock",
		"picTime",
		"nightTime",
		"dualTime",
		"ifrTime",
		"crossCountryTime",
		"sicTime",
		"dualGivenTime",
		"multiPilotTime",
		"soloTime",
		"simulatedFlightTime",
		"function",
		"landings",
		"remarks",
	}
	if !reflect.DeepEqual(FlightListColumns, want) {
		t.Errorf("FlightListColumns = %v, want %v (update api-spec/openapi.yaml and the frontend registry too)", FlightListColumns, want)
	}
}
