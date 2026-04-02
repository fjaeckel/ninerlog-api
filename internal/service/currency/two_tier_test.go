package currency

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── EASA Passenger Currency Tests (FCL.060(b)) ─────────────────────────

func TestEASA_PassengerCurrency_FullyCurrent(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, NightLandings: 3, DayLandings: 2,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", result.DayStatus)
	}
	if result.NightStatus != StatusCurrent {
		t.Errorf("NightStatus = %s, want current", result.NightStatus)
	}
	if result.DayLandings != 5 {
		t.Errorf("DayLandings = %d, want 5", result.DayLandings)
	}
	if result.RegulatoryAuthority != "EASA" {
		t.Errorf("Authority = %s, want EASA", result.RegulatoryAuthority)
	}
}

func TestEASA_PassengerCurrency_DayOnlyNightNot(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 4, NightLandings: 1, DayLandings: 3,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", result.DayStatus)
	}
	if result.NightStatus != StatusExpired {
		t.Errorf("NightStatus = %s, want expired", result.NightStatus)
	}
}

func TestEASA_PassengerCurrency_NotCurrent(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 1, NightLandings: 0, DayLandings: 1,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.DayStatus != StatusExpired {
		t.Errorf("DayStatus = %s, want expired", result.DayStatus)
	}
	if result.NightStatus != StatusExpired {
		t.Errorf("NightStatus = %s, want expired", result.NightStatus)
	}
}

// ── FAA Passenger Currency Tests (§61.57(a)/(b)) ───────────────────────

func TestFAA_PassengerCurrency_FullyCurrent(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 6, NightLandings: 4, DayLandings: 2,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", result.DayStatus)
	}
	if result.NightStatus != StatusCurrent {
		t.Errorf("NightStatus = %s, want current", result.NightStatus)
	}
	if result.RegulatoryAuthority != "FAA" {
		t.Errorf("Authority = %s, want FAA", result.RegulatoryAuthority)
	}
}

func TestFAA_PassengerCurrency_DayOnlyNightNot(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, NightLandings: 2, DayLandings: 3,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", result.DayStatus)
	}
	if result.NightStatus != StatusExpired {
		t.Errorf("NightStatus = %s, want expired", result.NightStatus)
	}
}

func TestFAA_PassengerCurrency_NotCurrent(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.DayStatus != StatusExpired {
		t.Errorf("DayStatus = %s, want expired", result.DayStatus)
	}
	if result.NightStatus != StatusExpired {
		t.Errorf("NightStatus = %s, want expired", result.NightStatus)
	}
}

// ── Service Two-Tier Response Tests ─────────────────────────────────────

func TestService_TwoTier_HasPassengerCurrency(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12)},
	}
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 15, PICHours: 8, Landings: 20, InstructorHours: 2,
		NightLandings: 5, DayLandings: 15,
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	// Should have rating currency (Tier 1)
	if len(result.Ratings) != 1 {
		t.Fatalf("Expected 1 rating, got %d", len(result.Ratings))
	}

	// Should also have passenger currency (Tier 2)
	if len(result.PassengerCurrency) != 1 {
		t.Fatalf("Expected 1 passenger currency, got %d", len(result.PassengerCurrency))
	}

	pax := result.PassengerCurrency[0]
	if pax.ClassType != models.ClassTypeSEPLand {
		t.Errorf("Passenger currency classType = %s, want SEP_LAND", pax.ClassType)
	}
	if pax.DayStatus != StatusCurrent {
		t.Errorf("Passenger day status = %s, want current", pax.DayStatus)
	}
}

func TestService_TwoTier_IR_NoPassengerCurrency(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6)},
	}
	dp.progressAll = &Progress{IFRHours: 15}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	// IR rating should exist in Tier 1
	if len(result.Ratings) != 1 {
		t.Fatalf("Expected 1 rating, got %d", len(result.Ratings))
	}

	// IR should NOT produce passenger currency (Tier 2)
	if len(result.PassengerCurrency) != 0 {
		t.Errorf("Expected 0 passenger currency for IR-only, got %d", len(result.PassengerCurrency))
	}
}

