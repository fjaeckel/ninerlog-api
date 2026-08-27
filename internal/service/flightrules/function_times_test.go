package flightrules

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestPaperColumnTimes(t *testing.T) {
	f := &models.Flight{PICTime: 60, PICUSTime: 30, SPICTime: 10, SICTime: 20, ReliefTime: 5, DualTime: 15}
	if got := PICColumnTime(f); got != 100 {
		t.Errorf("PICColumnTime = %d, want 100", got)
	}
	if got := CoPilotColumnTime(f); got != 25 {
		t.Errorf("CoPilotColumnTime = %d, want 25", got)
	}
	if got := FAASICColumnTime(f); got != 55 {
		t.Errorf("FAASICColumnTime = %d, want 55", got)
	}
	if got := FAADualColumnTime(f); got != 25 {
		t.Errorf("FAADualColumnTime = %d, want 25", got)
	}
}

func TestCombinedRemarks_FunctionTimeAnnotations(t *testing.T) {
	remarks := "LHR-FRA"
	f := &models.Flight{Remarks: &remarks, PICUSTime: 125, ReliefTime: 60}
	got := CombinedRemarks(f)
	want := "LHR-FRA [PICUS 2:05] [Relief 1:00]"
	if got != want {
		t.Errorf("CombinedRemarks = %q, want %q", got, want)
	}
}

func TestCombinedRemarks_NoAnnotationWithoutFunctionTimes(t *testing.T) {
	remarks := "local flight"
	f := &models.Flight{Remarks: &remarks}
	if got := CombinedRemarks(f); got != "local flight" {
		t.Errorf("CombinedRemarks = %q, want %q", got, "local flight")
	}
}

func TestDetermineRole_DeclaredFunctionTimeWithoutCrew(t *testing.T) {
	f := &models.Flight{TotalTime: 90, SPICTime: 90}
	if got := DetermineRole(f, "Test User", nil); got != RoleSIC {
		t.Errorf("DetermineRole = %v, want RoleSIC", got)
	}
}
