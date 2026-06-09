package currency

import (
	"context"
	"fmt"
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
		{name: "Day Passenger Currency", metric: mLandings, threshold: 3, unit: "landings", msgFmt: "%d / 3 takeoffs & landings in 90 days"},
		{name: "Night Passenger Currency", metric: mNightLandings, threshold: 3, unit: "landings", msgFmt: "%d / 3 night full-stop landings in 90 days"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = fmt.Sprintf("FAA %s — unable to evaluate currency", rating.ClassType)
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
			rt.result.Message = fmt.Sprintf("FAA %s — not current for passengers (need %d more landing%s)", rating.ClassType, needed, plural(needed))
		} else if !nightReq.Met {
			rt.result.Status = StatusExpiring
			needed := 3 - progress.NightLandings
			rt.result.Message = fmt.Sprintf("FAA %s — day current, night not current (need %d more night landing%s)", rating.ClassType, needed, plural(needed))
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = fmt.Sprintf("FAA %s — current for day and night passengers", rating.ClassType)
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
		{name: "Instrument Approaches", metric: mApproaches, threshold: 6, unit: "approaches", msgFmt: "%d / 6 instrument approaches in 6 months"},
		{name: "Holding Procedures", metric: mHolds, threshold: 1, unit: "holds", msgFmt: "%d / 1 holding procedure in 6 months"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = "FAA IR — unable to evaluate currency"
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
			rt.result.Message = fmt.Sprintf("FAA IR expired on %s", *rt.result.ExpiryDate)
		} else if allMet {
			rt.result.Status = StatusCurrent
			rt.result.Message = "FAA IR — instrument currency requirements met"
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
				rt.result.Message = "FAA IR — instrument currency lapsed (6+ months), can regain by practice with safety pilot (§61.57(c))"
				rt.result.RuleDescription = "Instrument currency lapsed past 6 months. Can regain within 12 months by completing 6 approaches + holding with safety pilot. After 12 months, IPC required (14 CFR 61.57(c)/(d))"
			} else {
				// Not met within 12 months either — IPC required
				rt.result.Status = StatusExpired
				rt.result.Message = "FAA IR — instrument currency expired (>12 months without required experience), IPC required (§61.57(d))"
				rt.result.RuleDescription = "Instrument currency expired. Instrument Proficiency Check (IPC) required to regain currency (14 CFR 61.57(d))"
			}
		}
	},
}

// faaGliderRule — FAA §61.57(a) for glider category. Gliders use "launches"
// instead of "takeoffs": 3 launches and landings in the preceding 90 days.
// Night and IR currency are not applicable for gliders.
//
// The description deliberately defaults to the passenger text and is only
// upgraded to the glider-specific text after a successful data fetch, matching
// the historical behaviour on the error path.
var faaGliderRule = ratingRule{
	displayKey:  "faa_pax_day_night",
	description: "Requires 3 takeoffs & landings in preceding 90 days in same category/class for day passenger currency; 3 full-stop night takeoffs & landings in 90 days for night currency (14 CFR 61.57)",
	window:      windowSpec{kind: windowRollingNow, days: 90},
	scope:       scopeByClass,
	baseReqs: []reqSpec{
		{name: "Launches & Landings", metric: mLandings, threshold: 3, unit: "launches", msgFmt: "%d / 3 launches & landings in 90 days"},
	},
	finalize: func(ctx context.Context, rt *ratingRuntime) {
		rating := rt.rating
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.Message = fmt.Sprintf("FAA %s (Glider) — unable to evaluate currency", rating.ClassType)
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
			rt.result.Message = fmt.Sprintf("FAA Glider — not current (need %d more launch%s)", needed, plural(needed))
		} else {
			rt.result.Status = StatusCurrent
			rt.result.Message = "FAA Glider — current for passengers"
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
		rt.result.Message = fmt.Sprintf("FAA %s — instrument currency not applicable for %s Pilot (§61.315)", rt.rating.ClassType, rt.license.LicenseType)
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

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// EvaluatePassengerCurrency evaluates FAA §61.57(a)/(b) as Tier 2 passenger currency.
// This is separate from rating currency — FAA certificates don't expire, so passenger
// currency IS the primary rolling metric for non-IR class ratings.
func (e *FAAEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, _ []*models.ClassRating, dp FlightDataProvider) PassengerCurrency {
	since := time.Now().AddDate(0, 0, -90)
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

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, classType, since)
	if err != nil {
		result.DayStatus = StatusUnknown
		result.NightStatus = StatusUnknown
		result.Message = fmt.Sprintf("FAA %s — unable to evaluate passenger currency", classType)
		return result
	}

	result.DayLandings = progress.Landings
	result.NightLandings = progress.NightLandings

	if progress.Landings >= 3 {
		result.DayStatus = StatusCurrent
	} else {
		result.DayStatus = StatusExpired
	}

	if !hasNight {
		// Night not applicable — mark as N/A (unknown = not evaluated)
		result.NightStatus = StatusUnknown
		if result.DayStatus == StatusCurrent {
			result.Message = fmt.Sprintf("FAA %s — current for day passengers (night not applicable for %s)", classType, license.LicenseType)
		} else {
			needed := 3 - progress.Landings
			result.Message = fmt.Sprintf("FAA %s — not current for passengers (need %d more landing%s)", classType, needed, plural(needed))
		}
	} else {
		if progress.NightLandings >= 3 {
			result.NightStatus = StatusCurrent
		} else {
			result.NightStatus = StatusExpired
		}

		if result.DayStatus == StatusCurrent && result.NightStatus == StatusCurrent {
			result.Message = fmt.Sprintf("FAA %s — current for day and night passengers", classType)
		} else if result.DayStatus == StatusCurrent {
			needed := 3 - progress.NightLandings
			result.Message = fmt.Sprintf("FAA %s — day current, night not current (need %d more night landing%s)", classType, needed, plural(needed))
		} else {
			needed := 3 - progress.Landings
			result.Message = fmt.Sprintf("FAA %s — not current for passengers (need %d more landing%s)", classType, needed, plural(needed))
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
			Status:  StatusUnknown,
			Message: "Unable to determine flight review status",
		}
	}

	if lastReview == nil {
		return &FlightReviewStatus{
			Status:  StatusExpired,
			Message: "No flight review on record — required every 24 calendar months (14 CFR 61.56)",
		}
	}

	completedStr := lastReview.Format("2006-01-02")

	// §61.56: "since the beginning of the 24th calendar month before the month
	// in which that person acts as pilot in command"
	// Simplified: review is valid for 24 calendar months from the end of the month it was completed.
	expiresOn := time.Date(lastReview.Year(), lastReview.Month()+25, 0, 0, 0, 0, 0, time.UTC) // last day of month + 24
	expiresStr := expiresOn.Format("2006-01-02")

	now := time.Now()
	daysUntilExpiry := int(expiresOn.Sub(now).Hours() / 24)

	result := &FlightReviewStatus{
		LastCompleted: &completedStr,
		ExpiresOn:     &expiresStr,
	}

	if now.After(expiresOn) {
		result.Status = StatusExpired
		result.Message = fmt.Sprintf("Flight review expired — last completed %s (14 CFR 61.56)", completedStr)
	} else if daysUntilExpiry <= 90 {
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("Flight review expires in %d days — last completed %s (14 CFR 61.56)", daysUntilExpiry, completedStr)
	} else {
		result.Status = StatusCurrent
		result.Message = fmt.Sprintf("Flight review current — completed %s, valid until %s (14 CFR 61.56)", completedStr, expiresStr)
	}

	return result
}
