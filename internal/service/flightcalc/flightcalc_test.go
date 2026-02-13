package flightcalc

import (
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
)

func strPtr(s string) *string { return &s }

func baseFlight() *models.Flight {
	return &models.Flight{
		Date:          time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		AircraftReg:   "D-EFGH",
		AircraftType:  "C172",
		DepartureICAO: strPtr("EDDF"),
		ArrivalICAO:   strPtr("EDDH"),
		DepartureTime: strPtr("14:30:00"),
		ArrivalTime:   strPtr("16:00:00"),
		TotalTime:     1.5,
		IsPIC:         true,
		IsDual:        false,
		PICTime:       1.5,
		LandingsDay:   2,
		LandingsNight: 1,
	}
}

func TestSoloTime_PIC(t *testing.T) {
	f := baseFlight()
	ApplyAutoCalculations(f)
	if f.SoloTime != f.TotalTime {
		t.Errorf("expected soloTime=%v when PIC, got %v", f.TotalTime, f.SoloTime)
	}
}

func TestSoloTime_Dual(t *testing.T) {
	f := baseFlight()
	f.IsPIC = false
	f.IsDual = true
	ApplyAutoCalculations(f)
	if f.SoloTime != 0 {
		t.Errorf("expected soloTime=0 when dual, got %v", f.SoloTime)
	}
}

func TestSoloTime_NeitherPICNorDual(t *testing.T) {
	f := baseFlight()
	f.IsPIC = false
	f.IsDual = false
	ApplyAutoCalculations(f)
	if f.SoloTime != 0 {
		t.Errorf("expected soloTime=0, got %v", f.SoloTime)
	}
}

func TestCrossCountryTime_DifferentAirports(t *testing.T) {
	f := baseFlight()
	ApplyAutoCalculations(f)
	if f.CrossCountryTime != f.TotalTime {
		t.Errorf("expected crossCountryTime=%v, got %v", f.TotalTime, f.CrossCountryTime)
	}
}

func TestCrossCountryTime_SameAirport(t *testing.T) {
	f := baseFlight()
	f.DepartureICAO = strPtr("EDDF")
	f.ArrivalICAO = strPtr("EDDF")
	ApplyAutoCalculations(f)
	if f.CrossCountryTime != 0 {
		t.Errorf("expected crossCountryTime=0 for same airport, got %v", f.CrossCountryTime)
	}
}

func TestCrossCountryTime_NilAirports(t *testing.T) {
	f := baseFlight()
	f.DepartureICAO = nil
	f.ArrivalICAO = nil
	ApplyAutoCalculations(f)
	if f.CrossCountryTime != 0 {
		t.Errorf("expected crossCountryTime=0, got %v", f.CrossCountryTime)
	}
}

func TestAllLandings_Sum(t *testing.T) {
	f := baseFlight()
	f.LandingsDay = 3
	f.LandingsNight = 2
	f.LandingsDayOverride = true
	f.LandingsNightOverride = true
	ApplyAutoCalculations(f)
	if f.AllLandings != 5 {
		t.Errorf("expected allLandings=5, got %d", f.AllLandings)
	}
}

func TestDistance_Haversine(t *testing.T) {
	dist := haversineNM(50.0379, 8.5622, 53.6304, 10.0065)
	if dist < 200 || dist > 250 {
		t.Errorf("EDDF-EDDH distance should be ~220 NM, got %.1f", dist)
	}
}

func TestDistance_SameLocation(t *testing.T) {
	dist := haversineNM(50.0, 8.0, 50.0, 8.0)
	if dist != 0 {
		t.Errorf("same point distance should be 0, got %.1f", dist)
	}
}

func TestTakeoffOverride_Respected(t *testing.T) {
	f := baseFlight()
	f.TakeoffsDay = 2
	f.TakeoffsNight = 1
	f.TakeoffsDayOverride = true
	f.TakeoffsNightOverride = true
	ApplyAutoCalculations(f)
	if f.TakeoffsDay != 2 || f.TakeoffsNight != 1 {
		t.Errorf("overridden takeoffs modified: day=%d night=%d", f.TakeoffsDay, f.TakeoffsNight)
	}
}

func TestLandingOverride_Respected(t *testing.T) {
	f := baseFlight()
	f.LandingsDay = 5
	f.LandingsNight = 3
	f.LandingsDayOverride = true
	f.LandingsNightOverride = true
	ApplyAutoCalculations(f)
	if f.LandingsDay != 5 || f.LandingsNight != 3 {
		t.Errorf("overridden landings modified: day=%d night=%d", f.LandingsDay, f.LandingsNight)
	}
	if f.AllLandings != 8 {
		t.Errorf("allLandings should be 8, got %d", f.AllLandings)
	}
}

func TestParseTimeOfDay(t *testing.T) {
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		input  string
		hour   int
		minute int
	}{
		{"14:30:00", 14, 30},
		{"08:15", 8, 15},
		{"23:59:59", 23, 59},
	}
	for _, tc := range cases {
		result, err := parseTimeOfDay(date, tc.input)
		if err != nil {
			t.Errorf("parseTimeOfDay(%q) failed: %v", tc.input, err)
			continue
		}
		if result.Hour() != tc.hour || result.Minute() != tc.minute {
			t.Errorf("parseTimeOfDay(%q) = %v, expected %02d:%02d", tc.input, result.Format("15:04"), tc.hour, tc.minute)
		}
	}
}

func TestNormalizeICAO(t *testing.T) {
	cases := []struct {
		input    *string
		expected string
	}{
		{strPtr("eddf"), "EDDF"},
		{strPtr("EDDH"), "EDDH"},
		{strPtr(" EDDS "), "EDDS"},
		{nil, ""},
		{strPtr(""), ""},
	}
	for _, tc := range cases {
		result := normalizeICAO(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeICAO = %q, expected %q", result, tc.expected)
		}
	}
}
