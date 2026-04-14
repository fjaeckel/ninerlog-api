package currency

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// FAAEvaluator implements FAA 14 CFR 61.57 currency rules
type FAAEvaluator struct{}

// NewFAAEvaluator creates a new FAA currency evaluator
func NewFAAEvaluator() *FAAEvaluator {
	return &FAAEvaluator{}
}

func (e *FAAEvaluator) Authority() string {
	return "FAA"
}

func (e *FAAEvaluator) Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dataProvider FlightDataProvider) ClassRatingCurrency {
	result := ClassRatingCurrency{
		ClassRatingID:       rating.ID,
		ClassType:           rating.ClassType,
		LicenseID:           rating.LicenseID,
		RegulatoryAuthority: license.RegulatoryAuthority,
		LicenseType:         license.LicenseType,
		RuleDescription:     faaRuleDescription(rating.ClassType),
		RuleDescriptionKey:  faaRuleDescriptionKey(rating.ClassType),
	}

	if rating.ExpiryDate != nil {
		expStr := rating.ExpiryDate.Format("2006-01-02")
		result.ExpiryDate = &expStr
	}

	// License-type-aware dispatch:
	// - Sport Pilot (§61.315): suppress IR evaluation entirely (day VFR only)
	// - Glider: use launches instead of landings
	lt := strings.ToUpper(license.LicenseType)

	switch rating.ClassType {
	case models.ClassTypeIR:
		// Sport Pilot cannot fly IFR — skip IR evaluation
		if lt == "SPORT" || lt == "RECREATIONAL" {
			result.Status = StatusUnknown
			result.Message = fmt.Sprintf("FAA %s — instrument currency not applicable for %s Pilot (§61.315)", rating.ClassType, license.LicenseType)
			result.RuleDescription = "Instrument privileges not available for Sport/Recreational Pilot certificates"
			return result
		}
		return e.evaluateInstrumentCurrency(ctx, rating, license, dataProvider, result)
	default:
		// Glider uses launches instead of landings
		if lt == "GLIDER" {
			return e.evaluateGliderCurrency(ctx, rating, license, dataProvider, result)
		}
		return e.evaluatePassengerCurrency(ctx, rating, license, dataProvider, result)
	}
}

// evaluatePassengerCurrency evaluates FAA 14 CFR 61.57(a) and 61.57(b):
//
// (a) Day passenger currency: 3 takeoffs and landings in preceding 90 days
// in same category, class, and type (if type rating required).
//
// (b) Night passenger currency: 3 takeoffs and landings to a full stop
// during the period beginning 1 hour after sunset and ending 1 hour
// before sunrise within the preceding 90 days.
func (e *FAAEvaluator) evaluatePassengerCurrency(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	since := time.Now().AddDate(0, 0, -90)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, rating.ClassType, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("FAA %s — unable to evaluate currency", rating.ClassType)
		return result
	}
	result.Progress = progress

	// FAA 61.57(a): 3 T&L in 90 days for day passenger currency
	dayReq := Requirement{
		Name: "Day Passenger Currency", Met: progress.Landings >= 3,
		Current: float64(progress.Landings), Required: 3, Unit: "landings",
		Message: fmt.Sprintf("%d / 3 takeoffs & landings in 90 days", progress.Landings),
	}

	// FAA 61.57(b): 3 full-stop night T&L in 90 days
	nightReq := Requirement{
		Name: "Night Passenger Currency", Met: progress.NightLandings >= 3,
		Current: float64(progress.NightLandings), Required: 3, Unit: "landings",
		Message: fmt.Sprintf("%d / 3 night full-stop landings in 90 days", progress.NightLandings),
	}

	result.Requirements = []Requirement{dayReq, nightReq}

	// Status based on worst case:
	// - Day not met → expired (cannot carry passengers at all)
	// - Day met, night not met → expiring (can fly day only with passengers)
	// - Both met → current
	if !dayReq.Met {
		result.Status = StatusExpired
		needed := 3 - progress.Landings
		result.Message = fmt.Sprintf("FAA %s — not current for passengers (need %d more landing%s)", rating.ClassType, needed, plural(needed))
	} else if !nightReq.Met {
		result.Status = StatusExpiring
		needed := 3 - progress.NightLandings
		result.Message = fmt.Sprintf("FAA %s — day current, night not current (need %d more night landing%s)", rating.ClassType, needed, plural(needed))
	} else {
		result.Status = StatusCurrent
		result.Message = fmt.Sprintf("FAA %s — current for day and night passengers", rating.ClassType)
	}

	return result
}

