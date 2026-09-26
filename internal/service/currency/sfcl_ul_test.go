package currency

import (
	"context"
	"slices"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── AMC1 SFCL.160: Annex I sailplane hours ──────────────────────────────

func TestSFCL160a_ULSailplaneAndMotorgliderPICHoursCount(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{PICMinutes: 120, Launches: 15, TrainingFlights: 2}
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindSailplane:            {PICMinutes: 120, InstructorMinutes: 300, Launches: 20, TrainingFlights: 4},
		models.ULKindThreeAxisMotorglider: {PICMinutes: 60},
	}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeGlider, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	res := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if got := findReq(res.Requirements, ReqKeyFlightTime); got == nil || got.Current != 300 || !got.Met {
		t.Errorf("flight time = %+v, want 300 (glider + UL PIC, UL dual excluded)", got)
	}
	if got := findReq(res.Requirements, ReqKeyLaunches); got == nil || got.Current != 15 {
		t.Errorf("launches = %+v, want 15 (UL launches not credited)", got)
	}
	if got := findReq(res.Requirements, ReqKeyTrainingFlights); got == nil || got.Current != 2 {
		t.Errorf("training flights = %+v, want 2 (UL training flights not credited)", got)
	}
	if !slices.Equal(res.CreditedULKinds, []models.ULKind{models.ULKindSailplane, models.ULKindThreeAxisMotorglider}) {
		t.Errorf("CreditedULKinds = %v", res.CreditedULKinds)
	}
	if res.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", res.Status)
	}
}

func TestSFCL160b_ULMotorgliderCountsTowardTMGHours(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{PICMinutes: 300}
	dp.progressByClass[models.ClassTypeTMG] = &Progress{PICMinutes: 180, Landings: 12, LongestTrainingFlightMinutes: 60}
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindSailplane:            {PICMinutes: 60},
		models.ULKindThreeAxisMotorglider: {PICMinutes: 180, Landings: 12, LongestTrainingFlightMinutes: 90},
	}
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeTMG, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	res := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if got := findReq(res.Requirements, ReqKeyFlightTime); got == nil || got.Current != 720 {
		t.Errorf("flight time = %+v, want 720 (TMG + glider + UL sailplane + UL motorglider)", got)
	}
	if got := findReq(res.Requirements, ReqKeyTMGTime); got == nil || got.Current != 360 {
		t.Errorf("TMG time = %+v, want 360 (TMG + UL motorglider)", got)
	}
	if got := findReq(res.Requirements, ReqKeyTMGLandings); got == nil || got.Current != 12 {
		t.Errorf("TMG landings = %+v, want 12 (TMG only)", got)
	}
	if res.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", res.Status)
	}
}

// ── SFCL.160(c) ─────────────────────────────────────────────────────────

func TestSFCL160c_PartFCLTMGExempts(t *testing.T) {
	tests := []struct {
		name       string
		otherType  string
		otherClass models.ClassType
		want       bool
	}{
		{"PPL with TMG", "PPL", models.ClassTypeTMG, true},
		{"LAPL with TMG", "LAPL", models.ClassTypeTMG, true},
		{"PPL without TMG", "PPL", models.ClassTypeSEPLand, false},
		{"second SPL with TMG", "SPL", models.ClassTypeTMG, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			licRepo := newMockLicenseRepo()
			crRepo := newMockCRRepo()
			userID := uuid.New()
			spl := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "SPL"}
			other := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: tt.otherType}
			licRepo.licenses[spl.ID] = spl
			licRepo.licenses[other.ID] = other
			splTMG := &models.ClassRating{ID: uuid.New(), LicenseID: spl.ID, ClassType: models.ClassTypeTMG}
			crRepo.ratings[spl.ID] = []*models.ClassRating{splTMG}
			crRepo.ratings[other.ID] = []*models.ClassRating{{ID: uuid.New(), LicenseID: other.ID, ClassType: tt.otherClass, ExpiryDate: futureDate(6)}}

			reg := NewRegistry()
			reg.Register(NewEASAEvaluator())
			res, err := NewService(reg, licRepo, crRepo, newMockFlightDataProvider()).EvaluateAll(context.Background(), userID)
			if err != nil {
				t.Fatal(err)
			}
			var got *ClassRatingCurrency
			for i := range res.Ratings {
				if res.Ratings[i].ClassRatingID == splTMG.ID {
					got = &res.Ratings[i]
				}
			}
			if got == nil {
				t.Fatal("SPL TMG result missing")
			}
			exempt := got.MessageKey == MsgRatingSFCLTMGExempt
			if exempt != tt.want {
				t.Errorf("exempt = %v (status %s, key %s), want %v", exempt, got.Status, got.MessageKey, tt.want)
			}
			if exempt && (got.Status != StatusCurrent || got.Requirements != nil) {
				t.Errorf("exempt result = %+v, want current with no requirements", got)
			}
		})
	}
}
