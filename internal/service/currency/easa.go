package currency

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// EASAEvaluator implements EASA FCL currency rules per class type
type EASAEvaluator struct{}

// NewEASAEvaluator creates a new EASA currency evaluator
func NewEASAEvaluator() *EASAEvaluator {
	return &EASAEvaluator{}
}

func (e *EASAEvaluator) Authority() string {
	return "EASA"
}

func (e *EASAEvaluator) Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dataProvider FlightDataProvider) ClassRatingCurrency {
	result := ClassRatingCurrency{
		ClassRatingID:       rating.ID,
		ClassType:           rating.ClassType,
		LicenseID:           rating.LicenseID,
		RegulatoryAuthority: license.RegulatoryAuthority,
		LicenseType:         license.LicenseType,
		RuleDescription:     easaRuleDescription(rating.ClassType),
	}

	if rating.ExpiryDate != nil {
		expStr := rating.ExpiryDate.Format("2006-01-02")
		result.ExpiryDate = &expStr
	}

	// License-type-aware dispatch: LAPL/SPL use different regulations than PPL/CPL/ATPL
	lt := strings.ToUpper(license.LicenseType)

	// IR is always FCL.625.A regardless of license type
	if rating.ClassType == models.ClassTypeIR {
		return e.evaluateIR(ctx, rating, license, dataProvider, result)
	}

	// LAPL uses FCL.140.A (rolling 24 months from now, no PIC requirement)
	if lt == "LAPL" || lt == "LAPL(A)" {
		result.RuleDescription = "Requires 12h flight time + 12 takeoffs & landings + 1h training flight with instructor within the last 24 months (EASA FCL.140.A)"
		return e.evaluateLAPL_A(ctx, rating, license, dataProvider, result)
	}

	// SPL/LAPL(S) uses FCL.140.S (rolling 24 months, launches not landings)
	if lt == "SPL" || lt == "LAPL(S)" {
		if rating.ClassType == models.ClassTypeTMG {
			result.RuleDescription = "Requires 12h flight time + 12 takeoffs & landings on TMG within the last 24 months (EASA FCL.140.S(b)(2))"
			return e.evaluateSPL_TMG(ctx, rating, license, dataProvider, result)
		}
		result.RuleDescription = "Requires 5h PIC flight time + 15 launches + 2 training flights with instructor within the last 24 months (EASA FCL.140.S)"
		return e.evaluateSPL(ctx, rating, license, dataProvider, result)
	}

	// PPL/CPL/ATPL use FCL.740.A (from expiry date)
	switch rating.ClassType {
	case models.ClassTypeSEPLand, models.ClassTypeSEPSea, models.ClassTypeTMG:
		return e.evaluateSEPTMG(ctx, rating, license, dataProvider, result)
	case models.ClassTypeMEPLand, models.ClassTypeMEPSea, models.ClassTypeSETLand, models.ClassTypeSETSea:
		return e.evaluateMEPSET(ctx, rating, license, dataProvider, result)
	default:
		return e.evaluateExpiryOnly(rating, result)
	}
}

