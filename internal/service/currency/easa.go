package currency

import (
	"context"
	"fmt"
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
		{name: "Total Time", metric: mTotalMinutes, threshold: 720, unit: "minutes", msgFmt: "%d / 720 minutes in class"},
		{name: "PIC Time", metric: mPICMinutes, threshold: 360, unit: "minutes", msgFmt: "%d / 360 PIC minutes"},
		{name: "Takeoffs & Landings", metric: mLandings, threshold: 12, unit: "landings", msgFmt: "%d / 12 takeoffs & landings"},
		{name: "Refresher Training", metric: mInstructorMinutes, threshold: 60, unit: "minutes", msgFmt: "%d / 60 minutes with instructor"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		if rating.ExpiryDate == nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = fmt.Sprintf("EASA %s — no expiry date set", rating.ClassType)
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
			rt.result.Message = fmt.Sprintf("EASA %s — unable to evaluate currency", rating.ClassType)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		rt.result.Requirements = reqs
		allMet := allReqsMet(reqs)

		if rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.Message = fmt.Sprintf("EASA %s expired on %s", rating.ClassType, *rt.result.ExpiryDate)
		} else if !allMet {
			rt.result.Status = StatusExpiring
			rt.result.Message = fmt.Sprintf("EASA %s — revalidation requirements not fully met", rating.ClassType)
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			rt.result.Status = StatusExpiring
			rt.result.Message = fmt.Sprintf("EASA %s expires in %d days — requirements met", rating.ClassType, daysLeft)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = fmt.Sprintf("EASA %s current — all revalidation requirements met", rating.ClassType)
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
		{name: "Route Sectors", metric: mFlights, threshold: 10, unit: "flights", msgFmt: "%d / 10 route sectors"},
		{name: "Refresher Training", metric: mInstructorMinutes, threshold: 60, unit: "minutes", msgFmt: "%d / 60 minutes with instructor"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		if rating.ExpiryDate == nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = fmt.Sprintf("EASA %s — no expiry date set", rating.ClassType)
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
			rt.result.Message = fmt.Sprintf("EASA %s — unable to evaluate currency", rating.ClassType)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		reqSectors := reqs[0]
		reqInstructor := reqs[1]

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, rating.ClassType, since)
		hasProfCheck := profCheckDate != nil
		reqProfCheck := Requirement{
			Name: "Proficiency Check", Met: hasProfCheck,
			Current: 0, Required: 1, Unit: "check",
			Message: "Not completed in validity period",
		}
		if hasProfCheck {
			reqProfCheck.Current = 1
			reqProfCheck.Message = fmt.Sprintf("Proficiency check completed %s", profCheckDate.Format("2006-01-02"))
		}

		rt.result.Requirements = []Requirement{reqSectors, reqInstructor, reqProfCheck}

		allMetByExperience := reqSectors.Met && reqInstructor.Met
		allMet := allMetByExperience || hasProfCheck

		if rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.Message = fmt.Sprintf("EASA %s expired on %s", rating.ClassType, *rt.result.ExpiryDate)
		} else if !allMet {
			rt.result.Status = StatusExpiring
			rt.result.Message = fmt.Sprintf("EASA %s — revalidation requirements not fully met (proficiency check may apply)", rating.ClassType)
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			rt.result.Status = StatusExpiring
			rt.result.Message = fmt.Sprintf("EASA %s expires in %d days — requirements met", rating.ClassType, daysLeft)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = fmt.Sprintf("EASA %s current — all revalidation requirements met", rating.ClassType)
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
		{name: "IFR Time", metric: mIFRMinutes, threshold: 600, unit: "minutes", msgFmt: "%d / 600 IFR minutes"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		if rating.ExpiryDate == nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = "EASA IR — no expiry date set"
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
			rt.result.Message = "EASA IR — unable to evaluate currency"
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		reqIFRHours := reqs[0]

		profCheckDate, _ := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, models.ClassTypeIR, since)
		hasProfCheck := profCheckDate != nil
		reqProfCheck := Requirement{
			Name: "Proficiency Check", Met: hasProfCheck,
			Current: 0, Required: 1, Unit: "check",
			Message: "Annual proficiency check not completed in validity period",
		}
		if hasProfCheck {
			reqProfCheck.Current = 1
			reqProfCheck.Message = fmt.Sprintf("Proficiency check completed %s", profCheckDate.Format("2006-01-02"))
		}

		rt.result.Requirements = []Requirement{reqIFRHours, reqProfCheck}

		allMet := reqIFRHours.Met && hasProfCheck

		if rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.Message = fmt.Sprintf("EASA IR expired on %s", *rt.result.ExpiryDate)
		} else if !allMet {
			rt.result.Status = StatusExpiring
			if !reqIFRHours.Met && !hasProfCheck {
				rt.result.Message = "EASA IR — IFR hours and proficiency check not met"
			} else if !reqIFRHours.Met {
				rt.result.Message = "EASA IR — IFR hour requirement not met"
			} else {
				rt.result.Message = "EASA IR — annual proficiency check not completed"
			}
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			rt.result.Status = StatusExpiring
			rt.result.Message = fmt.Sprintf("EASA IR expires in %d days — requirements met", daysLeft)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = "EASA IR current — all requirements met"
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
			rt.result.Message = fmt.Sprintf("EASA %s — no expiry date set", rating.ClassType)
			return
		}
		if rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.Message = fmt.Sprintf("EASA %s expired on %s", rating.ClassType, *rt.result.ExpiryDate)
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			rt.result.Status = StatusExpiring
			rt.result.Message = fmt.Sprintf("EASA %s expires in %d days", rating.ClassType, daysLeft)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = fmt.Sprintf("EASA %s valid until %s", rating.ClassType, *rt.result.ExpiryDate)
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
		{name: "Flight Time", metric: mTotalMinutes, threshold: 720, unit: "minutes", msgFmt: "%d / 720 minutes in last 24 months"},
		{name: "Takeoffs & Landings", metric: mLandings, threshold: 12, unit: "landings", msgFmt: "%d / 12 takeoffs & landings in last 24 months"},
		{name: "Training Flight", metric: mInstructorMinutes, threshold: 60, unit: "minutes", msgFmt: "%d / 60 minutes with instructor in last 24 months"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = fmt.Sprintf("EASA LAPL %s — unable to evaluate recency", rating.ClassType)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		rt.result.Requirements = reqs

		if !allReqsMet(reqs) {
			rt.result.Status = StatusExpiring
			rt.result.Message = fmt.Sprintf("EASA LAPL %s — recency requirements not fully met (FCL.140.A)", rating.ClassType)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = fmt.Sprintf("EASA LAPL %s — current (FCL.140.A)", rating.ClassType)
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
		{name: "PIC Flight Time", metric: mPICMinutes, threshold: 300, unit: "minutes", msgFmt: "%d / 300 PIC minutes in last 24 months"},
		{name: "Launches", metric: mLandings, threshold: 15, unit: "launches", msgFmt: "%d / 15 launches in last 24 months"},
		{name: "Training Flights", metric: mInstructorMinutes, threshold: 60, unit: "minutes", msgFmt: "%d / 60 minutes training flights in last 24 months"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		since := rt.rule.window.rollingSince(time.Now())
		rt.since = since
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = fmt.Sprintf("EASA SPL %s — unable to evaluate recency", rating.ClassType)
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
					Method:   method,
					Launches: count,
					Required: 5,
					Met:      count >= 5,
					Message:  fmt.Sprintf("%d / 5 %s launches in last 24 months", count, method),
				})
			}
		}
		rt.result.LaunchMethodCurrency = launchMethodCurrency

		if !allMet {
			rt.result.Status = StatusExpiring
			rt.result.Message = fmt.Sprintf("EASA SPL %s — recency requirements not fully met (FCL.140.S)", rating.ClassType)
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = fmt.Sprintf("EASA SPL %s — current (FCL.140.S)", rating.ClassType)
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
		{name: "TMG Flight Time", metric: mTotalMinutes, threshold: 720, unit: "minutes", msgFmt: "%d / 720 minutes on TMG in last 24 months"},
		{name: "TMG Takeoffs & Landings", metric: mLandings, threshold: 12, unit: "landings", msgFmt: "%d / 12 takeoffs & landings on TMG in last 24 months"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = "EASA SPL TMG — unable to evaluate recency"
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		rt.result.Requirements = reqs

		if !allReqsMet(reqs) {
			rt.result.Status = StatusExpiring
			rt.result.Message = "EASA SPL TMG — recency requirements not fully met (FCL.140.S(b)(2))"
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = "EASA SPL TMG — current (FCL.140.S(b)(2))"
		}
	},
}

