package handlers

import (
	"testing"

	"github.com/oapi-codegen/nullable"
)

func TestApplyOverride(t *testing.T) {
	tests := []struct {
		name         string
		in           nullable.Nullable[int]
		wantValue    int
		wantOverride bool
	}{
		{"omitted leaves value and flag", nullable.Nullable[int]{}, 42, true},
		{"null zeroes the value and clears the flag", nullable.NewNullNullable[int](), 0, false},
		{"number sets value and flag", nullable.NewNullableWithValue(7), 7, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, override := 42, true
			applyOverride(&value, &override, tt.in)
			if value != tt.wantValue || override != tt.wantOverride {
				t.Errorf("got (%d, %v), want (%d, %v)", value, override, tt.wantValue, tt.wantOverride)
			}
		})
	}

	t.Run("number on an unset flag sets it", func(t *testing.T) {
		value, override := 0, false
		applyOverride(&value, &override, nullable.NewNullableWithValue(15))
		if value != 15 || !override {
			t.Errorf("got (%d, %v), want (15, true)", value, override)
		}
	})
}
