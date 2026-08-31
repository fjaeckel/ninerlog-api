package currency

import (
	"context"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// FAAEvaluator implements FAA 14 CFR 61.57 currency rules. It is a thin adapter:
// Evaluate() selects the applicable rule from the FAA rule set and runs it
// through the engine. The regulatory data lives in the ratingRule definitions.
type FAAEvaluator struct{}

// NewFAAEvaluator creates a new FAA currency evaluator
func NewFAAEvaluator() *FAAEvaluator {
	return &FAAEvaluator{}
}

func (e *FAAEvaluator) Authority() string {
	return "FAA"
}

func (e *FAAEvaluator) Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dataProvider FlightDataProvider) ClassRatingCurrency {
	return evalRatingRule(ctx, faaSelectRule(rating, license), rating, license, dataProvider)
}

// faaSelectRule dispatches a (license type, class type) pair to its rule.
//   - Sport/Recreational Pilot (§61.315): IR is suppressed (day VFR only)
//   - Glider: uses launches instead of landings
func faaSelectRule(rating *models.ClassRating, license *models.License) *ratingRule {
	lt := strings.ToUpper(license.LicenseType)

	switch rating.ClassType {
	case models.ClassTypeIR:
		// Sport Pilot cannot fly IFR — skip IR evaluation
		if lt == "SPORT" || lt == "RECREATIONAL" {
			return &faaSuppressedIRRule
		}
		return &faaInstrumentRule
	default:
		// Glider uses launches instead of landings
		if lt == "GLIDER" {
			return &faaGliderRule
		}
		return &faaPassengerRatingRule
	}
}

// ── FAA rule definitions ────────────────────────────────────────────────────

// faaPassengerRatingRule — FAA 14 CFR 61.57(a)/(b) Tier-1 rating currency:
//
//	(a) Day: 3 takeoffs and landings in preceding 90 days in same category/class.
//	(b) Night: 3 full-stop takeoffs and landings at night within preceding 90 days.
var faaPassengerRatingRule = ratingRule{
	displayKey:  "faa_pax_day_night",
	description: "Requires 3 takeoffs & landings in preceding 90 days in same category/class for day passenger currency; 3 full-stop night takeoffs & landings in 90 days for night currency (14 CFR 61.57)",
	window:      windowSpec{kind: windowRollingNow, days: 90},
	scope:       scopeByClass,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyDayLandings, metric: mLandings, threshold: 3, unit: "landings"},
		{nameKey: ReqKeyNightLandings, metric: mNightLandings, threshold: 3, unit: "landings"},
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
		dayReq := reqs[0]
		nightReq := reqs[1]
		rt.result.Requirements = reqs

		// Status based on worst case:
		// - Day not met → expired (cannot carry passengers at all)
		// - Day met, night not met → expiring (can fly day only with passengers)
		// - Both met → current
		if !dayReq.Met {
			rt.result.Status = StatusExpired
			needed := 3 - progress.Landings
			rt.result.setMsg(MsgRatingPaxNotCurrent, msgNeeded(needed))
		} else if !nightReq.Met {
			rt.result.Status = StatusExpiring
			needed := 3 - progress.NightLandings
			rt.result.setMsg(MsgRatingPaxDayCurrentNightNo, msgNeeded(needed))
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingPaxCurrentDayNight, nil)
		}
	},
}

