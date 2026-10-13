package currency

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── German UL Evaluator Tests (LuftPersV §45) ──────────────────────────

func ulKindPtr(k models.ULKind) *models.ULKind { return &k }

func ulRating(kind *models.ULKind) *models.ClassRating {
	return &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeUL, ULKind: kind, LicenseID: uuid.New()}
}

func ulLicense(rating *models.ClassRating, authority string) *models.License {
	return &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: authority, LicenseType: "UL"}
}

func reqByKey(reqs []Requirement, key string) *Requirement {
	for i := range reqs {
		if reqs[i].NameKey == key {
			return &reqs[i]
		}
	}
	return nil
}

func TestGermanUL_ThreeAxis_Current(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindThreeAxis: {TotalMinutes: 900, PICMinutes: 600, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60, Flights: 12},
	}
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "LBA"), dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
	if result.RuleDescriptionKey != "ul_luftpersv" {
		t.Errorf("RuleDescriptionKey = %q, want ul_luftpersv", result.RuleDescriptionKey)
	}
	for _, key := range []string{ReqKeyTotalTime, ReqKeyPICTime, ReqKeyLandings, ReqKeyTrainingFlight} {
		if r := reqByKey(result.Requirements, key); r == nil || !r.Met {
			t.Errorf("requirement %q = %+v, want met", key, r)
		}
	}
	if r := reqByKey(result.Requirements, ReqKeyProficiencyCheck); r == nil || r.Met {
		t.Errorf("proficiency check = %+v, want listed and not met", r)
	}
	if len(result.CountedClasses) != 2 {
		t.Errorf("CountedClasses = %v, want [SEP_LAND TMG]", result.CountedClasses)
	}
	if len(result.CreditedULKinds) != 2 {
		t.Errorf("CreditedULKinds = %v, want [THREE_AXIS THREE_AXIS_MOTORGLIDER]", result.CreditedULKinds)
	}
}

func TestGermanUL_UnknownWhenRatingHasNoKind(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 900, PICMinutes: 600, Landings: 20}
	dp.progressByClass[models.ClassTypeUL] = &Progress{TotalMinutes: 900, PICMinutes: 600, Landings: 20, InstructorMinutes: 60, LongestTrainingFlightMinutes: 60}
	rating := ulRating(nil)

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "DULV"), dp)
	if result.Status != StatusUnknown || result.MessageKey != MsgRatingULKindRequired {
		t.Errorf("Status = %s, MessageKey = %s, want unknown / %s", result.Status, result.MessageKey, MsgRatingULKindRequired)
	}
	if len(result.Requirements) != 0 || result.Progress != nil {
		t.Errorf("Requirements = %v, Progress = %v, want none", result.Requirements, result.Progress)
	}
	if len(result.CountedClasses) != 0 || len(result.CreditedULKinds) != 0 {
		t.Errorf("CountedClasses = %v, CreditedULKinds = %v, want none", result.CountedClasses, result.CreditedULKinds)
	}
	if dp.lastULSel != nil {
		t.Errorf("UL flights read with %+v, want no read", dp.lastULSel)
	}
}

func TestGermanUL_ThreeAxis_PICRequired(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindThreeAxis: {TotalMinutes: 900, PICMinutes: 300, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60},
	}
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "LBA"), dp)
	if result.Status != StatusLapsed {
		t.Errorf("Status = %s, want lapsed (5h PIC < 6h)", result.Status)
	}
}

func TestGermanUL_ThreeAxis_SEPAndTMGTimeCounts(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 480, PICMinutes: 480, Landings: 8, InstructorMinutes: 300, LongestTrainingFlightMinutes: 60}
	dp.progressByClass[models.ClassTypeTMG] = &Progress{TotalMinutes: 120, PICMinutes: 120, Landings: 2}
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindThreeAxisMotorglider: {TotalMinutes: 120, Landings: 2, InstructorMinutes: 60, LongestTrainingFlightMinutes: 60},
	}
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "LBA"), dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
	if result.Progress.TotalMinutes != 720 || result.Progress.Landings != 12 {
		t.Errorf("Progress = %+v, want 720 min and 12 landings pooled", result.Progress)
	}
	if result.Progress.InstructorMinutes != 60 {
		t.Errorf("InstructorMinutes = %d, want 60 (SEP/TMG dual excluded)", result.Progress.InstructorMinutes)
	}
}

