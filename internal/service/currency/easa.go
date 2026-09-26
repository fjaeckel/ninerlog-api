package currency

import (
	"context"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// EASAEvaluator implements EASA FCL currency rules per class type. It is a thin
// adapter: Evaluate() selects the applicable rule from the EASA rule set and
// runs it through the engine. The regulatory data lives in the ratingRule
// definitions below.
type EASAEvaluator struct{}

// NewEASAEvaluator creates a new EASA currency evaluator
func NewEASAEvaluator() *EASAEvaluator {
	return &EASAEvaluator{}
}

func (e *EASAEvaluator) Authority() string {
	return "EASA"
}

func (e *EASAEvaluator) Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dataProvider FlightDataProvider) ClassRatingCurrency {
	return e.EvaluateWithPeers(ctx, rating, license, nil, dataProvider)
}

// EvaluateWithPeers evaluates a rating given the other class ratings on its license.
func (e *EASAEvaluator) EvaluateWithPeers(ctx context.Context, rating *models.ClassRating, license *models.License, peerRatings []*models.ClassRating, dataProvider FlightDataProvider) ClassRatingCurrency {
	return evalRatingRuleWithPeers(ctx, easaSelectRule(rating, license), rating, license, peerRatings, dataProvider)
}

// isEASALAPLA reports whether licenseType is an EASA LAPL for aeroplanes
// ("LAPL" or "LAPL(A)", case-insensitive).
func isEASALAPLA(licenseType string) bool {
	lt := strings.ToUpper(strings.TrimSpace(licenseType))
	return lt == "LAPL" || lt == "LAPL(A)"
}

// isEASASailplane reports whether licenseType is an EASA sailplane licence
// ("SPL" or "LAPL(S)", case-insensitive).
func isEASASailplane(licenseType string) bool {
	lt := strings.ToUpper(strings.TrimSpace(licenseType))
	return lt == "SPL" || lt == "LAPL(S)"
}

// easaSelectRule dispatches a (license type, class type) pair to its rule.
// License-type-aware: LAPL/SPL use recency regulations (FCL.140.A, SFCL.160) while
// PPL/CPL/ATPL use revalidation regulations (FCL.740.A). IR is always
// FCL.625.A regardless of license type.
func easaSelectRule(rating *models.ClassRating, license *models.License) *ratingRule {
	lt := strings.ToUpper(license.LicenseType)

	// IR is always FCL.625.A regardless of license type
	if rating.ClassType == models.ClassTypeIR {
		return &easaIRRule
	}

	// Glider uses SFCL.160(a) and gyroplane FCL.240.G regardless of license type;
	// ultralight is national law, expiry only
	switch rating.ClassType {
	case models.ClassTypeGlider:
		return &easaSPLRule
	case models.ClassTypeGyro:
		return &easaGPLRule
	case models.ClassTypeUL:
		return &easaExpiryOnlyRule
	}

	// LAPL uses FCL.140.A (rolling 24 months from now, aeroplanes and TMG pooled)
	if isEASALAPLA(lt) {
		return &easaLAPLRule
	}

	// SPL/LAPL(S) uses SFCL.160 (rolling 24 months; TMG under SFCL.160(b))
	if isEASASailplane(lt) {
		if rating.ClassType == models.ClassTypeTMG {
			return &easaSPLTMGRule
		}
		return &easaSPLRule
	}

	// PPL/CPL/ATPL use FCL.740.A (from expiry date)
	switch rating.ClassType {
	case models.ClassTypeSEPLand, models.ClassTypeSEPSea, models.ClassTypeTMG:
		return &easaSEPTMGRule
	case models.ClassTypeMEPLand, models.ClassTypeMEPSea, models.ClassTypeSETLand, models.ClassTypeSETSea:
		return &easaMEPSETRule
	default:
		return &easaExpiryOnlyRule
	}
}

// ── EASA rule definitions ───────────────────────────────────────────────────

