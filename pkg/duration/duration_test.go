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
