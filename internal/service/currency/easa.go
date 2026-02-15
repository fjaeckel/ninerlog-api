package currency

import (
	"context"
	"fmt"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
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
	}

	if rating.ExpiryDate != nil {
		expStr := rating.ExpiryDate.Format("2006-01-02")
		result.ExpiryDate = &expStr
	}

	switch rating.ClassType {
	case models.ClassTypeSEPLand, models.ClassTypeSEPSea, models.ClassTypeTMG:
		return e.evaluateSEPTMG(ctx, rating, license, dataProvider, result)
	case models.ClassTypeMEPLand, models.ClassTypeMEPSea, models.ClassTypeSETLand, models.ClassTypeSETSea:
		return e.evaluateMEPSET(ctx, rating, license, dataProvider, result)
	case models.ClassTypeIR:
		return e.evaluateIR(ctx, rating, license, dataProvider, result)
	default:
		return e.evaluateExpiryOnly(rating, result)
	}
}

// evaluateSEPTMG evaluates EASA FCL.740.A(a) revalidation requirements for SEP/TMG:
//   - 12 hours of flight time in class
//   - 6 hours as PIC in class
//   - 12 takeoffs and 12 landings
//   - 1 hour refresher training with instructor (dual received)
//
// All within 24 months preceding the expiry date.
func (e *EASAEvaluator) evaluateSEPTMG(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	if rating.ExpiryDate == nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — no expiry date set", rating.ClassType)
		return result
	}

	// Look back 24 months from expiry date
	since := rating.ExpiryDate.AddDate(-2, 0, 0)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, rating.ClassType, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — unable to evaluate currency", rating.ClassType)
		return result
	}
	result.Progress = progress

	// Build requirements
	reqTotalHours := Requirement{
		Name: "Total Hours", Met: progress.TotalHours >= 12,
		Current: progress.TotalHours, Required: 12, Unit: "hours",
		Message: fmt.Sprintf("%.1f / 12.0 hours in class", progress.TotalHours),
	}
	reqPICHours := Requirement{
		Name: "PIC Hours", Met: progress.PICHours >= 6,
		Current: progress.PICHours, Required: 6, Unit: "hours",
		Message: fmt.Sprintf("%.1f / 6.0 PIC hours", progress.PICHours),
	}
	reqLandings := Requirement{
		Name: "Takeoffs & Landings", Met: progress.Landings >= 12,
		Current: float64(progress.Landings), Required: 12, Unit: "landings",
		Message: fmt.Sprintf("%d / 12 takeoffs & landings", progress.Landings),
	}
	reqInstructor := Requirement{
		Name: "Refresher Training", Met: progress.InstructorHours >= 1,
		Current: progress.InstructorHours, Required: 1, Unit: "hours",
		Message: fmt.Sprintf("%.1f / 1.0 hours with instructor", progress.InstructorHours),
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

// evaluateMEPSET evaluates EASA FCL.740.A(b) revalidation requirements for MEP/SET:
//   - Proficiency check (manual tracking), OR:
//   - 10 route sectors (flights) in class
//   - 1 hour refresher training with instructor
//
// In the last 3 months of the validity period.
func (e *EASAEvaluator) evaluateMEPSET(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider, result ClassRatingCurrency) ClassRatingCurrency {
	if rating.ExpiryDate == nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("EASA %s — no expiry date set", rating.ClassType)
		return result
	}

	// Look back 3 months from expiry date
	since := rating.ExpiryDate.AddDate(0, -3, 0)

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
		Name: "Refresher Training", Met: progress.InstructorHours >= 1,
		Current: progress.InstructorHours, Required: 1, Unit: "hours",
		Message: fmt.Sprintf("%.1f / 1.0 hours with instructor", progress.InstructorHours),
	}

	result.Requirements = []Requirement{reqSectors, reqInstructor}

	allMet := reqSectors.Met && reqInstructor.Met

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
		Name: "IFR Hours", Met: progress.IFRHours >= 10,
		Current: progress.IFRHours, Required: 10, Unit: "hours",
		Message: fmt.Sprintf("%.1f / 10.0 IFR hours", progress.IFRHours),
	}

	result.Requirements = []Requirement{reqIFRHours}

	if rating.IsExpired() {
		result.Status = StatusExpired
		result.Message = fmt.Sprintf("EASA IR expired on %s", *result.ExpiryDate)
	} else if !reqIFRHours.Met {
		result.Status = StatusExpiring
		result.Message = "EASA IR — IFR hour requirement not met"
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
