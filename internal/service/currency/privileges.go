package currency

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ActivityQuery selects flights for a privilege rule. Simulator and passenger
// flights are never selected. Classes and ULKinds are alternatives: a flight
// matches when its aircraft is of one of Classes or is an ULTRALIGHT of one of
// ULKinds; both empty matches every aircraft.
type ActivityQuery struct {
	Classes []models.ClassType
	ULKinds []models.ULKind
	// ExcludeUL drops flights on ULTRALIGHT aircraft.
	ExcludeUL bool
	// TowOnly keeps flights flagged is_tow_flight.
	TowOnly bool
	// IFROnly keeps flights with IFR time.
	IFROnly bool
	// PICOnly keeps flights with PIC time.
	PICOnly bool
	// DualGivenOnly keeps flights with instruction given.
	DualGivenOnly bool
	// DualReceivedOnly keeps flights with dual time received.
	DualReceivedOnly bool
	// CrossCountryOnly keeps flights with cross-country time.
	CrossCountryOnly bool
	// LaunchMethod keeps flights with this launch method; "" keeps all.
	LaunchMethod string
}

// ActivityDay aggregates the flights an ActivityQuery selects on one date.
type ActivityDay struct {
	Date             time.Time
	Flights          int
	PICMinutes       int
	IFRMinutes       int
	DualGivenMinutes int
	// Launches counts the flights' launches; a flight without a launch count
	// counts its take-offs, at least one.
	Launches int
	// MultiLandingFlights counts flights with at least two landings.
	MultiLandingFlights int
	DistanceNM          float64
}

// PrivilegeDataProvider is the flight read behind privilege rules.
type PrivilegeDataProvider interface {
	// GetActivityDays returns per-date aggregates of the flights q selects
	// since the given date, newest date first.
	GetActivityDays(ctx context.Context, userID uuid.UUID, q ActivityQuery, since time.Time) ([]ActivityDay, error)
}

// PrivilegeLister lists a user's licence privileges.
type PrivilegeLister interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.LicencePrivilege, error)
}

// Rule description keys of privilege rules.
const (
	RulePrivilegeExpiry      = "privilege_expiry"
	RuleSFCL205Towing        = "sfcl_205_towing"
	RuleSFCL205BannerTowing  = "sfcl_205_banner_towing"
	RuleFAA6169Towing        = "faa_61_69_towing"
	RuleSFCL215CloudFlying   = "sfcl_215_cloud_flying"
	RuleSFCL360FIS           = "sfcl_360_fi_s"
	RuleDULVULTowing         = "dulv_ul_towing"
	RuleSFCL155LaunchTrained = "sfcl_155_launch_method"
)

// privilegeRule evaluates the recency of one privilege kind. It sets
// Requirements and returns whether the rule is met.
type privilegeRule struct {
	key      string
	evaluate func(ctx context.Context, pc *privilegeContext) (bool, error)
}

// privilegeContext carries one privilege evaluation.
type privilegeContext struct {
	privilege *models.LicencePrivilege
	license   *models.License
	dp        PrivilegeDataProvider
	now       time.Time
	result    *PrivilegeCurrency
}

// privilegeRuleFor returns the recency rule for a privilege on a licence, nil
// for kinds whose status follows their expiry date only.
func privilegeRuleFor(p *models.LicencePrivilege, license *models.License) *privilegeRule {
	switch p.Kind {
	case models.PrivilegeSailplaneTowing:
		if strings.EqualFold(license.RegulatoryAuthority, "FAA") {
			return &faaTowingRule
		}
		return &sfclTowingRule
	case models.PrivilegeBannerTowing:
		return &sfclBannerTowingRule
	case models.PrivilegeCloudFlying:
		return &sfclCloudFlyingRule
	case models.PrivilegeFIS:
		return &sfclFISRule
	case models.PrivilegeULTowing:
		return &dulvULTowingRule
	}
	return nil
}