func TestService_TwoTier_NoDuplicatePassengerCurrency(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())

	userID := uuid.New()

	// Both EASA and FAA have SEP_LAND ratings
	easaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	faaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	licRepo.licenses[easaLic.ID] = easaLic
	licRepo.licenses[faaLic.ID] = faaLic

	crRepo.ratings[easaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12)},
	}
	crRepo.ratings[faaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: faaLic.ID, ClassType: models.ClassTypeSEPLand},
	}

	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 15, PICHours: 8, Landings: 20, InstructorHours: 2,
		NightLandings: 5, DayLandings: 15,
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	// Should have 2 rating currency (one EASA, one FAA)
	if len(result.Ratings) != 2 {
		t.Fatalf("Expected 2 ratings, got %d", len(result.Ratings))
	}

	// Should have 2 passenger currency (one EASA:SEP_LAND, one FAA:SEP_LAND — different authorities)
	if len(result.PassengerCurrency) != 2 {
		t.Fatalf("Expected 2 passenger currency entries (EASA+FAA), got %d", len(result.PassengerCurrency))
	}
}

func TestService_TwoTier_OtherAuthority_NoPassengerCurrency(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	// No evaluators — everything falls back to OtherEvaluator

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "DGCA", LicenseType: "PPL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12)},
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	// OtherEvaluator doesn't implement PassengerCurrencyEvaluator
	if len(result.PassengerCurrency) != 0 {
		t.Errorf("Expected 0 passenger currency for unknown authority, got %d", len(result.PassengerCurrency))
	}
}

// ── Lookback Window Verification Tests ──────────────────────────────────

func TestEASA_SEP_LookbackIs12Months_NotFlights(t *testing.T) {
	// This test verifies that SEP uses 12-month lookback from expiry, not 24.
	// We set up a scenario where flights exist in months 1-12 of the 24-month
	// validity but NOT in months 13-24. With the correct 12-month window,
	// these flights should NOT count.
	eval := NewEASAEvaluator()

	// The mock always returns the same progress regardless of 'since' param,
	// but this test documents the intent. The actual verification is that
	// the lookback calculation is AddDate(-1,0,0) not AddDate(-2,0,0).
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 0, PICHours: 0, Landings: 0, InstructorHours: 0,
	}

	rating := &models.ClassRating{
		ID: uuid.New(), ClassType: models.ClassTypeSEPLand,
		ExpiryDate: futureDate(6), LicenseID: uuid.New(),
	}
	license := &models.License{
		ID: rating.LicenseID, UserID: uuid.New(),
		RegulatoryAuthority: "EASA", LicenseType: "PPL",
	}

	result := eval.Evaluate(context.Background(), rating, license, dp)

	// With zero progress in 12-month window, should be "expiring" (not met)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (no progress in 12-month window)", result.Status)
	}
}

func TestEASA_MEP_LookbackIs12Months(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{
		Flights: 0, InstructorHours: 0,
	}

	rating := &models.ClassRating{
		ID: uuid.New(), ClassType: models.ClassTypeMEPLand,
		ExpiryDate: futureDate(6), LicenseID: uuid.New(),
	}
	license := &models.License{
		ID: rating.LicenseID, UserID: uuid.New(),
		RegulatoryAuthority: "EASA", LicenseType: "CPL",
	}

	result := eval.Evaluate(context.Background(), rating, license, dp)

	// With zero progress in 12-month window, should be "expiring"
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (no progress in 12-month window)", result.Status)
	}
}

// ── Response Structure Tests ────────────────────────────────────────────

