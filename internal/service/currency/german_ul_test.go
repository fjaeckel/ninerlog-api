package currency

import (
	"context"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── German UL Evaluator Tests (LuftPersV §45) ──────────────────────────

func TestGermanUL_Current(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, Landings: 20, InstructorMinutes: 120, Flights: 12,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "LBA", LicenseType: "UL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
	if len(result.Requirements) != 3 {
		t.Fatalf("Expected 3 requirements, got %d", len(result.Requirements))
	}
	for _, req := range result.Requirements {
		if !req.Met {
			t.Errorf("Requirement %q not met", req.Name)
		}
	}
}

func TestGermanUL_InsufficientHours(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 300, Landings: 20, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "DULV", LicenseType: "UL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (insufficient hours)", result.Status)
	}
}

func TestGermanUL_InsufficientLandings(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, Landings: 5, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "DAeC", LicenseType: "UL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (insufficient landings)", result.Status)
	}
}

func TestGermanUL_NoInstructorMinutes(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, Landings: 20, InstructorMinutes: 0,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "LBA", LicenseType: "UL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (no instructor)", result.Status)
	}
}

func TestGermanUL_RuleDescription(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, Landings: 20, InstructorMinutes: 120,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "LBA", LicenseType: "UL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.RuleDescription == "" {
		t.Error("RuleDescription should not be empty")
	}
}

func TestGermanUL_Authorities(t *testing.T) {
	eval := NewGermanULEvaluator()
	authorities := eval.Authorities()
	if len(authorities) < 3 {
		t.Errorf("Expected at least 3 authorities, got %d", len(authorities))
	}
}

// ── German UL Passenger Currency Tests ──────────────────────────────────

func TestGermanUL_PassengerCurrency_Current(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, NightLandings: 0,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "LBA", LicenseType: "UL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", result.DayStatus)
	}
	if result.NightPrivilege {
		t.Error("NightPrivilege should be false for UL")
	}
	if result.NightRequired != 0 {
		t.Errorf("NightRequired = %d, want 0", result.NightRequired)
	}
}

func TestGermanUL_PassengerCurrency_NotCurrent(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 1,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "DULV", LicenseType: "UL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.DayStatus != StatusExpired {
		t.Errorf("DayStatus = %s, want expired", result.DayStatus)
	}
}

// ── Multi-Authority Registration Tests ──────────────────────────────────

func TestRegistryRegisterMulti(t *testing.T) {
	reg := NewRegistry()
	eval := NewGermanULEvaluator()
	reg.RegisterMulti(eval, eval.Authorities()...)

	for _, auth := range []string{"LBA", "DULV", "DAeC", "DAEC"} {
		if !reg.HasEvaluator(auth) {
			t.Errorf("Expected HasEvaluator(%s) = true", auth)
		}
		if reg.Get(auth) != eval {
			t.Errorf("Expected Get(%s) to return the UL evaluator", auth)
		}
	}
}

func TestRegistryRegisterMulti_DoesNotAffectOthers(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())
	reg.RegisterMulti(NewGermanULEvaluator(), "LBA", "DULV", "DAeC")

	if !reg.HasEvaluator("EASA") {
		t.Error("EASA should still be registered")
	}
	if !reg.HasEvaluator("FAA") {
		t.Error("FAA should still be registered")
	}
	if !reg.HasEvaluator("LBA") {
		t.Error("LBA should be registered")
	}
}

// ── Service Integration with German UL ──────────────────────────────────

func TestService_GermanUL_Integration(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())
	ulEval := NewGermanULEvaluator()
	reg.RegisterMulti(ulEval, ulEval.Authorities()...)

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "DULV", LicenseType: "UL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand},
	}
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, Landings: 20, InstructorMinutes: 120, Flights: 12,
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	// Tier 1: UL rating currency
	if len(result.Ratings) != 1 {
		t.Fatalf("Expected 1 rating, got %d", len(result.Ratings))
	}
	if result.Ratings[0].Status != StatusCurrent {
		t.Errorf("UL rating status = %s, want current", result.Ratings[0].Status)
	}
	if result.Ratings[0].RegulatoryAuthority != "DULV" {
		t.Errorf("Authority = %s, want DULV", result.Ratings[0].RegulatoryAuthority)
	}

	// Tier 2: UL passenger currency
	if len(result.PassengerCurrency) != 1 {
		t.Fatalf("Expected 1 passenger currency, got %d", len(result.PassengerCurrency))
	}
	if result.PassengerCurrency[0].NightPrivilege {
		t.Error("UL passenger currency should have NightPrivilege=false")
	}

	// No flight review for UL
	if result.FlightReview != nil {
		t.Error("UL should not have flight review")
	}
}

