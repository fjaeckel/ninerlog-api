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
	return evalRatingRule(ctx, easaSelectRule(rating, license), rating, license, dataProvider)
}

// easaSelectRule dispatches a (license type, class type) pair to its rule.
// License-type-aware: LAPL/SPL use recency regulations (FCL.140.x) while
// PPL/CPL/ATPL use revalidation regulations (FCL.740.A). IR is always
// FCL.625.A regardless of license type.
func easaSelectRule(rating *models.ClassRating, license *models.License) *ratingRule {
	lt := strings.ToUpper(license.LicenseType)

	// IR is always FCL.625.A regardless of license type
	if rating.ClassType == models.ClassTypeIR {
		return &easaIRRule
	}

	// LAPL uses FCL.140.A (rolling 24 months from now, no PIC requirement)
	if lt == "LAPL" || lt == "LAPL(A)" {
		return &easaLAPLRule
	}

	// SPL/LAPL(S) uses FCL.140.S (rolling 24 months, launches not landings)
	if lt == "SPL" || lt == "LAPL(S)" {
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
// All within the 12 months preceding the expiry date of the rating.
var easaSEPTMGRule = ratingRule{
	displayKey:  "easa_sep_tmg",
	description: "Requires 12h total flight time + 6h as PIC + 12 takeoffs & landings + 1h refresher training with instructor, all within the 12 months preceding the expiry date (EASA FCL.740.A(b)(1))",
	window:      windowSpec{kind: windowPrecedingExpiry, years: 1},
	scope:       scopeByClass,
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

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, rating.ClassType, since)
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

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, models.ClassTypeIR, since)
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

// easaLAPLRule — EASA FCL.140.A(a) recency for LAPL(A):
//   - 12 hours flight time (as PIC, dual, or solo under supervision)
//   - 12 takeoffs & landings
//   - 1 hour dual instruction
//   - NO PIC hour requirement (key difference from FCL.740.A)
//
// Lookback: rolling 24 months from NOW.
var easaLAPLRule = ratingRule{
	displayKey:  "easa_lapl",
	description: "Requires 12h flight time + 12 takeoffs & landings + 1h training flight with instructor within the last 24 months (EASA FCL.140.A)",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeByClass,
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
		rt.result.Requirements = reqs

		if !allReqsMet(reqs) {
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRecencyNotMet, nil)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRecencyCurrent, nil)
		}
	},
}

// easaSPLRule — EASA FCL.140.S(a) recency for SPL/LAPL(S):
//   - 5 hours flight time as PIC on sailplanes
//   - 15 launches (NOT landings)
//   - 2 training flights with instructor
//
// Lookback: rolling 24 months from NOW. Also evaluates launch method currency
// per FCL.140.S(b)(1).
var easaSPLRule = ratingRule{
	displayKey:  "easa_spl",
	description: "Requires 5h PIC flight time + 15 launches + 2 training flights with instructor within the last 24 months (EASA FCL.140.S)",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeByClass,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyPICTime, metric: mPICMinutes, threshold: 300, unit: "minutes"},
		{nameKey: ReqKeyLaunches, metric: mLandings, threshold: 15, unit: "launches"},
		{nameKey: ReqKeyTrainingFlight, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		since := rt.rule.window.rollingSince(time.Now())
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

		launchCounts, _ := rt.dp.GetLaunchCounts(ctx, rt.license.UserID, since)
		var launchMethodCurrency []LaunchMethodCurrency
		for _, method := range []string{"winch", "aerotow", "self-launch"} {
			count := launchCounts[method]
			if count > 0 || launchCounts[method] > 0 {
				launchMethodCurrency = append(launchMethodCurrency, LaunchMethodCurrency{
					Method:     method,
					Launches:   count,
					Required:   5,
					Met:        count >= 5,
					MessageKey: MsgLaunchMethodProgress,
				})
			}
		}
		rt.result.LaunchMethodCurrency = launchMethodCurrency

		if !allMet {
			rt.result.Status = StatusExpiring
			rt.result.setMsg(MsgRatingRecencyNotMet, nil)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingRecencyCurrent, nil)
		}
	},
}

// easaSPLTMGRule — EASA FCL.140.S(b)(2) TMG extension for SPL:
//   - 12 hours flight time on TMG
//   - 12 takeoffs & landings on TMG
//
// Lookback: rolling 24 months from NOW. Distinct from PPL TMG (FCL.740.A).
var easaSPLTMGRule = ratingRule{
	displayKey:    "easa_spl_tmg",
	description:   "Requires 12h flight time + 12 takeoffs & landings on TMG within the last 24 months (EASA FCL.140.S(b)(2))",
	window:        windowSpec{kind: windowRollingNow, years: 2},
	scope:         scopeByClassOverride,
	classOverride: models.ClassTypeTMG,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 12, unit: "landings"},
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
		rt.result.Requirements = reqs

		if !allReqsMet(reqs) {
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

	hasNightPrivilege := HasNightPrivilege(license.LicenseType, license.RegulatoryAuthority)
	hasValidIR := hasValidIRRating(peerRatings)

	result := PassengerCurrency{
		ClassType:           classType,
		RegulatoryAuthority: "EASA",
		DayRequired:         3,
		NightRequired:       1,
		NightPrivilege:      hasNightPrivilege,
		RuleDescription:     "3 takeoffs & landings (day) and 1 takeoff & landing at night in same type/class within preceding 90 days to carry passengers; the night requirement is waived for pilots holding a valid IR (EASA FCL.060(b))",
		RuleDescriptionKey:  "easa_pax",
	}

	// FCL.060(b)(2)(ii): IR holders are exempt from the night-landing requirement.
	if hasValidIR {
		result.NightRequired = 0
	}

	days, err := dp.GetLandingDaysByAircraftClass(ctx, license.UserID, classType, since)
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

	// Summary message
	switch {
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