// EvaluatePrivilege evaluates one privilege at the context's instant: expired
// past its expiry date, otherwise current or lapsed by its recency rule, or
// current when it has none. dp may be nil for kinds without a recency rule.
func EvaluatePrivilege(ctx context.Context, p *models.LicencePrivilege, license *models.License, dp PrivilegeDataProvider) PrivilegeCurrency {
	now := nowFrom(ctx)
	result := PrivilegeCurrency{
		PrivilegeID:        p.ID,
		LicenseID:          p.LicenseID,
		Kind:               p.Kind,
		Detail:             p.Detail,
		RuleDescriptionKey: RulePrivilegeExpiry,
		ExpiresOn:          p.ExpiresOn,
		LicenseType:        license.LicenseType,
	}
	var params *MessageParams
	if p.ExpiresOn != nil {
		params = msgDate(p.ExpiresOn.Format("2006-01-02"))
	}
	if p.Kind == models.PrivilegeLaunchMethodTrained {
		result.RuleDescriptionKey = RuleSFCL155LaunchTrained
	}
	rule := privilegeRuleFor(p, license)
	if rule != nil {
		result.RuleDescriptionKey = rule.key
	}
	if p.IsExpiredOn(now) {
		result.Status = StatusExpired
		result.MessageKey, result.MessageParams = MsgPrivilegeExpired, params
		return result
	}
	if rule == nil {
		result.Status = StatusCurrent
		result.MessageKey, result.MessageParams = MsgPrivilegeValid, params
		return result
	}
	if dp == nil {
		result.Status = StatusUnknown
		result.MessageKey = MsgPrivilegeEvaluationFailed
		return result
	}
	met, err := rule.evaluate(ctx, &privilegeContext{privilege: p, license: license, dp: dp, now: now, result: &result})
	switch {
	case err != nil:
		result.Status = StatusUnknown
		result.Requirements = nil
		result.MessageKey = MsgPrivilegeEvaluationFailed
	case met:
		result.Status = StatusCurrent
		result.MessageKey, result.MessageParams = MsgPrivilegeRecencyCurrent, params
	default:
		result.Status = StatusLapsed
		result.MessageKey, result.MessageParams = MsgPrivilegeRecencyNotMet, params
	}
	return result
}

// rollingWindow is a recency window of whole months ending now; calendar
// windows start on the first day of the month.
type rollingWindow struct {
	months   int
	calendar bool
}