// easaSEPTMGRule — EASA FCL.740.A(b)(1) revalidation for SEP/TMG:
//   - 12 hours of flight time in class
//   - 6 hours as PIC in class
//   - 12 takeoffs and 12 landings
//   - 1 hour refresher training with instructor (dual received)
//
// All within the 12 months preceding the expiry date of the rating. With both
// SEP(land) and TMG ratings on the license, flights in either class count.
// THREE_AXIS ultralights count as SEP(land) and THREE_AXIS_MOTORGLIDER as TMG
// toward time and landings, not the refresher (FCL.035(a)(4)).
var easaSEPTMGRule = ratingRule{
	displayKey:  "easa_sep_tmg",
	description: "Requires 12h total flight time + 6h as PIC + 12 takeoffs & landings + 1h refresher training with instructor, all within the 12 months preceding the expiry date; holders of both SEP(land) and TMG ratings may combine flights in either class (EASA FCL.740.A(b)(1)); three-axis ultralight time and landings count, the refresher does not (FCL.035(a)(4))",
	window:      windowSpec{kind: windowPrecedingExpiry, years: 1},
	scope:       scopeClassGroup,
	classGroup:  easaSEPTMGClasses,
	ulCredit:    easaAnnexICredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyPICTime, metric: mPICMinutes, threshold: 360, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 12, unit: "landings"},
		{nameKey: ReqKeyRefresherTraining, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		if rating.ExpiryDate == nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingNoExpiryDate, nil)
			return
		}
		since := rating.ExpiryDate.AddDate(-1, 0, 0)
		if r, closed := applyClosedWindow(rating, &since, *rt.result); closed {
			*rt.result = r
			return
		}
		rt.since = since
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		rt.result.Requirements = reqs
		allMet := allReqsMet(reqs)

		if rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.setMsg(MsgRatingExpired, nil)
		} else if !allMet {
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRevalidationNotMet, nil)
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRevalidationExpiringMet, msgDays(daysLeft))
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRevalidationCurrent, nil)
		}
	},
}

// easaMEPSETRule — EASA FCL.740.A(b)(2) revalidation for MEP/SET:
//   - Proficiency check (manual tracking), OR:
//   - 10 route sectors (flights) in class
//   - 1 hour refresher training with instructor
//
// Experience must be within the 12 months preceding the expiry date.
var easaMEPSETRule = ratingRule{
	displayKey:  "easa_mep_set",
	description: "Requires proficiency check, or 10 route sectors + 1h refresher training with instructor within the 12 months preceding the expiry date (EASA FCL.740.A(b)(2))",
	window:      windowSpec{kind: windowPrecedingExpiry, years: 1},
	scope:       scopeByClass,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyRouteSectors, metric: mFlights, threshold: 10, unit: "flights"},
		{nameKey: ReqKeyRefresherTraining, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		if rating.ExpiryDate == nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingNoExpiryDate, nil)
			return
		}
		since := rating.ExpiryDate.AddDate(-1, 0, 0)
		if r, closed := applyClosedWindow(rating, &since, *rt.result); closed {
			*rt.result = r
			return
		}
		rt.since = since
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		reqSectors := reqs[0]
		reqInstructor := reqs[1]

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, rt.classes, since)
		hasProfCheck := profCheckDate != nil
		reqProfCheck := Requirement{
			NameKey: ReqKeyProficiencyCheck, Met: hasProfCheck,
			Current: 0, Required: 1, Unit: "check",
			MessageKey: MsgRequirementProfCheckMissing,
		}
		if hasProfCheck {
			reqProfCheck.Current = 1
			reqProfCheck.MessageKey = MsgRequirementProfCheckCompleted
			reqProfCheck.MessageParams = msgDate(profCheckDate.Format("2006-01-02"))
		}

		rt.result.Requirements = []Requirement{reqSectors, reqInstructor, reqProfCheck}

		allMetByExperience := reqSectors.Met && reqInstructor.Met
		allMet := allMetByExperience || hasProfCheck

		if rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.setMsg(MsgRatingExpired, nil)
		} else if !allMet {
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRevalidationNotMetProfCheck, nil)
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRevalidationExpiringMet, msgDays(daysLeft))
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRevalidationCurrent, nil)
		}
	},
}