// faaInstrumentRule — FAA 14 CFR 61.57(c) instrument currency:
//
// Within the preceding 6 calendar months: 6 instrument approaches, holding
// procedures, and intercepting/tracking courses. Includes the §61.57(c)/(d)
// 12-month grace period: 6–12 months → recoverable with safety pilot;
// >12 months → IPC required.
var faaInstrumentRule = ratingRule{
	displayKey:  "faa_ir",
	description: "Requires 6 instrument approaches + holding procedures within preceding 6 calendar months (14 CFR 61.57(c))",
	window:      windowSpec{kind: windowRollingNow, months: 6},
	scope:       scopeAll,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyApproaches, metric: mApproaches, threshold: 6, unit: "approaches"},
		{nameKey: ReqKeyHolds, metric: mHolds, threshold: 1, unit: "holds"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		rt.result.Progress = progress
		reqs := buildReqs(progress, rt.rule.baseReqs)
		reqApproaches := reqs[0]
		reqHolds := reqs[1]
		rt.result.Requirements = reqs

		allMet := reqApproaches.Met && reqHolds.Met

		if rating.ExpiryDate != nil && rating.IsExpired() {
			rt.result.Status = StatusExpired
			rt.result.setMsg(MsgRatingExpired, nil)
		} else if allMet {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingIRCurrent, nil)
		} else {
			// Check 12-month grace period: §61.57(c)/(d)
			// 0-6 months: current (checked above)
			// 6-12 months: can regain by practice with safety pilot
			// >12 months: IPC required
			since12 := time.Now().AddDate(-1, 0, 0)
			progress12, err12 := rt.dp.GetProgressAll(ctx, rt.license.UserID, since12)
			if err12 == nil && (progress12.Approaches >= 6 && progress12.Holds >= 1) {
				// Met within 12 months but not within 6 — lapsed but recoverable
				rt.result.Status = StatusExpiring
				rt.result.setMsg(MsgRatingIRLapsedSafetyPilot, nil)
				rt.result.RuleDescription = "Instrument currency lapsed past 6 months. Can regain within 12 months by completing 6 approaches + holding with safety pilot. After 12 months, IPC required (14 CFR 61.57(c)/(d))"
			} else {
				// Not met within 12 months either — IPC required
				rt.result.Status = StatusExpired
				rt.result.setMsg(MsgRatingIRExpiredIPC, nil)
				rt.result.RuleDescription = "Instrument currency expired. Instrument Proficiency Check (IPC) required to regain currency (14 CFR 61.57(d))"
			}
		}
	},
}

// faaGliderRule — FAA §61.57(a) for glider category. Gliders use "launches"
// instead of "takeoffs": 3 launches and landings in the preceding 90 days.
// Night and IR currency are not applicable for gliders.
//
// The description defaults to the passenger text and is upgraded to the
// glider-specific text after a successful data fetch.
var faaGliderRule = ratingRule{
	displayKey:  "faa_pax_day_night",
	description: "Requires 3 takeoffs & landings in preceding 90 days in same category/class for day passenger currency; 3 full-stop night takeoffs & landings in 90 days for night currency (14 CFR 61.57)",
	window:      windowSpec{kind: windowRollingNow, days: 90},
	scope:       scopeByClass,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyLaunchesAndLanding, metric: mLandings, threshold: 3, unit: "launches"},
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
		rt.result.RuleDescription = "Requires 3 launches & landings in preceding 90 days in same category (14 CFR 61.57(a)) — night and IFR not applicable for gliders"

		reqs := buildReqs(progress, rt.rule.baseReqs)
		launchReq := reqs[0]
		rt.result.Requirements = reqs

		if !launchReq.Met {
			rt.result.Status = StatusExpired
			needed := 3 - progress.Landings
			rt.result.setMsg(MsgRatingGliderNotCurrent, msgNeeded(needed))
		} else {
			rt.result.Status = StatusCurrent
			rt.result.setMsg(MsgRatingGliderCurrent, nil)
		}
	},
}

// faaSuppressedIRRule — Sport/Recreational Pilot certificates have no instrument
// privileges (§61.315); IR evaluation is suppressed.
var faaSuppressedIRRule = ratingRule{
	displayKey:  "faa_ir",
	description: "Instrument privileges not available for Sport/Recreational Pilot certificates",
	scope:       scopeAll,
	finalize: func(_ context.Context, rt *ratingRuntime) {
		rt.result.Status = StatusUnknown
		rt.result.setMsg(MsgRatingIRNotApplicable, nil)
	},
}

// HasNightPrivilege returns whether the given license type has night flying privileges.
// Used by the frontend to show/hide night currency sections.
func HasNightPrivilege(licenseType, authority string) bool {
	lt := strings.ToUpper(licenseType)
	auth := strings.ToUpper(authority)

	switch {
	case auth == "FAA" && (lt == "SPORT" || lt == "RECREATIONAL" || lt == "GLIDER"):
		return false
	case auth == "EASA" && (lt == "SPL" || lt == "LAPL(S)"):
		return false
	case auth == "EASA" && lt == "LAPL":
		return false // LAPL requires separate night rating extension
	case auth == "LBA" || auth == "DULV" || auth == "DAEC":
		return false // German UL — no night flying
	default:
		return true // PPL, CPL, ATPL, FAA Private/Commercial/ATP
	}
}

