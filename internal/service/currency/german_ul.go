package currency

import (
	"context"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// GermanULEvaluator implements German ultralight recency (LuftPersV §45) and
// passenger recency (§45a) per ultralight kind. Every other class rating on a
// licence of these authorities is evaluated by the EASA evaluator.
//
// Rule per kind (rolling window from now):
//   - THREE_AXIS (§45(2)): 12h in 24 months on three-axis UL, TMG or SEP(land),
//     incl. 6h PIC, 12 takeoffs & landings and a 1h training flight with an
//     instructor on a three-axis UL; or a proficiency check (§45(3))
//   - HELICOPTER (§45(2a)): 6h in 12 months incl. 6 takeoffs & landings and a
//     1h flight with an instructor; or a proficiency check
//   - GYROPLANE (DULV, §45(4)): 12h in 24 months on gyroplanes incl. 6h PIC,
//     12 takeoffs & landings and a 1h flight with an instructor; or a check
//   - WEIGHT_SHIFT (§45(4)): DAeC 12h + 12 takeoffs & landings in 24 months;
//     DULV and LBA 12h as PIC in 24 months
//   - POWERED_PARAGLIDER (§45(4)): 30 takeoffs & landings in 24 months
//   - SAILPLANE (DAeC, §45(4)): 5 takeoffs & landings in 12 months
//
// No night privilege for ultralights (§44(2)).
type GermanULEvaluator struct {
	easa *EASAEvaluator
}

func NewGermanULEvaluator() *GermanULEvaluator {
	return &GermanULEvaluator{easa: NewEASAEvaluator()}
}

// Authority returns the primary authority — this evaluator is registered for
// multiple authorities via RegisterMulti / Authorities().
func (e *GermanULEvaluator) Authority() string {
	return "LBA"
}

// Authorities returns all German UL authorities this evaluator handles.
func (e *GermanULEvaluator) Authorities() []string {
	return []string{"LBA", "DULV", "DAeC", "DAEC"}
}

func (e *GermanULEvaluator) Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider) ClassRatingCurrency {
	return e.EvaluateWithPeers(ctx, rating, license, nil, dp)
}

// EvaluateWithPeers evaluates an ULTRALIGHT rating by its kind and delegates
// every other class to the EASA evaluator.
func (e *GermanULEvaluator) EvaluateWithPeers(ctx context.Context, rating *models.ClassRating, license *models.License, peers []*models.ClassRating, dp FlightDataProvider) ClassRatingCurrency {
	if rating.ClassType != models.ClassTypeUL {
		return e.easa.EvaluateWithPeers(ctx, rating, license, peers, dp)
	}
	return evalRatingRuleWithPeers(ctx, germanULSelectRule(ratingULKind(rating), license), rating, license, peers, dp)
}

// ratingULKind returns the rating's ultralight kind, THREE_AXIS when unset.
func ratingULKind(rating *models.ClassRating) models.ULKind {
	if rating.ULKind == nil {
		return models.ULKindThreeAxis
	}
	return *rating.ULKind
}

// germanULSelectRule returns the recency rule for an ultralight kind.
func germanULSelectRule(kind models.ULKind, license *models.License) *ratingRule {
	switch kind {
	case models.ULKindHelicopter:
		return &germanULHelicopterRule
	case models.ULKindGyroplane:
		return &germanULGyroplaneRule
	case models.ULKindWeightShift:
		if strings.EqualFold(strings.TrimSpace(license.RegulatoryAuthority), "DAeC") {
			return &germanULTrikeDAeCRule
		}
		return &germanULTrikeDULVRule
	case models.ULKindPoweredParaglider:
		return &germanULPoweredParagliderRule
	case models.ULKindSailplane:
		return &germanULSailplaneRule
	default:
		return &germanULRule
	}
}

// germanULNativeCredit counts the rating's own ultralights, and ultralights of
// no kind, with their dual time and proficiency checks.
func germanULNativeCredit(rating *models.ClassRating, _ []models.ClassType) *ulCredit {
	kind := ratingULKind(rating)
	return &ulCredit{
		sel:         ULSelector{Kinds: models.AircraftKindsForRating(kind), IncludeUnspecified: true},
		native:      true,
		countsTowed: kind == models.ULKindSailplane,
	}
}

// germanULSEPTMGClasses returns the Part-FCL classes §45(2) counts for a
// three-axis rating.
func germanULSEPTMGClasses(_ *models.ClassRating, _ []*models.ClassRating) []models.ClassType {
	return []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG}
}

