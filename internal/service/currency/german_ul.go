package currency

import (
	"context"
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

// Authority returns the primary authority — this evaluator is registered for
// multiple authorities via RegisterMulti / Authorities().
func (e *GermanULEvaluator) Authority() string {
	return "LBA"
}

// Authorities returns all German UL authorities this evaluator handles.
func (e *GermanULEvaluator) Authorities() []string {
	return []string{"LBA", "DULV", "DAeC", "DAEC"}
}

func (e *GermanULEvaluator) Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dp FlightDataProvider) ClassRatingCurrency {
	return evalRatingRule(ctx, &germanULRule, rating, license, dp)
}

// germanULRule — German ultralight recency (LuftPersV §45), rolling 24 months.
var germanULRule = ratingRule{
	displayKey:  "ul_luftpersv",
	description: "Erfordert 12h Flugzeit + 12 Starts & Landungen + 1h Übungsflug mit Fluglehrer in 24 Monaten (LuftPersV §45)",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeByClass,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 12, unit: "landings"},
		{nameKey: ReqKeyRefresherTraining, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
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

// EvaluatePassengerCurrency evaluates German UL passenger-carrying recency.
// The assessment is informational only.
func (e *GermanULEvaluator) EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, _ []*models.ClassRating, dp FlightDataProvider) PassengerCurrency {
	since := paxWindowStart(time.Now())

	result := PassengerCurrency{
		ClassType:           classType,
		RegulatoryAuthority: license.RegulatoryAuthority,
		DayRequired:         3,
		NightRequired:       0,
		NightPrivilege:      false, // UL — no night flying
		RuleDescription:     "Passagierberechtigung erforderlich — 3 Starts & Landungen in 90 Tagen (Passagierflugberechtigung nach LuftPersV)",
		RuleDescriptionKey:  "ul_pax",
	}

	days, err := dp.GetLandingDaysByAircraftClass(ctx, license.UserID, classType, since)
	if err != nil {
		result.DayStatus = StatusUnknown
		result.NightStatus = StatusUnknown
		result.setMsg(MsgPaxEvaluationFailed, nil)
		return result
	}

	landings, _ := paxTotals(days)
	result.DayLandings = landings
	result.NightLandings = 0
	result.NightStatus = StatusUnknown // Night not applicable
	result.DayExpiresOn = paxExpiryString(paxExpiryDate(days, result.DayRequired, allLandings))

	if landings >= 3 {
		result.DayStatus = StatusCurrent
		result.setMsg(MsgPaxCurrentPrivilegeSeparat, nil)
	} else {
		result.DayStatus = StatusExpired
		needed := 3 - landings
		result.setMsg(MsgPaxNotCurrent, msgNeeded(needed))
	}

	return result
}
