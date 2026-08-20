package currency

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// This file evaluates user-authored ("custom") currency rules. Metrics and
// filters are identifiers from the rule's controlled vocabulary; the flight
// aggregates they name are fetched through CustomFlightDataProvider,
// implemented in internal/repository/postgres. The rule body is validated
// (models.CustomCurrencyRuleBody.Validate) before it is persisted or
// evaluated; an unknown identifier is an internal error.

// CustomFlightDataProvider supplies the flight aggregates a custom rule
// evaluation needs. Metrics are identifiers from the rule vocabulary (e.g.
// "landings", "pic_time"); time metrics are returned in minutes.
type CustomFlightDataProvider interface {
	// AggregateMetrics returns one aggregated value per requested metric for
	// the user's flights on or after `since`, restricted by the filters.
	AggregateMetrics(ctx context.Context, userID uuid.UUID, since time.Time, metrics []string, filters []models.CurrencyFilter) ([]int64, error)

	// MetricRowsByDate returns each matching flight's per-metric contribution,
	// ordered by flight date ascending, for lapse-date computation.
	MetricRowsByDate(ctx context.Context, userID uuid.UUID, since time.Time, metrics []string, filters []models.CurrencyFilter) ([]FlightMetricRow, error)
}

// FlightMetricRow is one flight's contribution to each requested metric,
// index-aligned with the metrics slice passed to MetricRowsByDate.
type FlightMetricRow struct {
	Date   time.Time
	Values []int64
}

// metricLabels provides a human-friendly default name for each metric, used
// when a requirement does not supply its own label.
var metricLabels = map[string]string{
	"flights":            "Flights",
	"total_time":         "Total time",
	"pic_time":           "PIC time",
	"dual_time":          "Dual time",
	"night_time":         "Night time",
	"ifr_time":           "Instrument time",
	"cross_country_time": "Cross-country time",
	"landings":           "Landings",
	"day_landings":       "Day landings",
	"night_landings":     "Night landings",
	"takeoffs":           "Takeoffs",
	"day_takeoffs":       "Day takeoffs",
	"night_takeoffs":     "Night takeoffs",
	"approaches":         "Approaches",
	"holds":              "Holds",
}

// CustomCurrencyResult is the evaluation outcome for a single custom rule. It
// reuses the shared Requirement type so the frontend can render custom rules
// with the same progress components as regulatory currency.
type CustomCurrencyResult struct {
	Status       Status        `json:"status"`
	WindowLabel  string        `json:"windowLabel"`
	Requirements []Requirement `json:"requirements"`
	// ExpiresOn is the last date the rule remains current assuming no further
	// flights — the earliest date any requirement drops below its threshold as
	// experience ages out of the rolling window. Set only when the rule is
	// currently met and a lapse date is computable; nil otherwise.
	ExpiresOn   *string `json:"expiresOn,omitempty"`
	EvaluatedAt string  `json:"evaluatedAt"`
}

// expiringThresholdDays returns how far ahead a lapse counts as "expiring
// soon": 30 days, but never more than half the window, and at least 1 day.
func expiringThresholdDays(w models.CurrencyWindow) int {
	days := windowDays(w)
	t := days / 2
	if t > 30 {
		t = 30
	}
	if t < 1 {
		t = 1
	}
	return t
}

// windowDays approximates a window's length in days for threshold math.
func windowDays(w models.CurrencyWindow) int {
	switch w.Unit {
	case "weeks":
		return w.Amount * 7
	case "months":
		return w.Amount * 30
	case "years":
		return w.Amount * 365
	default:
		return w.Amount
	}
}

// CustomEvaluator evaluates a validated rule body against a user's flights.
type CustomEvaluator struct {
	data CustomFlightDataProvider
	now  func() time.Time
}

// NewCustomEvaluator creates an evaluator backed by the given data provider.
func NewCustomEvaluator(data CustomFlightDataProvider) *CustomEvaluator {
	return &CustomEvaluator{data: data, now: time.Now}
}

// Evaluate runs the rule for the given user and returns the currency result.
// The body is expected to have passed models validation already.
func (e *CustomEvaluator) Evaluate(ctx context.Context, userID uuid.UUID, body *models.CustomCurrencyRuleBody) (CustomCurrencyResult, error) {
	since := windowSince(e.now().UTC(), body.Window)

	// Aggregate each distinct metric exactly once, then map results back per
	// requirement.
	metricIndex := map[string]int{}
	var metrics []string
	for _, r := range body.Requirements {
		if _, ok := metricIndex[r.Metric]; !ok {
			metricIndex[r.Metric] = len(metrics)
			metrics = append(metrics, r.Metric)
		}
	}

	values, err := e.data.AggregateMetrics(ctx, userID, since, metrics, body.Filters)
	if err != nil {
		return CustomCurrencyResult{}, err
	}

	reqs := make([]Requirement, 0, len(body.Requirements))
	allMet := true
	for _, r := range body.Requirements {
		raw := values[metricIndex[r.Metric]]
		req := buildCustomRequirement(r, raw)
		if !req.Met {
			allMet = false
		}
		reqs = append(reqs, req)
	}

	result := CustomCurrencyResult{
		Status:       StatusExpired,
		WindowLabel:  windowLabel(body.Window),
		Requirements: reqs,
		EvaluatedAt:  e.now().UTC().Format(time.RFC3339),
	}
	if !allMet {
		return result, nil
	}
	result.Status = StatusCurrent

	// The rule is currently met: determine when it will lapse as experience
	// ages out of the rolling window, and flag "expiring" if that is soon.
	expiresOn, err := e.computeExpiry(ctx, userID, since, body, metrics, metricIndex, values)
	if err != nil {
		return result, err
	}
	if expiresOn != nil {
		exp := expiresOn.Format("2006-01-02")
		result.ExpiresOn = &exp
		if daysBetween(e.now().UTC(), *expiresOn) <= expiringThresholdDays(body.Window) {
			result.Status = StatusExpiring
		}
	}
	return result, nil
}