// since returns the first instant of the window at now.
func (w rollingWindow) since(now time.Time) time.Time {
	if w.calendar {
		start := now.AddDate(0, -w.months, 0)
		return time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return now.AddDate(0, -w.months, 0)
}

// lastDay returns the last date a flight on date still counts.
func (w rollingWindow) lastDay(date time.Time) time.Time {
	d := midnightUTC(date)
	if w.calendar {
		return time.Date(d.Year(), d.Month()+time.Month(w.months)+1, 0, 0, 0, 0, 0, time.UTC)
	}
	return d.AddDate(0, w.months, -1)
}

// countRow builds a requirement from days (newest first) summed by value
// against threshold, with a remedy when unmet and, for a non-nil window, a
// validUntil when met.
func countRow(nameKey, unit string, days []ActivityDay, value func(ActivityDay) int, threshold int, w *rollingWindow, remedy string) Requirement {
	total := 0
	var reached *time.Time
	for _, d := range days {
		total += value(d)
		if reached == nil && total >= threshold {
			date := d.Date
			reached = &date
		}
	}
	r := Requirement{
		NameKey:    nameKey,
		Met:        total >= threshold,
		Current:    float64(total),
		Required:   float64(threshold),
		Unit:       unit,
		MessageKey: MsgRequirementProgress,
	}
	if r.Met {
		if w != nil {
			last := w.lastDay(*reached)
			r.ValidUntil = dateString(&last)
		}
		return r
	}
	if remedy != "" {
		missing := threshold - total
		u := unit
		r.RemedyKey = remedy
		r.RemedyParams = &MessageParams{Missing: &missing, Unit: &u}
	}
	return r
}

// untrackedRow is an informational requirement NinerLog cannot count.
func untrackedRow(nameKey, unit string) Requirement {
	return Requirement{NameKey: nameKey, Current: 0, Required: 1, Unit: unit, MessageKey: MsgRequirementUntracked}
}

func launchesOf(d ActivityDay) int  { return d.Launches }
func flightsOf(d ActivityDay) int   { return d.Flights }
func ifrMinutes(d ActivityDay) int  { return d.IFRMinutes }
func dualGivenOf(d ActivityDay) int { return d.DualGivenMinutes }

// towRule counts tows (the take-offs of tow flights) in a window.
func towRule(key string, threshold int, w *rollingWindow, remedy string) privilegeRule {
	return privilegeRule{key: key, evaluate: func(ctx context.Context, pc *privilegeContext) (bool, error) {
		days, err := pc.dp.GetActivityDays(ctx, pc.license.UserID, ActivityQuery{TowOnly: true, ExcludeUL: true}, w.since(pc.now))
		if err != nil {
			return false, err
		}
		row := countRow(ReqKeyTows, "tows", days, launchesOf, threshold, w, remedy)
		pc.result.Requirements = []Requirement{row}
		return row.Met, nil
	}}
}

// sfclTowingRule — SFCL.205(c): 5 tows in the last 24 months; a lapsed pilot
// flies the missing tows with or under the supervision of an instructor.
var sfclTowingRule = towRule(RuleSFCL205Towing, 5, &rollingWindow{months: 24}, RemedyPrivilegeWithInstructor)

// sfclBannerTowingRule — SFCL.205(c) for banner towing, counted from the same tow flag.
var sfclBannerTowingRule = towRule(RuleSFCL205BannerTowing, 5, &rollingWindow{months: 24}, RemedyPrivilegeWithInstructor)

// faaTowingRule — 14 CFR 61.69(a)(5): 3 tows, or 3 flights as PIC of an
// aerotowed glider, in 24 calendar months.
var faaTowingRule = privilegeRule{key: RuleFAA6169Towing, evaluate: func(ctx context.Context, pc *privilegeContext) (bool, error) {
	w := &rollingWindow{months: 24, calendar: true}
	since := w.since(pc.now)
	tows, err := pc.dp.GetActivityDays(ctx, pc.license.UserID, ActivityQuery{TowOnly: true, ExcludeUL: true}, since)
	if err != nil {
		return false, err
	}
	towed, err := pc.dp.GetActivityDays(ctx, pc.license.UserID, ActivityQuery{
		Classes: []models.ClassType{models.ClassTypeGlider}, PICOnly: true, LaunchMethod: models.LaunchMethodAerotow,
	}, since)
	if err != nil {
		return false, err
	}
	rows := []Requirement{
		countRow(ReqKeyTows, "tows", tows, launchesOf, 3, w, RemedyFlyMore),
		countRow(ReqKeyTowedGliderFlights, "flights", towed, launchesOf, 3, w, RemedyFlyMore),
	}
	pc.result.Requirements = rows
	return rows[0].Met || rows[1].Met, nil
}}

// sfclCloudFlyingRule — SFCL.215: 1 h or 5 flights as PIC exercising cloud
// flying privileges on sailplanes, excluding TMGs, in 24 months, counted from
// IFR time on GLIDER flights.
var sfclCloudFlyingRule = privilegeRule{key: RuleSFCL215CloudFlying, evaluate: func(ctx context.Context, pc *privilegeContext) (bool, error) {
	w := &rollingWindow{months: 24}
	days, err := pc.dp.GetActivityDays(ctx, pc.license.UserID, ActivityQuery{
		Classes: []models.ClassType{models.ClassTypeGlider}, IFROnly: true, PICOnly: true,
	}, w.since(pc.now))
	if err != nil {
		return false, err
	}
	rows := []Requirement{
		countRow(ReqKeyCloudFlyingTime, "minutes", days, ifrMinutes, 60, w, RemedyPrivilegeWithInstructor),
		countRow(ReqKeyCloudFlyingFlights, "flights", days, flightsOf, 5, w, RemedyPrivilegeWithInstructor),
	}
	pc.result.Requirements = rows
	return rows[0].Met || rows[1].Met, nil
}}

// sfclFISRule — SFCL.360: 30 h or 60 launches of instruction given on
// sailplanes including TMGs in 3 years; the refresher training is reported,
// not tracked.
var sfclFISRule = privilegeRule{key: RuleSFCL360FIS, evaluate: func(ctx context.Context, pc *privilegeContext) (bool, error) {
	w := &rollingWindow{months: 36}
	days, err := pc.dp.GetActivityDays(ctx, pc.license.UserID, ActivityQuery{
		Classes: []models.ClassType{models.ClassTypeGlider, models.ClassTypeTMG}, DualGivenOnly: true,
	}, w.since(pc.now))
	if err != nil {
		return false, err
	}
	rows := []Requirement{
		countRow(ReqKeyInstructionTime, "minutes", days, dualGivenOf, 1800, w, RemedyFlyMore),
		countRow(ReqKeyInstructionLaunches, "launches", days, launchesOf, 60, w, RemedyFlyMore),
		untrackedRow(ReqKeyFIRefresher, "training"),
	}
	pc.result.Requirements = rows
	return rows[0].Met || rows[1].Met, nil
}}

// dulvULTowingRule — DULV towing authorisation: 10 tows in 24 months on
// ultralights of the privilege's kind.
var dulvULTowingRule = privilegeRule{key: RuleDULVULTowing, evaluate: func(ctx context.Context, pc *privilegeContext) (bool, error) {
	w := &rollingWindow{months: 24}
	var days []ActivityDay
	if pc.privilege.Detail != nil {
		var err error
		days, err = pc.dp.GetActivityDays(ctx, pc.license.UserID, ActivityQuery{
			ULKinds: models.AircraftKindsForRating(models.ULKind(*pc.privilege.Detail)), TowOnly: true,
		}, w.since(pc.now))
		if err != nil {
			return false, err
		}
	}
	row := countRow(ReqKeyTows, "tows", days, launchesOf, 10, w, RemedyFlyMore)
	pc.result.Requirements = []Requirement{row}
	return row.Met, nil
}}

// spl115PassengerPrerequisites returns the SFCL.115(a)(2) rows: 10 h or 30
// launches as PIC on sailplanes including TMGs since licence issue, and the
// passenger competence training flight, which is not tracked.
func spl115PassengerPrerequisites(ctx context.Context, license *models.License, dp PrivilegeDataProvider) ([]Requirement, error) {
	days, err := dp.GetActivityDays(ctx, license.UserID, ActivityQuery{
		Classes: []models.ClassType{models.ClassTypeGlider, models.ClassTypeTMG}, PICOnly: true,
	}, midnightUTC(license.IssueDate))
	if err != nil {
		return nil, err
	}
	return []Requirement{
		countRow(ReqKeyPaxPrerequisiteTime, "minutes", days, func(d ActivityDay) int { return d.PICMinutes }, 600, nil, RemedyFlyMore),
		countRow(ReqKeyPaxPrerequisiteLaunches, "launches", days, launchesOf, 30, nil, RemedyFlyMore),
		untrackedRow(ReqKeyPaxCompetenceFlight, "flight"),
	}, nil
}

// ulPassengerAuthorisationProgress returns progress toward the LuftPersV §84a
// passenger authorisation on ultralights of kind: 5 cross-country flights
// with an instructor, 2 of them with an intermediate landing, 200 km in total.
func ulPassengerAuthorisationProgress(ctx context.Context, userID uuid.UUID, kind models.ULKind, dp PrivilegeDataProvider) ([]Requirement, error) {
	days, err := dp.GetActivityDays(ctx, userID, ActivityQuery{
		ULKinds: models.AircraftKindsForRating(kind), DualReceivedOnly: true, CrossCountryOnly: true,
	}, time.Time{})
	if err != nil {
		return nil, err
	}
	distanceNM := 0.0
	for _, d := range days {
		distanceNM += d.DistanceNM
	}
	km := int(math.Round(distanceNM * 1.852))
	return []Requirement{
		countRow(ReqKeyULXCFlights, "flights", days, flightsOf, 5, nil, RemedyFlyMore),
		countRow(ReqKeyULXCLandingFlights, "flights", days, func(d ActivityDay) int { return d.MultiLandingFlights }, 2, nil, RemedyFlyMore),
		countRow(ReqKeyULXCDistance, "km", []ActivityDay{{Flights: km}}, flightsOf, 200, nil, RemedyFlyMore),
	}, nil
}