// applyClosedWindow handles the period between a rating's revalidation and
// the opening of its 12-month experience-counting window (EASA FCL.740.A,
// FCL.625.A). It populates the WindowOpensAt / WindowOpen fields on the
// result, and — if the window is still closed — fills in a "recently
// revalidated" message and returns (result, true) so the caller can return
// early without consulting the FlightDataProvider.
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
	result.Message = fmt.Sprintf(
		"EASA %s — recently revalidated; experience window opens %s",
		rating.ClassType, windowStr,
	)
	return result, true
}

// EvaluatePassengerCurrency evaluates EASA FCL.060(b) passenger-carrying recency.
// This is SEPARATE from rating revalidation — a pilot can have a valid rating but
// cannot carry passengers without meeting FCL.060(b).
//
// FCL.060(b)(1): 3 takeoffs, approaches and landings in same type or class
// within the preceding 90 days (rolling from now) for any passenger flight.
//
// FCL.060(b)(2): To carry passengers as PIC at night additionally:
//
//	(i)  at least 1 takeoff, approach and landing at night in the preceding 90 days, OR
//	(ii) holds an IR — in which case no night-landing recency is required.
func (e *EASAEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, peerRatings []*models.ClassRating, dp FlightDataProvider) PassengerCurrency {
	since := time.Now().AddDate(0, 0, -90)

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

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, classType, since)
	if err != nil {
		result.DayStatus = StatusUnknown
		result.NightStatus = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — unable to evaluate passenger currency", classType)
		return result
	}

	result.DayLandings = progress.Landings
	result.NightLandings = progress.NightLandings

	// Day passenger currency — FCL.060(b)(1)
	if progress.Landings >= 3 {
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
	case progress.NightLandings >= 1:
		result.NightStatus = StatusCurrent
	default:
		result.NightStatus = StatusExpired
	}

	// Summary message
	switch {
	case result.DayStatus != StatusCurrent:
		needed := 3 - progress.Landings
		result.Message = fmt.Sprintf("EASA %s — not current for passengers (need %d more landing%s in 90 days)", classType, needed, plural(needed))
	case !hasNightPrivilege:
		result.Message = fmt.Sprintf("EASA %s — passenger current for day (night not applicable for %s)", classType, license.LicenseType)
	case hasValidIR:
		result.Message = fmt.Sprintf("EASA %s — passenger current for day and night (night requirement waived under FCL.060(b)(2)(ii) — IR holder)", classType)
	case result.NightStatus == StatusCurrent:
		result.Message = fmt.Sprintf("EASA %s — passenger current (day and night)", classType)
	default:
		result.Message = fmt.Sprintf("EASA %s — day passenger current, night not current (need 1 night landing in 90 days, or hold an IR)", classType)
	}

	return result
}

// hasValidIRRating returns true if the given list of class ratings contains a
// current Instrument Rating — i.e. one with a non-nil expiry date that has not
// yet passed. Ratings without an expiry date are treated as unknown/not current
// because currency cannot be confirmed.
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
