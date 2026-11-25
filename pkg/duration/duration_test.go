package duration

import (
	"testing"
)

func TestMinutesToDecimalHours(t *testing.T) {
	tests := []struct {
		minutes int
		want    float64
	}{
		{0, 0},
		{60, 1.0},
		{90, 1.5},
		{83, 1.3833333333333333},
		{150, 2.5},
	}
	for _, tt := range tests {
		got := MinutesToDecimalHours(tt.minutes)
		if got != tt.want {
			t.Errorf("MinutesToDecimalHours(%d) = %f, want %f", tt.minutes, got, tt.want)
		}
	}
}

func TestDecimalHoursToMinutes(t *testing.T) {
	tests := []struct {
		hours float64
		want  int
	}{
		{0, 0},
		{1.0, 60},
		{1.5, 90},
		{1.38, 83},
		{2.5, 150},
		{0.75, 45},
	}
	for _, tt := range tests {
		got := DecimalHoursToMinutes(tt.hours)
		if got != tt.want {
			t.Errorf("DecimalHoursToMinutes(%f) = %d, want %d", tt.hours, got, tt.want)
		}
	}
}

func TestFormatHM(t *testing.T) {
	tests := []struct {
		minutes int
		want    string
	}{
		{0, "0h 0m"},
		{60, "1h 0m"},
		{90, "1h 30m"},
		{83, "1h 23m"},
		{150, "2h 30m"},
		{45, "0h 45m"},
	}
	for _, tt := range tests {
		got := FormatHM(tt.minutes)
		if got != tt.want {
			t.Errorf("FormatHM(%d) = %q, want %q", tt.minutes, got, tt.want)
		}
	}
}

func TestFormatDecimal(t *testing.T) {
	tests := []struct {
		minutes int
		want    string
	}{
		{0, "0.0h"},
		{60, "1.0h"},
		{90, "1.5h"},
		{83, "1.4h"},
		{150, "2.5h"},
		{45, "0.8h"},
	}
	for _, tt := range tests {
		got := FormatDecimal(tt.minutes)
		if got != tt.want {
			t.Errorf("FormatDecimal(%d) = %q, want %q", tt.minutes, got, tt.want)
		}
	}
}

func TestFormatColonHM(t *testing.T) {
	tests := []struct {
		minutes int
		want    string
	}{
		{0, "0:00"},
		{60, "1:00"},
		{90, "1:30"},
		{83, "1:23"},
		{150, "2:30"},
	}
	for _, tt := range tests {
		got := FormatColonHM(tt.minutes)
		if got != tt.want {
			t.Errorf("FormatColonHM(%d) = %q, want %q", tt.minutes, got, tt.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		// Colon format
		{"1:23", 83, false},
		{"01:30", 90, false},
		{"0:45", 45, false},
		{"2:00", 120, false},
		{"10:05", 605, false},

		// HM format
		{"1h23m", 83, false},
		{"1h 23m", 83, false},
		{"1h", 60, false},
		{"23m", 23, false},
		{"2h30m", 150, false},

		// Decimal hours
		{"1.5", 90, false},
		{"1.38", 83, false},
		{"0.75", 45, false},
		{"2.5", 150, false},
		{"0.0", 0, false},

		// Decimal hours with "h" suffix (own CSV export format, FormatDecimal)
		{"1.5h", 90, false},
		{"2.1h", 126, false},
		{"0.0h", 0, false},
		{"1.4h", 84, false},

		// Decimal comma variants (European locale exports)
		{"1,5", 90, false},
		{"2,1h", 126, false},
		{"0,0h", 0, false},

		// Bare minutes
		{"83", 83, false},
		{"0", 0, false},
		{"150", 150, false},

		// Whitespace
		{" 1:23 ", 83, false},
		{" 1h 23m ", 83, false},

		// Errors
		{"", 0, true},
		{"abc", 0, true},
		{"-1", 0, true},
		{"-1.5", 0, true},
		{"1:60", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseDuration(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseDuration(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseDurationAs(t *testing.T) {
	tests := []struct {
		input   string
		unit    Unit
		want    int
		wantErr bool
	}{
		// Conversion table from issue #297.
		{"0.1", UnitDecimalHours, 6, false},
		{"0.5", UnitDecimalHours, 30, false},
		{"1.3", UnitDecimalHours, 78, false},
		{"1.5", UnitDecimalHours, 90, false},
		{"2.0", UnitDecimalHours, 120, false},
		{"10.2", UnitDecimalHours, 612, false},
		{"2", UnitDecimalHours, 120, false},
		{"1,5", UnitDecimalHours, 90, false},
		{"0", UnitDecimalHours, 0, false},
		{"1:18", UnitDecimalHours, 78, false},
		{"1h 18m", UnitDecimalHours, 78, false},

		{"90", UnitMinutes, 90, false},
		{"2", UnitMinutes, 2, false},
		{"1:30", UnitMinutes, 90, false},
		{"1.5", UnitMinutes, 0, true},

		{"2", UnitAuto, 2, false},
		{"1.5", UnitAuto, 90, false},
		{"2", "", 2, false},

		{"-1", UnitDecimalHours, 0, true},
		{"abc", UnitDecimalHours, 0, true},
		{"", UnitDecimalHours, 0, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.unit)+"/"+tt.input, func(t *testing.T) {
			got, err := ParseDurationAs(tt.input, tt.unit)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDurationAs(%q, %q) error = %v, wantErr %v", tt.input, tt.unit, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseDurationAs(%q, %q) = %d, want %d", tt.input, tt.unit, got, tt.want)
			}
		})
	}
}

func TestInferUnit(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   Unit
	}{
		{"decimal hours with whole numbers", []string{"1.3", "2", "", "0.8"}, UnitDecimalHours},
		{"comma decimal", []string{"1,5", "2"}, UnitDecimalHours},
		{"integers only", []string{"90", "45", "2"}, UnitAuto},
		{"colon form", []string{"1:30", "2"}, UnitAuto},
		{"mixed colon and decimal", []string{"1:30", "1.5"}, UnitAuto},
		{"suffixed hours", []string{"1.5h", "2"}, UnitAuto},
		{"empty", []string{"", " "}, UnitAuto},
		{"nil", nil, UnitAuto},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InferUnit(tt.values); got != tt.want {
				t.Errorf("InferUnit(%q) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}
