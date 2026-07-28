package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// The analytics endpoint backs the whole Reports page from a single request.
// Every section is a small aggregate query scoped by analyticsScope; the only
// work done in Go is what SQL cannot express cheaply (airport/country lookup
// against the OurAirports dataset, month streaks, and the derived records).

const (
	defaultAnalyticsRowLimit = 25
	maxAnalyticsRowLimit     = 200
	maxAnalyticsMonths       = 600
	isoDate                  = "2006-01-02"
	isoMonth                 = "2006-01"
)

// analyticsScope carries the user and timeframe predicate shared by every
// query in this file. All queries alias the flights table as `f`.
type analyticsScope struct {
	userID uuid.UUID
	months int
	// filter is ANDed onto each WHERE clause; empty when covering all time.
	filter string
	args   []any
}

func newAnalyticsScope(userID uuid.UUID, months int) analyticsScope {
	s := analyticsScope{userID: userID, months: months, args: []any{userID}}
	if months > 0 {
		s.filter = " AND f.date >= date_trunc('month', CURRENT_DATE - ($2 || ' months')::interval)"
		s.args = append(s.args, months)
	}
	return s
}

func (s analyticsScope) allTime() bool { return s.months <= 0 }

// withLimit returns the scope args plus a row limit, and the placeholder that
// refers to it.
func (s analyticsScope) withLimit(limit int) ([]any, string) {
	args := make([]any, 0, len(s.args)+1)
	args = append(args, s.args...)
	args = append(args, limit)
	return args, fmt.Sprintf("$%d", len(args))
}

// from returns the first date covered by the timeframe, or the zero time when
// the scope covers all time.
func (s analyticsScope) from(now time.Time) time.Time {
	if s.allTime() {
		return time.Time{}
	}
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start.AddDate(0, -s.months, 0)
}

// GetFlightAnalytics implements GET /reports/analytics
func (h *APIHandler) GetFlightAnalytics(c *gin.Context, params generated.GetFlightAnalyticsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	months := 12
	if params.Months != nil {
		if *params.Months < 0 || *params.Months > maxAnalyticsMonths {
			h.sendError(c, http.StatusBadRequest, "months must be between 0 and 600")
			return
		}
		months = *params.Months
	}

	limit := defaultAnalyticsRowLimit
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > maxAnalyticsRowLimit {
			h.sendError(c, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = *params.Limit
	}

	ctx := c.Request.Context()
	scope := newAnalyticsScope(userID, months)
	now := time.Now().UTC()

	out := generated.FlightAnalytics{
		Range: generated.AnalyticsRange{
			Months:  months,
			AllTime: scope.allTime(),
			To:      strPtr(now.Format(isoDate)),
		},
	}
	if !scope.allTime() {
		out.Range.From = strPtr(scope.from(now).Format(isoDate))
	}

	// Each loader fills one section. They run sequentially against a single
	// pooled connection; a failure in any one is a 500 for the whole page
	// rather than a silently half-rendered report.
	type loader struct {
		name string
		run  func() error
	}
	loaders := []loader{
		{"totals", func() (err error) { out.Totals, err = h.analyticsTotals(ctx, scope); return }},
		{"monthly", func() (err error) { out.Monthly, err = h.analyticsMonthly(ctx, scope); return }},
		{"yearly", func() (err error) { out.Yearly, err = h.analyticsYearly(ctx, scope); return }},
		{"aircraft type", func() (err error) {
			out.ByAircraftType, err = h.analyticsByAircraft(ctx, scope, limit, false)
			return
		}},
		{"registration", func() (err error) {
			out.ByRegistration, err = h.analyticsByAircraft(ctx, scope, limit, true)
			return
		}},
		{"class", func() (err error) { out.ByClass, err = h.analyticsByClass(ctx, scope); return }},
		{"category", func() (err error) { out.ByCategory, err = h.analyticsByCategory(ctx, scope); return }},
		{"airports", func() (err error) { out.ByAirport, err = h.analyticsByAirport(ctx, scope, limit); return }},
		{"routes", func() (err error) { out.ByRoute, err = h.analyticsByRoute(ctx, scope, limit); return }},
		{"instructors", func() (err error) {
			out.ByInstructor, err = h.analyticsByInstructor(ctx, scope, limit)
			return
		}},
		{"crew", func() (err error) { out.ByCrew, err = h.analyticsByCrew(ctx, scope, limit); return }},
		{"approaches", func() (err error) { out.ApproachTypes, err = h.analyticsApproachTypes(ctx, scope); return }},
		{"patterns", func() error { return h.analyticsPatterns(ctx, scope, &out) }},
		{"records", func() (err error) { out.Records, err = h.analyticsRecords(ctx, scope, now); return }},
	}
	for _, l := range loaders {
		if err := l.run(); err != nil {
			h.sendError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to compute %s analytics", l.name))
			return
		}
	}

	// byCountry and the airport-derived records need the resolved airport
	// rows, so they are folded in once byAirport is populated.
	out.ByCountry = countriesFromAirports(out.ByAirport)
	applyAirportRecords(&out)

	// Cumulative career hours: everything logged before the timeframe plus
	// the user's initial-hours snapshot, so the curve is not truncated by the
	// selected range.
	carried, err := h.analyticsCarriedForwardMinutes(ctx, scope, now)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to compute cumulative hours")
		return
	}
	running := carried
	for i := range out.Monthly {
		running += out.Monthly[i].TotalMinutes
		out.Monthly[i].CumulativeMinutes = running
	}

	out.Totals.DistinctAirports = len(out.ByAirport)
	out.Totals.DistinctCountries = len(out.ByCountry)

	c.JSON(http.StatusOK, out)
}

