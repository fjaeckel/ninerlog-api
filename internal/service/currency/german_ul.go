package currency

import (
	"context"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// GermanULEvaluator implements German ultralight currency rules per LuftPersV §45.
// German UL is NOT regulated by EASA Part-FCL — it uses national rules.
//
// Requirements (rolling 24 months from now):
//   - 12 hours flight time
//   - 12 takeoffs & landings
//   - 1 hour dual instruction with flight instructor
//
// No night flying privilege for UL.
type GermanULEvaluator struct{}

func NewGermanULEvaluator() *GermanULEvaluator {
	return &GermanULEvaluator{}
}

// Authority returns empty — this evaluator is registered for multiple authorities via RegisterMulti.
func (e *GermanULEvaluator) Authority() string {
	return "LBA"
}

// Authorities returns all German UL authorities this evaluator handles.
func (e *GermanULEvaluator) Authorities() []string {
	return []string{"LBA", "DULV", "DAeC", "DAEC"}
}

func (e *GermanULEvaluator) Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider) ClassRatingCurrency {
	result := ClassRatingCurrency{
		ClassRatingID:       rating.ID,
		ClassType:           rating.ClassType,
		LicenseID:           rating.LicenseID,
		RegulatoryAuthority: license.RegulatoryAuthority,
		LicenseType:         license.LicenseType,
		RuleDescription:     "Erfordert 12h Flugzeit + 12 Starts & Landungen + 1h Übungsflug mit Fluglehrer in 24 Monaten (LuftPersV §45)",
	}

	// Rolling 24 months from now (same pattern as EASA LAPL)
	since := time.Now().AddDate(-2, 0, 0)

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, rating.ClassType, since)
	if err != nil {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("UL %s — Flugerfahrung konnte nicht ermittelt werden", rating.ClassType)
		return result
	}
	result.Progress = progress

	reqTotalHours := Requirement{
		Name: "Flugzeit", Met: progress.TotalHours >= 12,
		Current: progress.TotalHours, Required: 12, Unit: "hours",
		Message: fmt.Sprintf("%.1f / 12.0 Stunden Flugzeit", progress.TotalHours),
	}
	reqLandings := Requirement{
		Name: "Starts & Landungen", Met: progress.Landings >= 12,
		Current: float64(progress.Landings), Required: 12, Unit: "landings",
		Message: fmt.Sprintf("%d / 12 Starts & Landungen", progress.Landings),
	}
	reqInstructor := Requirement{
		Name: "Übungsflug mit Fluglehrer", Met: progress.InstructorHours >= 1,
		Current: progress.InstructorHours, Required: 1, Unit: "hours",
		Message: fmt.Sprintf("%.1f / 1.0 Stunden mit Fluglehrer", progress.InstructorHours),
	}

	result.Requirements = []Requirement{reqTotalHours, reqLandings, reqInstructor}

	allMet := reqTotalHours.Met && reqLandings.Met && reqInstructor.Met

	if !allMet {
		result.Status = StatusExpiring
		result.Message = fmt.Sprintf("UL %s — Flugerfahrungsanforderungen nicht vollständig erfüllt (LuftPersV §45)", rating.ClassType)
	} else {
		result.Status = StatusCurrent
		result.Message = fmt.Sprintf("UL %s — alle Anforderungen erfüllt (LuftPersV §45)", rating.ClassType)
	}

	return result
}

// EvaluatePassengerCurrency evaluates German UL passenger-carrying recency.
// Note: UL Passagierberechtigung requires separate endorsement training that
// cannot be auto-evaluated. This returns an informational-only assessment.
func (e *GermanULEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, dp FlightDataProvider) PassengerCurrency {
	since := time.Now().AddDate(0, 0, -90)

	result := PassengerCurrency{
		ClassType:           classType,
		RegulatoryAuthority: license.RegulatoryAuthority,
		DayRequired:         3,
		NightRequired:       0,
		NightPrivilege:      false, // UL — no night flying
		RuleDescription:     "Passagierberechtigung erforderlich — 3 Starts & Landungen in 90 Tagen (Passagierflugberechtigung nach LuftPersV)",
	}

	progress, err := dp.GetProgressByAircraftClass(ctx, license.UserID, classType, since)
	if err != nil {
		result.DayStatus = StatusUnknown
		result.NightStatus = StatusUnknown
		result.Message = fmt.Sprintf("UL %s — Passagier-Flugerfahrung konnte nicht ermittelt werden", classType)
		return result
	}

	result.DayLandings = progress.Landings
	result.NightLandings = 0
	result.NightStatus = StatusUnknown // Night not applicable

	if progress.Landings >= 3 {
		result.DayStatus = StatusCurrent
		result.Message = fmt.Sprintf("UL %s — Passagierberechtigung: Flugerfahrung erfüllt (Passagierberechtigung muss separat nachgewiesen werden)", classType)
	} else {
		result.DayStatus = StatusExpired
		needed := 3 - progress.Landings
		result.Message = fmt.Sprintf("UL %s — %d weitere Starts & Landungen erforderlich für Passagierflüge", classType, needed)
	}

	return result
}
