package currency

import (
	"context"
	"fmt"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
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
	}

	if rating.ExpiryDate != nil {
		expStr := rating.ExpiryDate.Format("2006-01-02")
		result.ExpiryDate = &expStr
	}

	switch rating.ClassType {
	case models.ClassTypeIR:
		return e.evaluateInstrumentCurrency(ctx, rating, license, dataProvider, result)
	default:
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
//
// Until approach tracking is added (Week 3), we track IFR hours as a proxy.
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

	// IFR hours as a proxy metric until approach tracking is implemented
	reqIFRHours := Requirement{
		Name: "IFR Flight Time", Met: progress.IFRHours >= 6,
		Current: progress.IFRHours, Required: 6, Unit: "hours",
		Message: fmt.Sprintf("%.1f / 6.0 IFR hours in 6 months", progress.IFRHours),
	}

	result.Requirements = []Requirement{reqIFRHours}

	if rating.ExpiryDate != nil && rating.IsExpired() {
		result.Status = StatusExpired
		result.Message = fmt.Sprintf("FAA IR expired on %s", *result.ExpiryDate)
	} else if !reqIFRHours.Met {
		result.Status = StatusExpiring
		result.Message = "FAA IR — instrument currency requirements not fully met"
	} else {
		result.Status = StatusCurrent
		result.Message = "FAA IR — instrument currency requirements met"
	}

	return result
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
