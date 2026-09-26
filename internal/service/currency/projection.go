package currency

import (
	"context"
	"math"
	"time"
)

// projectValidUntil sets ValidUntil on the met requirements and met launch
// methods of a rolling-window result and, when current, on the result: the
// last date each stays met with no further flying. eval re-evaluates the
// rule at the instant its context carries.
func projectValidUntil(ctx context.Context, w windowSpec, result *ClassRatingCurrency, cache *dailyCache, eval func(context.Context) ClassRatingCurrency) {
	if w.kind != windowRollingNow || (w.years == 0 && w.months == 0 && w.days == 0) {
		return
	}
	reqLast := make([]*time.Time, len(result.Requirements))
	lmLast := make([]*time.Time, len(result.LaunchMethodCurrency))
	var ratingLast *time.Time
	for _, day := range cache.lastDaysCounted(w, midnightUTC(nowFrom(ctx))) {
		p := eval(withNow(ctx, dateAt(day)))
		stillMet := false
		for i, r := range result.Requirements {
			if r.Met && i < len(p.Requirements) && p.Requirements[i].NameKey == r.NameKey && p.Requirements[i].Met {
				reqLast[i] = &day
				stillMet = true
			}
		}
		for i, lm := range result.LaunchMethodCurrency {
			if lm.Met && i < len(p.LaunchMethodCurrency) && p.LaunchMethodCurrency[i].Method == lm.Method && p.LaunchMethodCurrency[i].Met {
				lmLast[i] = &day
				stillMet = true
			}
		}
		if result.Status == StatusCurrent && p.Status == StatusCurrent {
			ratingLast = &day
			stillMet = true
		}
		if !stillMet {
			break
		}
	}
	for i, d := range reqLast {
		result.Requirements[i].ValidUntil = dateString(d)
	}
	for i, d := range lmLast {
		result.LaunchMethodCurrency[i].ValidUntil = dateString(d)
	}
	result.ValidUntil = dateString(ratingLast)
}

// dateString formats a date as YYYY-MM-DD, nil for nil.
func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// annotateRemedies sets RemedyKey and RemedyParams on the unmet requirements
// and launch methods of result.
func annotateRemedies(result *ClassRatingCurrency) {
	for i := range result.Requirements {
		r := &result.Requirements[i]
		if r.Met || r.NameKey == "" {
			continue
		}
		r.RemedyKey, r.RemedyParams = requirementRemedy(*r)
	}
	for i := range result.LaunchMethodCurrency {
		lm := &result.LaunchMethodCurrency[i]
		if lm.Met {
			continue
		}
		missing := lm.Required - lm.Launches
		method := lm.Method
		lm.RemedyKey = RemedyLaunchMethodDual
		lm.RemedyParams = &MessageParams{Method: &method, Missing: &missing}
	}
}

// requirementRemedy returns the remedy key and params for an unmet
// regulatory requirement, "" when none applies.
func requirementRemedy(r Requirement) (string, *MessageParams) {
	switch {
	case r.Unit == "check":
		return RemedyProficiencyCheck, nil
	case r.Unit == "review":
		return "", nil
	case r.NameKey == ReqKeyTrainingFlight || r.NameKey == ReqKeyTMGTrainingFlight:
		return RemedyTrainingFlight, nil
	}
	missing := int(math.Ceil(r.Required - r.Current))
	unit := r.Unit
	return RemedyFlyMore, &MessageParams{Missing: &missing, Unit: &unit}
}
