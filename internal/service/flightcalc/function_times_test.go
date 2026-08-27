package flightcalc

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// Declared PICUS/SPIC/relief minutes carve out of the derived function time.

// Full-sector PICUS on a multi-pilot operation: the captain is listed as PIC,
// the pilot declares the whole block as PICUS and logs no co-pilot time.
func TestPICUS_FullSector_OtherPIC(t *testing.T) {
	f := baseFlight()
	f.AircraftReg = "D-AIBC"
	f.PICUSTime = f.TotalTime
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Captain Smith", Role: models.CrewRolePIC},
	}
	ApplyAutoCalculations(f, "Test User", multiPilotAircraft())
	if f.IsPIC || f.PICTime != 0 {
		t.Errorf("IsPIC=%v PICTime=%d, want false/0", f.IsPIC, f.PICTime)
	}
	if f.SICTime != 0 {
		t.Errorf("SICTime = %d, want 0 (full sector declared PICUS)", f.SICTime)
	}
	if f.PICUSTime != f.TotalTime {
		t.Errorf("PICUSTime = %d, want %d (declared value kept)", f.PICUSTime, f.TotalTime)
	}
	if f.MultiPilotTime != f.TotalTime {
		t.Errorf("MultiPilotTime = %d, want %d", f.MultiPilotTime, f.TotalTime)
	}
}

// A partial relief declaration leaves the remainder as co-pilot time.
func TestRelief_Partial_OtherPIC(t *testing.T) {
	f := baseFlight()
	f.AircraftReg = "D-AIBC"
	f.ReliefTime = 30
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Captain Smith", Role: models.CrewRolePIC},
	}
	ApplyAutoCalculations(f, "Test User", multiPilotAircraft())
	if want := f.TotalTime - 30; f.SICTime != want {
		t.Errorf("SICTime = %d, want %d", f.SICTime, want)
	}
}

// With no crew list at all, a declared PICUS time places the user in the
// supervised seat rather than defaulting to PIC.
func TestPICUS_NoCrew_UserNotPIC(t *testing.T) {
	f := baseFlight()
	f.PICUSTime = f.TotalTime
	ApplyAutoCalculations(f, "Test User", nil)
	if f.IsPIC || f.PICTime != 0 {
		t.Errorf("IsPIC=%v PICTime=%d, want false/0", f.IsPIC, f.PICTime)
	}
	if f.SICTime != 0 {
		t.Errorf("SICTime = %d, want 0", f.SICTime)
	}
	if f.IsPassenger {
		t.Error("IsPassenger = true, want false (declared PICUS is crewing)")
	}
}

// SPIC with a third-party instructor on board carves out of dual received.
func TestSPIC_FullSector_InstructorOnBoard(t *testing.T) {
	f := baseFlight()
	f.SPICTime = f.TotalTime
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "FI Jones", Role: models.CrewRoleInstructor},
	}
	ApplyAutoCalculations(f, "Test User", nil)
	if !f.IsDual {
		t.Error("IsDual = false, want true (instructor on board)")
	}
	if f.DualTime != 0 {
		t.Errorf("DualTime = %d, want 0 (full sector declared SPIC)", f.DualTime)
	}
	if f.SPICTime != f.TotalTime {
		t.Errorf("SPICTime = %d, want %d", f.SPICTime, f.TotalTime)
	}
}

// Any declared function time means another pilot was on board, so no solo.
func TestSoloTime_ZeroWithDeclaredFunctionTime(t *testing.T) {
	f := baseFlight()
	f.ExaminerTime = 30
	ApplyAutoCalculations(f, "Test User", nil)
	if f.SoloTime != 0 {
		t.Errorf("SoloTime = %d, want 0", f.SoloTime)
	}
}

// FSTD sessions and passenger flights clear declared function times.
func TestSessionAndPassenger_ClearFunctionTimes(t *testing.T) {
	fstd := "FNPT II"
	f := baseFlight()
	f.IsSimulator = true
	f.FSTDType = &fstd
	f.SimulatedFlightTime = 60
	f.PICUSTime = 30
	f.ReliefTime = 10
	ApplyAutoCalculations(f, "Test User", nil)
	if f.PICUSTime != 0 || f.ReliefTime != 0 {
		t.Errorf("session PICUSTime=%d ReliefTime=%d, want 0/0", f.PICUSTime, f.ReliefTime)
	}
}
