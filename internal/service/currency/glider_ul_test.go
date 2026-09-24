package currency

import (
	"context"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── GLIDER / ULTRALIGHT class type dispatch ─────────────────────────────

func TestGliderULClass_RuleSelection(t *testing.T) {
	tests := []struct {
		name      string
		authority string
		licType   string
		class     models.ClassType
		want      *ratingRule
	}{
		{"EASA SPL glider", "EASA", "SPL", models.ClassTypeGlider, &easaSPLRule},
		{"EASA PPL glider", "EASA", "PPL", models.ClassTypeGlider, &easaSPLRule},
		{"EASA LAPL glider", "EASA", "LAPL", models.ClassTypeGlider, &easaSPLRule},
		{"EASA UL ultralight", "EASA", "UL", models.ClassTypeUL, &easaExpiryOnlyRule},
		{"EASA PPL ultralight", "EASA", "PPL", models.ClassTypeUL, &easaExpiryOnlyRule},
		{"EASA SPL TMG unchanged", "EASA", "SPL", models.ClassTypeTMG, &easaSPLTMGRule},
		{"EASA PPL other unchanged", "EASA", "PPL", models.ClassTypeOther, &easaExpiryOnlyRule},
		{"FAA private glider", "FAA", "PRIVATE", models.ClassTypeGlider, &faaGliderRule},
		{"FAA private SEP unchanged", "FAA", "PRIVATE", models.ClassTypeSEPLand, &faaPassengerRatingRule},
		{"FAA ultralight", "FAA", "PRIVATE", models.ClassTypeUL, &otherExpiryRule},
		{"unknown authority glider", "", "SPL", models.ClassTypeGlider, &easaSPLRule},
		{"unknown authority ultralight", "", "UL", models.ClassTypeUL, &otherExpiryRule},
		{"unknown authority other unchanged", "", "PPL", models.ClassTypeOther, &otherExpiryRule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rating := &models.ClassRating{ClassType: tt.class}
			license := &models.License{LicenseType: tt.licType, RegulatoryAuthority: tt.authority}
			var got *ratingRule
			switch tt.authority {
			case "EASA":
				got = easaSelectRule(rating, license)
			case "FAA":
				got = faaSelectRule(rating, license)
			default:
				got = otherSelectRule(rating)
			}
			if got != tt.want {
				t.Errorf("selected rule %q, want %q", got.displayKey, tt.want.displayKey)
			}
		})
	}
}

func TestGermanULEvaluator_GliderUsesSPLRule(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{
		TotalMinutes: 600, PICMinutes: 400, Landings: 20, InstructorMinutes: 60,
	}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeGlider, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "DAeC", LicenseType: "SPL"}

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.RuleDescriptionKey != easaSPLRule.displayKey {
		t.Errorf("RuleDescriptionKey = %q, want %q", result.RuleDescriptionKey, easaSPLRule.displayKey)
	}
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
}

func TestEASA_GliderClass_CountsGliderFlights(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{
		TotalMinutes: 600, PICMinutes: 400, Landings: 20, InstructorMinutes: 60,
	}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeGlider, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
	if result.Progress == nil || result.Progress.Landings != 20 {
		t.Errorf("Progress = %+v, want glider-class progress", result.Progress)
	}
}

func TestEASA_ULClass_ExpiryOnly(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeUL] = &Progress{
		TotalMinutes: 900, Landings: 20, InstructorMinutes: 120,
	}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeUL, LicenseID: uuid.New(), ExpiryDate: futureDate(12)}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "UL"}

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.RuleDescriptionKey != "" {
		t.Errorf("RuleDescriptionKey = %q, want expiry-only", result.RuleDescriptionKey)
	}
	if len(result.Requirements) != 0 {
		t.Errorf("Requirements = %d, want none", len(result.Requirements))
	}
}

