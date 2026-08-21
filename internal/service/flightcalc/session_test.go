package flightcalc

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func str(s string) *string { return &s }

// A device session is not flown between places. Everything that describes a
// flight is cleared; the session duration and the instrument work survive.
func TestApplyAutoCalculationsClearsFlightFieldsOnSession(t *testing.T) {
	fstd := "FNPT II"
	f := &models.Flight{
		Date:                    time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
		IsSimulator:             true,
		FSTDType:                &fstd,
		AircraftType:            "PA34",
		SimulatedFlightTime:     120,
		SimulatedInstrumentTime: 90,
		Holds:                   2,
		ApproachesCount:         3,

		// Everything below is what a client would send if it treated the
		// session as a flight.
		AircraftReg:          "FSTD-01",
		DepartureICAO:        str("EDDF"),
		ArrivalICAO:          str("EDDM"),
		OffBlockTime:         str("09:00:00"),
		OnBlockTime:          str("11:00:00"),
		TotalTime:            120,
		IsPIC:                true,
		PICTime:              120,
		DualGivenTime:        120,
		MultiPilotTime:       120,
		SoloTime:             120,
		CrossCountryTime:     120,
		NightTime:            30,
		IFRTime:              60,
		ActualInstrumentTime: 60,
		LandingsDay:          2,
		AllLandings:          2,
		Distance:             169.4,
	}

	ApplyAutoCalculations(f, "Test Pilot")

	zeros := map[string]int{
		"TotalTime": f.TotalTime, "PICTime": f.PICTime, "DualTime": f.DualTime,
		"SICTime": f.SICTime, "DualGivenTime": f.DualGivenTime,
		"MultiPilotTime": f.MultiPilotTime, "SoloTime": f.SoloTime,
		"CrossCountryTime": f.CrossCountryTime, "NightTime": f.NightTime,
		"IFRTime": f.IFRTime, "ActualInstrumentTime": f.ActualInstrumentTime,
		"LandingsDay": f.LandingsDay, "LandingsNight": f.LandingsNight,
		"AllLandings": f.AllLandings, "TakeoffsDay": f.TakeoffsDay,
		"TakeoffsNight": f.TakeoffsNight,
	}
	for name, got := range zeros {
		if got != 0 {
			t.Errorf("%s = %d, want 0 on a device session", name, got)
		}
	}
	if f.IsPIC || f.IsDual {
		t.Errorf("IsPIC=%v IsDual=%v, want both false on a device session", f.IsPIC, f.IsDual)
	}
	if f.Distance != 0 {
		t.Errorf("Distance = %v, want 0", f.Distance)
	}
	if f.AircraftReg != "" {
		t.Errorf("AircraftReg = %q, want empty — an FSTD has no registration", f.AircraftReg)
	}
	for name, got := range map[string]*string{
		"DepartureICAO": f.DepartureICAO, "ArrivalICAO": f.ArrivalICAO,
		"OffBlockTime": f.OffBlockTime, "OnBlockTime": f.OnBlockTime,
	} {
		if got != nil {
			t.Errorf("%s = %q, want nil on a device session", name, *got)
		}
	}

	if f.SimulatedFlightTime != 120 {
		t.Errorf("SimulatedFlightTime = %d, want the session duration 120", f.SimulatedFlightTime)
	}
	if f.SimulatedInstrumentTime != 90 {
		t.Errorf("SimulatedInstrumentTime = %d, want 90", f.SimulatedInstrumentTime)
	}
	if f.Holds != 2 || f.ApproachesCount != 3 {
		t.Errorf("holds=%d approaches=%d, want the instrument work preserved", f.Holds, f.ApproachesCount)
	}

	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("session does not validate after auto-calculation: %v", err)
	}
}

// Instrument time cannot exceed the session it was flown in.
func TestApplyAutoCalculationsCapsSessionInstrumentTime(t *testing.T) {
	fstd := "FFS A320"
	f := &models.Flight{
		IsSimulator: true, FSTDType: &fstd, AircraftType: "A320",
		SimulatedFlightTime: 60, SimulatedInstrumentTime: 200,
	}
	ApplyAutoCalculations(f, "Test Pilot")
	if f.SimulatedInstrumentTime != 60 {
		t.Errorf("SimulatedInstrumentTime = %d, want it capped at the session duration 60", f.SimulatedInstrumentTime)
	}
}

// A flight is untouched by the session path.
func TestApplyAutoCalculationsLeavesFlightsAlone(t *testing.T) {
	f := &models.Flight{
		Date:          time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
		AircraftReg:   "D-EABC",
		AircraftType:  "C172",
		DepartureICAO: str("EDDF"),
		ArrivalICAO:   str("EDDM"),
		OffBlockTime:  str("09:00:00"),
		OnBlockTime:   str("11:00:00"),
		TotalTime:     120,
		AllLandings:   1,
	}
	ApplyAutoCalculations(f, "Test Pilot")
	if f.TotalTime != 120 || f.PICTime != 120 {
		t.Errorf("totalTime=%d picTime=%d, want 120/120", f.TotalTime, f.PICTime)
	}
	if f.CrossCountryTime != 120 {
		t.Errorf("crossCountryTime = %d, want 120", f.CrossCountryTime)
	}
}

// A logbook row declaring co-pilot time with no crew list is the co-pilot's
// own entry: PIC time must not also be claimed, or the function times would
// exceed block time.
func TestDeclaredSICTimeWithoutCrewDoesNotAlsoClaimPIC(t *testing.T) {
	f := &models.Flight{
		Date:          time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
		AircraftReg:   "D-ABCD",
		AircraftType:  "A320",
		DepartureICAO: str("EDDF"),
		ArrivalICAO:   str("EDDM"),
		OffBlockTime:  str("09:00:00"),
		OnBlockTime:   str("11:00:00"),
		TotalTime:     120,
		SICTime:       120,
		AllLandings:   1,
	}
	ApplyAutoCalculations(f, "Test Pilot")

	if f.PICTime != 0 {
		t.Errorf("PICTime = %d, want 0 — the user flew as co-pilot", f.PICTime)
	}
	if f.SICTime != 120 {
		t.Errorf("SICTime = %d, want 120", f.SICTime)
	}
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("declared co-pilot time does not validate: %v", err)
	}
}
