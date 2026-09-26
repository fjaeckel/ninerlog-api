package currency

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ── EASA LAPL(A) FCL.140.A Tests ────────────────────────────────────────

func TestEASA_LAPL_Current(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 480, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("LAPL status = %s, want current", result.Status)
	}
	// LAPL has 3 experience requirements plus the proficiency-check alternative (no PIC hour requirement)
	if len(result.Requirements) != 4 {
		t.Fatalf("Expected 4 requirements for LAPL, got %d", len(result.Requirements))
	}
	if result.Requirements[3].NameKey != ReqKeyProficiencyCheck {
		t.Errorf("last LAPL requirement = %s, want %s", result.Requirements[3].NameKey, ReqKeyProficiencyCheck)
	}
	// Verify PIC Hours is NOT a requirement
	for _, req := range result.Requirements {
		if req.NameKey == ReqKeyPICTime {
			t.Error("LAPL should NOT have PIC Hours requirement (FCL.140.A has no PIC requirement)")
		}
	}
}

func TestEASA_LAPL_NoPICRequirement(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 0, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60, // Zero PIC — LAPL should still be current
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
		TotalMinutes: 300, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusLapsed {
		t.Errorf("LAPL status = %s, want lapsed (insufficient hours)", result.Status)
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
	if result.Status != StatusLapsed {
		t.Errorf("LAPL status = %s, want lapsed (no activity in 24 months rolling from now)", result.Status)
	}
}

// ── EASA SPL SFCL.160(a) Tests ──────────────────────────────────────────

// splGlider returns a GLIDER rating on an EASA SPL.
func splGlider() (*models.ClassRating, *models.License) {
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeGlider, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}
	return rating, license
}

func TestEASA_SPL_Current(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{
		PICMinutes: 240, InstructorMinutes: 60, LongestTrainingFlightMinutes: 60, Launches: 15, TrainingFlights: 2,
	}
	rating, license := splGlider()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("SPL status = %s, want current", result.Status)
	}
	wantKeys := []string{ReqKeyFlightTime, ReqKeyLaunches, ReqKeyTrainingFlights, ReqKeyProficiencyCheck}
	if len(result.Requirements) != len(wantKeys) {
		t.Fatalf("got %d requirements, want %d", len(result.Requirements), len(wantKeys))
	}
	for i, k := range wantKeys {
		if result.Requirements[i].NameKey != k {
			t.Errorf("requirement[%d] = %s, want %s", i, result.Requirements[i].NameKey, k)
		}
	}
	if !dp.includeTowed[models.ClassTypeGlider] {
		t.Error("glider progress must include towed launches")
	}
}

func TestEASA_SPL_DualTimeCountsTowardFlightTime(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{
		InstructorMinutes: 300, LongestTrainingFlightMinutes: 60, Launches: 20, TrainingFlights: 20,
	}
	rating, license := splGlider()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if r := findReq(result.Requirements, ReqKeyFlightTime); r == nil || !r.Met || r.Current != 300 {
		t.Errorf("flight time requirement = %+v, want met with 300 minutes of dual", r)
	}
	if result.Status != StatusCurrent {
		t.Errorf("status = %s, want current", result.Status)
	}
}

func TestEASA_SPL_TMGHoursCountOnlyTowardFlightTime(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{
		PICMinutes: 120, Launches: 10, TrainingFlights: 1,
	}
	dp.progressByClass[models.ClassTypeTMG] = &Progress{
		PICMinutes: 150, InstructorMinutes: 30, Launches: 20, TrainingFlights: 3,
	}
	rating, license := splGlider()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if r := findReq(result.Requirements, ReqKeyFlightTime); r == nil || !r.Met || r.Current != 300 {
		t.Errorf("flight time = %+v, want 300 minutes pooled from GLIDER and TMG", r)
	}
	if r := findReq(result.Requirements, ReqKeyLaunches); r == nil || r.Met || r.Current != 10 {
		t.Errorf("launches = %+v, want 10 glider launches only", r)
	}
	if r := findReq(result.Requirements, ReqKeyTrainingFlights); r == nil || r.Met || r.Current != 1 {
		t.Errorf("training flights = %+v, want 1 glider training flight only", r)
	}
	if !slices.Equal(result.CountedClasses, []models.ClassType{models.ClassTypeGlider, models.ClassTypeTMG}) {
		t.Errorf("countedClasses = %v, want [GLIDER TMG]", result.CountedClasses)
	}
	if result.Status != StatusLapsed {
		t.Errorf("status = %s, want lapsed", result.Status)
	}
}