// evaluateSEPTMG evaluates EASA FCL.740.A(b)(1) revalidation requirements for SEP/TMG:
//   - 12 hours of flight time in class
//   - 6 hours as PIC in class
//   - 12 takeoffs and 12 landings
//   - 1 hour refresher training with instructor (dual received)
//
// All within the 12 months preceding the expiry date of the rating.
// Note: the rating validity period is 24 months, but the experience must be
// accumulated in the LAST 12 months — not the full 24. Per FCL.740.A(b)(1):
// "within the 12 months preceding the expiry date of the rating".
func (e *EASAEvaluator) evaluateSEPTMG(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	if rating.ExpiryDate == nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — no expiry date set", rating.ClassType)
		return result
	}

	// Look back 12 months from expiry date — FCL.740.A(b)(1)
	since := rating.ExpiryDate.AddDate(-1, 0, 0)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, rating.ClassType, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — unable to evaluate currency", rating.ClassType)
		return result
	}
	result.Progress = progress

	// Build requirements (thresholds in minutes: 12h=720m, 6h=360m, 1h=60m)
	reqTotalHours := Requirement{
		Name: "Total Time", Met: progress.TotalMinutes >= 720,
		Current: float64(progress.TotalMinutes), Required: 720, Unit: "minutes",
		Message: fmt.Sprintf("%d / 720 minutes in class", progress.TotalMinutes),
	}
	reqPICHours := Requirement{
		Name: "PIC Time", Met: progress.PICMinutes >= 360,
		Current: float64(progress.PICMinutes), Required: 360, Unit: "minutes",
		Message: fmt.Sprintf("%d / 360 PIC minutes", progress.PICMinutes),
	}
	reqLandings := Requirement{
		Name: "Takeoffs & Landings", Met: progress.Landings >= 12,
		Current: float64(progress.Landings), Required: 12, Unit: "landings",
		Message: fmt.Sprintf("%d / 12 takeoffs & landings", progress.Landings),
	}
	reqInstructor := Requirement{
		Name: "Refresher Training", Met: progress.InstructorMinutes >= 60,
		Current: float64(progress.InstructorMinutes), Required: 60, Unit: "minutes",
		Message: fmt.Sprintf("%d / 60 minutes with instructor", progress.InstructorMinutes),
	}

	result.Requirements = []Requirement{reqTotalHours, reqPICHours, reqLandings, reqInstructor}

	allMet := reqTotalHours.Met && reqPICHours.Met && reqLandings.Met && reqInstructor.Met

	if rating.IsExpired() {
		result.Status = StatusExpired
		result.Message = fmt.Sprintf("EASA %s expired on %s", rating.ClassType, *result.ExpiryDate)
	} else if !allMet {
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("EASA %s — revalidation requirements not fully met", rating.ClassType)
	} else if rating.IsExpiringSoon(90) {
		daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("EASA %s expires in %d days — requirements met", rating.ClassType, daysLeft)
	} else {
		result.Status = StatusCurrent
		result.Message = fmt.Sprintf("EASA %s current — all revalidation requirements met", rating.ClassType)
	}

	return result
}

// evaluateMEPSET evaluates EASA FCL.740.A(b)(2) revalidation requirements for MEP/SET:
//   - Proficiency check (manual tracking), OR:
//   - 10 route sectors (flights) in class
//   - 1 hour refresher training with instructor
//
// Experience must be within the 12 months preceding the expiry date — FCL.740.A(b)(2).
// Note: the "3 months" in FCL.740.A(d) refers to when revalidation can occur without
// losing validity overlap, NOT the experience accumulation window.
func (e *EASAEvaluator) evaluateMEPSET(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	if rating.ExpiryDate == nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — no expiry date set", rating.ClassType)
		return result
	}

	// Look back 12 months from expiry date — FCL.740.A(b)(2)
	since := rating.ExpiryDate.AddDate(-1, 0, 0)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, rating.ClassType, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — unable to evaluate currency", rating.ClassType)
		return result
	}
	result.Progress = progress

	reqSectors := Requirement{
		Name: "Route Sectors", Met: progress.Flights >= 10,
		Current: float64(progress.Flights), Required: 10, Unit: "flights",
		Message: fmt.Sprintf("%d / 10 route sectors", progress.Flights),
	}
	reqInstructor := Requirement{
		Name: "Refresher Training", Met: progress.InstructorMinutes >= 60,
		Current: float64(progress.InstructorMinutes), Required: 60, Unit: "minutes",
		Message: fmt.Sprintf("%d / 60 minutes with instructor", progress.InstructorMinutes),
	}

	// Check for proficiency check — if found, it satisfies the revalidation (alternative path)
	profCheckDate, _ := dp.GetLastProficiencyCheck(ctx, license.UserID, rating.ClassType, since)
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

	result.Requirements = []Requirement{reqSectors, reqInstructor, reqProfCheck}

	// MEP/SET can be revalidated by EITHER experience (sectors+instructor) OR proficiency check
	allMetByExperience := reqSectors.Met && reqInstructor.Met
	allMet := allMetByExperience || hasProfCheck

	if rating.IsExpired() {
		result.Status = StatusExpired
		result.Message = fmt.Sprintf("EASA %s expired on %s", rating.ClassType, *result.ExpiryDate)
	} else if !allMet {
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("EASA %s — revalidation requirements not fully met (proficiency check may apply)", rating.ClassType)
	} else if rating.IsExpiringSoon(90) {
		daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("EASA %s expires in %d days — requirements met", rating.ClassType, daysLeft)
	} else {
		result.Status = StatusCurrent
		result.Message = fmt.Sprintf("EASA %s current — all revalidation requirements met", rating.ClassType)
	}

	return result
}

