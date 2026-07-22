package currency

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// This file evaluates user-authored ("custom") currency rules. Unlike the
// regulatory evaluators, the rule shape here is user data, so the query is
// assembled at runtime. Safety rests on two rules:
//
//  1. Every identifier that reaches SQL comes from a fixed lookup table keyed
//     by the rule's controlled vocabulary — no user string is ever interpolated
//     as a column, table, or operator.
//  2. Every user-supplied value is bound as a query parameter.
//
// The rule body is validated (models.CustomCurrencyRuleBody.Validate) before it
// is ever persisted or evaluated, so the maps below are guaranteed to contain
// any identifier the evaluator sees; a miss is treated as an internal error.

// metricSQL maps a metric identifier to its aggregate expression over the
// joined flights (f) / aircraft (a) rows. Time metrics aggregate minutes.
var metricSQL = map[string]string{
	"flights":            "COUNT(*)",
	"total_time":         "COALESCE(SUM(f.total_time), 0)",
	"pic_time":           "COALESCE(SUM(f.pic_time), 0)",
	"dual_time":          "COALESCE(SUM(f.dual_time), 0)",
	"night_time":         "COALESCE(SUM(f.night_time), 0)",
	"ifr_time":           "COALESCE(SUM(f.ifr_time), 0)",
	"cross_country_time": "COALESCE(SUM(f.cross_country_time), 0)",
	"landings":           "COALESCE(SUM(f.landings_day + f.landings_night), 0)",
	"day_landings":       "COALESCE(SUM(f.landings_day), 0)",
	"night_landings":     "COALESCE(SUM(f.landings_night), 0)",
	"takeoffs":           "COALESCE(SUM(f.takeoffs_day + f.takeoffs_night), 0)",
	"day_takeoffs":       "COALESCE(SUM(f.takeoffs_day), 0)",
	"night_takeoffs":     "COALESCE(SUM(f.takeoffs_night), 0)",
	"approaches":         "COALESCE(SUM(f.approaches_count), 0)",
	"holds":              "COALESCE(SUM(f.holds), 0)",
}

// filterColumn maps value-bearing filter fields (eq/in) to their SQL column.
var filterColumn = map[string]string{
	"aircraft_class":        "a.aircraft_class",
	"aircraft_type":         "f.aircraft_type",
	"aircraft_registration": "f.aircraft_reg",
	"launch_method":         "f.launch_method",
}

// boolPredicate maps is_true filter fields to a fixed boolean SQL predicate.
var boolPredicate = map[string]string{
	"aircraft_complex":          "a.is_complex = true",
	"aircraft_high_performance": "a.is_high_performance = true",
	"aircraft_tailwheel":        "a.is_tailwheel = true",
	"is_pic":                    "f.is_pic = true",
	"is_dual":                   "f.is_dual = true",
	"has_night":                 "f.night_time > 0",
	"has_ifr":                   "f.ifr_time > 0",
	"is_cross_country":          "f.cross_country_time > 0",
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

// metricRowSQL maps a metric to its per-flight (non-aggregated) contribution
// expression, used to compute when experience ages out of the window. It must
// stay consistent with metricSQL — the same fields, without SUM/COUNT.
var metricRowSQL = map[string]string{
	"flights":            "1",
	"total_time":         "f.total_time",
	"pic_time":           "f.pic_time",
	"dual_time":          "f.dual_time",
	"night_time":         "f.night_time",
	"ifr_time":           "f.ifr_time",
	"cross_country_time": "f.cross_country_time",
	"landings":           "(f.landings_day + f.landings_night)",
	"day_landings":       "f.landings_day",
	"night_landings":     "f.landings_night",
	"takeoffs":           "(f.takeoffs_day + f.takeoffs_night)",
	"day_takeoffs":       "f.takeoffs_day",
	"night_takeoffs":     "f.takeoffs_night",
	"approaches":         "f.approaches_count",
	"holds":              "f.holds",
}

// expiringThresholdDays returns how far ahead a lapse counts as "expiring
// soon": 30 days, but never more than half the window (so short-window rules
// don't sit permanently in the amber state), and at least 1 day.
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
	db  *sql.DB
	now func() time.Time
}

// NewCustomEvaluator creates an evaluator backed by the given database.
func NewCustomEvaluator(db *sql.DB) *CustomEvaluator {
	return &CustomEvaluator{db: db, now: time.Now}
}

