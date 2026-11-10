package flightcalc

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
)

func picWhileDualFlight() *models.Flight {
	f := baseFlight()
	f.IsPIC = true
	f.IsDual = true
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Jane Instructor", Role: models.CrewRoleInstructor},
	}
	return f
}

func TestPICWhileDual_InstructorOnBoard_KeepsBoth(t *testing.T) {
	f := picWhileDualFlight()
	ApplyAutoCalculations(f, "Test User", nil)
	if !f.IsPIC || !f.IsDual {
		t.Fatalf("IsPIC=%v IsDual=%v, want true/true", f.IsPIC, f.IsDual)
	}
	if f.PICTime != f.TotalTime || f.DualTime != f.TotalTime {
		t.Errorf("PICTime=%d DualTime=%d, want %d/%d", f.PICTime, f.DualTime, f.TotalTime, f.TotalTime)
	}
	if f.SoloTime != 0 {
		t.Errorf("SoloTime = %d, want 0", f.SoloTime)
	}
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
	}
}

func TestPICWhileDual_ExaminerOnBoard_KeepsBoth(t *testing.T) {
	f := picWhileDualFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Sam Examiner", Role: models.CrewRoleExaminer},
	}
	ApplyAutoCalculations(f, "Test User", nil)
	if !f.IsPIC || !f.IsDual || f.PICTime != f.TotalTime {
		t.Errorf("IsPIC=%v IsDual=%v PICTime=%d, want true/true/%d", f.IsPIC, f.IsDual, f.PICTime, f.TotalTime)
	}
}

func TestPICWhileDual_Recalculation_IsStable(t *testing.T) {
	f := picWhileDualFlight()
	ApplyAutoCalculations(f, "Test User", nil)
	ApplyAutoCalculations(f, "Test User", nil)
	if !f.IsPIC || !f.IsDual || f.PICTime != f.TotalTime || f.DualTime != f.TotalTime {
		t.Errorf("IsPIC=%v IsDual=%v PICTime=%d DualTime=%d after second run", f.IsPIC, f.IsDual, f.PICTime, f.DualTime)
	}
}

func TestPICWhileDual_DeclaredSPIC_CarvesBoth(t *testing.T) {
	f := picWhileDualFlight()
	f.SPICTime = 30
	ApplyAutoCalculations(f, "Test User", nil)
	want := f.TotalTime - 30
	if f.PICTime != want || f.DualTime != want {
		t.Errorf("PICTime=%d DualTime=%d, want %d/%d", f.PICTime, f.DualTime, want, want)
	}
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
	}
}

func TestPICWhileDual_NotDeclared_DualOnly(t *testing.T) {
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Jane Instructor", Role: models.CrewRoleInstructor},
	}
	ApplyAutoCalculations(f, "Test User", nil)
	if f.IsPIC || !f.IsDual || f.PICTime != 0 {
		t.Errorf("IsPIC=%v IsDual=%v PICTime=%d, want false/true/0", f.IsPIC, f.IsDual, f.PICTime)
	}
}

func TestPICWhileDual_NoInstructor_PICOnly(t *testing.T) {
	f := picWhileDualFlight()
	f.CrewMembers = nil
	ApplyAutoCalculations(f, "Test User", nil)
	if !f.IsPIC || f.IsDual || f.DualTime != 0 || f.PICTime != f.TotalTime {
		t.Errorf("IsPIC=%v IsDual=%v PICTime=%d DualTime=%d, want true/false/%d/0", f.IsPIC, f.IsDual, f.PICTime, f.DualTime, f.TotalTime)
	}
}

func TestPICWhileDual_CoPilot_ClearsBoth(t *testing.T) {
	f := picWhileDualFlight()
	f.AircraftReg = "D-AIBC"
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Captain Smith", Role: models.CrewRolePIC},
	}
	ApplyAutoCalculations(f, "Test User", &flightrules.AircraftFacts{Registration: "D-AIBC", IsMultiPilot: true})
	if f.IsPIC || f.IsDual || f.PICTime != 0 || f.DualTime != 0 {
		t.Errorf("IsPIC=%v IsDual=%v PICTime=%d DualTime=%d, want cleared", f.IsPIC, f.IsDual, f.PICTime, f.DualTime)
	}
}

func TestPICWhileDual_Session_ClearsBoth(t *testing.T) {
	f := picWhileDualFlight()
	f.IsSimulator = true
	fstd := "FNPT II"
	f.FSTDType = &fstd
	f.SimulatedFlightTime = 60
	ApplyAutoCalculations(f, "Test User", nil)
	if f.IsPIC || f.IsDual {
		t.Errorf("IsPIC=%v IsDual=%v, want false/false", f.IsPIC, f.IsDual)
	}
}