// evaluateIR evaluates EASA FCL.625.A instrument rating currency:
//   - 10 hours IFR flight time in 12 months preceding expiry
//   - Proficiency check (manual tracking)
func (e *EASAEvaluator) evaluateIR(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	if rating.ExpiryDate == nil {
		result.Status = StatusUnknown
		result.Message = "EASA IR — no expiry date set"
		return result
	}

	// Look back 12 months from expiry date; use all aircraft (IR is cross-class)
	since := rating.ExpiryDate.AddDate(-1, 0, 0)

	progress, err := dp.GetProgressAll(ctx, license.UserID, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = "EASA IR — unable to evaluate currency"
		return result
	}
	result.Progress = progress

	reqIFRHours := Requirement{
		Name: "IFR Time", Met: progress.IFRMinutes >= 600,
		Current: float64(progress.IFRMinutes), Required: 600, Unit: "minutes",
		Message: fmt.Sprintf("%d / 600 IFR minutes", progress.IFRMinutes),
	}

	// Check for annual proficiency check (FCL.625.A requires one)
	profCheckDate, _ := dp.GetLastProficiencyCheck(ctx, license.UserID, models.ClassTypeIR, since)
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

	result.Requirements = []Requirement{reqIFRHours, reqProfCheck}

	allMet := reqIFRHours.Met && hasProfCheck

	if rating.IsExpired() {
		result.Status = StatusExpired
		result.Message = fmt.Sprintf("EASA IR expired on %s", *result.ExpiryDate)
	} else if !allMet {
		result.Status = StatusExpiring
		if !reqIFRHours.Met && !hasProfCheck {
			result.Message = "EASA IR — IFR hours and proficiency check not met"
		} else if !reqIFRHours.Met {
			result.Message = "EASA IR — IFR hour requirement not met"
		} else {
			result.Message = "EASA IR — annual proficiency check not completed"
		}
	} else if rating.IsExpiringSoon(90) {
		daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("EASA IR expires in %d days — requirements met", daysLeft)
	} else {
		result.Status = StatusCurrent
		result.Message = "EASA IR current — all requirements met"
	}

	return result
}

// evaluateExpiryOnly falls back to expiry-only tracking for unknown class types
func (e *EASAEvaluator) evaluateExpiryOnly(rating *models.ClassRating, result ClassRatingCurrency) ClassRatingCurrency {
	if rating.ExpiryDate == nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — no expiry date set", rating.ClassType)
		return result
	}

	if rating.IsExpired() {
		result.Status = StatusExpired
		result.Message = fmt.Sprintf("EASA %s expired on %s", rating.ClassType, *result.ExpiryDate)
	} else if rating.IsExpiringSoon(90) {
		daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("EASA %s expires in %d days", rating.ClassType, daysLeft)
	} else {
		result.Status = StatusCurrent
		result.Message = fmt.Sprintf("EASA %s valid until %s", rating.ClassType, *result.ExpiryDate)
	}

	return result
}