func TestGermanUL_ThreeAxis_DualOnSEPDoesNotCount(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 900, PICMinutes: 600, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60}
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "LBA"), dp)
	if result.Status != StatusLapsed {
		t.Errorf("Status = %s, want lapsed (training flight must be on a UL)", result.Status)
	}
	if r := reqByKey(result.Requirements, ReqKeyTrainingFlight); r == nil || r.Met {
		t.Errorf("training flight = %+v, want not met", r)
	}
}

func TestGermanUL_ProficiencyCheckReplacesExperience(t *testing.T) {
	check := time.Now().AddDate(0, -3, 0)
	dp := newMockFlightDataProvider()
	dp.ulProfCheck = &check
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "LBA"), dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (proficiency check)", result.Status)
	}
	if r := reqByKey(result.Requirements, ReqKeyProficiencyCheck); r == nil || !r.Met {
		t.Errorf("proficiency check = %+v, want met", r)
	}
}

func TestGermanUL_ProficiencyCheckOnSEPCountsForThreeAxis(t *testing.T) {
	check := time.Now().AddDate(0, -1, 0)
	dp := newMockFlightDataProvider()
	dp.lastProficiencyCheck = &check
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "LBA"), dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (check in SEP/TMG, §45(3))", result.Status)
	}
}

func TestGermanUL_Gyroplane_OnlyGyroTimeCounts(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{TotalMinutes: 900, PICMinutes: 900, Landings: 30, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60}
	dp.progressByUL = map[models.ULKind]*Progress{
		models.ULKindThreeAxis: {TotalMinutes: 900, PICMinutes: 900, Landings: 30, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60},
		models.ULKindGyroplane: {TotalMinutes: 240, PICMinutes: 240, Landings: 4},
	}
	rating := ulRating(ulKindPtr(models.ULKindGyroplane))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "DULV"), dp)
	if result.RuleDescriptionKey != "ul_gyroplane" {
		t.Errorf("RuleDescriptionKey = %q, want ul_gyroplane", result.RuleDescriptionKey)
	}
	if result.Progress.TotalMinutes != 240 {
		t.Errorf("TotalMinutes = %d, want 240 (gyroplane only)", result.Progress.TotalMinutes)
	}
	if result.Status != StatusLapsed {
		t.Errorf("Status = %s, want lapsed", result.Status)
	}
	if result.CountedClasses != nil {
		t.Errorf("CountedClasses = %v, want none", result.CountedClasses)
	}
}

func TestGermanUL_KindRules(t *testing.T) {
	tests := []struct {
		name      string
		kind      models.ULKind
		authority string
		progress  Progress
		wantKey   string
		wantReqs  int
		want      Status
	}{
		{"helicopter current", models.ULKindHelicopter, "DULV", Progress{TotalMinutes: 360, Landings: 6, InstructorMinutes: 60, LongestTrainingFlightMinutes: 60}, "ul_luftpersv_helicopter", 4, StatusCurrent},
		{"helicopter short", models.ULKindHelicopter, "DULV", Progress{TotalMinutes: 300, Landings: 6, InstructorMinutes: 60, LongestTrainingFlightMinutes: 60}, "ul_luftpersv_helicopter", 4, StatusLapsed},
		{"trike DULV PIC", models.ULKindWeightShift, "DULV", Progress{TotalMinutes: 900, PICMinutes: 720}, "ul_trike_dulv", 1, StatusCurrent},
		{"trike LBA dual only", models.ULKindWeightShift, "LBA", Progress{TotalMinutes: 900, PICMinutes: 0, Landings: 20}, "ul_trike_dulv", 1, StatusLapsed},
		{"trike DAeC", models.ULKindWeightShift, "DAeC", Progress{TotalMinutes: 720, Landings: 12}, "ul_trike_daec", 2, StatusCurrent},
		{"powered paraglider", models.ULKindPoweredParaglider, "DULV", Progress{Landings: 30}, "ul_powered_paraglider", 1, StatusCurrent},
		{"powered paraglider short", models.ULKindPoweredParaglider, "DULV", Progress{Landings: 29}, "ul_powered_paraglider", 1, StatusLapsed},
		{"UL sailplane", models.ULKindSailplane, "DAeC", Progress{Landings: 5}, "ul_sailplane", 1, StatusCurrent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			p := tt.progress
			dp.progressByUL = map[models.ULKind]*Progress{tt.kind: &p}
			rating := ulRating(ulKindPtr(tt.kind))

			result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, tt.authority), dp)
			if result.RuleDescriptionKey != tt.wantKey {
				t.Errorf("RuleDescriptionKey = %q, want %q", result.RuleDescriptionKey, tt.wantKey)
			}
			if len(result.Requirements) != tt.wantReqs {
				t.Errorf("requirements = %d, want %d", len(result.Requirements), tt.wantReqs)
			}
			if result.Status != tt.want {
				t.Errorf("Status = %s, want %s", result.Status, tt.want)
			}
		})
	}
}

