package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
)

// This file runs the queries behind user-authored ("custom") currency rules.
// Unlike the regulatory evaluators, the rule shape here is user data, so the
// query is assembled at runtime. Safety rests on two rules:
//
//  1. Every identifier that reaches SQL comes from a fixed lookup table keyed
//     by the rule's controlled vocabulary — no user string is ever interpolated
//     as a column, table, or operator.
//  2. Every user-supplied value is bound as a query parameter.
//
// The rule body is validated (models.CustomCurrencyRuleBody.Validate) before it
// is ever persisted or evaluated, so the maps below are guaranteed to contain
// any identifier this provider sees; a miss is treated as an internal error.

// customMetricSQL maps a metric identifier to its aggregate expression over
// the joined flights (f) / aircraft (a) rows. Time metrics aggregate minutes.
var customMetricSQL = map[string]string{
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

// customMetricRowSQL maps a metric to its per-flight (non-aggregated)
// contribution expression, used to compute when experience ages out of the
// window. It must stay consistent with customMetricSQL — the same fields,
// without SUM/COUNT.
var customMetricRowSQL = map[string]string{
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

// customFilterColumn maps value-bearing filter fields (eq/in) to their SQL column.
var customFilterColumn = map[string]string{
	"aircraft_class":        "a.aircraft_class",
	"aircraft_type":         "f.aircraft_type",
	"aircraft_registration": "f.aircraft_reg",
	"launch_method":         "f.launch_method",
}

// customBoolPredicate maps is_true filter fields to a fixed boolean SQL predicate.
var customBoolPredicate = map[string]string{
	"aircraft_complex":          "a.is_complex = true",
	"aircraft_high_performance": "a.is_high_performance = true",
	"aircraft_tailwheel":        "a.is_tailwheel = true",
	"is_pic":                    "f.is_pic = true",
	"is_dual":                   "f.is_dual = true",
	"has_night":                 "f.night_time > 0",
	"has_ifr":                   "f.ifr_time > 0",
	"is_cross_country":          "f.cross_country_time > 0",
}

// customCurrencyDataProvider implements currency.CustomFlightDataProvider.
type customCurrencyDataProvider struct {
	db *sql.DB
}

// NewCustomCurrencyDataProvider creates the flight-data provider the custom
// currency rule evaluator runs on.
func NewCustomCurrencyDataProvider(db *sql.DB) currency.CustomFlightDataProvider {
	return &customCurrencyDataProvider{db: db}
}

func (p *customCurrencyDataProvider) AggregateMetrics(ctx context.Context, userID uuid.UUID, since time.Time, metrics []string, filters []models.CurrencyFilter) ([]int64, error) {
	selects := make([]string, len(metrics))
	for i, m := range metrics {
		expr, ok := customMetricSQL[m]
		if !ok {
			return nil, fmt.Errorf("unsupported metric %q", m)
		}
		selects[i] = fmt.Sprintf("%s AS m%d", expr, i)
	}

	whereSQL, args, err := buildCustomWhere(userID, since, filters)
	if err != nil {
		return nil, err
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
	if err := p.db.QueryRowContext(ctx, query, args...).Scan(dests...); err != nil {
		return nil, err
	}
	return values, nil
}

func (p *customCurrencyDataProvider) MetricRowsByDate(ctx context.Context, userID uuid.UUID, since time.Time, metrics []string, filters []models.CurrencyFilter) ([]currency.FlightMetricRow, error) {
	rowSelects := make([]string, len(metrics))
	for i, m := range metrics {
		expr, ok := customMetricRowSQL[m]
		if !ok {
			return nil, fmt.Errorf("unsupported metric %q", m)
		}
		rowSelects[i] = fmt.Sprintf("%s AS m%d", expr, i)
	}

	whereSQL, args, err := buildCustomWhere(userID, since, filters)
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

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flights []currency.FlightMetricRow
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
		flights = append(flights, currency.FlightMetricRow{Date: date, Values: vals})
	}
	return flights, rows.Err()
}

// buildCustomWhere assembles the parameterized WHERE clause shared by the
// aggregate and per-flight queries. The first two parameters are always user
// id and the window start; filter values follow.
func buildCustomWhere(userID uuid.UUID, since time.Time, filters []models.CurrencyFilter) (string, []interface{}, error) {
	args := []interface{}{userID, since}
	where := []string{"f.user_id = $1", "f.date >= $2"}
	for _, f := range filters {
		clause, err := buildCustomFilterClause(f, &args)
		if err != nil {
			return "", nil, err
		}
		where = append(where, clause)
	}
	return strings.Join(where, " AND "), args, nil
}

// buildCustomFilterClause returns the SQL predicate for a filter, appending any
// bound values to args. Placeholders are numbered from the current arg count.
func buildCustomFilterClause(f models.CurrencyFilter, args *[]interface{}) (string, error) {
	switch f.Op {
	case "is_true":
		pred, ok := customBoolPredicate[f.Field]
		if !ok {
			return "", fmt.Errorf("unsupported boolean filter %q", f.Field)
		}
		return pred, nil
	case "eq":
		col, ok := customFilterColumn[f.Field]
		if !ok {
			return "", fmt.Errorf("unsupported filter field %q", f.Field)
		}
		*args = append(*args, f.Value)
		return fmt.Sprintf("%s = $%d", col, len(*args)), nil
	case "in":
		col, ok := customFilterColumn[f.Field]
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