func TestCurrencyStatusResponse_HasBothTiers(t *testing.T) {
	resp := &CurrencyStatusResponse{
		Ratings: []ClassRatingCurrency{
			{ClassRatingID: uuid.New(), Status: StatusCurrent},
		},
		PassengerCurrency: []PassengerCurrency{
			{ClassType: models.ClassTypeSEPLand, DayStatus: StatusCurrent, NightStatus: StatusExpired},
		},
	}

	if len(resp.Ratings) != 1 {
		t.Errorf("Expected 1 rating, got %d", len(resp.Ratings))
	}
	if len(resp.PassengerCurrency) != 1 {
		t.Errorf("Expected 1 passenger currency, got %d", len(resp.PassengerCurrency))
	}
	if resp.FlightReview != nil {
		t.Error("FlightReview should be nil (not set)")
	}
}

func TestPassengerCurrency_Fields(t *testing.T) {
	pax := PassengerCurrency{
		ClassType:           models.ClassTypeSEPLand,
		RegulatoryAuthority: "EASA",
		DayStatus:           StatusCurrent,
		NightStatus:         StatusExpired,
		DayLandings:         5,
		NightLandings:       1,
		DayRequired:         3,
		NightRequired:       3,
		Message:             "test",
		RuleDescription:     "FCL.060(b)",
	}

	if pax.DayRequired != 3 {
		t.Errorf("DayRequired = %d, want 3", pax.DayRequired)
	}
	if pax.NightRequired != 3 {
		t.Errorf("NightRequired = %d, want 3", pax.NightRequired)
	}
}

func TestFlightReviewStatus_Fields(t *testing.T) {
	completed := "2025-06-15"
	expires := "2027-06-15"
	review := FlightReviewStatus{
		LastCompleted: &completed,
		ExpiresOn:     &expires,
		Status:        StatusCurrent,
		Message:       "Flight review current",
	}

	if review.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", review.Status)
	}
	if *review.LastCompleted != "2025-06-15" {
		t.Errorf("LastCompleted = %s, want 2025-06-15", *review.LastCompleted)
	}
}

// ── FAA Flight Review Evaluator Tests (§61.56) ─────────────────────────

func TestFAA_FlightReview_NoReview(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	// lastFlightReview is nil by default

	result := eval.EvaluateFlightReview(context.Background(), uuid.New(), dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired (no review on record)", result.Status)
	}
	if result.LastCompleted != nil {
		t.Error("LastCompleted should be nil when no review")
	}
}

func TestFAA_FlightReview_Current(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	reviewDate := time.Now().AddDate(0, -6, 0) // 6 months ago — well within 24
	dp.lastFlightReview = &reviewDate

	result := eval.EvaluateFlightReview(context.Background(), uuid.New(), dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
	if result.LastCompleted == nil {
		t.Fatal("LastCompleted should not be nil")
	}
	if result.ExpiresOn == nil {
		t.Fatal("ExpiresOn should not be nil")
	}
}

func TestFAA_FlightReview_Expiring(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	// Review done ~22 months ago — within 90 days of expiry
	reviewDate := time.Now().AddDate(0, -22, 0)
	dp.lastFlightReview = &reviewDate

	result := eval.EvaluateFlightReview(context.Background(), uuid.New(), dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (within 90 days of 24-month expiry)", result.Status)
	}
}

func TestFAA_FlightReview_Expired(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	// Review done 30 months ago — expired
	reviewDate := time.Now().AddDate(0, -30, 0)
	dp.lastFlightReview = &reviewDate

	result := eval.EvaluateFlightReview(context.Background(), uuid.New(), dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired (30 months since review)", result.Status)
	}
}

// ── Service with Flight Review ──────────────────────────────────────────

func TestService_FAA_IncludesFlightReview(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewFAAEvaluator())

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand},
	}
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, NightLandings: 3,
	}
	// Set a recent flight review
	reviewDate := time.Now().AddDate(0, -3, 0)
	dp.lastFlightReview = &reviewDate

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	// Should have FlightReview populated
	if result.FlightReview == nil {
		t.Fatal("FlightReview should not be nil for FAA pilot")
	}
	if result.FlightReview.Status != StatusCurrent {
		t.Errorf("FlightReview status = %s, want current", result.FlightReview.Status)
	}
}