// easaIRRule — EASA FCL.625.A instrument rating currency:
//   - 10 hours IFR flight time in 12 months preceding expiry
//   - Proficiency check (manual tracking)
var easaIRRule = ratingRule{
	displayKey:  "easa_ir",
	description: "Requires 10h IFR flight time within 12 months before expiry, plus annual proficiency check (EASA FCL.625.A)",
	window:      windowSpec{kind: windowPrecedingExpiry, years: 1},
	scope:       scopeAll,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyIFRTime, metric: mIFRMinutes, threshold: 600, unit: "minutes"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		if rating.ExpiryDate == nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingNoExpiryDate, nil)
			return
		}
		since := rating.ExpiryDate.AddDate(-1, 0, 0)
		if r, closed := applyClosedWindow(rating, &since, *rt.result); closed {
			*rt.result = r
			return
		}
		rt.since = since
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		reqIFRHours := reqs[0]

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, []models.ClassType{models.ClassTypeIR}, since)
		hasProfCheck := profCheckDate != nil
		reqProfCheck := Requirement{
			NameKey: ReqKeyProficiencyCheck, Met: hasProfCheck,
			Current: 0, Required: 1, Unit: "check",
			MessageKey: MsgRequirementProfCheckMissing,
		}
		if hasProfCheck {
			reqProfCheck.Current = 1
			reqProfCheck.MessageKey = MsgRequirementProfCheckCompleted
			reqProfCheck.MessageParams = msgDate(profCheckDate.Format("2006-01-02"))
		}

		rt.result.Requirements = []Requirement{reqIFRHours, reqProfCheck}

		allMet := reqIFRHours.Met && hasProfCheck

		if rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.setMsg(MsgRatingExpired, nil)
		} else if !allMet {
			rt.result.Status = StatusExpiring
			if !reqIFRHours.Met && !hasProfCheck {
				rt.result.setMsg(MsgRatingIRHoursAndCheckNotMet, nil)
			} else if !reqIFRHours.Met {
				rt.result.setMsg(MsgRatingIRHoursNotMet, nil)
			} else {
				rt.result.setMsg(MsgRatingIRCheckNotMet, nil)
			}
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRevalidationExpiringMet, msgDays(daysLeft))
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRevalidationCurrent, nil)
		}
	},
}

// easaExpiryOnlyRule — fallback expiry-only tracking for unknown class types.
var easaExpiryOnlyRule = ratingRule{
	displayKey:  "",
	description: "EASA class rating — currency tracked by expiry date",
	scope:       scopeByClass,
	finalize: func(_ context.Context, rt *ratingRuntime) {
		rating := rt.rating
		if rating.ExpiryDate == nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingNoExpiryDate, nil)
			return
		}
		if rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.setMsg(MsgRatingExpired, nil)
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingExpiring, msgDays(daysLeft))
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingValidUntil, nil)
		}
	},
}

// easaLAPLRule — EASA FCL.140.A recency for LAPL(A), on aeroplanes and TMG pooled:
//   - 12 hours flight time (as PIC, dual, or solo under supervision)
//   - 12 takeoffs & landings
//   - 1 hour dual instruction
//   - NO PIC hour requirement (key difference from FCL.740.A)
//   - OR a LAPL(A) proficiency check (FCL.140.A(a)(2))
//   - with both SEP(land) and SEP(sea) ratings: 1 hour and 6 landings in each (FCL.140.A(b))
//   - THREE_AXIS and THREE_AXIS_MOTORGLIDER ultralights count toward time and
//     landings, not the training flight (FCL.035(a)(4))
//
// Lookback: rolling 24 months from NOW.
var easaLAPLRule = ratingRule{
	displayKey:  "easa_lapl",
	description: "Requires 12h flight time + 12 takeoffs & landings + 1h training flight with instructor on aeroplanes or TMG within the last 24 months, or a LAPL(A) proficiency check; holders of SEP(land) and SEP(sea) need 1h and 6 takeoffs & landings in each (EASA FCL.140.A); three-axis ultralight time and landings count, the training flight does not (FCL.035(a)(4))",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeClassGroup,
	classGroup:  easaLAPLClasses,
	ulCredit:    easaAnnexICredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 12, unit: "landings"},
		{nameKey: ReqKeyTrainingFlight, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)

		if hasClass(rt.peers, models.ClassTypeSEPLand) && hasClass(rt.peers, models.ClassTypeSEPSea) {
			split, err := easaLAPLLandSeaReqs(ctx, rt)
			if err != nil {
				rt.result.Status = StatusUnknown
				rt.result.setMsg(MsgRatingEvaluationFailed, nil)
				return
			}
			reqs = append(reqs, split...)
		}
		allMetByExperience := allReqsMet(reqs)

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, rt.classes, rt.since)
		hasProfCheck := profCheckDate != nil
		reqProfCheck := Requirement{
			NameKey: ReqKeyProficiencyCheck, Met: hasProfCheck,
			Current: 0, Required: 1, Unit: "check",
			MessageKey: MsgRequirementProfCheckMissing,
		}
		if hasProfCheck {
			reqProfCheck.Current = 1
			reqProfCheck.MessageKey = MsgRequirementProfCheckCompleted
			reqProfCheck.MessageParams = msgDate(profCheckDate.Format("2006-01-02"))
		}
		rt.result.Requirements = append(reqs, reqProfCheck)

		if !allMetByExperience && !hasProfCheck {
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRecencyNotMet, nil)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRecencyCurrent, nil)
		}
	},
}

