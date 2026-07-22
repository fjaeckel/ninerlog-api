package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CustomCurrencyRule is a user-authored, modular currency definition. The rule
// body (window + filters + requirements) lives in Definition and is evaluated
// against the user's flights by the currency engine. Rules are private by
// default; enabling sharing exposes a read-only copy via ShareToken that other
// users can import.
type CustomCurrencyRule struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"userId"`
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	Emoji       *string                `json:"emoji,omitempty"`
	Definition  CustomCurrencyRuleBody `json:"definition"`
	// Enabled is false when the rule is paused: it is kept and listed but not
	// evaluated or surfaced as active currency.
	Enabled      bool       `json:"enabled"`
	IsShared     bool       `json:"isShared"`
	ShareToken   *string    `json:"shareToken,omitempty"`
	ImportedFrom *uuid.UUID `json:"importedFrom,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// CustomCurrencyRuleBody is the declarative rule document. It mirrors the shape
// of the built-in engine's rules: a lookback Window, a set of Filters that
// select which flights count, and one or more Requirements measured against
// aggregated flight metrics. Requirements are combined with logical AND — the
// rule is "current" only when every requirement is met.
type CustomCurrencyRuleBody struct {
	Window       CurrencyWindow        `json:"window"`
	Filters      []CurrencyFilter      `json:"filters,omitempty"`
	Requirements []CurrencyRequirement `json:"requirements"`
}

// CurrencyWindow is a rolling lookback anchored at the moment of evaluation:
// only flights on or after (now − Amount×Unit) count. Expiry-anchored windows
// (as used by some regulatory revalidation rules) are intentionally out of
// scope for user-authored rules in this version.
type CurrencyWindow struct {
	Amount int    `json:"amount"`
	Unit   string `json:"unit"` // days | weeks | months | years
}