// computeExpiry finds the earliest date any met requirement will fall below its
// threshold as flights age out of the window, assuming no new flights. It
// fetches the per-flight contribution of each metric within the window and,
// per requirement, removes the oldest flights until the running total would
// drop below the threshold; the flight that tips it determines that
// requirement's lapse date. The rule's expiry is the earliest across
// requirements. Returns nil if no lapse is computable.
func (e *CustomEvaluator) computeExpiry(
	ctx context.Context, userID uuid.UUID, since time.Time,
	body *models.CustomCurrencyRuleBody, metrics []string, metricIndex map[string]int, totals []int64,
) (*time.Time, error) {
	flights, err := e.data.MetricRowsByDate(ctx, userID, since, metrics, body.Filters)
	if err != nil {
		return nil, err
	}

	var earliest *time.Time
	for _, r := range body.Requirements {
		mi := metricIndex[r.Metric]
		threshold := rawThreshold(r)
		total := totals[mi]
		// Flights are ordered oldest→newest; they leave the window in that
		// order. Remove contributions until the remaining total would fall
		// below the threshold; that flight's leave date is the lapse date.
		excess := float64(total) - threshold
		var removed float64
		for _, fr := range flights {
			c := fr.Values[mi]
			if c == 0 {
				continue
			}
			removed += float64(c)
			if removed > excess {
				leave := windowEnd(fr.Date, body.Window)
				if earliest == nil || leave.Before(*earliest) {
					l := leave
					earliest = &l
				}
				break
			}
		}
	}
	return earliest, nil
}

// rawThreshold returns a requirement's threshold in the metric's raw storage
// unit (minutes for time metrics, a count otherwise).
func rawThreshold(r models.CurrencyRequirement) float64 {
	if models.IsTimeMetric(r.Metric) && r.Unit != "minutes" {
		return r.Min * 60.0
	}
	return r.Min
}

// windowEnd returns the last instant a flight on the given date remains inside
// a rolling window of the given length — flightDate + window length. It mirrors
// windowSince (which subtracts the same span from now).
func windowEnd(flightDate time.Time, w models.CurrencyWindow) time.Time {
	switch w.Unit {
	case "weeks":
		return flightDate.AddDate(0, 0, 7*w.Amount)
	case "months":
		return flightDate.AddDate(0, w.Amount, 0)
	case "years":
		return flightDate.AddDate(w.Amount, 0, 0)
	default:
		return flightDate.AddDate(0, 0, w.Amount)
	}
}

// daysBetween returns the number of whole days from now until t (may be
// negative if t is in the past).
func daysBetween(now, t time.Time) int {
	return int(t.Sub(now).Hours() / 24)
}

// buildCustomRequirement converts an aggregated raw value (minutes for time
// metrics, a count otherwise) into a user-facing Requirement.
func buildCustomRequirement(r models.CurrencyRequirement, raw int64) Requirement {
	name := r.Label
	if name == "" {
		name = metricLabels[r.Metric]
	}

	var current float64
	var unit string
	if models.IsTimeMetric(r.Metric) {
		if r.Unit == "minutes" {
			current = float64(raw)
			unit = "minutes"
		} else {
			current = float64(raw) / 60.0
			unit = "hours"
		}
	} else {
		current = float64(raw)
	}

	met := current >= r.Min
	return Requirement{
		Name:     name,
		Met:      met,
		Current:  current,
		Required: r.Min,
		Unit:     unit,
		Message:  fmt.Sprintf("%s / %s %s", formatAmount(current), formatAmount(r.Min), unit),
	}
}

// formatAmount renders a float without a trailing ".0" for whole numbers.
func formatAmount(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}

// windowSince resolves a rolling window to its earliest included instant.
func windowSince(now time.Time, w models.CurrencyWindow) time.Time {
	switch w.Unit {
	case "weeks":
		return now.AddDate(0, 0, -7*w.Amount)
	case "months":
		return now.AddDate(0, -w.Amount, 0)
	case "years":
		return now.AddDate(-w.Amount, 0, 0)
	default: // days
		return now.AddDate(0, 0, -w.Amount)
	}
}

// windowLabel renders a window as a short phrase, e.g. "last 90 days".
func windowLabel(w models.CurrencyWindow) string {
	unit := w.Unit
	if w.Amount == 1 && strings.HasSuffix(unit, "s") {
		unit = strings.TrimSuffix(unit, "s")
	}
	return fmt.Sprintf("last %d %s", w.Amount, unit)
}