// easaLAPLLandSeaReqs returns the per-class FCL.140.A(b) minimums for SEP(land) and SEP(sea).
func easaLAPLLandSeaReqs(ctx context.Context, rt *ratingRuntime) ([]Requirement, error) {
	classes := []struct {
		ct       models.ClassType
		timeKey  string
		landsKey string
	}{
		{models.ClassTypeSEPLand, ReqKeySEPLandTime, ReqKeySEPLandLandings},
		{models.ClassTypeSEPSea, ReqKeySEPSeaTime, ReqKeySEPSeaLandings},
	}
	var reqs []Requirement
	for _, c := range classes {
		p, err := rt.dp.GetProgressByAircraftClass(ctx, rt.license.UserID, []models.ClassType{c.ct}, false, rt.since)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, buildReqs(p, []reqSpec{
			{nameKey: c.timeKey, metric: mTotalMinutes, threshold: 60, unit: "minutes"},
			{nameKey: c.landsKey, metric: mLandings, threshold: 6, unit: "landings"},
		})...)
	}
	return reqs, nil
}

// easaSPLRule — EASA SFCL.160(a) recency for sailplanes, excluding TMGs:
//   - 5 hours flight time as PIC, dual or supervised solo on sailplanes (GLIDER and TMG)
//   - 15 launches on sailplanes, excluding TMGs
//   - 2 training flights with an FI(S) on sailplanes, excluding TMGs
//   - OR a proficiency check with an FE(S) on a sailplane, excluding TMGs (SFCL.160(a)(2))
//
// Lookback: rolling 24 months from NOW. Also reports launch method recency
// per SFCL.155(c).
var easaSPLRule = ratingRule{
	displayKey:  "easa_spl",
	description: "Requires 5h flight time as PIC, dual or supervised solo on sailplanes including TMGs, with 15 launches and 2 training flights with an instructor on sailplanes excluding TMGs, within the last 24 months, or a proficiency check with an examiner (EASA SFCL.160(a)); each launch method needs 5 launches in 24 months, bungee 2 (SFCL.155(c))",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeByClass,
	countsTowed: true,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyFlightTime, metric: mPICOrDualMinutes, threshold: 300, unit: "minutes"},
		{nameKey: ReqKeyLaunches, metric: mLaunches, threshold: 15, unit: "launches"},
		{nameKey: ReqKeyTrainingFlights, metric: mTrainingFlights, threshold: 2, unit: "flights"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rt.since = rt.rule.window.rollingSince(time.Now())
		sailplane, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		hours := *sailplane
		tmg := &Progress{}
		if rt.rating.ClassType == models.ClassTypeGlider {
			tmg, err = rt.dp.GetProgressByAircraftClass(ctx, rt.license.UserID, []models.ClassType{models.ClassTypeTMG}, false, rt.since)
			if err != nil {
				rt.result.Status = StatusUnknown
				rt.result.setMsg(MsgRatingEvaluationFailed, nil)
				return
			}
			hours.PICMinutes += tmg.PICMinutes
			hours.InstructorMinutes += tmg.InstructorMinutes
			rt.result.CountedClasses = []models.ClassType{models.ClassTypeGlider, models.ClassTypeTMG}
			ulMinutes, err := rt.ulHoursCredit(ctx, models.ULKindSailplane, models.ULKindThreeAxisMotorglider)
			if err != nil {
				rt.result.Status = StatusUnknown
				rt.result.setMsg(MsgRatingEvaluationFailed, nil)
				return
			}
			hours.PICMinutes += ulMinutes
		}
		rt.result.Progress = sailplane
		reqs := []Requirement{
			buildReq(&hours, rt.rule.baseReqs[0]),
			buildReq(sailplane, rt.rule.baseReqs[1]),
			buildReq(sailplane, rt.rule.baseReqs[2]),
		}
		allMetByExperience := allReqsMet(reqs)

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, []models.ClassType{rt.rating.ClassType}, rt.since)
		reqProfCheck := profCheckRequirement(profCheckDate)
		rt.result.Requirements = append(reqs, reqProfCheck)

		rt.result.LaunchMethodCurrency = easaLaunchMethodCurrency(ctx, rt, tmg.Launches)

		if !allMetByExperience && !reqProfCheck.Met {
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRecencyNotMet, nil)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRecencyCurrent, nil)
		}
	},
}

