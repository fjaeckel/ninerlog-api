package service

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestApplyRecency(t *testing.T) {
	regStats := []*models.AircraftStats{
		{Registration: "D-EAAA"},
		{Registration: "D-EBBB"},
		{Registration: "D-ENON"}, // no flights in window
	}
	typeStats := []*models.AircraftTypeStats{
		{AircraftType: "C172"},
	}
	// Rows newest first: both registrations are C172
	rows := []*models.AircraftRecencyRow{
		{Registration: "D-EAAA", AircraftType: "C172", Date: day("2026-07-10"), Landings: 1},
		{Registration: "D-EBBB", AircraftType: "C172", Date: day("2026-07-05"), Landings: 2},
		{Registration: "D-EAAA", AircraftType: "C172", Date: day("2026-06-01"), Landings: 2},
	}

	applyRecency(regStats, typeStats, rows)

	// D-EAAA: 3 landings; the 3rd-most-recent landing is on 2026-06-01
	if regStats[0].LandingsLast90Days != 3 {
		t.Errorf("D-EAAA landings: want 3, got %d", regStats[0].LandingsLast90Days)
	}
	if regStats[0].RecencyLapsesOn == nil || !regStats[0].RecencyLapsesOn.Equal(day("2026-06-01").AddDate(0, 0, 90)) {
		t.Errorf("D-EAAA lapse: want 2026-08-30, got %v", regStats[0].RecencyLapsesOn)
	}

	// D-EBBB: only 2 landings — no lapse date
	if regStats[1].LandingsLast90Days != 2 {
		t.Errorf("D-EBBB landings: want 2, got %d", regStats[1].LandingsLast90Days)
	}
	if regStats[1].RecencyLapsesOn != nil {
		t.Errorf("D-EBBB lapse: want nil, got %v", regStats[1].RecencyLapsesOn)
	}

	// D-ENON: untouched zero values
	if regStats[2].LandingsLast90Days != 0 || regStats[2].RecencyLapsesOn != nil {
		t.Errorf("D-ENON: expected zero recency, got %d / %v", regStats[2].LandingsLast90Days, regStats[2].RecencyLapsesOn)
	}

	// C172 type aggregates across both registrations: 5 landings; the type
	// count reaches 3 already at 2026-07-05 (1 + 2 newest-first)
	if typeStats[0].LandingsLast90Days != 5 {
		t.Errorf("C172 landings: want 5, got %d", typeStats[0].LandingsLast90Days)
	}
	if typeStats[0].RecencyLapsesOn == nil || !typeStats[0].RecencyLapsesOn.Equal(day("2026-07-05").AddDate(0, 0, 90)) {
		t.Errorf("C172 lapse: want 2026-10-03, got %v", typeStats[0].RecencyLapsesOn)
	}
}
