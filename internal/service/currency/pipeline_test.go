package currency

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// TestFullPipeline_EASAMultiRating tests the end-to-end pipeline:
// License → ClassRatings (SEP + IR) → Aircraft (class=SEP_LAND) → Flights → Currency
func TestFullPipeline_EASAMultiRating(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())

	userID := uuid.New()

	// Create EASA PPL with SEP_LAND and IR ratings
	easaLic := &models.License{
		ID: uuid.New(), UserID: userID,
		RegulatoryAuthority: "EASA", LicenseType: "PPL",
		LicenseNumber: "DE.FCL.12345", IssuingAuthority: "LBA",
		IssueDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	licRepo.licenses[easaLic.ID] = easaLic

	sepExpiry := time.Now().AddDate(0, 18, 0) // 18 months out
	irExpiry := time.Now().AddDate(0, 6, 0)   // 6 months out

	crRepo.ratings[easaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: &sepExpiry,
			IssueDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeIR, ExpiryDate: &irExpiry,
			IssueDate: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
	}

	// Simulate flight progress for SEP_LAND (good progress)
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 1200, PICMinutes: 720, Landings: 30, InstructorMinutes: 120,
		Flights: 15, NightMinutes: 180, DayLandings: 25, NightLandings: 5,
	}

	// Simulate flight progress for IR (across all classes)
	dp.progressAll = &Progress{
		IFRMinutes: 720, TotalMinutes: 1800, Flights: 20,
		Approaches: 8, Holds: 3,
	}
	// Add proficiency check for IR
	profDate := time.Now().AddDate(0, -3, 0)
	dp.lastProficiencyCheck = &profDate

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	if len(result.Ratings) != 2 {
		t.Fatalf("Expected 2 ratings, got %d", len(result.Ratings))
	}

	// Find SEP and IR ratings in results
	var sepResult, irResult *ClassRatingCurrency
	for i := range result.Ratings {
		r := &result.Ratings[i]
		if r.ClassType == models.ClassTypeSEPLand {
			sepResult = r
		}
		if r.ClassType == models.ClassTypeIR {
			irResult = r
		}
	}

	// Verify SEP_LAND is current (all requirements met, not expiring soon)
	if sepResult == nil {
		t.Fatal("Expected SEP_LAND rating in results")
	}
	if sepResult.Status != StatusCurrent {
		t.Errorf("SEP status = %s, want current", sepResult.Status)
	}
	if sepResult.RegulatoryAuthority != "EASA" {
		t.Errorf("SEP authority = %s, want EASA", sepResult.RegulatoryAuthority)
	}
	if len(sepResult.Requirements) != 4 {
		t.Errorf("SEP requirements count = %d, want 4", len(sepResult.Requirements))
	}
	for _, req := range sepResult.Requirements {
		if !req.Met {
			t.Errorf("SEP requirement %q not met (current=%.0f, required=%.0f)", req.Name, req.Current, req.Required)
		}
	}
	if sepResult.Progress == nil {
		t.Fatal("SEP progress should not be nil")
	}
	if sepResult.Progress.TotalMinutes != 1200 {
		t.Errorf("SEP progress totalMinutes = %d, want 1200", sepResult.Progress.TotalMinutes)
	}

	// Verify IR is current (10h IFR met)
	if irResult == nil {
		t.Fatal("Expected IR rating in results")
	}
	if irResult.Status != StatusCurrent {
		t.Errorf("IR status = %s, want current", irResult.Status)
	}
	if len(irResult.Requirements) != 2 {
		t.Errorf("IR requirements count = %d, want 2 (IFR hours + proficiency check)", len(irResult.Requirements))
	}
	if irResult.Requirements[0].Name != "IFR Time" {
		t.Errorf("IR requirement name = %s, want IFR Time", irResult.Requirements[0].Name)
	}
	if !irResult.Requirements[0].Met {
		t.Error("IR IFR Time requirement should be met")
	}
}

// TestFullPipeline_FAAPilotMultiClass tests FAA rules with multiple class ratings
func TestFullPipeline_FAAPilotMultiClass(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewFAAEvaluator())

	userID := uuid.New()

	faaLic := &models.License{
		ID: uuid.New(), UserID: userID,
		RegulatoryAuthority: "FAA", LicenseType: "PPL",
		LicenseNumber: "1234567", IssuingAuthority: "FAA",
		IssueDate: time.Date(2019, 6, 15, 0, 0, 0, 0, time.UTC),
	}
	licRepo.licenses[faaLic.ID] = faaLic

	crRepo.ratings[faaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: faaLic.ID, ClassType: models.ClassTypeSEPLand,
			IssueDate: time.Date(2019, 6, 15, 0, 0, 0, 0, time.UTC)},
		{ID: uuid.New(), LicenseID: faaLic.ID, ClassType: models.ClassTypeMEPLand,
			IssueDate: time.Date(2021, 3, 1, 0, 0, 0, 0, time.UTC)},
		{ID: uuid.New(), LicenseID: faaLic.ID, ClassType: models.ClassTypeIR,
			IssueDate: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	// SEP: current (5 landings, 3 night)
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, DayLandings: 2, NightLandings: 3, Flights: 5, TotalMinutes: 480,
	}
	// MEP: day current, night not (2 landings, 0 night)
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{
		Landings: 4, DayLandings: 4, NightLandings: 0, Flights: 4, TotalMinutes: 360,
	}
	// IR: not current (only 3 approaches, 0 holds)
	dp.progressAll = &Progress{
		IFRMinutes: 480, Approaches: 3, Holds: 0, Flights: 10, TotalMinutes: 1200,
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	if len(result.Ratings) != 3 {
		t.Fatalf("Expected 3 ratings, got %d", len(result.Ratings))
	}

	statusMap := map[models.ClassType]Status{}
	for _, r := range result.Ratings {
		statusMap[r.ClassType] = r.Status
	}

	// SEP should be current (5 landings >= 3, 3 night >= 3)
	if statusMap[models.ClassTypeSEPLand] != StatusCurrent {
		t.Errorf("SEP status = %s, want current", statusMap[models.ClassTypeSEPLand])
	}

	// MEP should be expiring (day current but night not: 0 < 3)
	if statusMap[models.ClassTypeMEPLand] != StatusExpiring {
		t.Errorf("MEP status = %s, want expiring (night not current)", statusMap[models.ClassTypeMEPLand])
	}

	// IR should be expired (3 approaches < 6, 0 holds < 1, and not met within 12 months either → IPC required)
	if statusMap[models.ClassTypeIR] != StatusExpired {
		t.Errorf("IR status = %s, want expired (IPC required — not met within 12 months)", statusMap[models.ClassTypeIR])
	}
}