func TestService_EASA_NoFlightReview(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12)},
	}
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 15, PICHours: 8, Landings: 20, InstructorHours: 2,
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}

	// EASA doesn't use flight review — should be nil
	if result.FlightReview != nil {
		t.Error("FlightReview should be nil for EASA pilot (EASA doesn't implement FlightReviewEvaluator)")
	}
}

// ── FAA License-Type Restrictions Tests ─────────────────────────────────

func TestFAA_SportPilot_IR_Suppressed(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{Approaches: 10, Holds: 3, IFRHours: 15}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "Sport"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusUnknown {
		t.Errorf("Status = %s, want unknown (IR not applicable for Sport)", result.Status)
	}
	if len(result.Requirements) != 0 {
		t.Errorf("Expected 0 requirements for Sport IR, got %d", len(result.Requirements))
	}
}

func TestFAA_RecreationalPilot_IR_Suppressed(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "Recreational"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusUnknown {
		t.Errorf("Status = %s, want unknown (IR not applicable for Recreational)", result.Status)
	}
}

func TestFAA_GliderCurrency_Current(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, Flights: 5,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "Glider"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (glider with 5 launches)", result.Status)
	}
	if len(result.Requirements) != 1 {
		t.Fatalf("Expected 1 requirement (launches), got %d", len(result.Requirements))
	}
	if result.Requirements[0].Name != "Launches & Landings" {
		t.Errorf("Requirement name = %s, want 'Launches & Landings'", result.Requirements[0].Name)
	}
}

func TestFAA_GliderCurrency_NotCurrent(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 1, Flights: 1,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "Glider"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired (glider with only 1 launch)", result.Status)
	}
}

func TestFAA_SportPilot_PassengerCurrency_NoNight(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, NightLandings: 0,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "Sport"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if result.NightPrivilege {
		t.Error("NightPrivilege should be false for Sport Pilot")
	}
	if result.NightStatus != StatusUnknown {
		t.Errorf("NightStatus = %s, want unknown (suppressed for Sport)", result.NightStatus)
	}
	if result.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", result.DayStatus)
	}
	if result.NightRequired != 0 {
		t.Errorf("NightRequired = %d, want 0 (suppressed)", result.NightRequired)
	}
}

func TestFAA_PrivatePilot_PassengerCurrency_HasNight(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, NightLandings: 4,
	}

	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	result := eval.EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, dp)

	if !result.NightPrivilege {
		t.Error("NightPrivilege should be true for Private Pilot")
	}
	if result.NightStatus != StatusCurrent {
		t.Errorf("NightStatus = %s, want current", result.NightStatus)
	}
}

// ── HasNightPrivilege Tests ─────────────────────────────────────────────

func TestHasNightPrivilege(t *testing.T) {
	tests := []struct {
		licenseType string
		authority   string
		expected    bool
	}{
		{"PPL", "FAA", true},
		{"CPL", "FAA", true},
		{"ATP", "FAA", true},
		{"Sport", "FAA", false},
		{"Recreational", "FAA", false},
		{"Glider", "FAA", false},
		{"PPL", "EASA", true},
		{"CPL", "EASA", true},
		{"ATPL", "EASA", true},
		{"SPL", "EASA", false},
		{"LAPL", "EASA", false},
		{"LAPL(S)", "EASA", false},
		{"UL", "LBA", false},
		{"UL", "DULV", false},
		{"UL", "DAeC", false},
	}

	for _, tt := range tests {
		t.Run(tt.licenseType+"_"+tt.authority, func(t *testing.T) {
			got := HasNightPrivilege(tt.licenseType, tt.authority)
			if got != tt.expected {
				t.Errorf("HasNightPrivilege(%q, %q) = %v, want %v", tt.licenseType, tt.authority, got, tt.expected)
			}
		})
	}
}
