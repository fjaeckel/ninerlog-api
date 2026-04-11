package currency

import (
	"context"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── EASA LAPL(A) FCL.140.A Tests ────────────────────────────────────────

func TestEASA_LAPL_Current(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 480, Landings: 20, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("LAPL status = %s, want current", result.Status)
	}
	// LAPL should have 3 requirements (no PIC hour requirement)
	if len(result.Requirements) != 3 {
		t.Fatalf("Expected 3 requirements for LAPL, got %d", len(result.Requirements))
	}
	// Verify PIC Hours is NOT a requirement
	for _, req := range result.Requirements {
		if req.Name == "PIC Time" {
			t.Error("LAPL should NOT have PIC Hours requirement (FCL.140.A has no PIC requirement)")
		}
	}
}

func TestEASA_LAPL_NoPICRequirement(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 0, Landings: 20, InstructorMinutes: 120, // Zero PIC — LAPL should still be current
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("LAPL status = %s, want current (no PIC hours but LAPL doesn't require PIC)", result.Status)
	}
}

func TestEASA_LAPL_InsufficientHours(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 300, Landings: 20, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("LAPL status = %s, want expiring (insufficient hours)", result.Status)
	}
}

func TestEASA_LAPL_RollingFromNow(t *testing.T) {
	// LAPL uses rolling 24 months from now — NOT from expiry date
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 0, Landings: 0, InstructorMinutes: 0,
	}

	// Even with a far-future expiry, LAPL should be "expiring" if no recent activity
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(24), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL(A)"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("LAPL status = %s, want expiring (no activity in 24 months rolling from now)", result.Status)
	}
}

// ── EASA SPL FCL.140.S Tests ────────────────────────────────────────────

func TestEASA_SPL_Current(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		PICMinutes: 480, Landings: 20, InstructorMinutes: 120, // 20 "landings" = 20 launches
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("SPL status = %s, want current", result.Status)
	}
	// SPL should have 3 requirements (PIC hours, launches, training)
	if len(result.Requirements) != 3 {
		t.Fatalf("Expected 3 requirements for SPL, got %d", len(result.Requirements))
	}
	// Verify "Launches" requirement exists (not "Takeoffs & Landings")
	found := false
	for _, req := range result.Requirements {
		if req.Name == "Launches" {
			found = true
		}
	}
	if !found {
		t.Error("SPL should have 'Launches' requirement (not landings)")
	}
}

func TestEASA_SPL_InsufficientLaunches(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		PICMinutes: 480, Landings: 10, InstructorMinutes: 120, // 10 launches < 15
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("SPL status = %s, want expiring (insufficient launches)", result.Status)
	}
}

func TestEASA_SPL_LaunchMethodCurrency(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		PICMinutes: 480, Landings: 20, InstructorMinutes: 120,
	}
	dp.launchCounts = map[string]int{
		"winch":   8,
		"aerotow": 3, // Below required 5
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	// Should have launch method currency data
	if len(result.LaunchMethodCurrency) != 2 {
		t.Fatalf("Expected 2 launch method entries, got %d", len(result.LaunchMethodCurrency))
	}

	for _, lmc := range result.LaunchMethodCurrency {
		if lmc.Method == "winch" && !lmc.Met {
			t.Error("Winch should be met (8 >= 5)")
		}
		if lmc.Method == "aerotow" && lmc.Met {
			t.Error("Aerotow should NOT be met (3 < 5)")
		}
	}
}

// ── EASA SPL TMG Extension Tests ────────────────────────────────────────

func TestEASA_SPL_TMG_Current(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{
		TotalMinutes: 900, Landings: 20,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeTMG, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("SPL TMG status = %s, want current", result.Status)
	}
	// SPL TMG should have 2 requirements (total hours + landings — NO PIC or instructor requirement)
	if len(result.Requirements) != 2 {
		t.Fatalf("Expected 2 requirements for SPL TMG, got %d", len(result.Requirements))
	}
}

