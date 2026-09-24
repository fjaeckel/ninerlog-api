package currency

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func crossClassSetup(licenseType string, classes ...models.ClassType) (*models.License, []*models.ClassRating) {
	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: licenseType}
	var ratings []*models.ClassRating
	for _, ct := range classes {
		ratings = append(ratings, &models.ClassRating{ID: uuid.New(), LicenseID: license.ID, ClassType: ct, ExpiryDate: futureDate(6)})
	}
	return license, ratings
}

func findReq(reqs []Requirement, key string) *Requirement {
	for i := range reqs {
		if reqs[i].NameKey == key {
			return &reqs[i]
		}
	}
	return nil
}

// ── LAPL(A) FCL.140.A pooling ─────────────────────────────────────────

func TestEASA_LAPL_TMGHoursCountTowardSEPRating(t *testing.T) {
	license, ratings := crossClassSetup("LAPL(A)", models.ClassTypeSEPLand, models.ClassTypeTMG)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{TotalMinutes: 720, Landings: 12, InstructorMinutes: 60}

	for _, r := range ratings {
		res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), r, license, ratings, dp)
		if res.Status != StatusCurrent {
			t.Errorf("%s status = %s, want current from TMG-only hours", r.ClassType, res.Status)
		}
		if !slices.Equal(res.CountedClasses, easaAeroplaneClasses) {
			t.Errorf("%s countedClasses = %v, want %v", r.ClassType, res.CountedClasses, easaAeroplaneClasses)
		}
	}
}

func TestEASA_LAPL_MixedSEPAndTMGHours(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand, models.ClassTypeTMG)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 400, Landings: 6, InstructorMinutes: 60}
	dp.progressByClass[models.ClassTypeTMG] = &Progress{TotalMinutes: 320, Landings: 6}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Status != StatusCurrent {
		t.Errorf("status = %s, want current from 400+320 min and 6+6 landings", res.Status)
	}
	if got := findReq(res.Requirements, ReqKeyTotalTime); got == nil || got.Current != 720 {
		t.Errorf("total time requirement = %+v, want current 720", got)
	}
}

func TestEASA_LAPL_MixedHoursStillShort(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand, models.ClassTypeTMG)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 300, Landings: 6, InstructorMinutes: 60}
	dp.progressByClass[models.ClassTypeTMG] = &Progress{TotalMinutes: 300, Landings: 5}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Status != StatusExpiring {
		t.Errorf("status = %s, want expiring with 600 min and 11 landings", res.Status)
	}
}

func TestEASA_LAPL_CountsMEPHours(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{TotalMinutes: 720, Landings: 12, InstructorMinutes: 60}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Status != StatusCurrent {
		t.Errorf("status = %s, want current from aeroplane (MEP) hours", res.Status)
	}
}

func TestEASA_LAPL_IgnoresGliderAndUltralightHours(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{TotalMinutes: 720, Landings: 12, InstructorMinutes: 60}
	dp.progressByClass[models.ClassTypeUL] = &Progress{TotalMinutes: 720, Landings: 12, InstructorMinutes: 60}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Status != StatusExpiring {
		t.Errorf("status = %s, want expiring — glider/UL hours are not aeroplane hours", res.Status)
	}
	if slices.Contains(dp.lastClasses, models.ClassTypeGlider) || slices.Contains(dp.lastClasses, models.ClassTypeUL) {
		t.Errorf("queried classes %v include glider/UL", dp.lastClasses)
	}
}

func TestEASA_LAPL_ProficiencyCheckAlternative(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	dp.lastProficiencyCheck = futureDate(-2)

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Status != StatusCurrent {
		t.Errorf("status = %s, want current via proficiency check", res.Status)
	}
	req := findReq(res.Requirements, ReqKeyProficiencyCheck)
	if req == nil || !req.Met || req.MessageKey != MsgRequirementProfCheckCompleted {
		t.Errorf("proficiency check requirement = %+v, want met/completed", req)
	}
	if !slices.Equal(dp.lastProfCheckClasses, easaAeroplaneClasses) {
		t.Errorf("proficiency check classes = %v, want aeroplane pool", dp.lastProfCheckClasses)
	}
}