// ── Totals ───────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsTotals(ctx context.Context, s analyticsScope) (generated.AnalyticsTotals, error) {
	var t generated.AnalyticsTotals
	var first, last sql.NullTime

	err := h.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(f.total_time), 0),
			COALESCE(SUM(f.pic_time), 0),
			COALESCE(SUM(f.sic_time), 0),
			COALESCE(SUM(f.dual_time), 0),
			COALESCE(SUM(f.dual_given_time), 0),
			COALESCE(SUM(f.solo_time), 0),
			COALESCE(SUM(f.night_time), 0),
			COALESCE(SUM(f.ifr_time), 0),
			COALESCE(SUM(f.actual_instrument_time), 0),
			COALESCE(SUM(f.simulated_instrument_time), 0),
			COALESCE(SUM(f.cross_country_time), 0),
			COALESCE(SUM(f.multi_pilot_time), 0),
			COALESCE(SUM(f.simulated_flight_time), 0),
			COALESCE(SUM(f.ground_training_time), 0),
			COALESCE(SUM(f.landings_day), 0),
			COALESCE(SUM(f.landings_night), 0),
			COALESCE(SUM(f.takeoffs_day), 0),
			COALESCE(SUM(f.takeoffs_night), 0),
			COALESCE(SUM(f.approaches_count), 0),
			COALESCE(SUM(f.holds), 0),
			COALESCE(SUM(f.distance), 0)::float8,
			COUNT(DISTINCT f.aircraft_reg),
			COUNT(DISTINCT f.aircraft_type),
			MIN(f.date), MAX(f.date)
		FROM flights f
		WHERE f.user_id = $1`+s.filter,
		s.args...,
	).Scan(
		&t.TotalFlights, &t.TotalMinutes, &t.PicMinutes, &t.SicMinutes, &t.DualMinutes,
		&t.DualGivenMinutes, &t.SoloMinutes, &t.NightMinutes, &t.IfrMinutes,
		&t.ActualInstrumentMinutes, &t.SimulatedInstrumentMinutes, &t.CrossCountryMinutes,
		&t.MultiPilotMinutes, &t.SimulatedFlightMinutes, &t.GroundTrainingMinutes,
		&t.LandingsDay, &t.LandingsNight, &t.TakeoffsDay, &t.TakeoffsNight,
		&t.Approaches, &t.Holds, &t.DistanceNm,
		&t.DistinctRegistrations, &t.DistinctTypes,
		&first, &last,
	)
	if err != nil {
		return t, err
	}
	if first.Valid {
		t.FirstFlightDate = strPtr(first.Time.Format(isoDate))
	}
	if last.Valid {
		t.LastFlightDate = strPtr(last.Time.Format(isoDate))
	}
	return t, nil
}

// ── Time series ──────────────────────────────────────────────────────────

func (h *APIHandler) analyticsMonthly(ctx context.Context, s analyticsScope) ([]generated.AnalyticsMonthPoint, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT
			TO_CHAR(date_trunc('month', f.date), 'YYYY-MM'),
			COUNT(*),
			COALESCE(SUM(f.total_time), 0),
			COALESCE(SUM(f.pic_time), 0),
			COALESCE(SUM(f.sic_time), 0),
			COALESCE(SUM(f.dual_time), 0),
			COALESCE(SUM(f.dual_given_time), 0),
			COALESCE(SUM(f.solo_time), 0),
			COALESCE(SUM(f.night_time), 0),
			COALESCE(SUM(f.ifr_time), 0),
			COALESCE(SUM(f.landings_day), 0),
			COALESCE(SUM(f.landings_night), 0),
			COALESCE(SUM(f.distance), 0)::float8
		FROM flights f
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY date_trunc('month', f.date)
		ORDER BY date_trunc('month', f.date) ASC`,
		s.args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []generated.AnalyticsMonthPoint{}
	for rows.Next() {
		var p generated.AnalyticsMonthPoint
		if err := rows.Scan(
			&p.Month, &p.Flights, &p.TotalMinutes, &p.PicMinutes, &p.SicMinutes,
			&p.DualMinutes, &p.DualGivenMinutes, &p.SoloMinutes, &p.NightMinutes,
			&p.IfrMinutes, &p.LandingsDay, &p.LandingsNight, &p.DistanceNm,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (h *APIHandler) analyticsYearly(ctx context.Context, s analyticsScope) ([]generated.AnalyticsYearPoint, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT
			EXTRACT(YEAR FROM f.date)::int,
			COUNT(*),
			COALESCE(SUM(f.total_time), 0),
			COALESCE(SUM(f.pic_time), 0),
			COALESCE(SUM(f.dual_time), 0),
			COALESCE(SUM(f.night_time), 0),
			COALESCE(SUM(f.ifr_time), 0),
			COALESCE(SUM(f.landings_day + f.landings_night), 0),
			COALESCE(SUM(f.distance), 0)::float8
		FROM flights f
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY 1
		ORDER BY 1 ASC`,
		s.args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []generated.AnalyticsYearPoint{}
	for rows.Next() {
		var p generated.AnalyticsYearPoint
		if err := rows.Scan(
			&p.Year, &p.Flights, &p.TotalMinutes, &p.PicMinutes, &p.DualMinutes,
			&p.NightMinutes, &p.IfrMinutes, &p.Landings, &p.DistanceNm,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// analyticsCarriedForwardMinutes returns the block time accumulated before the
// timeframe starts: the initial-hours snapshot plus every flight logged
// earlier than the first month in range.
func (h *APIHandler) analyticsCarriedForwardMinutes(ctx context.Context, s analyticsScope, now time.Time) (int, error) {
	var baseline int
	err := h.db.QueryRowContext(ctx,
		`SELECT COALESCE(total_minutes, 0) FROM flight_baselines WHERE user_id = $1`,
		s.userID,
	).Scan(&baseline)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if s.allTime() {
		return baseline, nil
	}

	var earlier int
	if err := h.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(f.total_time), 0)
		FROM flights f
		WHERE f.user_id = $1
		  AND f.date < date_trunc('month', CURRENT_DATE - ($2 || ' months')::interval)`,
		s.userID, s.months,
	).Scan(&earlier); err != nil {
		return 0, err
	}
	return baseline + earlier, nil
}

// ── Aircraft ─────────────────────────────────────────────────────────────

// analyticsByAircraft groups either by aircraft type or by registration. The
// registration variant joins the aircraft table to label each row with its
// type and make/model where the aircraft is known.
func (h *APIHandler) analyticsByAircraft(ctx context.Context, s analyticsScope, limit int, byRegistration bool) ([]generated.AnalyticsAircraftRow, error) {
	groupCol := "f.aircraft_type"
	subLabel := `NULLIF(TRIM(CONCAT_WS(' ', MIN(a.make), MIN(a.model))), '')`
	join := ""
	if byRegistration {
		groupCol = "f.aircraft_reg"
		// MIN() picks a stable representative when a registration has been
		// flown under more than one type designation.
		subLabel = `COALESCE(NULLIF(TRIM(CONCAT_WS(' ', MIN(a.make), MIN(a.model))), ''), MIN(f.aircraft_type))`
	}
	join = " LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id"

	args, limitPh := s.withLimit(limit)
	rows, err := h.db.QueryContext(ctx, `
		SELECT
			`+groupCol+`,
			`+subLabel+`,
			COUNT(*),
			COALESCE(SUM(f.total_time), 0),
			COALESCE(SUM(f.pic_time), 0),
			COALESCE(SUM(f.dual_time), 0),
			COALESCE(SUM(f.night_time), 0),
			COALESCE(SUM(f.ifr_time), 0),
			COALESCE(SUM(f.landings_day + f.landings_night), 0),
			COALESCE(SUM(f.distance), 0)::float8,
			MIN(f.date), MAX(f.date)
		FROM flights f`+join+`
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY `+groupCol+`
		ORDER BY COALESCE(SUM(f.total_time), 0) DESC, COUNT(*) DESC
		LIMIT `+limitPh,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []generated.AnalyticsAircraftRow{}
	for rows.Next() {
		var r generated.AnalyticsAircraftRow
		var sub sql.NullString
		var first, last sql.NullTime
		if err := rows.Scan(
			&r.Label, &sub, &r.Flights, &r.TotalMinutes, &r.PicMinutes, &r.DualMinutes,
			&r.NightMinutes, &r.IfrMinutes, &r.Landings, &r.DistanceNm, &first, &last,
		); err != nil {
			return nil, err
		}
		if sub.Valid && sub.String != r.Label {
			r.SubLabel = strPtr(sub.String)
		}
		if first.Valid {
			r.FirstFlightDate = strPtr(first.Time.Format(isoDate))
		}
		if last.Valid {
			r.LastFlightDate = strPtr(last.Time.Format(isoDate))
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (h *APIHandler) analyticsByClass(ctx context.Context, s analyticsScope) ([]generated.AnalyticsGroupRow, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT
			COALESCE(a.aircraft_class, 'Unclassified'),
			COUNT(*),
			COALESCE(SUM(f.total_time), 0),
			COALESCE(SUM(f.pic_time), 0),
			COALESCE(SUM(f.dual_time), 0),
			COALESCE(SUM(f.landings_day + f.landings_night), 0)
		FROM flights f
		LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY COALESCE(a.aircraft_class, 'Unclassified')
		ORDER BY 3 DESC`,
		s.args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupRows(rows)
}

func (h *APIHandler) analyticsByCategory(ctx context.Context, s analyticsScope) ([]generated.AnalyticsGroupRow, error) {
	// A flight can be tailwheel *and* complex, so the categories are unioned
	// rather than grouped — rows deliberately overlap.
	rows, err := h.db.QueryContext(ctx, `
		SELECT label, COUNT(*),
			COALESCE(SUM(total_time), 0),
			COALESCE(SUM(pic_time), 0),
			COALESCE(SUM(dual_time), 0),
			COALESCE(SUM(landings), 0)
		FROM (
			SELECT 'Tailwheel' AS label, f.total_time, f.pic_time, f.dual_time,
			       f.landings_day + f.landings_night AS landings
			FROM flights f
			JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND a.is_tailwheel`+s.filter+`
			UNION ALL
			SELECT 'Complex', f.total_time, f.pic_time, f.dual_time,
			       f.landings_day + f.landings_night
			FROM flights f
			JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND a.is_complex`+s.filter+`
			UNION ALL
			SELECT 'High Performance', f.total_time, f.pic_time, f.dual_time,
			       f.landings_day + f.landings_night
			FROM flights f
			JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND a.is_high_performance`+s.filter+`
		) sub
		GROUP BY label
		ORDER BY 3 DESC`,
		s.args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupRows(rows)
}

func scanGroupRows(rows *sql.Rows) ([]generated.AnalyticsGroupRow, error) {
	out := []generated.AnalyticsGroupRow{}
	for rows.Next() {
		var r generated.AnalyticsGroupRow
		if err := rows.Scan(&r.Label, &r.Flights, &r.TotalMinutes, &r.PicMinutes, &r.DualMinutes, &r.Landings); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── Places ───────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsByAirport(ctx context.Context, s analyticsScope, limit int) ([]generated.AnalyticsAirportRow, error) {
	// `flights` counts each flight once even when it departs and arrives at
	// the same airport, which is why the departure/arrival legs are counted
	// separately from the DISTINCT flight id.
	args, limitPh := s.withLimit(limit)
	rows, err := h.db.QueryContext(ctx, `
		SELECT icao,
			COALESCE(SUM(CASE WHEN role = 'dep' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN role = 'arr' THEN 1 ELSE 0 END), 0),
			COUNT(DISTINCT flight_id)
		FROM (
			SELECT UPPER(f.departure_icao) AS icao, 'dep' AS role, f.id AS flight_id
			FROM flights f
			WHERE f.user_id = $1 AND f.departure_icao IS NOT NULL AND f.departure_icao <> ''`+s.filter+`
			UNION ALL
			SELECT UPPER(f.arrival_icao), 'arr', f.id
			FROM flights f
			WHERE f.user_id = $1 AND f.arrival_icao IS NOT NULL AND f.arrival_icao <> ''`+s.filter+`
		) legs
		GROUP BY icao
		ORDER BY COUNT(DISTINCT flight_id) DESC, icao ASC
		LIMIT `+limitPh,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []generated.AnalyticsAirportRow{}
	for rows.Next() {
		var r generated.AnalyticsAirportRow
		if err := rows.Scan(&r.Icao, &r.Departures, &r.Arrivals, &r.Flights); err != nil {
			return nil, err
		}
		if ap := airports.Lookup(r.Icao); ap != nil {
			r.Name = strPtr(ap.Name)
			r.Latitude = f64Ptr(ap.Latitude)
			r.Longitude = f64Ptr(ap.Longitude)
			if ap.Country != "" {
				r.Country = strPtr(ap.Country)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (h *APIHandler) analyticsByRoute(ctx context.Context, s analyticsScope, limit int) ([]generated.AnalyticsRouteRow, error) {
	args, limitPh := s.withLimit(limit)
	rows, err := h.db.QueryContext(ctx, `
		SELECT UPPER(f.departure_icao), UPPER(f.arrival_icao),
			COUNT(*),
			COALESCE(SUM(f.total_time), 0),
			COALESCE(MAX(f.distance), 0)::float8
		FROM flights f
		WHERE f.user_id = $1
		  AND f.departure_icao IS NOT NULL AND f.departure_icao <> ''
		  AND f.arrival_icao IS NOT NULL AND f.arrival_icao <> ''`+s.filter+`
		GROUP BY 1, 2
		ORDER BY COUNT(*) DESC, COALESCE(SUM(f.total_time), 0) DESC
		LIMIT `+limitPh,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []generated.AnalyticsRouteRow{}
	for rows.Next() {
		var r generated.AnalyticsRouteRow
		if err := rows.Scan(&r.DepartureIcao, &r.ArrivalIcao, &r.Flights, &r.TotalMinutes, &r.DistanceNm); err != nil {
			return nil, err
		}
		// Fall back to the airport database when the leg predates distance
		// auto-calculation and has no stored value.
		if r.DistanceNm == 0 && r.DepartureIcao != r.ArrivalIcao {
			dep, arr := airports.Lookup(r.DepartureIcao), airports.Lookup(r.ArrivalIcao)
			if dep != nil && arr != nil {
				r.DistanceNm = airports.DistanceNM(dep.Latitude, dep.Longitude, arr.Latitude, arr.Longitude)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// countriesFromAirports rolls the resolved airport rows up by country.
// Airports the database does not know are skipped rather than bucketed into a
// synthetic "unknown" country.
func countriesFromAirports(rows []generated.AnalyticsAirportRow) []generated.AnalyticsCountryRow {
	type acc struct {
		airports int
		flights  int
	}
	byCountry := map[string]*acc{}
	for _, r := range rows {
		if r.Country == nil || *r.Country == "" {
			continue
		}
		a, ok := byCountry[*r.Country]
		if !ok {
			a = &acc{}
			byCountry[*r.Country] = a
		}
		a.airports++
		a.flights += r.Flights
	}

	out := make([]generated.AnalyticsCountryRow, 0, len(byCountry))
	for code, a := range byCountry {
		out = append(out, generated.AnalyticsCountryRow{
			Country:  code,
			Airports: a.airports,
			Flights:  a.flights,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Flights != out[j].Flights {
			return out[i].Flights > out[j].Flights
		}
		return out[i].Country < out[j].Country
	})
	return out
}

// ── People ───────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsByInstructor(ctx context.Context, s analyticsScope, limit int) ([]generated.AnalyticsPersonRow, error) {
	args, limitPh := s.withLimit(limit)
	rows, err := h.db.QueryContext(ctx, `
		SELECT TRIM(f.instructor_name), COUNT(*),
			COALESCE(SUM(f.dual_time), 0), MAX(f.date)
		FROM flights f
		WHERE f.user_id = $1 AND TRIM(COALESCE(f.instructor_name, '')) <> ''`+s.filter+`
		GROUP BY TRIM(f.instructor_name)
		ORDER BY 3 DESC, 2 DESC
		LIMIT `+limitPh,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPersonRows(rows, false)
}

func (h *APIHandler) analyticsByCrew(ctx context.Context, s analyticsScope, limit int) ([]generated.AnalyticsPersonRow, error) {
	args, limitPh := s.withLimit(limit)
	rows, err := h.db.QueryContext(ctx, `
		SELECT TRIM(cm.name), cm.role::text, COUNT(*),
			COALESCE(SUM(f.total_time), 0), MAX(f.date)
		FROM flight_crew_members cm
		JOIN flights f ON f.id = cm.flight_id
		WHERE f.user_id = $1 AND TRIM(COALESCE(cm.name, '')) <> ''`+s.filter+`
		GROUP BY TRIM(cm.name), cm.role
		ORDER BY 4 DESC, 3 DESC
		LIMIT `+limitPh,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPersonRows(rows, true)
}

func scanPersonRows(rows *sql.Rows, withRole bool) ([]generated.AnalyticsPersonRow, error) {
	out := []generated.AnalyticsPersonRow{}
	for rows.Next() {
		var r generated.AnalyticsPersonRow
		var role sql.NullString
		var last sql.NullTime
		var err error
		if withRole {
			err = rows.Scan(&r.Name, &role, &r.Flights, &r.TotalMinutes, &last)
		} else {
			err = rows.Scan(&r.Name, &r.Flights, &r.TotalMinutes, &last)
		}
		if err != nil {
			return nil, err
		}
		if role.Valid && role.String != "" {
			r.Role = strPtr(role.String)
		}
		if last.Valid {
			r.LastFlightDate = strPtr(last.Time.Format(isoDate))
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── Instrument ───────────────────────────────────────────────────────────

func (h *APIHandler) analyticsApproachTypes(ctx context.Context, s analyticsScope) ([]generated.AnalyticsApproachTypeRow, error) {
	// A flight with no structured approaches stores the JSON scalar `null`
	// rather than an empty array, and jsonb_array_elements rejects scalars —
	// so anything that is not an array is normalised away before unnesting.
	rows, err := h.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(elem->>'type', ''), 'Unknown'), COUNT(*)
		FROM flights f
		CROSS JOIN LATERAL jsonb_array_elements(
			CASE WHEN jsonb_typeof(f.approaches) = 'array'
			     THEN f.approaches ELSE '[]'::jsonb END
		) AS elem
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY 1
		ORDER BY 2 DESC, 1 ASC`,
		s.args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []generated.AnalyticsApproachTypeRow{}
	structured := 0
	for rows.Next() {
		var r generated.AnalyticsApproachTypeRow
		if err := rows.Scan(&r.Type, &r.Count); err != nil {
			return nil, err
		}
		structured += r.Count
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Flights logged before structured approaches existed carry only a count.
	// Surface the remainder so the breakdown adds up to totals.approaches.
	var counted int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(f.approaches_count), 0) FROM flights f WHERE f.user_id = $1`+s.filter,
		s.args...,
	).Scan(&counted); err != nil {
		return nil, err
	}
	if rest := counted - structured; rest > 0 {
		out = append(out, generated.AnalyticsApproachTypeRow{Type: "Unspecified", Count: rest})
	}
	return out, nil
}

// ── Patterns ─────────────────────────────────────────────────────────────

// analyticsPatterns fills the four distribution histograms. Day of week, month
// of year and duration buckets are dense (every bucket present, zeros
// included) so clients can render a fixed axis without gap-filling.
func (h *APIHandler) analyticsPatterns(ctx context.Context, s analyticsScope, out *generated.FlightAnalytics) error {
	dow, err := h.analyticsBuckets(ctx, s, "EXTRACT(ISODOW FROM f.date)::int", "")
	if err != nil {
		return err
	}
	out.DayOfWeek = denseBuckets(dow, 1, 7, dayOfWeekLabel)

	moy, err := h.analyticsBuckets(ctx, s, "EXTRACT(MONTH FROM f.date)::int", "")
	if err != nil {
		return err
	}
	out.MonthOfYear = denseBuckets(moy, 1, 12, monthOfYearLabel)

	hod, err := h.analyticsBuckets(ctx, s,
		"EXTRACT(HOUR FROM COALESCE(f.off_block_time, f.departure_time))::int",
		" AND COALESCE(f.off_block_time, f.departure_time) IS NOT NULL")
	if err != nil {
		return err
	}
	out.HourOfDay = denseBuckets(hod, 0, 23, func(k int) string { return fmt.Sprintf("%02d", k) })

	dur, err := h.analyticsBuckets(ctx, s, `CASE
			WHEN f.total_time < 30 THEN 0
			WHEN f.total_time < 60 THEN 1
			WHEN f.total_time < 120 THEN 2
			WHEN f.total_time < 180 THEN 3
			WHEN f.total_time < 300 THEN 4
			ELSE 5 END`, "")
	if err != nil {
		return err
	}
	out.DurationBuckets = denseBuckets(dur, 0, 5, durationBucketLabel)
	return nil
}

type bucketAgg struct {
	flights int
	minutes int
}

// analyticsBuckets groups flights by an arbitrary integer key expression.
func (h *APIHandler) analyticsBuckets(ctx context.Context, s analyticsScope, keyExpr, extra string) (map[int]bucketAgg, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT `+keyExpr+` AS k, COUNT(*), COALESCE(SUM(f.total_time), 0)
		FROM flights f
		WHERE f.user_id = $1`+s.filter+extra+`
		GROUP BY k`,
		s.args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int]bucketAgg{}
	for rows.Next() {
		var k int
		var a bucketAgg
		if err := rows.Scan(&k, &a.flights, &a.minutes); err != nil {
			return nil, err
		}
		out[k] = a
	}
	return out, rows.Err()
}

func denseBuckets(agg map[int]bucketAgg, lo, hi int, label func(int) string) []generated.AnalyticsBucketRow {
	out := make([]generated.AnalyticsBucketRow, 0, hi-lo+1)
	for k := lo; k <= hi; k++ {
		a := agg[k]
		out = append(out, generated.AnalyticsBucketRow{
			Key:          k,
			Label:        label(k),
			Flights:      a.flights,
			TotalMinutes: a.minutes,
		})
	}
	return out
}

func dayOfWeekLabel(k int) string {
	names := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	if k >= 1 && k <= 7 {
		return names[k-1]
	}
	return ""
}

func monthOfYearLabel(k int) string {
	if k >= 1 && k <= 12 {
		return time.Month(k).String()[:3]
	}
	return ""
}

func durationBucketLabel(k int) string {
	labels := []string{"<30m", "30-60m", "1-2h", "2-3h", "3-5h", ">5h"}
	if k >= 0 && k < len(labels) {
		return labels[k]
	}
	return ""
}

// ── Records ──────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsRecords(ctx context.Context, s analyticsScope, now time.Time) (generated.AnalyticsRecords, error) {
	var r generated.AnalyticsRecords

	longest, err := h.analyticsRecordFlight(ctx, s, "f.total_time")
	if err != nil {
		return r, err
	}
	r.LongestFlight = longest

	farthest, err := h.analyticsRecordFlight(ctx, s, "f.distance")
	if err != nil {
		return r, err
	}
	r.LongestDistanceFlight = farthest

	var busiestDay sql.NullTime
	var busiestDayFlights int
	err = h.db.QueryRowContext(ctx, `
		SELECT f.date, COUNT(*)
		FROM flights f
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY f.date
		ORDER BY COUNT(*) DESC, f.date DESC
		LIMIT 1`,
		s.args...,
	).Scan(&busiestDay, &busiestDayFlights)
	if err != nil && err != sql.ErrNoRows {
		return r, err
	}
	if busiestDay.Valid {
		r.BusiestDay = strPtr(busiestDay.Time.Format(isoDate))
		r.BusiestDayFlights = busiestDayFlights
	}

	// Days since last flight is deliberately measured against the whole
	// logbook — a narrow timeframe should not make a pilot look lapsed.
	var last sql.NullTime
	if err := h.db.QueryRowContext(ctx,
		`SELECT MAX(f.date) FROM flights f WHERE f.user_id = $1`, s.userID,
	).Scan(&last); err != nil {
		return r, err
	}
	if last.Valid {
		days := int(now.Truncate(24*time.Hour).Sub(last.Time.UTC().Truncate(24*time.Hour)).Hours() / 24)
		if days < 0 {
			days = 0
		}
		r.DaysSinceLastFlight = &days
	}
	return r, nil
}

func (h *APIHandler) analyticsRecordFlight(ctx context.Context, s analyticsScope, orderBy string) (*generated.AnalyticsFlightRef, error) {
	var (
		ref            generated.AnalyticsFlightRef
		id             uuid.UUID
		date           time.Time
		reg, typ       sql.NullString
		dep, arr       sql.NullString
		minutes        int
		distance       float64
		hasResultAtAll = true
	)
	err := h.db.QueryRowContext(ctx, `
		SELECT f.id, f.date, f.aircraft_reg, f.aircraft_type,
		       f.departure_icao, f.arrival_icao, f.total_time, f.distance::float8
		FROM flights f
		WHERE f.user_id = $1`+s.filter+`
		ORDER BY `+orderBy+` DESC, f.date DESC
		LIMIT 1`,
		s.args...,
	).Scan(&id, &date, &reg, &typ, &dep, &arr, &minutes, &distance)
	if err == sql.ErrNoRows {
		hasResultAtAll = false
	} else if err != nil {
		return nil, err
	}
	if !hasResultAtAll {
		return nil, nil
	}

	ref.Id = openapi_types.UUID(id)
	ref.Date = date.Format(isoDate)
	ref.TotalMinutes = minutes
	ref.DistanceNm = distance
	if reg.Valid {
		ref.AircraftReg = strPtr(reg.String)
	}
	if typ.Valid {
		ref.AircraftType = strPtr(typ.String)
	}
	if dep.Valid && dep.String != "" {
		ref.DepartureIcao = strPtr(strings.ToUpper(dep.String))
	}
	if arr.Valid && arr.String != "" {
		ref.ArrivalIcao = strPtr(strings.ToUpper(arr.String))
	}
	return &ref, nil
}

// applyAirportRecords derives the streak, busiest-period and home-base records
// from sections already loaded, so they stay consistent with what the page
// renders rather than being recomputed from a separate query.
func applyAirportRecords(out *generated.FlightAnalytics) {
	rec := &out.Records

	// Busiest month / year, taken from the series the charts use.
	for _, m := range out.Monthly {
		if rec.BusiestMonthMinutes == nil || m.TotalMinutes > *rec.BusiestMonthMinutes {
			rec.BusiestMonth = strPtr(m.Month)
			rec.BusiestMonthMinutes = intPtr(m.TotalMinutes)
		}
	}
	for _, y := range out.Yearly {
		if rec.BusiestYearMinutes == nil || y.TotalMinutes > *rec.BusiestYearMinutes {
			year := y.Year
			rec.BusiestYear = &year
			rec.BusiestYearMinutes = intPtr(y.TotalMinutes)
		}
	}

	rec.ActiveMonths = len(out.Monthly)
	rec.LongestStreakMonths, rec.CurrentStreakMonths = monthStreaks(out.Monthly, out.Range.To)

	// Home base is the most-visited airport; byAirport is already sorted by
	// flight count, so the farthest airport is measured from there.
	if len(out.ByAirport) == 0 {
		return
	}
	home := out.ByAirport[0]
	rec.HomeBase = strPtr(home.Icao)
	if home.Latitude == nil || home.Longitude == nil {
		return
	}
	var best *generated.AnalyticsAirportRow
	bestNM := 0.0
	for i := range out.ByAirport {
		a := out.ByAirport[i]
		if a.Latitude == nil || a.Longitude == nil {
			continue
		}
		d := airports.DistanceNM(*home.Latitude, *home.Longitude, *a.Latitude, *a.Longitude)
		if d > bestNM {
			bestNM = d
			best = &out.ByAirport[i]
		}
	}
	if best != nil {
		rec.FarthestAirport = best
		rec.FarthestAirportNm = f64Ptr(bestNM)
	}
}

// monthStreaks returns the longest run of consecutive months containing at
// least one flight, and the run ending at the current month. Monthly is
// expected to be sorted oldest first and to contain only months that have
// flights, so a gap in the sequence ends a streak.
func monthStreaks(monthly []generated.AnalyticsMonthPoint, to *string) (longest, current int) {
	if len(monthly) == 0 {
		return 0, 0
	}

	parse := func(s string) (time.Time, bool) {
		t, err := time.Parse(isoMonth, s)
		return t, err == nil
	}

	run := 0
	var prev time.Time
	for _, m := range monthly {
		t, ok := parse(m.Month)
		if !ok {
			continue
		}
		if run > 0 && prev.AddDate(0, 1, 0).Equal(t) {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
		prev = t
	}

	// The trailing run only counts as "current" when it reaches this month or
	// the one before it — a pilot who last flew in March is not on a streak.
	if to == nil {
		return longest, 0
	}
	now, err := time.Parse(isoDate, *to)
	if err != nil {
		return longest, 0
	}
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if prev.Equal(thisMonth) || prev.AddDate(0, 1, 0).Equal(thisMonth) {
		current = run
	}
	return longest, current
}

// ── Small helpers ────────────────────────────────────────────────────────

func strPtr(s string) *string   { return &s }
func intPtr(i int) *int         { return &i }
func f64Ptr(f float64) *float64 { return &f }