// launchMethods lists the SFCL.155 launch methods in display order.
var launchMethods = []string{"winch", "car", "aerotow", "self-launch", "bungee"}

// launchMethodRequired returns the SFCL.155(c) launch count for a method.
func launchMethodRequired(method string) int {
	if method == "bungee" {
		return 2
	}
	return 5
}

// easaLaunchMethodCurrency returns SFCL.155(c) launch recency for every method
// the pilot has ever logged on the rating's class. TMG take-offs count toward
// self-launch.
func easaLaunchMethodCurrency(ctx context.Context, rt *ratingRuntime, tmgTakeoffs int) []LaunchMethodCurrency {
	everUsed, err := rt.dp.GetLaunchCounts(ctx, rt.license.UserID, rt.rating.ClassType, time.Time{})
	if err != nil {
		return nil
	}
	inWindow, err := rt.dp.GetLaunchCounts(ctx, rt.license.UserID, rt.rating.ClassType, rt.since)
	if err != nil {
		return nil
	}
	var out []LaunchMethodCurrency
	for _, method := range launchMethods {
		if everUsed[method] == 0 {
			continue
		}
		count := inWindow[method]
		if method == "self-launch" {
			count += tmgTakeoffs
		}
		required := launchMethodRequired(method)
		out = append(out, LaunchMethodCurrency{
			Method:     method,
			Launches:   count,
			Required:   required,
			Met:        count >= required,
			MessageKey: MsgLaunchMethodProgress,
		})
	}
	return out
}

// profCheckRequirement builds the proficiency-check requirement from the date
// of the most recent check in the window, nil when there is none.
func profCheckRequirement(date *time.Time) Requirement {
	if date == nil {
		return Requirement{
			NameKey: ReqKeyProficiencyCheck, Met: false,
			Current: 0, Required: 1, Unit: "check",
			MessageKey: MsgRequirementProfCheckMissing,
		}
	}
	return Requirement{
		NameKey: ReqKeyProficiencyCheck, Met: true,
		Current: 1, Required: 1, Unit: "check",
		MessageKey:    MsgRequirementProfCheckCompleted,
		MessageParams: msgDate(date.Format("2006-01-02")),
	}
}