// ── Passenger Privilege Tests ───────────────────────────────────────────

func TestPassengerPrivilege_Fields(t *testing.T) {
	priv := &PassengerPrivilege{
		Eligible: true,
		Message:  "Eligible to carry 1 passenger (10h PIC completed — LAPL(A) FCL.140.A(b))",
	}
	if !priv.Eligible {
		t.Error("Eligible should be true")
	}
	if priv.Message == "" {
		t.Error("Message should not be empty")
	}
}

func TestPassengerCurrency_WithPrivilege(t *testing.T) {
	pax := PassengerCurrency{
		ClassType:      models.ClassTypeSEPLand,
		DayStatus:      StatusCurrent,
		NightStatus:    StatusUnknown,
		NightPrivilege: false,
		PassengerPrivilege: &PassengerPrivilege{
			Eligible: false,
			Message:  "Need 10h PIC after license issue — currently 5h (LAPL(A) FCL.140.A(b))",
		},
	}
	if pax.PassengerPrivilege == nil {
		t.Fatal("PassengerPrivilege should not be nil")
	}
	if pax.PassengerPrivilege.Eligible {
		t.Error("Should not be eligible")
	}
}

// ── HasNightPrivilege for UL Authorities ─────────────────────────────────

func TestHasNightPrivilege_ULAuthorities(t *testing.T) {
	for _, auth := range []string{"LBA", "DULV", "DAeC", "DAEC"} {
		if HasNightPrivilege("UL", auth) {
			t.Errorf("HasNightPrivilege(UL, %s) should be false", auth)
		}
	}
}

// ── German UL with zero activity ────────────────────────────────────────

func TestGermanUL_ZeroActivity(t *testing.T) {
	eval := NewGermanULEvaluator()
	dp := newMockFlightDataProvider()

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "LBA", LicenseType: "UL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (zero activity)", result.Status)
	}
	// All 3 requirements should be not met
	for _, req := range result.Requirements {
		if req.Met {
			t.Errorf("Requirement %q should NOT be met with zero activity", req.Name)
		}
	}
}

// ── Mixed Authorities with UL ───────────────────────────────────────────

func TestService_MixedAuthorities_WithUL(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	ulEval := NewGermanULEvaluator()
	reg.RegisterMulti(ulEval, ulEval.Authorities()...)

	userID := uuid.New()

	// EASA PPL
	easaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	licRepo.licenses[easaLic.ID] = easaLic
	crRepo.ratings[easaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12)},
	}

	// German UL
	ulLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "LBA", LicenseType: "UL"}
	licRepo.licenses[ulLic.ID] = ulLic
	crRepo.ratings[ulLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: ulLic.ID, ClassType: models.ClassTypeSEPLand},
	}

	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 480, Landings: 20, InstructorMinutes: 120,
		NightLandings: 5,
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	// Should have 2 ratings (EASA SEP + UL SEP)
	if len(result.Ratings) != 2 {
		t.Fatalf("Expected 2 ratings, got %d", len(result.Ratings))
	}

	// Should have 2 passenger currency entries (EASA:SEP_LAND + LBA:SEP_LAND)
	if len(result.PassengerCurrency) != 2 {
		t.Fatalf("Expected 2 passenger currency entries, got %d", len(result.PassengerCurrency))
	}

	// Verify one has night privilege (EASA PPL) and one doesn't (UL)
	nightCount := 0
	for _, pax := range result.PassengerCurrency {
		if pax.NightPrivilege {
			nightCount++
		}
	}
	if nightCount != 1 {
		t.Errorf("Expected exactly 1 entry with NightPrivilege=true, got %d", nightCount)
	}
}
