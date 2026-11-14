package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHobbsMinutes(t *testing.T) {
	for _, tt := range []struct {
		name       string
		start, end *int
		want       int
		err        error
	}{
		{"H1 exact 1.3 hours", ptrHobbs(15234), ptrHobbs(15247), 78, nil},
		{"one tenth", ptrHobbs(0), ptrHobbs(1), 6, nil},
		{"maximum", ptrHobbs(0), ptrHobbs(MaxHobbsTenths), 599999994, nil},
		{"missing start", nil, ptrHobbs(2), 0, ErrHobbsReadings},
		{"missing end", ptrHobbs(2), nil, 0, ErrHobbsReadings},
		{"negative", ptrHobbs(-1), ptrHobbs(2), 0, ErrHobbsReadings},
		{"overflow bound", ptrHobbs(0), ptrHobbs(MaxHobbsTenths + 1), 0, ErrHobbsReadings},
		{"equal", ptrHobbs(1), ptrHobbs(1), 0, ErrHobbsOrder},
		{"reversed", ptrHobbs(3), ptrHobbs(1), 0, ErrHobbsOrder},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HobbsMinutes(tt.start, tt.end)
			require.ErrorIs(t, err, tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseHobbsReading(t *testing.T) {
	for _, value := range []string{"1523.4", "1523,4", " 1523.4 "} {
		got, err := ParseHobbsReading(value)
		require.NoError(t, err)
		require.Equal(t, 15234, got)
	}
	for _, value := range []string{"", "-1", "NaN", "1e3", "1:30", "1.25", "1.", "10000000", "1,234.5"} {
		_, err := ParseHobbsReading(value)
		require.Error(t, err, value)
	}
	got, err := ParseHobbsReading("1523")
	require.NoError(t, err)
	require.Equal(t, 15230, got)
	require.Equal(t, "1523.4", FormatHobbsReading(ptrHobbs(15234)))
}

func TestFlightTimeSource_LegacyAndHobbs(t *testing.T) {
	legacy := Flight{TotalTime: 83}
	require.NoError(t, legacy.ValidateFlightTimeSource())
	f := Flight{FlightTimeMethod: FlightTimeHobbs, HobbsStartTenths: ptrHobbs(15234), HobbsEndTenths: ptrHobbs(15247), TotalTime: 78}
	require.NoError(t, f.ValidateFlightTimeSource())
	raw, err := json.Marshal(f)
	require.NoError(t, err)
	var restored Flight
	require.NoError(t, json.Unmarshal(raw, &restored))
	require.Equal(t, f, restored)
	f.TotalTime = 79
	require.ErrorIs(t, f.ValidateFlightTimeSource(), ErrHobbsTotal)
	f.IsPassenger = true
	f.TotalTime = 0
	require.NoError(t, f.ValidateFlightTimeSource())
	f.IsSimulator = true
	require.ErrorIs(t, f.ValidateFlightTimeSource(), ErrHobbsMethod)
}

func ptrHobbs(v int) *int { return &v }