func TestService_ULPassengerCurrency_GermanAuthorityOnly(t *testing.T) {
	tests := []struct {
		authority string
		wantPax   bool
	}{
		{"DULV", true},
		{"LBA", true},
		{"EASA", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.authority, func(t *testing.T) {
			licRepo := newMockLicenseRepo()
			crRepo := newMockCRRepo()
			reg := NewRegistry()
			reg.Register(NewEASAEvaluator())
			ulEval := NewGermanULEvaluator()
			reg.RegisterMulti(ulEval, ulEval.Authorities()...)

			userID := uuid.New()
			lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: tt.authority, LicenseType: "UL"}
			licRepo.licenses[lic.ID] = lic
			crRepo.ratings[lic.ID] = []*models.ClassRating{{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeUL}}

			result, err := NewService(reg, licRepo, crRepo, newMockFlightDataProvider()).EvaluateAll(context.Background(), userID)
			if err != nil {
				t.Fatalf("EvaluateAll() error = %v", err)
			}
			if got := len(result.PassengerCurrency) == 1; got != tt.wantPax {
				t.Errorf("passenger currency present = %v, want %v", got, tt.wantPax)
			}
		})
	}
}

func TestTowedFlights_IncludedOnlyForSailplaneRules(t *testing.T) {
	tests := []struct {
		name    string
		eval    Evaluator
		licType string
		class   models.ClassType
		want    bool
	}{
		{"EASA PPL SEP", NewEASAEvaluator(), "PPL", models.ClassTypeSEPLand, false},
		{"EASA LAPL SEP", NewEASAEvaluator(), "LAPL", models.ClassTypeSEPLand, false},
		{"EASA PPL TMG", NewEASAEvaluator(), "PPL", models.ClassTypeTMG, false},
		{"EASA SPL legacy SEP", NewEASAEvaluator(), "SPL", models.ClassTypeSEPLand, true},
		{"EASA PPL glider", NewEASAEvaluator(), "PPL", models.ClassTypeGlider, true},
		{"FAA private SEP", NewFAAEvaluator(), "PRIVATE", models.ClassTypeSEPLand, false},
		{"FAA glider licence", NewFAAEvaluator(), "GLIDER", models.ClassTypeSEPLand, true},
		{"German UL", NewGermanULEvaluator(), "UL", models.ClassTypeUL, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			rating := &models.ClassRating{ID: uuid.New(), ClassType: tt.class, LicenseID: uuid.New(), ExpiryDate: futureDate(6)}
			license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: tt.licType}
			tt.eval.Evaluate(context.Background(), rating, license, dp)
			if got, ok := dp.includeTowed[tt.class]; !ok || got != tt.want {
				t.Errorf("includeTowed = %v (queried %v), want %v", got, ok, tt.want)
			}
		})
	}
}

func TestPassengerCurrency_TowedFlights(t *testing.T) {
	tests := []struct {
		name    string
		eval    PassengerCurrencyEvaluator
		licType string
		class   models.ClassType
		want    bool
	}{
		{"EASA PPL SEP", NewEASAEvaluator(), "PPL", models.ClassTypeSEPLand, false},
		{"EASA SPL legacy SEP", NewEASAEvaluator(), "SPL", models.ClassTypeSEPLand, true},
		{"EASA glider class", NewEASAEvaluator(), "PPL", models.ClassTypeGlider, true},
		{"FAA private SEP", NewFAAEvaluator(), "PRIVATE", models.ClassTypeSEPLand, false},
		{"FAA glider licence", NewFAAEvaluator(), "GLIDER", models.ClassTypeSEPLand, true},
		{"German UL", NewGermanULEvaluator(), "UL", models.ClassTypeUL, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			license := &models.License{UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: tt.licType}
			tt.eval.EvaluatePassengerCurrency(context.Background(), tt.class, license, nil, dp)
			if got := dp.includeTowed[tt.class]; got != tt.want {
				t.Errorf("includeTowed = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEASA_SPL_LaunchCountsUseRatingClass(t *testing.T) {
	dp := newMockFlightDataProvider()
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeGlider, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if dp.launchClass != models.ClassTypeGlider {
		t.Errorf("launch counts queried for %q, want GLIDER", dp.launchClass)
	}
}

func TestEASA_Passenger_GliderClass_NoNightPrivilege(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{Landings: 3}
	license := &models.License{UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	pax := NewEASAEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeGlider, license, nil, dp)
	if pax.NightPrivilege {
		t.Error("NightPrivilege = true, want false for glider")
	}
	if pax.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", pax.DayStatus)
	}
}