// germanULNoClasses returns no Part-FCL classes.
func germanULNoClasses(_ *models.ClassRating, _ []*models.ClassRating) []models.ClassType {
	return nil
}

// germanULRule — three-axis ultralight recency (LuftPersV §45(2), (3)).
var germanULRule = ratingRule{
	displayKey:    "ul_luftpersv",
	description:   "Erfordert 12h Flugzeit auf Dreiachs-UL, Reisemotorsegler oder SEP(Land), davon 6h als verantwortlicher Pilot, 12 Starts & Landungen und 1h Übungsflug mit Fluglehrer auf Dreiachs-UL, in 24 Monaten; oder eine Befähigungsüberprüfung (LuftPersV §45 Abs. 2, 3)",
	window:        windowSpec{kind: windowRollingNow, years: 2},
	scope:         scopeClassGroup,
	classGroup:    germanULSEPTMGClasses,
	classesNoDual: true,
	ulCredit:      germanULNativeCredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyPICTime, metric: mPICMinutes, threshold: 360, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 12, unit: "landings"},
		{nameKey: ReqKeyTrainingFlight, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
	},
	finalize: germanULFinalize(true),
}

// germanULHelicopterRule — ultralight helicopter recency (LuftPersV §45(2a), (3)).
var germanULHelicopterRule = ratingRule{
	displayKey:  "ul_luftpersv_helicopter",
	description: "Erfordert 6h Flugzeit auf UL-Hubschraubern mit 6 Starts & Landungen und 1h Flug mit Fluglehrer in 12 Monaten; oder eine Befähigungsüberprüfung (LuftPersV §45 Abs. 2a, 3)",
	window:      windowSpec{kind: windowRollingNow, years: 1},
	scope:       scopeClassGroup,
	classGroup:  germanULNoClasses,
	ulCredit:    germanULNativeCredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 360, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 6, unit: "landings"},
		{nameKey: ReqKeyTrainingFlight, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
	},
	finalize: germanULFinalize(true),
}

// germanULGyroplaneRule — gyroplane recency (DULV under LuftPersV §45(4)).
var germanULGyroplaneRule = ratingRule{
	displayKey:  "ul_gyroplane",
	description: "Erfordert 12h Flugzeit ausschließlich auf Tragschraubern, davon 6h als verantwortlicher Pilot, 12 Starts & Landungen und 1h Übungsflug mit Fluglehrer, in 24 Monaten; oder eine Befähigungsüberprüfung (DULV, LuftPersV §45 Abs. 4)",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeClassGroup,
	classGroup:  germanULNoClasses,
	ulCredit:    germanULNativeCredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyPICTime, metric: mPICMinutes, threshold: 360, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 12, unit: "landings"},
		{nameKey: ReqKeyTrainingFlight, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
	},
	finalize: germanULFinalize(true),
}

// germanULTrikeDULVRule — weight-shift trike recency (DULV under LuftPersV §45(4)).
var germanULTrikeDULVRule = ratingRule{
	displayKey:  "ul_trike_dulv",
	description: "Erfordert 12h als verantwortlicher Pilot auf Trikes in 24 Monaten (DULV, LuftPersV §45 Abs. 4)",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeClassGroup,
	classGroup:  germanULNoClasses,
	ulCredit:    germanULNativeCredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyPICTime, metric: mPICMinutes, threshold: 720, unit: "minutes"},
	},
	finalize: germanULFinalize(false),
}

// germanULTrikeDAeCRule — weight-shift trike recency (DAeC under LuftPersV §45(4)).
var germanULTrikeDAeCRule = ratingRule{
	displayKey:  "ul_trike_daec",
	description: "Erfordert 12h Flugzeit mit 12 Starts & Landungen auf Trikes in 24 Monaten sowie ein Sicherheits-/Performance-Training, das nicht erfasst wird (DAeC, LuftPersV §45 Abs. 4)",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeClassGroup,
	classGroup:  germanULNoClasses,
	ulCredit:    germanULNativeCredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 12, unit: "landings"},
	},
	finalize: germanULFinalize(false),
}

// germanULPoweredParagliderRule — powered paraglider recency (LuftPersV §45(4)).
var germanULPoweredParagliderRule = ratingRule{
	displayKey:  "ul_powered_paraglider",
	description: "Erfordert 30 Starts & Landungen mit Motorschirm oder Motorschirm-Trike in 24 Monaten (DULV/DAeC, LuftPersV §45 Abs. 4)",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeClassGroup,
	classGroup:  germanULNoClasses,
	ulCredit:    germanULNativeCredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 30, unit: "landings"},
	},
	finalize: germanULFinalize(false),
}