// evaluateLAPL_A evaluates EASA FCL.140.A(a) recency for LAPL(A):
//   - 12 hours flight time (as PIC, dual, or solo under supervision)
//   - 12 takeoffs & landings
//   - 1 hour dual instruction
//   - NO PIC hour requirement (key difference from FCL.740.A)
//
// Lookback: rolling 24 months from NOW (not from any expiry date).
func (e *EASAEvaluator) evaluateLAPL_A(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	// Rolling 24 months from now — FCL.140.A is a recency requirement, not a revalidation
	since := time.Now().AddDate(-2, 0, 0)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, rating.ClassType, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA LAPL %s — unable to evaluate recency", rating.ClassType)
		return result
	}
	result.Progress = progress

	reqTotalHours := Requirement{
		Name: "Flight Time", Met: progress.TotalMinutes >= 720,
		Current: float64(progress.TotalMinutes), Required: 720, Unit: "minutes",
		Message: fmt.Sprintf("%d / 720 minutes in last 24 months", progress.TotalMinutes),
	}
	reqLandings := Requirement{
		Name: "Takeoffs & Landings", Met: progress.Landings >= 12,
		Current: float64(progress.Landings), Required: 12, Unit: "landings",
		Message: fmt.Sprintf("%d / 12 takeoffs & landings in last 24 months", progress.Landings),
	}
	reqInstructor := Requirement{
		Name: "Training Flight", Met: progress.InstructorMinutes >= 60,
		Current: float64(progress.InstructorMinutes), Required: 60, Unit: "minutes",
		Message: fmt.Sprintf("%d / 60 minutes with instructor in last 24 months", progress.InstructorMinutes),
	}

	result.Requirements = []Requirement{reqTotalHours, reqLandings, reqInstructor}

	allMet := reqTotalHours.Met && reqLandings.Met && reqInstructor.Met

	if !allMet {
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("EASA LAPL %s — recency requirements not fully met (FCL.140.A)", rating.ClassType)
	} else {
		result.Status = StatusCurrent
		result.Message = fmt.Sprintf("EASA LAPL %s — current (FCL.140.A)", rating.ClassType)
	}

	return result
}

// evaluateSPL evaluates EASA FCL.140.S(a) recency for SPL/LAPL(S):
//   - 5 hours flight time as PIC on sailplanes
//   - 15 launches (NOT landings)
//   - 2 training flights with instructor
//
// Lookback: rolling 24 months from NOW.
// Also evaluates launch method currency per FCL.140.S(b)(1).
func (e *EASAEvaluator) evaluateSPL(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	since := time.Now().AddDate(-2, 0, 0)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, rating.ClassType, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA SPL %s — unable to evaluate recency", rating.ClassType)
		return result
	}
	result.Progress = progress

	// SPL uses landings as a proxy for launches (each launch results in a landing)
	reqPICHours := Requirement{
		Name: "PIC Flight Time", Met: progress.PICMinutes >= 300,
		Current: float64(progress.PICMinutes), Required: 300, Unit: "minutes",
		Message: fmt.Sprintf("%d / 300 PIC minutes in last 24 months", progress.PICMinutes),
	}
	reqLaunches := Requirement{
		Name: "Launches", Met: progress.Landings >= 15,
		Current: float64(progress.Landings), Required: 15, Unit: "launches",
		Message: fmt.Sprintf("%d / 15 launches in last 24 months", progress.Landings),
	}
	reqTraining := Requirement{
		Name: "Training Flights", Met: progress.InstructorMinutes >= 60,
		Current: float64(progress.InstructorMinutes), Required: 60, Unit: "minutes",
		Message: fmt.Sprintf("%d / 60 minutes training flights in last 24 months", progress.InstructorMinutes),
	}

	result.Requirements = []Requirement{reqPICHours, reqLaunches, reqTraining}

	allMet := reqPICHours.Met && reqLaunches.Met && reqTraining.Met

	// Evaluate launch method currency — FCL.140.S(b)(1)
	launchCounts, _ := dp.GetLaunchCounts(ctx, license.UserID, since)
	var launchMethodCurrency []LaunchMethodCurrency
	for _, method := range []string{"winch", "aerotow", "self-launch"} {
		count := launchCounts[method]
		if count > 0 || launchCounts[method] > 0 {
			lmc := LaunchMethodCurrency{
				Method:   method,
				Launches: count,
				Required: 5,
				Met:      count >= 5,
				Message:  fmt.Sprintf("%d / 5 %s launches in last 24 months", count, method),
			}
			launchMethodCurrency = append(launchMethodCurrency, lmc)
		}
	}
	result.LaunchMethodCurrency = launchMethodCurrency

	if !allMet {
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("EASA SPL %s — recency requirements not fully met (FCL.140.S)", rating.ClassType)
	} else {
		result.Status = StatusCurrent
		result.Message = fmt.Sprintf("EASA SPL %s — current (FCL.140.S)", rating.ClassType)
	}

	return result
}

