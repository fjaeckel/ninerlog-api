package currency

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── EASA MEP/SET Proficiency Check Tests ────────────────────────────────

func TestEASA_MEP_ProfCheckOnly_Current(t *testing.T) {
	// MEP can be revalidated by proficiency check alone (no sectors/instructor needed)
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{
		Flights: 0, InstructorHours: 0, // No experience at all
	}
	profDate := time.Now().AddDate(0, -2, 0)
	dp.lastProficiencyCheck = &profDate

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeMEPLand, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "CPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (proficiency check should satisfy MEP revalidation)", result.Status)
	}
}

func TestEASA_MEP_NoProfCheck_NoExperience_Expiring(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{
		Flights: 3, InstructorHours: 0, // Insufficient experience
	}
	// No proficiency check

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeMEPLand, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "CPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (no prof check + insufficient experience)", result.Status)
	}
}

// ── EASA IR Proficiency Check Tests ─────────────────────────────────────

func TestEASA_IR_WithProfCheck_Current(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 15}
	profDate := time.Now().AddDate(0, -3, 0)
	dp.lastProficiencyCheck = &profDate

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (IR with prof check + IFR hours)", result.Status)
	}
	// Should have 2 requirements: IFR Hours + Proficiency Check
	if len(result.Requirements) != 2 {
		t.Fatalf("Expected 2 requirements, got %d", len(result.Requirements))
	}
}

func TestEASA_IR_NoProfCheck_Expiring(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 15} // IFR hours met
	// No proficiency check

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (IFR hours met but no proficiency check)", result.Status)
	}
}

func TestEASA_IR_ProfCheck_InsufficientIFR_Expiring(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 5} // Below 10h threshold
	profDate := time.Now().AddDate(0, -3, 0)
	dp.lastProficiencyCheck = &profDate

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (prof check done but IFR hours insufficient)", result.Status)
	}
}

// ── Proficiency Check Requirement Visibility ────────────────────────────

func TestEASA_MEP_ProfCheckRequirement_InResponse(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{Flights: 12, InstructorHours: 1.5}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeMEPLand, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "CPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)

	// Find proficiency check requirement
	found := false
	for _, req := range result.Requirements {
		if req.Name == "Proficiency Check" {
			found = true
			if req.Met {
				t.Error("Proficiency check should NOT be met when not set on mock")
			}
		}
	}
	if !found {
		t.Error("Expected 'Proficiency Check' requirement in response")
	}
}

func TestEASA_IR_ProfCheckRequirement_Met(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 15}
	profDate := time.Now().AddDate(0, -1, 0)
	dp.lastProficiencyCheck = &profDate

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)

	for _, req := range result.Requirements {
		if req.Name == "Proficiency Check" {
			if !req.Met {
				t.Error("Proficiency check should be met")
			}
			if req.Current != 1 {
				t.Errorf("Proficiency check current = %.0f, want 1", req.Current)
			}
		}
	}
}