// easaSPLTMGRule — EASA SFCL.160(b) recency for TMG privileges of an SPL:
//   - 12 hours flight time as PIC, dual or supervised solo on sailplanes (GLIDER and TMG), including on TMGs:
//   - 6 hours flight time
//   - 12 take-offs and landings
//   - 1 training flight of at least 1 hour total time with an instructor
//   - OR a proficiency check with an examiner on a TMG (SFCL.160(b)(2))
//
// Lookback: rolling 24 months from NOW. Distinct from PPL TMG (FCL.740.A).
var easaSPLTMGRule = ratingRule{
	displayKey:    "easa_spl_tmg",
	description:   "Requires 12h flight time as PIC, dual or supervised solo on sailplanes including TMGs, with 6h, 12 take-offs & landings and a training flight of at least 1h with an instructor on TMGs, within the last 24 months, or a proficiency check with an examiner on a TMG (EASA SFCL.160(b))",
	window:        windowSpec{kind: windowRollingNow, years: 2},
	scope:         scopeByClassOverride,
	classOverride: models.ClassTypeTMG,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyFlightTime, metric: mPICOrDualMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyTMGTime, metric: mPICOrDualMinutes, threshold: 360, unit: "minutes"},
		{nameKey: ReqKeyTMGLandings, metric: mLandings, threshold: 12, unit: "landings"},
		{nameKey: ReqKeyTMGTrainingFlight, metric: mLongestTrainingFlight, threshold: 60, unit: "minutes"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rt.since = rt.rule.window.rollingSince(time.Now())
		tmg, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		glider, err := rt.dp.GetProgressByAircraftClass(ctx, rt.license.UserID, []models.ClassType{models.ClassTypeGlider}, true, rt.since)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		ulSailplane, err := rt.ulHoursCredit(ctx, models.ULKindSailplane)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		ulMotorglider, err := rt.ulHoursCredit(ctx, models.ULKindThreeAxisMotorglider)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		rt.result.CreditedULKinds = []models.ULKind{models.ULKindSailplane, models.ULKindThreeAxisMotorglider}
		hours := *tmg
		hours.PICMinutes += glider.PICMinutes + ulSailplane + ulMotorglider
		hours.InstructorMinutes += glider.InstructorMinutes
		tmgHours := *tmg
		tmgHours.PICMinutes += ulMotorglider
		rt.result.CountedClasses = []models.ClassType{models.ClassTypeGlider, models.ClassTypeTMG}
		rt.result.Progress = tmg

		reqs := []Requirement{buildReq(&hours, rt.rule.baseReqs[0]), buildReq(&tmgHours, rt.rule.baseReqs[1])}
		reqs = append(reqs, buildReqs(tmg, rt.rule.baseReqs[2:])...)
		allMetByExperience := allReqsMet(reqs)

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, []models.ClassType{models.ClassTypeTMG}, rt.since)
		reqProfCheck := profCheckRequirement(profCheckDate)
		rt.result.Requirements = append(reqs, reqProfCheck)

		if !allMetByExperience && !reqProfCheck.Met {
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRecencyNotMet, nil)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRecencyCurrent, nil)
		}
	},
}

// applyClosedWindow handles the period between a rating's revalidation and
// the opening of its 12-month experience-counting window (EASA FCL.740.A,
// FCL.625.A). It populates the WindowOpensAt / WindowOpen fields on the
// result, and — if the window is still closed — fills in a "recently
// revalidated" message and returns (result, true).
//
// `since` must be the 12-month look-back anchor (rating.ExpiryDate − 12mo).
func applyClosedWindow(rating *models.ClassRating, since *time.Time, result ClassRatingCurrency) (ClassRatingCurrency, bool) {
	windowStr := since.Format("2006-01-02")
	result.WindowOpensAt = &windowStr
	if !time.Now().Before(*since) {
		result.WindowOpen = true
		return result, false
	}
	result.WindowOpen = false
	result.Status = StatusCurrent
	result.setMsg(MsgRatingWindowNotOpen, msgDate(windowStr))
	return result, true
}

