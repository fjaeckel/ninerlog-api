// Package logbookrules maps a licence's issuing country and type to the
// authority's rules on correcting electronic logbook entries.
package logbookrules

import (
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Window is the period in which a flight entry may be edited freely.
type Window string

const (
	// WindowFlightDay ends at the end of the flight's calendar day.
	WindowFlightDay Window = "flight_day"
	// WindowHours48 ends 48 hours after the entry was created.
	WindowHours48 Window = "hours_48"
)

// Requirement is how strongly a licence's rules call for strict mode.
type Requirement string

const (
	RequirementNone      Requirement = "none"
	RequirementSuggested Requirement = "suggested"
	RequirementRequired  Requirement = "required"
)

// Licence is the part of a licence the registry reads.
type Licence struct {
	ID                  uuid.UUID
	RegulatoryAuthority string
	LicenseType         string
	IssuingCountry      string
}

// Rule is the registry's verdict for one licence.
type Rule struct {
	Requirement Requirement
	// Window is empty unless the requirement is RequirementRequired.
	Window Window
	Basis  string
	// DownwardOnlyOverrides is true when manual overrides may only reduce
	// a calculated value.
	DownwardOnlyOverrides bool
}

// Match pairs a licence with its rule.
type Match struct {
	Licence Licence
	Rule    Rule
}

// Assessment is the verdict across all of a user's licences.
type Assessment struct {
	Required  []Match
	Suggested []Match
	// Window is the shortest window across Required; empty when none is required.
	Window Window
	// OverridesDownwardOnly is true when any required licence limits overrides.
	OverridesDownwardOnly bool
}

// RuleFor returns the rule for one licence. A licence without an issuing
// country has no rule.
func RuleFor(l Licence) Rule {
	country := strings.ToUpper(strings.TrimSpace(l.IssuingCountry))
	if country == "" {
		return Rule{Requirement: RequirementNone}
	}
	switch country {
	case "CH":
		return Rule{
			Requirement: RequirementRequired, Window: WindowHours48,
			Basis: "FOCA Accepted Logbook Formats §2.3(7)", DownwardOnlyOverrides: true,
		}
	case "DE":
		if LooksLikeUL(l.LicenseType) {
			return Rule{Requirement: RequirementSuggested, Basis: "§120 LuftPersV"}
		}
		return required(WindowFlightDay, "NfL 2212-21 §4.3.2")
	case "AT":
		if isATNational(l.LicenseType) {
			return required(WindowFlightDay, "ZLPV 2006 Anlage 6")
		}
		return Rule{Requirement: RequirementSuggested, Basis: "AMC1 FCL.050 (e)(2)"}
	case "AU":
		return required(WindowFlightDay, "CASR 61.355, RAAP 13")
	case "NZ":
		return required(WindowFlightDay, "Part 61.29(e)")
	case "JP":
		return required(WindowFlightDay, "国空乗390号")
	case "UY":
		return required(WindowFlightDay, "RAU 61.51")
	case "DK":
		return Rule{Requirement: RequirementSuggested, Basis: "BL 6-04 §3.2.6"}
	case "BR":
		return Rule{Requirement: RequirementSuggested, Basis: "CIV Digital"}
	case "GB":
		return Rule{Requirement: RequirementSuggested, Basis: "printed extracts"}
	case "US":
		return Rule{Requirement: RequirementSuggested, Basis: "AC 120-78B"}
	}
	if strings.EqualFold(strings.TrimSpace(l.RegulatoryAuthority), "EASA") {
		return Rule{Requirement: RequirementSuggested, Basis: "AMC1 FCL.050 (e)(2)"}
	}
	return Rule{Requirement: RequirementNone}
}

func required(w Window, basis string) Rule {
	return Rule{Requirement: RequirementRequired, Window: w, Basis: basis}
}

// Assess evaluates every licence and combines the verdicts.
func Assess(licences []Licence) Assessment {
	var a Assessment
	for _, l := range licences {
		r := RuleFor(l)
		switch r.Requirement {
		case RequirementRequired:
			a.Required = append(a.Required, Match{Licence: l, Rule: r})
			a.OverridesDownwardOnly = a.OverridesDownwardOnly || r.DownwardOnlyOverrides
			a.Window = shorter(a.Window, r.Window)
		case RequirementSuggested:
			a.Suggested = append(a.Suggested, Match{Licence: l, Rule: r})
		}
	}
	return a
}

func shorter(a, b Window) Window {
	if a == "" {
		return b
	}
	if a == WindowFlightDay || b == WindowFlightDay {
		return WindowFlightDay
	}
	return a
}

// Deadline is the instant the draft window of a flight entry closes: the
// earliest deadline across the required licences, or the end of the flight
// day when none is required. lonDeg is the departure longitude; pass 0 when
// it is unknown.
func (a Assessment) Deadline(createdAt, flightDate time.Time, lonDeg float64) time.Time {
	if len(a.Required) == 0 {
		return FlightDayEnd(flightDate, lonDeg)
	}
	var earliest time.Time
	for _, m := range a.Required {
		d := WindowDeadline(m.Rule.Window, createdAt, flightDate, lonDeg)
		if earliest.IsZero() || d.Before(earliest) {
			earliest = d
		}
	}
	return earliest
}

// WindowDeadline is the instant a single window closes for a flight.
func WindowDeadline(w Window, createdAt, flightDate time.Time, lonDeg float64) time.Time {
	if w == WindowHours48 {
		return createdAt.Add(48 * time.Hour)
	}
	return FlightDayEnd(flightDate, lonDeg)
}

// FlightDayEnd is the end of the flight's calendar date at the mean solar
// offset of lonDeg, which is round(lon/15) whole hours.
func FlightDayEnd(flightDate time.Time, lonDeg float64) time.Time {
	offset := time.Duration(math.Round(lonDeg/15)) * time.Hour
	y, m, d := flightDate.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC).Add(-offset)
}

// LooksLikeUL reports whether a licence type names an ultralight licence.
func LooksLikeUL(licenseType string) bool {
	t := strings.ToUpper(strings.TrimSpace(licenseType))
	return t == "UL" || strings.HasPrefix(t, "UL ") || strings.HasPrefix(t, "UL-") ||
		strings.Contains(t, "ULTRALIGHT") || strings.Contains(t, "ULTRALEICHT")
}

var atNationalTypes = []string{
	"HG", "PG", "HANG GLIDER", "PARAGLIDER", "HÄNGEGLEITER", "PARAGLEITER", "GPL", "NATIONAL GLIDER",
}

func isATNational(licenseType string) bool {
	if LooksLikeUL(licenseType) {
		return true
	}
	t := strings.ToUpper(strings.TrimSpace(licenseType))
	for _, n := range atNationalTypes {
		if strings.Contains(t, n) {
			return true
		}
	}
	return false
}