// Evaluate runs the rule for the given user and returns the currency result.
// The body is expected to have passed models validation already.
func (e *CustomEvaluator) Evaluate(ctx context.Context, userID uuid.UUID, body *models.CustomCurrencyRuleBody) (CustomCurrencyResult, error) {
	since := windowSince(e.now().UTC(), body.Window)

	// Collect the distinct metrics referenced by the requirements so each is
	// aggregated exactly once, then map results back per requirement.
	metricIndex := map[string]int{}
	var metrics []string
	for _, r := range body.Requirements {
		if _, ok := metricIndex[r.Metric]; !ok {
			metricIndex[r.Metric] = len(metrics)
			metrics = append(metrics, r.Metric)
		}
	}

	selects := make([]string, len(metrics))
	for i, m := range metrics {
		expr, ok := metricSQL[m]
		if !ok {
			return CustomCurrencyResult{}, fmt.Errorf("unsupported metric %q", m)
		}
		selects[i] = fmt.Sprintf("%s AS m%d", expr, i)
	}

	whereSQL, args, err := buildWhere(userID, since, body.Filters)
	if err != nil {
		return CustomCurrencyResult{}, err
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM flights f
		LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE %s
	`, strings.Join(selects, ", "), whereSQL)

	values := make([]int64, len(metrics))
	dests := make([]interface{}, len(metrics))
	for i := range values {
		dests[i] = &values[i]
	}
	if err := e.db.QueryRowContext(ctx, query, args...).Scan(dests...); err != nil {
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

// buildWhere assembles the parameterized WHERE clause shared by the aggregate
// and per-flight queries. The first two parameters are always user id and the
// window start; filter values follow.
func buildWhere(userID uuid.UUID, since time.Time, filters []models.CurrencyFilter) (string, []interface{}, error) {
	args := []interface{}{userID, since}
	where := []string{"f.user_id = $1", "f.date >= $2"}
	for _, f := range filters {
		clause, err := buildFilterClause(f, &args)
		if err != nil {
			return "", nil, err
		}
		where = append(where, clause)
	}
	return strings.Join(where, " AND "), args, nil
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
	rowSelects := make([]string, len(metrics))
	for i, m := range metrics {
		expr, ok := metricRowSQL[m]
		if !ok {
			return nil, fmt.Errorf("unsupported metric %q", m)
		}
		rowSelects[i] = fmt.Sprintf("%s AS m%d", expr, i)
	}

	whereSQL, args, err := buildWhere(userID, since, body.Filters)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT f.date, %s
		FROM flights f
		LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE %s
		ORDER BY f.date ASC
	`, strings.Join(rowSelects, ", "), whereSQL)

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type flightRow struct {
		date time.Time
		vals []int64
	}
	var flights []flightRow
	for rows.Next() {
		vals := make([]int64, len(metrics))
		dests := make([]interface{}, len(metrics)+1)
		var date time.Time
		dests[0] = &date
		for i := range vals {
			dests[i+1] = &vals[i]
		}
		if err := rows.Scan(dests...); err != nil {
			return nil, err
		}
		flights = append(flights, flightRow{date: date, vals: vals})
	}
	if err := rows.Err(); err != nil {
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
			c := fr.vals[mi]
			if c == 0 {
				continue
			}
			removed += float64(c)
			if removed > excess {
				leave := windowEnd(fr.date, body.Window)
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

// buildFilterClause returns the SQL predicate for a filter, appending any bound
// values to args. Placeholders are numbered from the current arg count.
func buildFilterClause(f models.CurrencyFilter, args *[]interface{}) (string, error) {
	switch f.Op {
	case "is_true":
		pred, ok := boolPredicate[f.Field]
		if !ok {
			return "", fmt.Errorf("unsupported boolean filter %q", f.Field)
		}
		return pred, nil
	case "eq":
		col, ok := filterColumn[f.Field]
		if !ok {
			return "", fmt.Errorf("unsupported filter field %q", f.Field)
		}
		*args = append(*args, f.Value)
		return fmt.Sprintf("%s = $%d", col, len(*args)), nil
	case "in":
		col, ok := filterColumn[f.Field]
		if !ok {
			return "", fmt.Errorf("unsupported filter field %q", f.Field)
		}
		placeholders := make([]string, len(f.Values))
		for i, v := range f.Values {
			*args = append(*args, v)
			placeholders[i] = fmt.Sprintf("$%d", len(*args))
		}
		return fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ", ")), nil
	default:
		return "", fmt.Errorf("unsupported operator %q", f.Op)
	}
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