// evaluateSPL_TMG evaluates EASA FCL.140.S(b)(2) TMG extension for SPL:
//   - 12 hours flight time on TMG
//   - 12 takeoffs & landings on TMG
//
// Lookback: rolling 24 months from NOW.
// Distinct from PPL TMG (FCL.740.A: requires 6h PIC + 1h instructor + expiry-based).
func (e *EASAEvaluator) evaluateSPL_TMG(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	since := time.Now().AddDate(-2, 0, 0)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, models.ClassTypeTMG, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = "EASA SPL TMG — unable to evaluate recency"
		return result
	}
	result.Progress = progress

	reqTotalHours := Requirement{
		Name: "TMG Flight Time", Met: progress.TotalMinutes >= 720,
		Current: float64(progress.TotalMinutes), Required: 720, Unit: "minutes",
		Message: fmt.Sprintf("%d / 720 minutes on TMG in last 24 months", progress.TotalMinutes),
	}
	reqLandings := Requirement{
		Name: "TMG Takeoffs & Landings", Met: progress.Landings >= 12,
		Current: float64(progress.Landings), Required: 12, Unit: "landings",
		Message: fmt.Sprintf("%d / 12 takeoffs & landings on TMG in last 24 months", progress.Landings),
	}

	result.Requirements = []Requirement{reqTotalHours, reqLandings}

	allMet := reqTotalHours.Met && reqLandings.Met

	if !allMet {
		result.Status = StatusExpiring
		result.Message = "EASA SPL TMG — recency requirements not fully met (FCL.140.S(b)(2))"
	} else {
		result.Status = StatusCurrent
		result.Message = "EASA SPL TMG — current (FCL.140.S(b)(2))"
	}

	return result
}

// easaRuleDescription returns a human-readable description of EASA currency rules for a class type
func easaRuleDescription(classType models.ClassType) string {
	switch classType {
	case models.ClassTypeSEPLand, models.ClassTypeSEPSea, models.ClassTypeTMG:
		return "Requires 12h total flight time + 6h as PIC + 12 takeoffs & landings + 1h refresher training with instructor, all within the 12 months preceding the expiry date (EASA FCL.740.A(b)(1))"
	case models.ClassTypeMEPLand, models.ClassTypeMEPSea, models.ClassTypeSETLand, models.ClassTypeSETSea:
		return "Requires proficiency check, or 10 route sectors + 1h refresher training with instructor within the 12 months preceding the expiry date (EASA FCL.740.A(b)(2))"
	case models.ClassTypeIR:
		return "Requires 10h IFR flight time within 12 months before expiry, plus annual proficiency check (EASA FCL.625.A)"
	default:
		return "EASA class rating — currency tracked by expiry date"
	}
}

// EvaluatePassengerCurrency evaluates EASA FCL.060(b) passenger-carrying recency.
// This is SEPARATE from rating revalidation — a pilot can have a valid rating but
// cannot carry passengers without meeting FCL.060(b).
//
// FCL.060(b)(1): 3 takeoffs, approaches and landings in same type or class
// within the preceding 90 days (rolling from now).
func (e *EASAEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, dp FlightDataProvider) PassengerCurrency {
	since := time.Now().AddDate(0, 0, -90)

	result := PassengerCurrency{
		ClassType:           classType,
		RegulatoryAuthority: "EASA",
		DayRequired:         3,
		NightRequired:       3,
		NightPrivilege:      HasNightPrivilege(license.LicenseType, license.RegulatoryAuthority),
		RuleDescription:     "3 takeoffs & landings in same type/class within preceding 90 days to carry passengers (EASA FCL.060(b))",
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

	// Day passenger currency
	if progress.Landings >= 3 {
		result.DayStatus = StatusCurrent
	} else {
		result.DayStatus = StatusExpired
	}

	// Night passenger currency
	if progress.NightLandings >= 3 {
		result.NightStatus = StatusCurrent
	} else {
		result.NightStatus = StatusExpired
	}

	// Summary message
	if result.DayStatus == StatusCurrent && result.NightStatus == StatusCurrent {
		result.Message = fmt.Sprintf("EASA %s — passenger current (day and night)", classType)
	} else if result.DayStatus == StatusCurrent {
		result.Message = fmt.Sprintf("EASA %s — day passenger current, night not current", classType)
	} else {
		result.Message = fmt.Sprintf("EASA %s — not current for passengers (need %d more landing(s) in 90 days)", classType, 3-progress.Landings)
	}

	return result
}