func TestEASA_SPL_TrainingFlightsCountFlightsNotMinutes(t *testing.T) {
	cases := []struct {
		name    string
		flights int
		minutes int
		met     bool
	}{
		{"two short winch circuits", 2, 16, true},
		{"one long dual flight", 1, 120, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			dp.progressByClass[models.ClassTypeGlider] = &Progress{
				PICMinutes: 400, InstructorMinutes: tc.minutes, Launches: 20, TrainingFlights: tc.flights,
			}
			rating, license := splGlider()
			result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
			r := findReq(result.Requirements, ReqKeyTrainingFlights)
			if r == nil || r.Met != tc.met || r.Unit != "flights" {
				t.Errorf("training flights = %+v, want met=%v in flights", r, tc.met)
			}
		})
	}
}

func TestEASA_SPL_InsufficientLaunches(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{
		PICMinutes: 480, Launches: 14, TrainingFlights: 2,
	}
	rating, license := splGlider()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusLapsed {
		t.Errorf("SPL status = %s, want lapsed (14 < 15 launches)", result.Status)
	}
}

func TestEASA_SPL_ProficiencyCheckAlternative(t *testing.T) {
	dp := newMockFlightDataProvider()
	check := time.Now().AddDate(0, -3, 0)
	dp.lastProficiencyCheck = &check
	rating, license := splGlider()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("status = %s, want current via SFCL.160(a)(2) proficiency check", result.Status)
	}
	if !slices.Equal(dp.lastProfCheckClasses, []models.ClassType{models.ClassTypeGlider}) {
		t.Errorf("proficiency check classes = %v, want [GLIDER]", dp.lastProfCheckClasses)
	}
}

func TestEASA_SPL_LaunchMethodCurrency(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeGlider] = &Progress{PICMinutes: 480, Launches: 20, TrainingFlights: 2}
	dp.progressByClass[models.ClassTypeTMG] = &Progress{Launches: 3}
	dp.launchCountsAllTime = map[string]int{"winch": 40, "aerotow": 12, "self-launch": 4, "bungee": 3}
	dp.launchCounts = map[string]int{"winch": 8, "self-launch": 2, "bungee": 2}
	rating, license := splGlider()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	want := []LaunchMethodCurrency{
		{Method: "winch", Launches: 8, Required: 5, Met: true, MessageKey: MsgLaunchMethodProgress},
		{Method: "aerotow", Launches: 0, Required: 5, Met: false, MessageKey: MsgLaunchMethodProgress},
		{Method: "self-launch", Launches: 5, Required: 5, Met: true, MessageKey: MsgLaunchMethodProgress},
		{Method: "bungee", Launches: 2, Required: 2, Met: true, MessageKey: MsgLaunchMethodProgress},
	}
	if !slices.Equal(result.LaunchMethodCurrency, want) {
		t.Errorf("launch methods =\n%+v\nwant\n%+v", result.LaunchMethodCurrency, want)
	}
	if result.Status != StatusCurrent {
		t.Errorf("status = %s, want current: a lapsed launch method does not affect SFCL.160 recency", result.Status)
	}
}