func TestEASA_LAPL_NoProficiencyCheckNoHours(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand)
	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, newMockFlightDataProvider())
	if res.Status != StatusExpiring {
		t.Errorf("status = %s, want expiring", res.Status)
	}
	req := findReq(res.Requirements, ReqKeyProficiencyCheck)
	if req == nil || req.Met || req.MessageKey != MsgRequirementProfCheckMissing {
		t.Errorf("proficiency check requirement = %+v, want missing", req)
	}
}

// ── LAPL(A) FCL.140.A(b) land/sea split ──────────────────────────────

func TestEASA_LAPL_LandSeaSplit_Met(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand, models.ClassTypeSEPSea)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 600, Landings: 6, InstructorMinutes: 60}
	dp.progressByClass[models.ClassTypeSEPSea] = &Progress{TotalMinutes: 120, Landings: 6}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Status != StatusCurrent {
		t.Errorf("status = %s, want current", res.Status)
	}
	for _, key := range []string{ReqKeySEPLandTime, ReqKeySEPLandLandings, ReqKeySEPSeaTime, ReqKeySEPSeaLandings} {
		if r := findReq(res.Requirements, key); r == nil || !r.Met {
			t.Errorf("%s = %+v, want met", key, r)
		}
	}
}

func TestEASA_LAPL_LandSeaSplit_SeaShort(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand, models.ClassTypeSEPSea)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 700, Landings: 10, InstructorMinutes: 60}
	dp.progressByClass[models.ClassTypeSEPSea] = &Progress{TotalMinutes: 30, Landings: 2}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[1], license, ratings, dp)
	if res.Status != StatusExpiring {
		t.Errorf("status = %s, want expiring — pooled totals met but sea minimum not", res.Status)
	}
	if r := findReq(res.Requirements, ReqKeySEPSeaTime); r == nil || r.Met {
		t.Errorf("sea time requirement = %+v, want unmet", r)
	}
	if r := findReq(res.Requirements, ReqKeySEPSeaLandings); r == nil || r.Met {
		t.Errorf("sea landings requirement = %+v, want unmet", r)
	}
}

func TestEASA_LAPL_LandSeaSplit_OnlyWithBothRatings(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand)
	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, newMockFlightDataProvider())
	if r := findReq(res.Requirements, ReqKeySEPSeaTime); r != nil {
		t.Errorf("unexpected land/sea split requirement without a SEP(sea) rating: %+v", r)
	}
}

func TestEASA_LAPL_LandSeaSplit_FetchError(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeSEPLand, models.ClassTypeSEPSea)
	dp := newMockFlightDataProvider()
	dp.progressErr = errors.New("db down")
	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Status != StatusUnknown || res.MessageKey != MsgRatingEvaluationFailed {
		t.Errorf("status = %s / %s, want unknown / evaluation_failed", res.Status, res.MessageKey)
	}
}

// ── PPL FCL.740.A(b)(1) SEP(land) + TMG ──────────────────────────────

func TestEASA_PPL_SEPLandAndTMG_Pooled(t *testing.T) {
	license, ratings := crossClassSetup("PPL", models.ClassTypeSEPLand, models.ClassTypeTMG)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 400, PICMinutes: 200, Landings: 6, InstructorMinutes: 60}
	dp.progressByClass[models.ClassTypeTMG] = &Progress{TotalMinutes: 400, PICMinutes: 200, Landings: 6}

	want := []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG}
	for _, r := range ratings {
		res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), r, license, ratings, dp)
		if res.Status != StatusCurrent {
			t.Errorf("%s status = %s, want current from combined SEP+TMG", r.ClassType, res.Status)
		}
		if !slices.Equal(res.CountedClasses, want) {
			t.Errorf("%s countedClasses = %v, want %v", r.ClassType, res.CountedClasses, want)
		}
	}
}

