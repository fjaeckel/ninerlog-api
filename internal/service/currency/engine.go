package currency

import (
	"context"
	"fmt"
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
// closures) that own the user-facing prose for each rule family.
//
// The architecture deliberately keeps the FlightDataProvider interface
// unchanged: rules declare *what* aggregate to ask for (scope + window); the
// provider owns *how* to query it.

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
	default:
		return 0
	}
}

// reqSpec is the declarative definition of a single currency requirement.
// msgFmt must contain exactly one %d verb, which receives the current value.
type reqSpec struct {
	name      string
	metric    metric
	threshold int
	unit      string
	msgFmt    string
}

// buildReq materializes a reqSpec against an aggregated Progress row.
func buildReq(p *Progress, s reqSpec) Requirement {
	cur := metricVal(p, s.metric)
	return Requirement{
		Name:     s.name,
		Met:      cur >= s.threshold,
		Current:  float64(cur),
		Required: float64(s.threshold),
		Unit:     s.unit,
		Message:  fmt.Sprintf(s.msgFmt, cur),
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
	baseReqs      []reqSpec
	finalize      func(ctx context.Context, rt *ratingRuntime)
}

// ratingRuntime carries the per-evaluation state threaded through the engine
// and into a rule's finalize strategy.
type ratingRuntime struct {
	rule     *ratingRule
	rating   *models.ClassRating
	license  *models.License
	dp       FlightDataProvider
	result   *ClassRatingCurrency
	progress *Progress
	since    time.Time
}

// fetchProgress aggregates flight data for the runtime's window and scope.
func (rt *ratingRuntime) fetchProgress(ctx context.Context) (*Progress, error) {
	switch rt.rule.scope {
	case scopeAll:
		return rt.dp.GetProgressAll(ctx, rt.license.UserID, rt.since)
	case scopeByClassOverride:
		return rt.dp.GetProgressByAircraftClass(ctx, rt.license.UserID, rt.rule.classOverride, rt.since)
	default:
		return rt.dp.GetProgressByAircraftClass(ctx, rt.license.UserID, rt.rating.ClassType, rt.since)
	}
}

// evalRatingRule is the engine entry point: it builds the base result shell
// (identity, authority, description, expiry) and dispatches to the rule's
// finalize strategy, which resolves the window, fetches data, builds the
// requirement breakdown, and sets the status + message.
func evalRatingRule(ctx context.Context, rule *ratingRule, rating *models.ClassRating, license *models.License, dp FlightDataProvider) ClassRatingCurrency {
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

	rt := &ratingRuntime{rule: rule, rating: rating, license: license, dp: dp, result: &result}
	rule.finalize(ctx, rt)
	return result
}