// EvaluatePassengerCurrency evaluates FAA §61.57(a)/(b) as Tier 2 passenger
// currency, separate from rating currency.
func (e *FAAEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, _ []*models.ClassRating, dp FlightDataProvider) PassengerCurrency {
	since := paxWindowStart(time.Now())
	hasNight := HasNightPrivilege(license.LicenseType, license.RegulatoryAuthority)

	result := PassengerCurrency{
		ClassType:           classType,
		RegulatoryAuthority: "FAA",
		DayRequired:         3,
		NightRequired:       3,
		NightPrivilege:      hasNight,
		RuleDescription:     "3 takeoffs & landings in preceding 90 days for day passenger currency; 3 full-stop night takeoffs & landings in 90 days for night currency (14 CFR 61.57(a)/(b))",
		RuleDescriptionKey:  "faa_pax_day_night",
	}

	// Suppress night for license types without night privilege
	if !hasNight {
		result.NightRequired = 0
		result.RuleDescription = "3 takeoffs & landings in preceding 90 days for day passenger currency (14 CFR 61.57(a)) — night not applicable for " + license.LicenseType
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
	result.NightExpiresOn = paxExpiryString(paxExpiryDate(days, result.NightRequired, nightLandings))

	if landings >= 3 {
		result.DayStatus = StatusCurrent
	} else {
		result.DayStatus = StatusExpired
	}

	if !hasNight {
		// Night not applicable — mark as N/A (unknown = not evaluated)
		result.NightStatus = StatusUnknown
		if result.DayStatus == StatusCurrent {
			result.setMsg(MsgPaxCurrentDayNoNight, nil)
		} else {
			needed := 3 - landings
			result.setMsg(MsgPaxNotCurrent, msgNeeded(needed))
		}
	} else {
		if nightCount >= 3 {
			result.NightStatus = StatusCurrent
		} else {
			result.NightStatus = StatusExpired
		}

		if result.DayStatus == StatusCurrent && result.NightStatus == StatusCurrent {
			result.setMsg(MsgPaxCurrentDayNight, nil)
		} else if result.DayStatus == StatusCurrent {
			needed := 3 - nightCount
			result.setMsg(MsgPaxDayCurrentNightNot, msgNeeded(needed))
		} else {
			needed := 3 - landings
			result.setMsg(MsgPaxNotCurrent, msgNeeded(needed))
		}
	}

	return result
}

// EvaluateFlightReview evaluates FAA §61.56 flight review currency.
// A flight review must be completed within the preceding 24 calendar months.
// This applies to ALL FAA certificate types and is per-pilot, not per-class-rating.
func (e *FAAEvaluator) EvaluateFlightReview(ctx context.Context, userID uuid.UUID, dp FlightDataProvider) *FlightReviewStatus {
	lastReview, err := dp.GetLastFlightReview(ctx, userID)
	if err != nil {
		return &FlightReviewStatus{
			Status:     StatusUnknown,
			MessageKey: MsgFlightReviewEvaluationFailed,
		}
	}

	if lastReview == nil {
		return &FlightReviewStatus{
			Status:     StatusExpired,
			MessageKey: MsgFlightReviewNoneOnRecord,
		}
	}

	return faaFlightReviewStatus(time.Now(), *lastReview)
}

// faaFlightReviewStatus is the pure decision behind EvaluateFlightReview.
func faaFlightReviewStatus(now, lastReview time.Time) *FlightReviewStatus {
	completedStr := lastReview.Format("2006-01-02")

	// §61.56: valid for 24 calendar months from the end of the month the
	// review was completed.
	expiresOn := time.Date(lastReview.Year(), lastReview.Month()+25, 0, 0, 0, 0, 0, time.UTC) // last day of month + 24
	expiresStr := expiresOn.Format("2006-01-02")

	// expiresOn is midnight at the start of the last valid day; validity runs
	// through that whole day.
	validUntil := expiresOn.AddDate(0, 0, 1)
	daysUntilExpiry := int(validUntil.Sub(now).Hours() / 24)

	result := &FlightReviewStatus{
		LastCompleted: &completedStr,
		ExpiresOn:     &expiresStr,
	}

	if !now.Before(validUntil) {
		result.Status = StatusExpired
		result.setMsg(MsgFlightReviewExpired, msgDate(completedStr))
	} else if daysUntilExpiry <= 90 {
		result.Status = StatusExpiring
		result.setMsg(MsgFlightReviewExpiring, msgDaysDate(daysUntilExpiry, completedStr))
	} else {
		result.Status = StatusCurrent
		result.setMsg(MsgFlightReviewCurrent, msgDate(completedStr))
	}

	return result
}