func TestGermanUL_SailplaneCountsTowedLaunches(t *testing.T) {
	dp := newMockFlightDataProvider()
	rating := ulRating(ulKindPtr(models.ULKindSailplane))
	NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "DAeC"), dp)
	if !dp.lastULTowed {
		t.Error("UL sailplane recency should count winch and aerotow launches")
	}
}

func TestGermanUL_NonULRatingDelegatesToEASA(t *testing.T) {
	dp := newMockFlightDataProvider()
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New(), ExpiryDate: futureDate(6)}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "LBA", LicenseType: "PPL"}

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.RuleDescriptionKey != "easa_sep_tmg" {
		t.Errorf("RuleDescriptionKey = %q, want easa_sep_tmg", result.RuleDescriptionKey)
	}
}

func TestGermanUL_ZeroActivity(t *testing.T) {
	dp := newMockFlightDataProvider()
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "LBA"), dp)
	if result.Status != StatusLapsed {
		t.Errorf("Status = %s, want lapsed (zero activity)", result.Status)
	}
	for _, req := range result.Requirements {
		if req.Met {
			t.Errorf("Requirement %q should NOT be met with zero activity", req.NameKey)
		}
	}
}

func TestGermanUL_EvaluationFailed(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressErr = context.DeadlineExceeded
	rating := ulRating(ulKindPtr(models.ULKindGyroplane))

	result := NewGermanULEvaluator().Evaluate(context.Background(), rating, ulLicense(rating, "LBA"), dp)
	if result.Status != StatusUnknown || result.MessageKey != MsgRatingEvaluationFailed {
		t.Errorf("Status = %s, MessageKey = %s, want unknown / evaluation failed", result.Status, result.MessageKey)
	}
}

func TestGermanUL_Authorities(t *testing.T) {
	eval := NewGermanULEvaluator()
	authorities := eval.Authorities()
	if len(authorities) < 3 {
		t.Errorf("Expected at least 3 authorities, got %d", len(authorities))
	}
}

// ── German UL Passenger Currency Tests (§45a) ───────────────────────────

func TestGermanUL_PassengerCurrency_Current(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeUL] = &Progress{Landings: 5}
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().EvaluateRatingPassengerCurrency(context.Background(), rating, ulLicense(rating, "LBA"), nil, dp)
	if result.DayStatus != StatusCurrent {
		t.Errorf("DayStatus = %s, want current", result.DayStatus)
	}
	if result.NightPrivilege {
		t.Error("NightPrivilege should be false for UL")
	}
	if result.NightRequired != 0 {
		t.Errorf("NightRequired = %d, want 0", result.NightRequired)
	}
	if result.ULKind == nil || *result.ULKind != models.ULKindThreeAxis {
		t.Errorf("ULKind = %v, want THREE_AXIS", result.ULKind)
	}
}

func TestGermanUL_PassengerCurrency_SameKindOnly(t *testing.T) {
	yesterday := truncateDay(time.Now().AddDate(0, 0, -1))
	dp := newMockFlightDataProvider()
	dp.landingDaysByUL = map[models.ULKind][]LandingDay{
		models.ULKindThreeAxis: {{Date: yesterday, DayLandings: 5, Takeoffs: 5}},
		models.ULKindGyroplane: {{Date: yesterday, DayLandings: 1, Takeoffs: 1}},
	}
	rating := ulRating(ulKindPtr(models.ULKindGyroplane))

	result := NewGermanULEvaluator().EvaluateRatingPassengerCurrency(context.Background(), rating, ulLicense(rating, "DULV"), nil, dp)
	if result.DayStatus != StatusExpired || result.DayLandings != 1 {
		t.Errorf("DayStatus = %s, landings = %d, want expired with 1 gyroplane landing", result.DayStatus, result.DayLandings)
	}
}

func TestGermanUL_PassengerCurrency_NotCurrent(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeUL] = &Progress{Landings: 1}
	rating := ulRating(ulKindPtr(models.ULKindThreeAxis))

	result := NewGermanULEvaluator().EvaluateRatingPassengerCurrency(context.Background(), rating, ulLicense(rating, "DULV"), nil, dp)
	if result.DayStatus != StatusExpired {
		t.Errorf("DayStatus = %s, want expired", result.DayStatus)
	}
}