// CurrencyFilter narrows the set of flights a rule counts. Op determines how
// Value/Values are interpreted:
//   - "eq":      the flight's field equals Value
//   - "in":      the flight's field is one of Values
//   - "is_true": the (boolean/derived) field is true; Value/Values ignored
type CurrencyFilter struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Value  string   `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
}

// CurrencyRequirement is a single "at least N" threshold on an aggregated
// metric over the filtered, windowed flight set. For time metrics, Unit selects
// how Min is expressed (hours or minutes); count metrics ignore Unit.
type CurrencyRequirement struct {
	Metric string  `json:"metric"`
	Min    float64 `json:"min"`
	Unit   string  `json:"unit,omitempty"` // hours | minutes (time metrics only)
	Label  string  `json:"label,omitempty"`
}

// --- Controlled vocabulary -------------------------------------------------
//
// These sets are the single source of truth for what a rule may reference. The
// evaluator maps each identifier to a parameterized SQL fragment; validation
// here rejects anything outside the vocabulary so no user-supplied string ever
// reaches a query as an identifier.

// ValidWindowUnits enumerates the accepted lookback units.
var ValidWindowUnits = map[string]bool{
	"days": true, "weeks": true, "months": true, "years": true,
}

// TimeMetrics are metrics measured in minutes in the database and expressed to
// the user in hours by default. Membership also drives Unit handling.
var TimeMetrics = map[string]bool{
	"total_time":         true,
	"pic_time":           true,
	"dual_time":          true,
	"night_time":         true,
	"ifr_time":           true,
	"cross_country_time": true,
}

// CountMetrics are integer-valued metrics (counts of flights, landings, etc.).
var CountMetrics = map[string]bool{
	"flights":        true,
	"landings":       true,
	"day_landings":   true,
	"night_landings": true,
	"takeoffs":       true,
	"day_takeoffs":   true,
	"night_takeoffs": true,
	"approaches":     true,
	"holds":          true,
}

// ValidFilterFields enumerates the fields a filter may reference, mapped to the
// operators each supports. "value"/"values" fields use eq/in; boolean/derived
// fields use is_true.
var ValidFilterFields = map[string]map[string]bool{
	"aircraft_class":            {"eq": true, "in": true},
	"aircraft_type":             {"eq": true, "in": true},
	"aircraft_registration":     {"eq": true, "in": true},
	"launch_method":             {"eq": true, "in": true},
	"aircraft_complex":          {"is_true": true},
	"aircraft_high_performance": {"is_true": true},
	"aircraft_tailwheel":        {"is_true": true},
	"is_pic":                    {"is_true": true},
	"is_dual":                   {"is_true": true},
	"has_night":                 {"is_true": true},
	"has_ifr":                   {"is_true": true},
	"is_cross_country":          {"is_true": true},
}

// IsTimeMetric reports whether a metric is measured in time (minutes).
func IsTimeMetric(metric string) bool { return TimeMetrics[metric] }

// IsValidMetric reports whether a metric is part of the controlled vocabulary.
func IsValidMetric(metric string) bool {
	return TimeMetrics[metric] || CountMetrics[metric]
}

const (
	maxCustomRuleFilters      = 20
	maxCustomRuleRequirements = 20
	maxCustomRuleFilterValues = 50
)

// Validate checks a rule body against the controlled vocabulary and structural
// limits. It returns a descriptive error suitable for surfacing to the author.
func (b *CustomCurrencyRuleBody) Validate() error {
	// Window
	if b.Window.Amount <= 0 {
		return fmt.Errorf("window amount must be greater than zero")
	}
	if b.Window.Amount > 1000 {
		return fmt.Errorf("window amount is too large")
	}
	if !ValidWindowUnits[b.Window.Unit] {
		return fmt.Errorf("invalid window unit %q (expected days, weeks, months or years)", b.Window.Unit)
	}

	// Requirements
	if len(b.Requirements) == 0 {
		return fmt.Errorf("a rule needs at least one requirement")
	}
	if len(b.Requirements) > maxCustomRuleRequirements {
		return fmt.Errorf("too many requirements (max %d)", maxCustomRuleRequirements)
	}
	for i := range b.Requirements {
		r := &b.Requirements[i]
		if !IsValidMetric(r.Metric) {
			return fmt.Errorf("unknown metric %q", r.Metric)
		}
		if r.Min <= 0 {
			return fmt.Errorf("requirement for %q must be greater than zero", r.Metric)
		}
		if r.Unit != "" {
			if !IsTimeMetric(r.Metric) {
				return fmt.Errorf("metric %q does not take a unit", r.Metric)
			}
			if r.Unit != "hours" && r.Unit != "minutes" {
				return fmt.Errorf("invalid unit %q for %q (expected hours or minutes)", r.Unit, r.Metric)
			}
		}
	}

	// Filters
	if len(b.Filters) > maxCustomRuleFilters {
		return fmt.Errorf("too many filters (max %d)", maxCustomRuleFilters)
	}
	for i := range b.Filters {
		f := &b.Filters[i]
		ops, ok := ValidFilterFields[f.Field]
		if !ok {
			return fmt.Errorf("unknown filter field %q", f.Field)
		}
		if !ops[f.Op] {
			return fmt.Errorf("operator %q is not valid for field %q", f.Op, f.Field)
		}
		switch f.Op {
		case "eq":
			if f.Value == "" {
				return fmt.Errorf("filter on %q needs a value", f.Field)
			}
		case "in":
			if len(f.Values) == 0 {
				return fmt.Errorf("filter on %q needs at least one value", f.Field)
			}
			if len(f.Values) > maxCustomRuleFilterValues {
				return fmt.Errorf("too many values for filter on %q", f.Field)
			}
			for _, v := range f.Values {
				if v == "" {
					return fmt.Errorf("filter on %q has an empty value", f.Field)
				}
			}
		case "is_true":
			// no value required
		}
	}
	return nil
}

// Compile-time guarantees that the body satisfies the database interfaces used
// to persist and load it as JSONB. Value() in particular must return the named
// driver.Value type — an (interface{}, error) signature silently fails to
// implement driver.Valuer and the struct reaches the driver unconverted.
var (
	_ driver.Valuer = CustomCurrencyRuleBody{}
	_ sql.Scanner   = (*CustomCurrencyRuleBody)(nil)
)

// Value implements driver.Valuer so the body can be stored directly in a JSONB
// column. It returns the JSON as a string (text) so lib/pq sends it in a form
// Postgres accepts for jsonb.
func (b CustomCurrencyRuleBody) Value() (driver.Value, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// Scan implements sql.Scanner so the body can be read from a JSONB column.
func (b *CustomCurrencyRuleBody) Scan(src interface{}) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, b)
	case string:
		return json.Unmarshal([]byte(v), b)
	case nil:
		return nil
	default:
		return fmt.Errorf("cannot scan %T into CustomCurrencyRuleBody", src)
	}
}
