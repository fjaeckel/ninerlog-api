package currency

import (
	"context"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// This file implements the rule-registry engine that backs all currency
// evaluation. Regulatory data (lookback windows, experience thresholds,
// requirement names/units/messages, rule descriptions) lives in declarative
// rule structs (ratingRule / paxRule) defined per authority. The engine
// resolves the window, fetches aggregated flight data via the existing
// FlightDataProvider, builds the requirement breakdown, and hands a populated
// runtime to a small, fixed library of status "computers" (the finalize
// closures) that own the user-facing prose for each rule family. Rules declare
// what aggregate to ask for (scope + window); the provider owns how to query
// it.

// metric identifies a single aggregated field on Progress that a requirement
// is measured against. The vocabulary is a fixed enumeration — adding a term
// requires a handler here and a row in any rule that references it.
type metric int

const (
	mTotalMinutes metric = iota
	mPICMinutes
	mIFRMinutes
	mInstructorMinutes
	mLandings
	mNightLandings
	mFlights
	mApproaches
	mHolds
	mPICDualOrSupervisedMinutes
	mLaunches
	mTrainingFlights
	mLongestTrainingFlight
)

// metricVal extracts the value of a metric from an aggregated Progress row.
func metricVal(p *Progress, m metric) int {
	switch m {
	case mTotalMinutes:
		return p.TotalMinutes
	case mPICMinutes:
		return p.PICMinutes
	case mIFRMinutes:
		return p.IFRMinutes
	case mInstructorMinutes:
		return p.InstructorMinutes
	case mLandings:
		return p.Landings
	case mNightLandings:
		return p.NightLandings
	case mFlights:
		return p.Flights
	case mApproaches:
		return p.Approaches
	case mHolds:
		return p.Holds
	case mPICDualOrSupervisedMinutes:
		return p.PICMinutes + p.InstructorMinutes + p.SPICMinutes
	case mLaunches:
		return p.Launches
	case mTrainingFlights:
		return p.TrainingFlights
	case mLongestTrainingFlight:
		return p.LongestTrainingFlightMinutes
	default:
		return 0
	}
}

// reqSpec is the declarative definition of a single currency requirement.
type reqSpec struct {
	nameKey   string
	metric    metric
	threshold int
	unit      string
}

// buildReq materializes a reqSpec against an aggregated Progress row.
func buildReq(p *Progress, s reqSpec) Requirement {
	cur := metricVal(p, s.metric)
	return Requirement{
		NameKey:    s.nameKey,
		Met:        cur >= s.threshold,
		Current:    float64(cur),
		Required:   float64(s.threshold),
		Unit:       s.unit,
		MessageKey: MsgRequirementProgress,
	}
}

// buildReqs materializes a slice of reqSpecs.
func buildReqs(p *Progress, specs []reqSpec) []Requirement {
	out := make([]Requirement, 0, len(specs))
	for _, s := range specs {
		out = append(out, buildReq(p, s))
	}
	return out
}

// allReqsMet reports whether every requirement in the slice is satisfied.
func allReqsMet(reqs []Requirement) bool {
	for _, r := range reqs {
		if !r.Met {
			return false
		}
	}
	return true
}

// windowKind names how a rule's experience-counting window is anchored.
type windowKind int

const (
	// windowRollingNow anchors the window at the current instant
	// (since = now − duration). Used by recency rules (FCL.140.x, §61.57).
	windowRollingNow windowKind = iota
	// windowPrecedingExpiry anchors the window at the rating's expiry date
	// (since = expiry − duration) and applies the closed-window guard. Used
	// by revalidation rules (FCL.740.A, FCL.625.A).
	windowPrecedingExpiry
)

// windowSpec declares a rule's lookback window.
type windowSpec struct {
	kind   windowKind
	years  int
	months int
	days   int
}

// rollingSince resolves a rolling-from-now window to its start instant.
func (w windowSpec) rollingSince(now time.Time) time.Time {
	return now.AddDate(-w.years, -w.months, -w.days)
}

// progressScope declares which aggregated flight set a rule evaluates against.
type progressScope int

const (
	// scopeByClass aggregates flights on aircraft matching the rating's class.
	scopeByClass progressScope = iota
	// scopeByClassOverride aggregates flights on a fixed class (classOverride),
	// regardless of the rating's class (e.g. SPL TMG extension forces TMG).
	scopeByClassOverride
	// scopeAll aggregates flights across all aircraft classes (IR rules).
	scopeAll
	// scopeClassGroup aggregates flights on the classes returned by classGroup.
	scopeClassGroup
)

// ratingRule is the declarative definition of a Tier-1 rating-currency rule.
// The finalize strategy owns the status decision and user-facing message for
// the rule family; everything else is data.
type ratingRule struct {
	displayKey    string
	description   string
	window        windowSpec
	scope         progressScope
	classOverride models.ClassType
	countsTowed   bool
	classGroup    func(rating *models.ClassRating, peers []*models.ClassRating) []models.ClassType
	// classesNoDual drops dual time on the rule's classes from the total.
	classesNoDual bool
	// ulCredit returns the ultralight flights the rule counts beside its
	// classes, or nil for none; held is every class rating the user holds.
	ulCredit func(rating *models.ClassRating, classes []models.ClassType, held []*models.ClassRating) *ulCredit
	// extraCredit returns the classes and ultralight kinds the rule's
	// finalize counts beside its classes and ulCredit, or nil for none.
	extraCredit func(rating *models.ClassRating) *extraCredit
	baseReqs    []reqSpec
	finalize    func(ctx context.Context, rt *ratingRuntime)
}

// extraCredit declares classes and ultralight kinds a rule's finalize fetches itself.
type extraCredit struct {
	classes []models.ClassType
	ulKinds []models.ULKind
}

// ulCredit declares ultralight flights a rule counts beside its classes.
type ulCredit struct {
	sel ULSelector
	// native marks the rating's own aircraft: their dual time and proficiency
	// checks count. Otherwise only time and landings are credited.
	native      bool
	countsTowed bool
	// reportUnclassified reports uncounted ULTRALIGHT flights of no kind in
	// ClassRatingCurrency.UnclassifiedFlights.
	reportUnclassified bool
}

// ulHoursCredit returns the PIC minutes flown on ultralights of kinds since the
// rule's window start, and records the kinds as credited on the result.
func (rt *ratingRuntime) ulHoursCredit(ctx context.Context, kinds ...models.ULKind) (int, error) {
	p, err := rt.dp.GetProgressByULKind(ctx, rt.license.UserID, ULSelector{Kinds: kinds}, true, rt.since)
	if err != nil {
		return 0, err
	}
	rt.result.CreditedULKinds = kinds
	return p.PICMinutes, nil
}

// ratingRuntime carries the per-evaluation state threaded through the engine
// and into a rule's finalize strategy.
type ratingRuntime struct {
	rule     *ratingRule
	rating   *models.ClassRating
	license  *models.License
	peers    []*models.ClassRating
	classes  []models.ClassType
	ul       *ulCredit
	dp       FlightDataProvider
	result   *ClassRatingCurrency
	progress *Progress
	since    time.Time
}

// fetchProgress aggregates flight data for the runtime's window and scope,
// adding the rule's ultralight credit.
func (rt *ratingRuntime) fetchProgress(ctx context.Context) (*Progress, error) {
	if rt.rule.scope == scopeAll {
		return rt.dp.GetProgressAll(ctx, rt.license.UserID, rt.since)
	}
	total := &Progress{}
	if len(rt.classes) > 0 {
		includeTowed := rt.rule.scope != scopeByClassOverride && includeTowedFlights(rt.rating.ClassType, rt.rule.countsTowed)
		p, err := rt.dp.GetProgressByAircraftClass(ctx, rt.license.UserID, rt.classes, includeTowed, rt.since)
		if err != nil {
			return nil, err
		}
		addProgress(total, p, !rt.rule.classesNoDual)
	}
	if rt.ul != nil {
		p, err := rt.dp.GetProgressByULKind(ctx, rt.license.UserID, rt.ul.sel, rt.ul.countsTowed, rt.since)
		if err != nil {
			return nil, err
		}
		addProgress(total, p, rt.ul.native)
	}
	return total, nil
}

// addProgress adds p to total, with p's dual time only when withDual is true.
func addProgress(total, p *Progress, withDual bool) {
	if p == nil {
		return
	}
	total.Flights += p.Flights
	total.TotalMinutes += p.TotalMinutes
	total.PICMinutes += p.PICMinutes
	total.IFRMinutes += p.IFRMinutes
	if withDual {
		total.InstructorMinutes += p.InstructorMinutes
		total.SPICMinutes += p.SPICMinutes
	}
	total.NightMinutes += p.NightMinutes
	total.Landings += p.Landings
	total.DayLandings += p.DayLandings
	total.NightLandings += p.NightLandings
	total.Approaches += p.Approaches
	total.Holds += p.Holds
	total.Launches += p.Launches
	if withDual {
		total.TrainingFlights += p.TrainingFlights
		total.LongestTrainingFlightMinutes = max(total.LongestTrainingFlightMinutes, p.LongestTrainingFlightMinutes)
	}
}

// countUnclassifiedUL sets UnclassifiedFlights when the rule reports
// ULTRALIGHT flights of no kind and does not count them.
func (rt *ratingRuntime) countUnclassifiedUL(ctx context.Context) error {
	if rt.ul == nil || !rt.ul.reportUnclassified || rt.ul.sel.IncludeUnspecified {
		return nil
	}
	p, err := rt.dp.GetProgressByULKind(ctx, rt.license.UserID, ULSelector{IncludeUnspecified: true}, rt.ul.countsTowed, rt.since)
	if err != nil {
		return err
	}
	rt.result.UnclassifiedFlights = p.Flights
	return nil
}

// lastProficiencyCheck returns the latest proficiency check in the window on
// the rule's classes or its native ultralights.
func (rt *ratingRuntime) lastProficiencyCheck(ctx context.Context) (*time.Time, error) {
	var latest *time.Time
	if len(rt.classes) > 0 {
		d, err := rt.dp.GetLastProficiencyCheck(ctx, rt.license.UserID, rt.classes, rt.since)
		if err != nil {
			return nil, err
		}
		latest = d
	}
	if rt.ul != nil && rt.ul.native {
		d, err := rt.dp.GetLastProficiencyCheckByULKind(ctx, rt.license.UserID, rt.ul.sel, rt.since)
		if err != nil {
			return nil, err
		}
		if d != nil && (latest == nil || d.After(*latest)) {
			latest = d
		}
	}
	return latest, nil
}

// resolveClasses returns the aircraft classes a rule counts for a rating; nil means all classes.
func resolveClasses(rule *ratingRule, rating *models.ClassRating, peers []*models.ClassRating) []models.ClassType {
	switch rule.scope {
	case scopeAll:
		return nil
	case scopeByClassOverride:
		return []models.ClassType{rule.classOverride}
	case scopeClassGroup:
		return rule.classGroup(rating, peers)
	default:
		return []models.ClassType{rating.ClassType}
	}
}

// includeTowedFlights reports whether towed launches (winch, aerotow, car, bungee) count toward a class.
func includeTowedFlights(classType models.ClassType, sailplane bool) bool {
	return sailplane || classType == models.ClassTypeGlider
}

// evalRatingRule is the engine entry point: it builds the base result shell
// (identity, authority, description, expiry) and dispatches to the rule's
// finalize strategy, which resolves the window, fetches data, builds the
// requirement breakdown, and sets the status + message.
func evalRatingRule(ctx context.Context, rule *ratingRule, rating *models.ClassRating, license *models.License, dp FlightDataProvider) ClassRatingCurrency {
	return evalRatingRuleWithPeers(ctx, rule, rating, license, nil, dp)
}

// evalRatingRuleWithPeers is evalRatingRule with the other class ratings on the license.
func evalRatingRuleWithPeers(ctx context.Context, rule *ratingRule, rating *models.ClassRating, license *models.License, peers []*models.ClassRating, dp FlightDataProvider) ClassRatingCurrency {
	return evalRatingRuleForHolder(ctx, rule, rating, license, peers, peers, dp)
}

// evalRatingRuleForHolder is evalRatingRuleWithPeers with every class rating
// the user holds across licences.
func evalRatingRuleForHolder(ctx context.Context, rule *ratingRule, rating *models.ClassRating, license *models.License, peers, held []*models.ClassRating, dp FlightDataProvider) ClassRatingCurrency {
	result := ClassRatingCurrency{
		ClassRatingID:       rating.ID,
		ClassType:           rating.ClassType,
		LicenseID:           rating.LicenseID,
		RegulatoryAuthority: license.RegulatoryAuthority,
		LicenseType:         license.LicenseType,
		RuleDescription:     rule.description,
		RuleDescriptionKey:  rule.displayKey,
	}
	if rating.ExpiryDate != nil {
		expStr := rating.ExpiryDate.Format("2006-01-02")
		result.ExpiryDate = &expStr
	}

	classes := resolveClasses(rule, rating, peers)
	if len(classes) > 1 {
		result.CountedClasses = classes
	}
	var ul *ulCredit
	if rule.ulCredit != nil {
		ul = rule.ulCredit(rating, classes, held)
	}
	if ul != nil {
		result.CreditedULKinds = ul.sel.Kinds
	}

	rt := &ratingRuntime{rule: rule, rating: rating, license: license, peers: peers, classes: classes, ul: ul, dp: dp, result: &result}
	rule.finalize(ctx, rt)
	return result
}

// recencyFinalize returns the finalize strategy for a rolling-window recency
// rule; withCheck adds a proficiency check that replaces the experience.
func recencyFinalize(withCheck bool) func(ctx context.Context, rt *ratingRuntime) {
	return func(ctx context.Context, rt *ratingRuntime) {
		rt.since = rt.rule.window.rollingSince(time.Now())
		progress, err := rt.fetchProgress(ctx)
		if err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		rt.result.Progress = progress
		if err := rt.countUnclassifiedUL(ctx); err != nil {
			rt.result.Status = StatusUnknown
			rt.result.setMsg(MsgRatingEvaluationFailed, nil)
			return
		}
		reqs := buildReqs(progress, rt.rule.baseReqs)
		met := allReqsMet(reqs)

		if withCheck {
			checkDate, err := rt.lastProficiencyCheck(ctx)
			if err != nil {
				rt.result.Status = StatusUnknown
				rt.result.setMsg(MsgRatingEvaluationFailed, nil)
				return
			}
			reqCheck := profCheckRequirement(checkDate)
			met = met || reqCheck.Met
			reqs = append(reqs, reqCheck)
		}
		rt.result.Requirements = reqs
		setRecencyStatus(rt.result, met)
	}
}

// setRecencyStatus sets a rolling recency rule's status and message: current
// when met, lapsed otherwise.
func setRecencyStatus(result *ClassRatingCurrency, met bool) {
	if met {
		result.Status = StatusCurrent
		result.setMsg(MsgRatingRecencyCurrent, nil)
	} else {
		result.Status = StatusLapsed
		result.setMsg(MsgRatingRecencyNotMet, nil)
	}
}