// evaluateInstrumentCurrency evaluates FAA 14 CFR 61.57(c):
//
// Within the preceding 6 calendar months:
//   - 6 instrument approaches
//   - Holding procedures and tasks
//   - Intercepting and tracking courses through the use of navigational systems
func (e *FAAEvaluator) evaluateInstrumentCurrency(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	// 6 calendar months
	since := time.Now().AddDate(0, -6, 0)

	// IR applies across all aircraft classes
	progress, err := dp.GetProgressAll(ctx, license.UserID, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = "FAA IR — unable to evaluate currency"
		return result
	}
	result.Progress = progress

	// FAA 61.57(c)(1): 6 instrument approaches in 6 months
	reqApproaches := Requirement{
		Name: "Instrument Approaches", Met: progress.Approaches >= 6,
		Current: float64(progress.Approaches), Required: 6, Unit: "approaches",
		Message: fmt.Sprintf("%d / 6 instrument approaches in 6 months", progress.Approaches),
	}

	// FAA 61.57(c)(1): Holding procedures
	reqHolds := Requirement{
		Name: "Holding Procedures", Met: progress.Holds >= 1,
		Current: float64(progress.Holds), Required: 1, Unit: "holds",
		Message: fmt.Sprintf("%d / 1 holding procedure%s in 6 months", progress.Holds, plural(1)),
	}

	result.Requirements = []Requirement{reqApproaches, reqHolds}

	allMet := reqApproaches.Met && reqHolds.Met

	if rating.ExpiryDate != nil && rating.IsExpired() {
		result.Status = StatusExpired
		result.Message = fmt.Sprintf("FAA IR expired on %s", *result.ExpiryDate)
	} else if allMet {
		result.Status = StatusCurrent
		result.Message = "FAA IR — instrument currency requirements met"
	} else {
		// Check 12-month grace period: §61.57(c)/(d)
		// 0-6 months: current (checked above)
		// 6-12 months: can regain by practice with safety pilot
		// >12 months: IPC required
		since12 := time.Now().AddDate(-1, 0, 0)
		progress12, err12 := dp.GetProgressAll(ctx, license.UserID, since12)
		if err12 == nil && (progress12.Approaches >= 6 && progress12.Holds >= 1) {
			// Met within 12 months but not within 6 — lapsed but recoverable
			result.Status = StatusExpiring
			result.Message = "FAA IR — instrument currency lapsed (6+ months), can regain by practice with safety pilot (§61.57(c))"
			result.RuleDescription = "Instrument currency lapsed past 6 months. Can regain within 12 months by completing 6 approaches + holding with safety pilot. After 12 months, IPC required (14 CFR 61.57(c)/(d))"
		} else {
			// Not met within 12 months either — IPC required
			result.Status = StatusExpired
			result.Message = "FAA IR — instrument currency expired (>12 months without required experience), IPC required (§61.57(d))"
			result.RuleDescription = "Instrument currency expired. Instrument Proficiency Check (IPC) required to regain currency (14 CFR 61.57(d))"
		}
	}

	return result
}

// evaluateGliderCurrency evaluates FAA §61.57(a) for glider category.
// Gliders use "launches" instead of "takeoffs" — 3 launches and landings
// in the preceding 90 days for passenger currency.
// Night and IR currency are not applicable for gliders.
func (e *FAAEvaluator) evaluateGliderCurrency(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	since := time.Now().AddDate(0, 0, -90)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, rating.ClassType, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("FAA %s (Glider) — unable to evaluate currency", rating.ClassType)
		return result
	}
	result.Progress = progress
	result.RuleDescription = "Requires 3 launches & landings in preceding 90 days in same category (14 CFR 61.57(a)) — night and IFR not applicable for gliders"

	// Gliders count launches (= landings in our data model, since each launch results in a landing)
	launchReq := Requirement{
		Name: "Launches & Landings", Met: progress.Landings >= 3,
		Current: float64(progress.Landings), Required: 3, Unit: "launches",
		Message: fmt.Sprintf("%d / 3 launches & landings in 90 days", progress.Landings),
	}

	result.Requirements = []Requirement{launchReq}

	if !launchReq.Met {
		result.Status = StatusExpired
		needed := 3 - progress.Landings
		result.Message = fmt.Sprintf("FAA Glider — not current (need %d more launch%s)", needed, plural(needed))
	} else {
		result.Status = StatusCurrent
		result.Message = "FAA Glider — current for passengers"
	}

	return result
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

// faaRuleDescription returns a human-readable description of FAA currency rules for a class type
func faaRuleDescription(classType models.ClassType) string {
	switch classType {
	case models.ClassTypeIR:
		return "Requires 6 instrument approaches + holding procedures within preceding 6 calendar months (14 CFR 61.57(c))"
	default:
		return "Requires 3 takeoffs & landings in preceding 90 days in same category/class for day passenger currency; 3 full-stop night takeoffs & landings in 90 days for night currency (14 CFR 61.57)"
	}
}

func faaRuleDescriptionKey(classType models.ClassType) string {
	switch classType {
	case models.ClassTypeIR:
		return "faa_ir"
	default:
		return "faa_pax_day_night"
	}
}

// EvaluatePassengerCurrency evaluates FAA §61.57(a)/(b) as Tier 2 passenger currency.
// This is separate from rating currency — FAA certificates don't expire, so passenger
// currency IS the primary rolling metric for non-IR class ratings.
func (e *FAAEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, dp FlightDataProvider) PassengerCurrency {
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