func TestEASA_SPL_LaunchMethodNeverUsedIsOmitted(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{Launches: 30}
	dp.launchCountsAllTime = map[string]int{"winch": 5}
	dp.launchCounts = map[string]int{"winch": 5}
	rating, license := splGlider()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if len(result.LaunchMethodCurrency) != 1 || result.LaunchMethodCurrency[0].Method != "winch" {
		t.Errorf("launch methods = %+v, want winch only", result.LaunchMethodCurrency)
	}
}

// ── EASA SPL TMG SFCL.160(b) Tests ──────────────────────────────────────

// splTMG returns a TMG rating on an EASA SPL.
func splTMG() (*models.ClassRating, *models.License) {
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeTMG, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}
	return rating, license
}

func TestEASA_SPL_TMG_Current(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{
		PICMinutes: 300, InstructorMinutes: 60, Landings: 12, LongestTrainingFlightMinutes: 60,
	}
	dp.progressByClass[models.ClassTypeGlider] = &Progress{PICMinutes: 360}
	rating, license := splTMG()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("SPL TMG status = %s, want current", result.Status)
	}
	wantKeys := []string{ReqKeyFlightTime, ReqKeyTMGTime, ReqKeyTMGLandings, ReqKeyTMGTrainingFlight, ReqKeyProficiencyCheck}
	if len(result.Requirements) != len(wantKeys) {
		t.Fatalf("got %d requirements, want %d", len(result.Requirements), len(wantKeys))
	}
	for i, k := range wantKeys {
		if result.Requirements[i].NameKey != k {
			t.Errorf("requirement[%d] = %s, want %s", i, result.Requirements[i].NameKey, k)
		}
	}
	if !dp.includeTowed[models.ClassTypeGlider] || dp.includeTowed[models.ClassTypeTMG] {
		t.Errorf("includeTowed = %v, want GLIDER true and TMG false", dp.includeTowed)
	}
}

func TestEASA_SPL_TMG_Shortfalls(t *testing.T) {
	base := func() *Progress {
		return &Progress{PICMinutes: 300, InstructorMinutes: 60, Landings: 12, LongestTrainingFlightMinutes: 60}
	}
	cases := []struct {
		name   string
		tmg    func(p *Progress)
		glider int
		unmet  string
	}{
		{"glider hours do not replace the 6h on TMG", func(p *Progress) { p.PICMinutes = 240 }, 600, ReqKeyTMGTime},
		{"12h total needs sailplane hours", func(p *Progress) {}, 300, ReqKeyFlightTime},
		{"11 take-offs and landings", func(p *Progress) { p.Landings = 11 }, 360, ReqKeyTMGLandings},
		{"training flight shorter than 1h", func(p *Progress) { p.LongestTrainingFlightMinutes = 59 }, 360, ReqKeyTMGTrainingFlight},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			tmg := base()
			tc.tmg(tmg)
			dp.progressByClass[models.ClassTypeTMG] = tmg
			dp.progressByClass[models.ClassTypeGlider] = &Progress{PICMinutes: tc.glider}
			rating, license := splTMG()

			result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
			if result.Status != StatusLapsed {
				t.Errorf("status = %s, want lapsed", result.Status)
			}
			if r := findReq(result.Requirements, tc.unmet); r == nil || r.Met {
				t.Errorf("%s = %+v, want unmet", tc.unmet, r)
			}
		})
	}
}

func TestEASA_SPL_TMG_ProficiencyCheckAlternative(t *testing.T) {
	dp := newMockFlightDataProvider()
	check := time.Now().AddDate(-1, 0, 0)
	dp.lastProficiencyCheck = &check
	rating, license := splTMG()

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("status = %s, want current via SFCL.160(b)(2) proficiency check", result.Status)
	}
	if !slices.Equal(dp.lastProfCheckClasses, []models.ClassType{models.ClassTypeTMG}) {
		t.Errorf("proficiency check classes = %v, want [TMG]", dp.lastProfCheckClasses)
	}
}

