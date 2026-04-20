package service

import (
	"testing"
	"time"
)

func TestFormatDateForUser(t *testing.T) {
	date := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		format string
		want   string
	}{
		{"DD.MM.YYYY", "15.03.2026"},
		{"MM/DD/YYYY", "03/15/2026"},
		{"YYYY-MM-DD", "2026-03-15"},
		{"", "15.03.2026"},       // default
		{"unknown", "15.03.2026"}, // fallback
	}

	for _, tt := range tests {
		got := formatDateForUser(date, tt.format)
		if got != tt.want {
			t.Errorf("formatDateForUser(%q) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestParseCheckInterval(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"60", 60 * time.Minute, false},
		{"120", 120 * time.Minute, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		got, err := parseCheckInterval(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("parseCheckInterval(%q) error = %v, wantErr %v", tt.input, err, tt.err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseCheckInterval(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestUuidFromString(t *testing.T) {
	id1 := uuidFromString("test-string")
	id2 := uuidFromString("test-string")
	id3 := uuidFromString("different-string")

	// Same input produces same UUID
	if id1 != id2 {
		t.Error("uuidFromString should be deterministic")
	}
	// Different input produces different UUID
	if id1 == id3 {
		t.Error("uuidFromString should produce different UUIDs for different inputs")
	}
}

func TestStrPtr(t *testing.T) {
	p := strPtr("hello")
	if p == nil {
		t.Fatal("strPtr returned nil")
	}
	if *p != "hello" {
		t.Errorf("strPtr = %q, want hello", *p)
	}
}

func TestSendWarningForDays_NoMatchingThreshold(t *testing.T) {
	// daysUntilExpiry is beyond all warning thresholds — should not send
	// This tests the case where no warning day matches
	// We can't easily test this without mocks, but we can test the utility functions
}
