package currency

import (
	"context"
	"slices"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// ── FCL.035(a)(4) Annex I ultralight credit ───────────────────────────

func TestEASA_SEP_ThreeAxisULTimeAndLandingsCount(t *testing.T) {
	license, ratings := crossClassSetup("PPL", models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 240, PICMinutes: 120, Landings: 4, InstructorMinutes: 60}
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindThreeAxis: {TotalMinutes: 480, PICMinutes: 480, Landings: 8, InstructorMinutes: 120},
	}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Progress.TotalMinutes != 720 || res.Progress.PICMinutes != 600 || res.Progress.Landings != 12 {
		t.Errorf("Progress = %+v, want 720 total, 600 PIC, 12 landings", res.Progress)
	}
	if res.Progress.InstructorMinutes != 60 {
		t.Errorf("InstructorMinutes = %d, want 60 (UL dual is not the refresher)", res.Progress.InstructorMinutes)
	}
	if !slices.Equal(res.CreditedULKinds, []models.ULKind{models.ULKindThreeAxis}) {
		t.Errorf("CreditedULKinds = %v, want [THREE_AXIS]", res.CreditedULKinds)
	}
	if dp.lastULSel == nil || dp.lastULSel.IncludeUnspecified {
		t.Errorf("UL selector = %+v, want unspecified kinds excluded", dp.lastULSel)
	}
}

func TestEASA_SEP_ULRefresherDoesNotCount(t *testing.T) {
	license, ratings := crossClassSetup("PPL", models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindThreeAxis: {TotalMinutes: 900, PICMinutes: 900, Landings: 20, InstructorMinutes: 120},
	}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if got := findReq(res.Requirements, ReqKeyRefresherTraining); got == nil || got.Met {
		t.Errorf("refresher = %+v, want not met", got)
	}
	for _, key := range []string{ReqKeyTotalTime, ReqKeyPICTime, ReqKeyLandings} {
		if got := findReq(res.Requirements, key); got == nil || !got.Met {
			t.Errorf("%s = %+v, want met from UL flights", key, got)
		}
	}
}

func TestEASA_AnnexICredit_ByClass(t *testing.T) {
	tests := []struct {
		name        string
		licenseType string
		classes     []models.ClassType
		want        []models.ULKind
	}{
		{"SEP land", "PPL", []models.ClassType{models.ClassTypeSEPLand}, []models.ULKind{models.ULKindThreeAxis}},
		{"TMG", "PPL", []models.ClassType{models.ClassTypeTMG}, []models.ULKind{models.ULKindThreeAxisMotorglider}},
		{"SEP land + TMG pooled", "PPL", []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG}, []models.ULKind{models.ULKindThreeAxis, models.ULKindThreeAxisMotorglider}},
		{"SEP sea", "PPL", []models.ClassType{models.ClassTypeSEPSea}, nil},
		{"MEP", "PPL", []models.ClassType{models.ClassTypeMEPLand}, nil},
		{"LAPL", "LAPL(A)", []models.ClassType{models.ClassTypeSEPLand}, []models.ULKind{models.ULKindThreeAxis, models.ULKindThreeAxisMotorglider}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			license, ratings := crossClassSetup(tt.licenseType, tt.classes...)
			dp := newMockFlightDataProvider()
			res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
			if !slices.Equal(res.CreditedULKinds, tt.want) {
				t.Errorf("CreditedULKinds = %v, want %v", res.CreditedULKinds, tt.want)
			}
		})
	}
}

func TestEASA_LAPL_ULTimeCountsNotTrainingFlight(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindThreeAxis: {TotalMinutes: 720, Landings: 12, InstructorMinutes: 60},
	}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if got := findReq(res.Requirements, ReqKeyTotalTime); got == nil || !got.Met {
		t.Errorf("total time = %+v, want met from UL flights", got)
	}
	if got := findReq(res.Requirements, ReqKeyTrainingFlight); got == nil || got.Met {
		t.Errorf("training flight = %+v, want not met", got)
	}
	if res.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring", res.Status)
	}
}

func TestEASA_ULTRALIGHTRatingStaysExpiryOnly(t *testing.T) {
	license, ratings := crossClassSetup("PPL", models.ClassTypeUL)
	dp := newMockFlightDataProvider()
	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.CreditedULKinds != nil {
		t.Errorf("CreditedULKinds = %v, want none", res.CreditedULKinds)
	}
	if dp.lastULSel != nil {
		t.Error("expiry-only rating should not query ultralight flights")
	}
}