func TestEASA_PPL_SEPLandOnly_TMGNotCounted(t *testing.T) {
	license, ratings := crossClassSetup("PPL", models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{TotalMinutes: 800, PICMinutes: 400, Landings: 12, InstructorMinutes: 60}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.Status != StatusExpiring {
		t.Errorf("status = %s, want expiring — no TMG rating, TMG hours don't count", res.Status)
	}
	if res.CountedClasses != nil {
		t.Errorf("countedClasses = %v, want nil", res.CountedClasses)
	}
}

func TestEASA_PPL_SEPSea_NotPooledWithTMG(t *testing.T) {
	license, ratings := crossClassSetup("PPL", models.ClassTypeSEPLand, models.ClassTypeSEPSea, models.ClassTypeTMG)
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{TotalMinutes: 800, PICMinutes: 400, Landings: 12, InstructorMinutes: 60}

	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[1], license, ratings, dp)
	if res.Status != StatusExpiring {
		t.Errorf("SEP_SEA status = %s, want expiring", res.Status)
	}
	if !slices.Equal(dp.lastClasses, []models.ClassType{models.ClassTypeSEPSea}) {
		t.Errorf("queried classes = %v, want [SEP_SEA]", dp.lastClasses)
	}
}

func TestEASA_PPL_MEP_NotPooled(t *testing.T) {
	license, ratings := crossClassSetup("PPL", models.ClassTypeMEPLand, models.ClassTypeSEPLand, models.ClassTypeTMG)
	dp := newMockFlightDataProvider()
	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.CountedClasses != nil {
		t.Errorf("MEP countedClasses = %v, want nil", res.CountedClasses)
	}
	if !slices.Equal(dp.lastProfCheckClasses, []models.ClassType{models.ClassTypeMEPLand}) {
		t.Errorf("MEP proficiency check classes = %v, want [MEP_LAND]", dp.lastProfCheckClasses)
	}
}

// ── Unchanged rules ───────────────────────────────────────────────────

func TestEASA_SPL_TMG_NotPooled(t *testing.T) {
	license, ratings := crossClassSetup("SPL", models.ClassTypeTMG, models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.RuleDescriptionKey != "easa_spl_tmg" {
		t.Errorf("rule = %s, want easa_spl_tmg", res.RuleDescriptionKey)
	}
	if res.CountedClasses != nil || !slices.Equal(dp.lastClasses, []models.ClassType{models.ClassTypeTMG}) {
		t.Errorf("countedClasses = %v, queried = %v, want TMG only", res.CountedClasses, dp.lastClasses)
	}
}

func TestEASA_LAPL_GliderRatingUsesSPLRule(t *testing.T) {
	license, ratings := crossClassSetup("LAPL", models.ClassTypeGlider, models.ClassTypeSEPLand)
	dp := newMockFlightDataProvider()
	res := NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[0], license, ratings, dp)
	if res.RuleDescriptionKey != "easa_spl" {
		t.Errorf("rule = %s, want easa_spl", res.RuleDescriptionKey)
	}
	if res.CountedClasses != nil {
		t.Errorf("countedClasses = %v, want nil", res.CountedClasses)
	}
}

// ── Service wiring ────────────────────────────────────────────────────

func TestService_PassesPeerRatings(t *testing.T) {
	userID := uuid.New()
	licRepo := newMockLicenseRepo()
	lic := &models.License{UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	_ = licRepo.Create(context.Background(), lic)
	crRepo := newMockCRRepo()
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(6)},
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeTMG, ExpiryDate: futureDate(6)},
	}
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{TotalMinutes: 800, PICMinutes: 400, Landings: 12, InstructorMinutes: 60}

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	resp, err := NewService(reg, licRepo, crRepo, dp).EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Ratings {
		if r.Status != StatusCurrent {
			t.Errorf("%s status = %s, want current via SEP+TMG pooling", r.ClassType, r.Status)
		}
	}
}

func TestEASA_PooledRules_ExcludeTowedLaunches(t *testing.T) {
	for _, lt := range []string{"LAPL(A)", "PPL"} {
		license, ratings := crossClassSetup(lt, models.ClassTypeSEPLand, models.ClassTypeTMG)
		dp := newMockFlightDataProvider()
		NewEASAEvaluator().EvaluateWithPeers(context.Background(), ratings[1], license, ratings, dp)
		for _, ct := range []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG} {
			towed, queried := dp.includeTowed[ct]
			if !queried || towed {
				t.Errorf("%s: includeTowed[%s] = %v (queried %v), want false", lt, ct, towed, queried)
			}
		}
	}
}