func TestGermanUL_PassengerCurrency_UnknownWithoutKind(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeUL] = &Progress{Landings: 5}
	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "DULV", LicenseType: "UL"}

	for name, result := range map[string]PassengerCurrency{
		"class only":        NewGermanULEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeUL, license, nil, dp),
		"rating of no kind": NewGermanULEvaluator().EvaluateRatingPassengerCurrency(context.Background(), ulRating(nil), license, nil, dp),
	} {
		t.Run(name, func(t *testing.T) {
			if result.DayStatus != StatusUnknown || result.MessageKey != MsgRatingULKindRequired || result.ULKind != nil {
				t.Errorf("DayStatus = %s, MessageKey = %s, ULKind = %v, want unknown / %s / nil", result.DayStatus, result.MessageKey, result.ULKind, MsgRatingULKindRequired)
			}
		})
	}
}

func TestGermanUL_PassengerCurrency_NonULDelegatesToEASA(t *testing.T) {
	dp := newMockFlightDataProvider()
	license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "LBA", LicenseType: "PPL"}
	result := NewGermanULEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeSEPLand, license, nil, dp)
	if result.RuleDescriptionKey == "ul_pax" {
		t.Errorf("SEP_LAND passenger currency on an LBA PPL used the UL rule")
	}
	if result.ULKind != nil {
		t.Errorf("ULKind = %v, want nil", result.ULKind)
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

func newULService(licRepo *mockLicenseRepo, crRepo *mockCRRepo, dp *mockFlightDataProvider) *Service {
	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())
	ulEval := NewGermanULEvaluator()
	reg.RegisterMulti(ulEval, ulEval.Authorities()...)
	return NewService(reg, licRepo, crRepo, dp)
}

func TestService_GermanUL_Integration(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "DULV", LicenseType: "UL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeUL, ULKind: ulKindPtr(models.ULKindThreeAxis)},
	}
	dp.progressByClass[models.ClassTypeUL] = &Progress{
		TotalMinutes: 900, PICMinutes: 600, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60, Flights: 12,
	}

	result, err := newULService(licRepo, crRepo, dp).EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	if len(result.Ratings) != 1 {
		t.Fatalf("Expected 1 rating, got %d", len(result.Ratings))
	}
	if result.Ratings[0].Status != StatusCurrent {
		t.Errorf("UL rating status = %s, want current", result.Ratings[0].Status)
	}
	if result.Ratings[0].RegulatoryAuthority != "DULV" {
		t.Errorf("Authority = %s, want DULV", result.Ratings[0].RegulatoryAuthority)
	}
	if len(result.PassengerCurrency) != 1 {
		t.Fatalf("Expected 1 passenger currency, got %d", len(result.PassengerCurrency))
	}
	if result.PassengerCurrency[0].NightPrivilege {
		t.Error("UL passenger currency should have NightPrivilege=false")
	}
	if result.FlightReview != nil {
		t.Error("UL should not have flight review")
	}
}

func TestService_GermanUL_PassengerPerKind(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "DULV", LicenseType: "UL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeUL, ULKind: ulKindPtr(models.ULKindThreeAxis)},
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeUL, ULKind: ulKindPtr(models.ULKindGyroplane)},
	}

	result, err := newULService(licRepo, crRepo, dp).EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	if len(result.PassengerCurrency) != 2 {
		t.Fatalf("Expected one passenger currency per UL kind, got %d", len(result.PassengerCurrency))
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

// ── Mixed Authorities with UL ───────────────────────────────────────────

func TestService_MixedAuthorities_WithUL(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	userID := uuid.New()

	easaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	licRepo.licenses[easaLic.ID] = easaLic
	crRepo.ratings[easaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12)},
	}

	ulLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "LBA", LicenseType: "UL"}
	licRepo.licenses[ulLic.ID] = ulLic
	crRepo.ratings[ulLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: ulLic.ID, ClassType: models.ClassTypeUL, ULKind: ulKindPtr(models.ULKindThreeAxis)},
	}

	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 480, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60,
		NightLandings: 5,
	}
	dp.progressByClass[models.ClassTypeUL] = &Progress{Landings: 4}

	result, err := newULService(licRepo, crRepo, dp).EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	if len(result.Ratings) != 2 {
		t.Fatalf("Expected 2 ratings, got %d", len(result.Ratings))
	}
	if len(result.PassengerCurrency) != 2 {
		t.Fatalf("Expected 2 passenger currency entries, got %d", len(result.PassengerCurrency))
	}
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