// TestFullPipeline_MixedAuthorities tests a user with licenses from multiple authorities
func TestFullPipeline_MixedAuthorities(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())

	userID := uuid.New()

	// EASA license with SEP
	easaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
		LicenseNumber: "DE.FCL.99999", IssuingAuthority: "LBA", IssueDate: time.Now().AddDate(-3, 0, 0)}
	licRepo.licenses[easaLic.ID] = easaLic

	easaExpiry := time.Now().AddDate(0, 12, 0)
	crRepo.ratings[easaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: &easaExpiry,
			IssueDate: time.Now().AddDate(-2, 0, 0)},
	}

	// FAA license with SEP (no expiry on FAA class ratings)
	faaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "FAA", LicenseType: "PPL",
		LicenseNumber: "9876543", IssuingAuthority: "FAA", IssueDate: time.Now().AddDate(-2, 0, 0)}
	licRepo.licenses[faaLic.ID] = faaLic

	crRepo.ratings[faaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: faaLic.ID, ClassType: models.ClassTypeSEPLand,
			IssueDate: time.Now().AddDate(-2, 0, 0)},
	}

	// Same aircraft class progress applies to both — 5 landings, 3 night
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 600, Landings: 20, InstructorMinutes: 90,
		Flights: 12, NightLandings: 5, DayLandings: 15, NightMinutes: 120,
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	if len(result.Ratings) != 2 {
		t.Fatalf("Expected 2 ratings, got %d", len(result.Ratings))
	}

	// Both should be current
	for _, r := range result.Ratings {
		if r.Status != StatusCurrent {
			t.Errorf("%s %s status = %s, want current", r.RegulatoryAuthority, r.ClassType, r.Status)
		}
		if r.RegulatoryAuthority != "EASA" && r.RegulatoryAuthority != "FAA" {
			t.Errorf("Unexpected authority: %s", r.RegulatoryAuthority)
		}
	}

	// Verify they have different requirement structures
	easaReqs, faaReqs := 0, 0
	for _, r := range result.Ratings {
		if r.RegulatoryAuthority == "EASA" {
			easaReqs = len(r.Requirements)
		}
		if r.RegulatoryAuthority == "FAA" {
			faaReqs = len(r.Requirements)
		}
	}
	// EASA SEP has 4 reqs (total hrs, PIC hrs, landings, instructor)
	if easaReqs != 4 {
		t.Errorf("EASA requirements = %d, want 4", easaReqs)
	}
	// FAA SEP has 2 reqs (day currency, night currency)
	if faaReqs != 2 {
		t.Errorf("FAA requirements = %d, want 2", faaReqs)
	}
}

// TestFullPipeline_UnknownAuthority tests fallback for unsupported authority
func TestFullPipeline_UnknownAuthority(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())

	userID := uuid.New()

	// DGCA India license — no evaluator registered
	dgcaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "DGCA", LicenseType: "PPL",
		LicenseNumber: "IN-PPL-1234", IssuingAuthority: "DGCA India", IssueDate: time.Now().AddDate(-1, 0, 0)}
	licRepo.licenses[dgcaLic.ID] = dgcaLic

	expiry := time.Now().AddDate(0, 6, 0)
	crRepo.ratings[dgcaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: dgcaLic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: &expiry,
			IssueDate: time.Now().AddDate(-1, 0, 0)},
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	if len(result.Ratings) != 1 {
		t.Fatalf("Expected 1 rating, got %d", len(result.Ratings))
	}

	// Should fall back to OtherEvaluator — expiry-only tracking
	r := result.Ratings[0]
	if r.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (expiry is 6 months out)", r.Status)
	}
	if r.RegulatoryAuthority != "DGCA" {
		t.Errorf("Authority = %s, want DGCA", r.RegulatoryAuthority)
	}
	// No requirements (OtherEvaluator doesn't set them)
	if len(r.Requirements) != 0 {
		t.Errorf("Requirements = %d, want 0 for unknown authority", len(r.Requirements))
	}
}

// TestFullPipeline_NoClassRatings tests user with license but no class ratings
func TestFullPipeline_NoClassRatings(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())

	userID := uuid.New()

	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
		LicenseNumber: "DE.FCL.00000", IssuingAuthority: "LBA", IssueDate: time.Now().AddDate(-1, 0, 0)}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{} // empty

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	if len(result.Ratings) != 0 {
		t.Errorf("Expected 0 ratings for empty class ratings, got %d", len(result.Ratings))
	}
}