// germanULSailplaneRule — ultralight sailplane recency (DAeC under LuftPersV §45(4)).
var germanULSailplaneRule = ratingRule{
	displayKey:  "ul_sailplane",
	description: "Erfordert 5 Starts & Landungen mit UL-Segelflugzeugen in 12 Monaten (DAeC, LuftPersV §45 Abs. 4)",
	window:      windowSpec{kind: windowRollingNow, years: 1},
	scope:       scopeClassGroup,
	classGroup:  germanULNoClasses,
	ulCredit:    germanULNativeCredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 5, unit: "landings"},
	},
	finalize: germanULFinalize(false),
}

// germanULFinalize returns the finalize strategy for German ultralight
// recency; withCheck adds a proficiency check that replaces the experience.
func germanULFinalize(withCheck bool) func(ctx context.Context, rt *ratingRuntime) {
	return func(ctx context.Context, rt *ratingRuntime) {
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		met := allReqsMet(reqs)

		if withCheck {
			checkDate, err := rt.lastProficiencyCheck(ctx)
			if err != nil {
				rt.result.Status = StatusUnknown
				rt.result.setMsg(MsgRatingEvaluationFailed, nil)
				return
			}
			reqCheck := Requirement{
				NameKey: ReqKeyProficiencyCheck, Current: 0, Required: 1, Unit: "check",
				MessageKey: MsgRequirementProfCheckMissing,
			}
			if checkDate != nil {
				reqCheck.Met = true
				reqCheck.Current = 1
				reqCheck.MessageKey = MsgRequirementProfCheckCompleted
				reqCheck.MessageParams = msgDate(checkDate.Format("2006-01-02"))
				met = true
			}
			reqs = append(reqs, reqCheck)
		}
		rt.result.Requirements = reqs

		if met {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRecencyCurrent, nil)
		} else {
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRecencyNotMet, nil)
		}
	}
}

// EvaluatePassengerCurrency evaluates passenger recency for a class. An
// ULTRALIGHT class is evaluated as THREE_AXIS; other classes go to EASA.
func (e *GermanULEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, peers []*models.ClassRating, dp FlightDataProvider) PassengerCurrency {
	return e.EvaluateRatingPassengerCurrency(ctx, &models.ClassRating{ClassType: classType}, license, peers, dp)
}

// EvaluateRatingPassengerCurrency evaluates German ultralight passenger
// recency (LuftPersV §45a): 3 takeoffs & landings in the preceding 90 days in
// an ultralight of the rating's kind. The passenger rating (§84a) is proved
// separately. Other classes go to EASA.
func (e *GermanULEvaluator) EvaluateRatingPassengerCurrency(ctx context.Context, rating *models.ClassRating, license *models.License, peers []*models.ClassRating, dp FlightDataProvider) PassengerCurrency {
	if rating.ClassType != models.ClassTypeUL {
		return e.easa.EvaluatePassengerCurrency(ctx, rating.ClassType, license, peers, dp)
	}
	kind := ratingULKind(rating)
	since := paxWindowStart(time.Now())

	result := PassengerCurrency{
		ClassType:           models.ClassTypeUL,
		ULKind:              &kind,
		RegulatoryAuthority: license.RegulatoryAuthority,
		DayRequired:         3,
		NightRequired:       0,
		NightPrivilege:      false,
		RuleDescription:     "Passagierberechtigung erforderlich (LuftPersV §84a) — 3 Starts & Landungen in 90 Tagen mit einem Luftsportgerät derselben Art (LuftPersV §45a)",
		RuleDescriptionKey:  "ul_pax",
	}

	sel := ULSelector{Kinds: models.AircraftKindsForRating(kind), IncludeUnspecified: true}
	days, err := dp.GetLandingDaysByULKind(ctx, license.UserID, sel, kind == models.ULKindSailplane, since)
	if err != nil {
		result.DayStatus = StatusUnknown
		result.NightStatus = StatusUnknown
		result.setMsg(MsgPaxEvaluationFailed, nil)
		return result
	}

	landings, _ := paxTotals(days)
	result.DayLandings = landings
	result.NightLandings = 0
	result.NightStatus = StatusUnknown
	result.DayExpiresOn = paxExpiryString(paxExpiryDate(days, result.DayRequired, allLandings))

	if landings >= 3 {
		result.DayStatus = StatusCurrent
		result.setMsg(MsgPaxCurrentPrivilegeSeparat, nil)
	} else {
		result.DayStatus = StatusExpired
		result.setMsg(MsgPaxNotCurrent, msgNeeded(3-landings))
	}

	return result
}