func TestEASA_SPL_TMG_VsPPL_TMG(t *testing.T) {
	// SPL TMG and PPL TMG should produce DIFFERENT requirements
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{
		TotalMinutes: 780, PICMinutes: 420, Landings: 15, InstructorMinutes: 90,
	}

	ratingID := uuid.New()
	licenseID := uuid.New()
	userID := uuid.New()

	// PPL TMG — should use FCL.740.A (4 requirements, requires PIC hours and instructor)
	pplRating := &models.ClassRating{ID: ratingID, ClassType: models.ClassTypeTMG, ExpiryDate: futureDate(12), LicenseID: licenseID}
	pplLicense := &models.License{ID: licenseID, UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	pplResult := eval.Evaluate(context.Background(), pplRating, pplLicense, dp)

	// SPL TMG — should use FCL.140.S(b)(2) (2 requirements, no PIC/instructor requirement)
	splRating := &models.ClassRating{ID: ratingID, ClassType: models.ClassTypeTMG, LicenseID: licenseID}
	splLicense := &models.License{ID: licenseID, UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "SPL"}
	splResult := eval.Evaluate(context.Background(), splRating, splLicense, dp)

	if len(pplResult.Requirements) == len(splResult.Requirements) {
		t.Error("PPL TMG and SPL TMG should have DIFFERENT number of requirements")
	}
	if len(pplResult.Requirements) != 4 {
		t.Errorf("PPL TMG should have 4 requirements, got %d", len(pplResult.Requirements))
	}
	if len(splResult.Requirements) != 2 {
		t.Errorf("SPL TMG should have 2 requirements, got %d", len(splResult.Requirements))
	}
}

// ── FAA Instrument Grace Period Tests ────────────────────────────────────

func TestFAA_IR_GracePeriod_Within6Months_Current(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{Approaches: 8, Holds: 2, IFRMinutes: 600}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (6 approaches + holds met within 6 months)", result.Status)
	}
}

func TestFAA_IR_GracePeriod_NotMetAnywhere_Expired(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	// 2 approaches, 0 holds — not met in 6 months OR 12 months (mock returns same data)
	dp.progressAll = &Progress{Approaches: 2, Holds: 0, IFRMinutes: 180}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired (IPC required — not met in 6 or 12 months)", result.Status)
	}
}

// ── EASA License-Type Dispatch Tests ────────────────────────────────────

func TestEASA_Dispatch_PPL_UsesFCL740A(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 480, Landings: 20, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	// PPL should have 4 requirements (total hours, PIC hours, landings, instructor)
	if len(result.Requirements) != 4 {
		t.Errorf("PPL should have 4 requirements (FCL.740.A), got %d", len(result.Requirements))
	}
}

func TestEASA_Dispatch_LAPL_UsesFCL140A(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 0, Landings: 20, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	// LAPL should have 3 requirements (NO PIC hours — FCL.140.A)
	if len(result.Requirements) != 3 {
		t.Errorf("LAPL should have 3 requirements (FCL.140.A), got %d", len(result.Requirements))
	}
}

func TestEASA_Dispatch_SPL_UsesFCL140S(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		PICMinutes: 480, Landings: 20, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	// SPL should have "Launches" requirement (not "Takeoffs & Landings")
	found := false
	for _, req := range result.Requirements {
		if req.Name == "Launches" {
			found = true
		}
	}
	if !found {
		t.Error("SPL should use FCL.140.S (launches), NOT FCL.740.A (landings)")
	}
}

func TestEASA_Dispatch_CPL_UsesFCL740A(t *testing.T) {
	// CPL should use same rules as PPL (FCL.740.A)
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 480, Landings: 20, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "CPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if len(result.Requirements) != 4 {
		t.Errorf("CPL should have 4 requirements (same FCL.740.A as PPL), got %d", len(result.Requirements))
	}
}

func TestEASA_Dispatch_IR_SameForAllLicenseTypes(t *testing.T) {
	// IR should always use FCL.625.A regardless of license type
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRMinutes: 900}
	profDate := futureDate(-3)
	dp.lastProficiencyCheck = profDate

	for _, lt := range []string{"PPL", "LAPL", "SPL", "CPL", "ATPL"} {
		rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
		license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: lt}

		result := eval.Evaluate(context.Background(), rating, license, dp)
		if len(result.Requirements) != 2 {
			t.Errorf("IR for %s should have 2 requirements (FCL.625.A), got %d", lt, len(result.Requirements))
		}
	}
}
