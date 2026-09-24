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
		{"EASA UL ultralight", "EASA", "UL", models.ClassTypeUL, &germanULRule},
		{"EASA PPL ultralight", "EASA", "PPL", models.ClassTypeUL, &germanULRule},
		{"EASA SPL TMG unchanged", "EASA", "SPL", models.ClassTypeTMG, &easaSPLTMGRule},
		{"EASA PPL other unchanged", "EASA", "PPL", models.ClassTypeOther, &easaExpiryOnlyRule},
		{"FAA private glider", "FAA", "PRIVATE", models.ClassTypeGlider, &faaGliderRule},
		{"FAA private SEP unchanged", "FAA", "PRIVATE", models.ClassTypeSEPLand, &faaPassengerRatingRule},
		{"unknown authority glider", "", "SPL", models.ClassTypeGlider, &easaSPLRule},
		{"unknown authority ultralight", "", "UL", models.ClassTypeUL, &germanULRule},
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

func TestEASA_ULClass_UsesLuftPersV(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeUL] = &Progress{
		TotalMinutes: 900, Landings: 20, InstructorMinutes: 120,
	}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeUL, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "UL"}

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.RuleDescriptionKey != germanULRule.displayKey {
		t.Errorf("RuleDescriptionKey = %q, want %q", result.RuleDescriptionKey, germanULRule.displayKey)
	}
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
}

func TestEASA_Passenger_ULClass_UsesULRule(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeUL] = &Progress{Landings: 3}
	license := &models.License{UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "UL"}

	pax := NewEASAEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeUL, license, nil, dp)
	if pax.RuleDescriptionKey != "ul_pax" {
		t.Errorf("RuleDescriptionKey = %q, want ul_pax", pax.RuleDescriptionKey)
	}
	if pax.NightPrivilege {
		t.Error("NightPrivilege = true, want false for ultralight")
	}
	if pax.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", pax.DayStatus)
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