func TestEASA_SPL_TMG_VsPPL_TMG(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{
		TotalMinutes: 780, PICMinutes: 420, Landings: 15, InstructorMinutes: 90, LongestTrainingFlightMinutes: 60,
	}
	licenseID := uuid.New()
	userID := uuid.New()

	pplRating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeTMG, ExpiryDate: futureDate(12), LicenseID: licenseID}
	pplLicense := &models.License{ID: licenseID, UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	pplResult := eval.Evaluate(context.Background(), pplRating, pplLicense, dp)

	splRating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeTMG, LicenseID: licenseID}
	splLicense := &models.License{ID: licenseID, UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "SPL"}
	splResult := eval.Evaluate(context.Background(), splRating, splLicense, dp)

	if pplResult.RuleDescriptionKey != "easa_sep_tmg" || len(pplResult.Requirements) != 4 {
		t.Errorf("PPL TMG = %s with %d requirements, want easa_sep_tmg with 4", pplResult.RuleDescriptionKey, len(pplResult.Requirements))
	}
	if splResult.RuleDescriptionKey != "easa_spl_tmg" || len(splResult.Requirements) != 5 {
		t.Errorf("SPL TMG = %s with %d requirements, want easa_spl_tmg with 5", splResult.RuleDescriptionKey, len(splResult.Requirements))
	}
}

// ── EASA SPL passenger recency SFCL.160(e) Tests ────────────────────────

func TestEASA_SPL_PassengerCurrencyCountsPICOnly(t *testing.T) {
	cases := []struct {
		licenseType string
		classType   models.ClassType
		picOnly     bool
		ruleKey     string
	}{
		{"SPL", models.ClassTypeGlider, true, "easa_spl_pax"},
		{"PPL", models.ClassTypeGlider, true, "easa_spl_pax"},
		{"SPL", models.ClassTypeTMG, true, "easa_spl_tmg_pax"},
		{"PPL", models.ClassTypeTMG, false, "easa_pax"},
		{"PPL", models.ClassTypeSEPLand, false, "easa_pax"},
	}
	for _, tc := range cases {
		t.Run(tc.licenseType+"/"+string(tc.classType), func(t *testing.T) {
			dp := newMockFlightDataProvider()
			license := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: tc.licenseType}
			pax := NewEASAEvaluator().EvaluatePassengerCurrency(context.Background(), tc.classType, license, nil, dp)
			if dp.picOnly[tc.classType] != tc.picOnly {
				t.Errorf("picOnly = %v, want %v", dp.picOnly[tc.classType], tc.picOnly)
			}
			if pax.RuleDescriptionKey != tc.ruleKey {
				t.Errorf("rule = %s, want %s", pax.RuleDescriptionKey, tc.ruleKey)
			}
		})
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
		TotalMinutes: 900, PICMinutes: 480, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60,
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
		TotalMinutes: 900, PICMinutes: 0, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "LAPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	// LAPL has 4 requirements: time, landings, training flight, proficiency check (NO PIC hours — FCL.140.A)
	if len(result.Requirements) != 4 {
		t.Errorf("LAPL should have 4 requirements (FCL.140.A), got %d", len(result.Requirements))
	}
}

func TestEASA_Dispatch_SPL_UsesSFCL160(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		PICMinutes: 480, Launches: 20, TrainingFlights: 2,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "SPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.RuleDescriptionKey != "easa_spl" || findReq(result.Requirements, ReqKeyLaunches) == nil {
		t.Errorf("rule = %s, want easa_spl with a launches requirement (SFCL.160, not FCL.740.A)", result.RuleDescriptionKey)
	}
}

func TestEASA_Dispatch_CPL_UsesFCL740A(t *testing.T) {
	// CPL should use same rules as PPL (FCL.740.A)
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 900, PICMinutes: 480, Landings: 20, InstructorMinutes: 120, LongestTrainingFlightMinutes: 60,
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