// EvaluatePassengerCurrency evaluates EASA FCL.060(b) passenger-carrying
// recency, separate from rating revalidation.
//
// FCL.060(b)(1): 3 takeoffs, approaches and landings in same type or class
// within the preceding 90 days (rolling from now) for any passenger flight.
//
// FCL.060(b)(2): To carry passengers as PIC at night additionally:
//
//	(i)  at least 1 takeoff, approach and landing at night in the preceding 90 days, OR
//	(ii) holds an IR — in which case no night-landing recency is required.
func (e *EASAEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, peerRatings []*models.ClassRating, dp FlightDataProvider) PassengerCurrency {
	since := paxWindowStart(time.Now())

	hasNightPrivilege := HasNightPrivilege(license.LicenseType, license.RegulatoryAuthority) && classType != models.ClassTypeGlider
	hasValidIR := hasValidIRRating(peerRatings)

	result := PassengerCurrency{
		ClassType:           classType,
		RegulatoryAuthority: license.RegulatoryAuthority,
		DayRequired:         3,
		NightRequired:       1,
		NightPrivilege:      hasNightPrivilege,
		RuleDescription:     "3 takeoffs & landings (day) and 1 takeoff & landing at night in same type/class within preceding 90 days to carry passengers; the night requirement is waived for pilots holding a valid IR (EASA FCL.060(b))",
		RuleDescriptionKey:  "easa_pax",
	}

	// SFCL.160(e): sailplane passenger recency counts only launches or take-offs and landings as PIC.
	picOnly := false
	switch {
	case classType == models.ClassTypeGlider:
		picOnly = true
		result.RuleDescription = "3 launches as PIC on sailplanes, excluding TMGs, within the preceding 90 days to carry passengers (EASA SFCL.160(e)(1))"
		result.RuleDescriptionKey = "easa_spl_pax"
	case classType == models.ClassTypeTMG && isEASASailplane(license.LicenseType):
		picOnly = true
		result.RuleDescription = "3 take-offs & landings as PIC on TMGs within the preceding 90 days to carry passengers in a TMG, one of them at night to carry passengers at night (EASA SFCL.160(e)(2))"
		result.RuleDescriptionKey = "easa_spl_tmg_pax"
	}

	// FCL.060(b)(2)(ii): IR holders are exempt from the night-landing requirement.
	if hasValidIR {
		result.NightRequired = 0
	}

	days, err := dp.GetLandingDaysByAircraftClass(ctx, license.UserID, classType, includeTowedFlights(classType, isEASASailplane(license.LicenseType)), picOnly, since)
	if err != nil {
		result.DayStatus = StatusUnknown
		result.NightStatus = StatusUnknown
		result.setMsg(MsgPaxEvaluationFailed, nil)
		return result
	}

	landings, nightCount := paxTotals(days)
	result.DayLandings = landings
	result.NightLandings = nightCount
	result.DayExpiresOn = paxExpiryString(paxExpiryDate(days, result.DayRequired, allLandings))
	if hasNightPrivilege {
		result.NightExpiresOn = paxExpiryString(paxExpiryDate(days, result.NightRequired, nightLandings))
	}

	// Day passenger currency — FCL.060(b)(1)
	if landings >= 3 {
		result.DayStatus = StatusCurrent
	} else {
		result.DayStatus = StatusExpired
	}

	// Night passenger currency — FCL.060(b)(2)
	switch {
	case !hasNightPrivilege:
		// Night not applicable for this license type (e.g. LAPL, SPL).
		result.NightStatus = StatusUnknown
	case hasValidIR:
		// FCL.060(b)(2)(ii): holding an IR exempts the night-landing requirement.
		result.NightStatus = StatusCurrent
	case nightCount >= 1:
		result.NightStatus = StatusCurrent
	default:
		result.NightStatus = StatusExpired
	}

	// FCL.205.G(a)(2): GPL passengers only after 10h PIC on gyroplanes since issue
	gplShortfall := 0
	if classType == models.ClassTypeGyro && isGPL(license.LicenseType) {
		p, err := dp.GetProgressByAircraftClass(ctx, license.UserID, []models.ClassType{models.ClassTypeGyro}, false, license.IssueDate)
		if err != nil {
			result.DayStatus = StatusUnknown
			result.NightStatus = StatusUnknown
			result.setMsg(MsgPaxEvaluationFailed, nil)
			return result
		}
		if p.PICMinutes < gplPassengerPICMinutes {
			gplShortfall = gplPassengerPICMinutes - p.PICMinutes
			result.DayStatus = StatusExpired
			result.DayExpiresOn = nil
		}
	}

	// Summary message
	switch {
	case gplShortfall > 0:
		result.setMsg(MsgPaxGPLExperienceNotMet, msgNeeded(gplShortfall))
	case result.DayStatus != StatusCurrent:
		needed := 3 - landings
		result.setMsg(MsgPaxNotCurrent, msgNeeded(needed))
	case !hasNightPrivilege:
		result.setMsg(MsgPaxCurrentDayNoNight, nil)
	case hasValidIR:
		result.setMsg(MsgPaxCurrentDayNightIRWaived, nil)
	case result.NightStatus == StatusCurrent:
		result.setMsg(MsgPaxCurrentDayNight, nil)
	default:
		result.setMsg(MsgPaxDayCurrentNightNot, msgNeeded(1))
	}

	return result
}

// hasValidIRRating returns true if the given list of class ratings contains a
// current Instrument Rating — one with a non-nil expiry date that has not yet
// passed. Ratings without an expiry date count as not current.
func hasValidIRRating(ratings []*models.ClassRating) bool {
	for _, r := range ratings {
		if r == nil || r.ClassType != models.ClassTypeIR {
			continue
		}
		if r.ExpiryDate == nil || r.IsExpired() {
			continue
		}
		return true
	}
	return false
}

// applySFCLTMGExemption marks an SPL TMG result current when the pilot holds
// Part-FCL TMG privileges (SFCL.160(c)).
func applySFCLTMGExemption(result *ClassRatingCurrency) {
	result.Status = StatusCurrent
	result.Requirements = nil
	result.setMsg(MsgRatingSFCLTMGExempt, nil)
}
