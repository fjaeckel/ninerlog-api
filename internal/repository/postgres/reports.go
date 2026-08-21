package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"time"
)

type reportsRepository struct {
	db *sql.DB
}

// NewReportsRepository creates the read-only repository behind the reports,
// analytics and map endpoints.
func NewReportsRepository(db *sql.DB) repository.ReportsRepository {
	return &reportsRepository{db: db}
}

// notSimulator excludes FSTD sessions from flight aggregates. Session time
// is recorded separately and is never summed with flight time
// (EASA AMC1 FCL.050); the Go-side counterpart is
// flightrules.CountsAsFlightTime.
const notSimulator = " AND NOT f.is_simulator"

// reportScope carries the user and timeframe predicate shared by the
// timeframe-aware queries. All queries alias the flights table as `f`.
type reportScope struct {
	userID uuid.UUID
	months int
	// filter is ANDed onto each WHERE clause. Always excludes FSTD
	// sessions; additionally bounds the timeframe unless covering all time.
	filter string
	args   []any
}

func newReportScope(userID uuid.UUID, months int) reportScope {
	s := reportScope{userID: userID, months: months, args: []any{userID}, filter: notSimulator}
	if months > 0 {
		s.filter += " AND f.date >= date_trunc('month', CURRENT_DATE - ($2 || ' months')::interval)"
		s.args = append(s.args, months)
	}
	return s
}

// withLimit returns the scope args plus a row limit, and the placeholder that
// refers to it.
func (s reportScope) withLimit(limit int) ([]any, string) {
	args := make([]any, 0, len(s.args)+1)
	args = append(args, s.args...)
	args = append(args, limit)
	return args, fmt.Sprintf("$%d", len(args))
}

// ── Map views ────────────────────────────────────────────────────────────

func (r *reportsRepository) RouteCounts(ctx context.Context, userID uuid.UUID) ([]*repository.RouteCount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT departure_icao, arrival_icao, COUNT(*) AS flight_count
		FROM flights
		WHERE user_id = $1 AND NOT is_simulator
		  AND departure_icao IS NOT NULL AND departure_icao != ''
		  AND arrival_icao IS NOT NULL AND arrival_icao != ''
		GROUP BY departure_icao, arrival_icao
		ORDER BY flight_count DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []*repository.RouteCount
	for rows.Next() {
		rc := &repository.RouteCount{}
		if err := rows.Scan(&rc.DepartureICAO, &rc.ArrivalICAO, &rc.FlightCount); err != nil {
			continue
		}
		routes = append(routes, rc)
	}
	return routes, rows.Err()
}

func (r *reportsRepository) AirportCounts(ctx context.Context, userID uuid.UUID) ([]*repository.AirportDirectionCount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT icao, direction, COUNT(*) AS cnt FROM (
			SELECT departure_icao AS icao, 'dep' AS direction FROM flights
			WHERE user_id = $1 AND NOT is_simulator AND departure_icao IS NOT NULL AND departure_icao != ''
			UNION ALL
			SELECT arrival_icao AS icao, 'arr' AS direction FROM flights
			WHERE user_id = $1 AND NOT is_simulator AND arrival_icao IS NOT NULL AND arrival_icao != ''
		) sub
		GROUP BY icao, direction
		ORDER BY icao
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []*repository.AirportDirectionCount
	for rows.Next() {
		c := &repository.AirportDirectionCount{}
		if err := rows.Scan(&c.ICAO, &c.Direction, &c.Count); err != nil {
			continue
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
}

// ── Legacy trends ────────────────────────────────────────────────────────

func (r *reportsRepository) MonthlyTrends(ctx context.Context, userID uuid.UUID, months int) ([]*repository.MonthlyTrendRow, error) {
	s := newReportScope(userID, months)
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			TO_CHAR(date_trunc('month', f.date), 'YYYY-MM') AS month,
			COUNT(*) AS total_flights,
			COALESCE(SUM(f.total_time), 0) AS total_minutes,
			COALESCE(SUM(f.pic_time), 0) AS pic_minutes,
			COALESCE(SUM(f.dual_time), 0) AS dual_minutes,
			COALESCE(SUM(f.night_time), 0) AS night_minutes,
			COALESCE(SUM(f.ifr_time), 0) AS ifr_minutes,
			COALESCE(SUM(f.landings_day), 0) AS landings_day,
			COALESCE(SUM(f.landings_night), 0) AS landings_night
		FROM flights f
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY date_trunc('month', f.date)
		ORDER BY date_trunc('month', f.date) ASC
	`, s.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monthly []*repository.MonthlyTrendRow
	for rows.Next() {
		t := &repository.MonthlyTrendRow{}
		if err := rows.Scan(
			&t.Month, &t.TotalFlights, &t.TotalMinutes,
			&t.PICMinutes, &t.DualMinutes, &t.NightMinutes, &t.IFRMinutes,
			&t.LandingsDay, &t.LandingsNight,
		); err != nil {
			return nil, err
		}
		monthly = append(monthly, t)
	}
	return monthly, rows.Err()
}

func (r *reportsRepository) AircraftTypeTrends(ctx context.Context, userID uuid.UUID, months int) ([]*repository.AircraftTypeTrendRow, error) {
	s := newReportScope(userID, months)
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			f.aircraft_type,
			COUNT(*) AS total_flights,
			COALESCE(SUM(f.total_time), 0) AS total_minutes
		FROM flights f
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY f.aircraft_type
		ORDER BY total_minutes DESC
	`, s.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var byAircraft []*repository.AircraftTypeTrendRow
	for rows.Next() {
		ab := &repository.AircraftTypeTrendRow{}
		if err := rows.Scan(&ab.AircraftType, &ab.TotalFlights, &ab.TotalMinutes); err != nil {
			return nil, err
		}
		byAircraft = append(byAircraft, ab)
	}
	return byAircraft, rows.Err()
}

func (r *reportsRepository) StatsByClass(ctx context.Context, userID uuid.UUID, months int) ([]*repository.ClassStatRow, error) {
	s := newReportScope(userID, months)
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(a.aircraft_class, 'Unclassified') as class,
			COUNT(*) as flights,
			COALESCE(SUM(f.total_time), 0) as minutes,
			COALESCE(SUM(f.pic_time), 0) as pic_minutes,
			COALESCE(SUM(f.dual_time), 0) as dual_minutes,
			COALESCE(SUM(f.landings_day + f.landings_night), 0) as landings
		FROM flights f
		LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY COALESCE(a.aircraft_class, 'Unclassified')
		ORDER BY minutes DESC
	`, s.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var byClass []*repository.ClassStatRow
	for rows.Next() {
		cs := &repository.ClassStatRow{}
		if err := rows.Scan(&cs.Class, &cs.Flights, &cs.Minutes, &cs.PICMinutes, &cs.DualMinutes, &cs.Landings); err != nil {
			continue
		}
		byClass = append(byClass, cs)
	}
	return byClass, rows.Err()
}

func (r *reportsRepository) StatsByCategory(ctx context.Context, userID uuid.UUID, months int) ([]*repository.CategoryStatRow, error) {
	s := newReportScope(userID, months)
	rows, err := r.db.QueryContext(ctx, `
		SELECT category, COUNT(*) as flights,
			COALESCE(SUM(pic_time), 0) as pic_minutes,
			COALESCE(SUM(dual_time), 0) as dual_minutes
		FROM (
			SELECT f.pic_time, f.dual_time, 'Tailwheel' as category
			FROM flights f JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND a.is_tailwheel`+s.filter+`
			UNION ALL
			SELECT f.pic_time, f.dual_time, 'Complex' as category
			FROM flights f JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND a.is_complex`+s.filter+`
			UNION ALL
			SELECT f.pic_time, f.dual_time, 'High Performance' as category
			FROM flights f JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND a.is_high_performance`+s.filter+`
		) sub
		GROUP BY category
		ORDER BY COALESCE(SUM(pic_time), 0) + COALESCE(SUM(dual_time), 0) DESC
	`, s.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var byCategory []*repository.CategoryStatRow
	for rows.Next() {
		cs := &repository.CategoryStatRow{}
		if err := rows.Scan(&cs.Category, &cs.Flights, &cs.PICMinutes, &cs.DualMinutes); err != nil {
			continue
		}
		byCategory = append(byCategory, cs)
	}
	return byCategory, rows.Err()
}

// ── Analytics: totals & time series ──────────────────────────────────────

func (r *reportsRepository) Totals(ctx context.Context, userID uuid.UUID, months int) (*repository.AnalyticsTotals, error) {
	s := newReportScope(userID, months)
	t := &repository.AnalyticsTotals{}
	var first, last sql.NullTime

	err := r.db.QueryRowContext(ctx, `
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
		&t.Approaches, &t.Holds, &t.DistanceNM,
		&t.DistinctRegistrations, &t.DistinctTypes,
		&first, &last,
	)
	if err != nil {
		return nil, err
	}
	if first.Valid {
		f := first.Time
		t.FirstFlightDate = &f
	}
	if last.Valid {
		l := last.Time
		t.LastFlightDate = &l
	}
	return t, nil
}

func (r *reportsRepository) Monthly(ctx context.Context, userID uuid.UUID, months int) ([]*repository.AnalyticsMonthRow, error) {
	s := newReportScope(userID, months)
	rows, err := r.db.QueryContext(ctx, `
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

	out := []*repository.AnalyticsMonthRow{}
	for rows.Next() {
		p := &repository.AnalyticsMonthRow{}
		if err := rows.Scan(
			&p.Month, &p.Flights, &p.TotalMinutes, &p.PicMinutes, &p.SicMinutes,
			&p.DualMinutes, &p.DualGivenMinutes, &p.SoloMinutes, &p.NightMinutes,
			&p.IfrMinutes, &p.LandingsDay, &p.LandingsNight, &p.DistanceNM,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *reportsRepository) Yearly(ctx context.Context, userID uuid.UUID, months int) ([]*repository.AnalyticsYearRow, error) {
	s := newReportScope(userID, months)
	rows, err := r.db.QueryContext(ctx, `
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

	out := []*repository.AnalyticsYearRow{}
	for rows.Next() {
		p := &repository.AnalyticsYearRow{}
		if err := rows.Scan(
			&p.Year, &p.Flights, &p.TotalMinutes, &p.PicMinutes, &p.DualMinutes,
			&p.NightMinutes, &p.IfrMinutes, &p.Landings, &p.DistanceNM,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *reportsRepository) CarriedForwardMinutes(ctx context.Context, userID uuid.UUID, months int) (int, error) {
	if months <= 0 {
		return 0, nil
	}
	var earlier int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(f.total_time), 0)
		FROM flights f
		WHERE f.user_id = $1 AND NOT f.is_simulator
		  AND f.date < date_trunc('month', CURRENT_DATE - ($2 || ' months')::interval)`,
		userID, months,
	).Scan(&earlier); err != nil {
		return 0, err
	}
	return earlier, nil
}

// ── Analytics: aircraft ──────────────────────────────────────────────────

func (r *reportsRepository) ByAircraft(ctx context.Context, userID uuid.UUID, months, limit int, byRegistration bool) ([]*repository.AnalyticsAircraftRow, error) {
	s := newReportScope(userID, months)
	groupCol := "f.aircraft_type"
	subLabel := `NULLIF(TRIM(CONCAT_WS(' ', MIN(a.make), MIN(a.model))), '')`
	if byRegistration {
		groupCol = "f.aircraft_reg"
		// MIN() picks a stable representative when a registration has been
		// flown under more than one type designation.
		subLabel = `COALESCE(NULLIF(TRIM(CONCAT_WS(' ', MIN(a.make), MIN(a.model))), ''), MIN(f.aircraft_type))`
	}
	join := " LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id"

	args, limitPh := s.withLimit(limit)
	rows, err := r.db.QueryContext(ctx, `
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

	out := []*repository.AnalyticsAircraftRow{}
	for rows.Next() {
		row := &repository.AnalyticsAircraftRow{}
		var sub sql.NullString
		var first, last sql.NullTime
		if err := rows.Scan(
			&row.Label, &sub, &row.Flights, &row.TotalMinutes, &row.PicMinutes, &row.DualMinutes,
			&row.NightMinutes, &row.IfrMinutes, &row.Landings, &row.DistanceNM, &first, &last,
		); err != nil {
			return nil, err
		}
		if sub.Valid {
			v := sub.String
			row.SubLabel = &v
		}
		if first.Valid {
			f := first.Time
			row.FirstFlightDate = &f
		}
		if last.Valid {
			l := last.Time
			row.LastFlightDate = &l
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *reportsRepository) ByClass(ctx context.Context, userID uuid.UUID, months int) ([]*repository.AnalyticsGroupRow, error) {
	s := newReportScope(userID, months)
	rows, err := r.db.QueryContext(ctx, `
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
	return scanAnalyticsGroupRows(rows)
}

func (r *reportsRepository) ByCategory(ctx context.Context, userID uuid.UUID, months int) ([]*repository.AnalyticsGroupRow, error) {
	s := newReportScope(userID, months)
	// A flight can be tailwheel *and* complex, so the categories are unioned
	// rather than grouped — rows deliberately overlap.
	rows, err := r.db.QueryContext(ctx, `
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
	return scanAnalyticsGroupRows(rows)
}

func scanAnalyticsGroupRows(rows *sql.Rows) ([]*repository.AnalyticsGroupRow, error) {
	out := []*repository.AnalyticsGroupRow{}
	for rows.Next() {
		r := &repository.AnalyticsGroupRow{}
		if err := rows.Scan(&r.Label, &r.Flights, &r.TotalMinutes, &r.PicMinutes, &r.DualMinutes, &r.Landings); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── Analytics: places ────────────────────────────────────────────────────

func (r *reportsRepository) ByAirport(ctx context.Context, userID uuid.UUID, months, limit int) ([]*repository.AnalyticsAirportRow, error) {
	s := newReportScope(userID, months)
	// `flights` counts each flight once even when it departs and arrives at
	// the same airport, which is why the departure/arrival legs are counted
	// separately from the DISTINCT flight id.
	args, limitPh := s.withLimit(limit)
	rows, err := r.db.QueryContext(ctx, `
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

	out := []*repository.AnalyticsAirportRow{}
	for rows.Next() {
		row := &repository.AnalyticsAirportRow{}
		if err := rows.Scan(&row.ICAO, &row.Departures, &row.Arrivals, &row.Flights); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *reportsRepository) ByRoute(ctx context.Context, userID uuid.UUID, months, limit int) ([]*repository.AnalyticsRouteRow, error) {
	s := newReportScope(userID, months)
	args, limitPh := s.withLimit(limit)
	rows, err := r.db.QueryContext(ctx, `
		SELECT UPPER(f.departure_icao), UPPER(f.arrival_icao),
			COUNT(*),
			COALESCE(SUM(f.total_time), 0),
			COALESCE(MAX(f.distance), 0)::float8
		FROM flights f
		WHERE f.user_id = $1 AND NOT f.is_simulator
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

	out := []*repository.AnalyticsRouteRow{}
	for rows.Next() {
		row := &repository.AnalyticsRouteRow{}
		if err := rows.Scan(&row.DepartureICAO, &row.ArrivalICAO, &row.Flights, &row.TotalMinutes, &row.DistanceNM); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ── Analytics: people ────────────────────────────────────────────────────

func (r *reportsRepository) ByInstructor(ctx context.Context, userID uuid.UUID, months, limit int) ([]*repository.AnalyticsPersonRow, error) {
	s := newReportScope(userID, months)
	args, limitPh := s.withLimit(limit)
	rows, err := r.db.QueryContext(ctx, `
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
	return scanAnalyticsPersonRows(rows, false)
}

func (r *reportsRepository) ByCrew(ctx context.Context, userID uuid.UUID, months, limit int) ([]*repository.AnalyticsPersonRow, error) {
	s := newReportScope(userID, months)
	args, limitPh := s.withLimit(limit)
	rows, err := r.db.QueryContext(ctx, `
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
	return scanAnalyticsPersonRows(rows, true)
}

func scanAnalyticsPersonRows(rows *sql.Rows, withRole bool) ([]*repository.AnalyticsPersonRow, error) {
	out := []*repository.AnalyticsPersonRow{}
	for rows.Next() {
		p := &repository.AnalyticsPersonRow{}
		var role sql.NullString
		var last sql.NullTime
		var err error
		if withRole {
			err = rows.Scan(&p.Name, &role, &p.Flights, &p.TotalMinutes, &last)
		} else {
			err = rows.Scan(&p.Name, &p.Flights, &p.TotalMinutes, &last)
		}
		if err != nil {
			return nil, err
		}
		if role.Valid && role.String != "" {
			v := role.String
			p.Role = &v
		}
		if last.Valid {
			l := last.Time
			p.LastFlightDate = &l
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ── Analytics: instrument ────────────────────────────────────────────────

func (r *reportsRepository) ApproachTypes(ctx context.Context, userID uuid.UUID, months int) ([]*repository.AnalyticsApproachTypeRow, error) {
	s := newReportScope(userID, months)
	// A flight with no structured approaches stores the JSON scalar `null`
	// rather than an empty array, and jsonb_array_elements rejects scalars —
	// so anything that is not an array is normalised away before unnesting.
	rows, err := r.db.QueryContext(ctx, `
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

	out := []*repository.AnalyticsApproachTypeRow{}
	for rows.Next() {
		row := &repository.AnalyticsApproachTypeRow{}
		if err := rows.Scan(&row.Type, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *reportsRepository) ApproachesCountTotal(ctx context.Context, userID uuid.UUID, months int) (int, error) {
	s := newReportScope(userID, months)
	var counted int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(f.approaches_count), 0) FROM flights f WHERE f.user_id = $1`+s.filter,
		s.args...,
	).Scan(&counted); err != nil {
		return 0, err
	}
	return counted, nil
}

// ── Analytics: pattern histograms ────────────────────────────────────────

func (r *reportsRepository) DayOfWeekBuckets(ctx context.Context, userID uuid.UUID, months int) (map[int]repository.AnalyticsBucket, error) {
	return r.buckets(ctx, userID, months, "EXTRACT(ISODOW FROM f.date)::int", "")
}

func (r *reportsRepository) MonthOfYearBuckets(ctx context.Context, userID uuid.UUID, months int) (map[int]repository.AnalyticsBucket, error) {
	return r.buckets(ctx, userID, months, "EXTRACT(MONTH FROM f.date)::int", "")
}

func (r *reportsRepository) HourOfDayBuckets(ctx context.Context, userID uuid.UUID, months int) (map[int]repository.AnalyticsBucket, error) {
	return r.buckets(ctx, userID, months,
		"EXTRACT(HOUR FROM COALESCE(f.off_block_time, f.departure_time))::int",
		" AND COALESCE(f.off_block_time, f.departure_time) IS NOT NULL")
}

func (r *reportsRepository) DurationBuckets(ctx context.Context, userID uuid.UUID, months int) (map[int]repository.AnalyticsBucket, error) {
	return r.buckets(ctx, userID, months, `CASE
			WHEN f.total_time < 30 THEN 0
			WHEN f.total_time < 60 THEN 1
			WHEN f.total_time < 120 THEN 2
			WHEN f.total_time < 180 THEN 3
			WHEN f.total_time < 300 THEN 4
			ELSE 5 END`, "")
}

// buckets groups flights by an arbitrary integer key expression. keyExpr and
// extra are fixed strings supplied by the methods above, never caller input.
func (r *reportsRepository) buckets(ctx context.Context, userID uuid.UUID, months int, keyExpr, extra string) (map[int]repository.AnalyticsBucket, error) {
	s := newReportScope(userID, months)
	rows, err := r.db.QueryContext(ctx, `
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

	out := map[int]repository.AnalyticsBucket{}
	for rows.Next() {
		var k int
		var b repository.AnalyticsBucket
		if err := rows.Scan(&k, &b.Flights, &b.TotalMinutes); err != nil {
			return nil, err
		}
		out[k] = b
	}
	return out, rows.Err()
}

// ── Analytics: records ───────────────────────────────────────────────────

func (r *reportsRepository) LongestFlight(ctx context.Context, userID uuid.UUID, months int) (*repository.AnalyticsFlightRef, error) {
	return r.recordFlight(ctx, userID, months, "f.total_time")
}

func (r *reportsRepository) FarthestFlight(ctx context.Context, userID uuid.UUID, months int) (*repository.AnalyticsFlightRef, error) {
	return r.recordFlight(ctx, userID, months, "f.distance")
}

// recordFlight returns the top flight by the given column (a fixed string
// supplied by the two methods above, never caller input), or nil when the
// timeframe holds no flights.
func (r *reportsRepository) recordFlight(ctx context.Context, userID uuid.UUID, months int, orderBy string) (*repository.AnalyticsFlightRef, error) {
	s := newReportScope(userID, months)
	var (
		ref      repository.AnalyticsFlightRef
		reg, typ sql.NullString
		dep, arr sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT f.id, f.date, f.aircraft_reg, f.aircraft_type,
		       f.departure_icao, f.arrival_icao, f.total_time, f.distance::float8
		FROM flights f
		WHERE f.user_id = $1`+s.filter+`
		ORDER BY `+orderBy+` DESC, f.date DESC
		LIMIT 1`,
		s.args...,
	).Scan(&ref.ID, &ref.Date, &reg, &typ, &dep, &arr, &ref.TotalMinutes, &ref.DistanceNM)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if reg.Valid {
		v := reg.String
		ref.AircraftReg = &v
	}
	if typ.Valid {
		v := typ.String
		ref.AircraftType = &v
	}
	if dep.Valid {
		v := dep.String
		ref.DepartureICAO = &v
	}
	if arr.Valid {
		v := arr.String
		ref.ArrivalICAO = &v
	}
	return &ref, nil
}

func (r *reportsRepository) BusiestDay(ctx context.Context, userID uuid.UUID, months int) (*time.Time, int, error) {
	s := newReportScope(userID, months)
	var day sql.NullTime
	var flights int
	err := r.db.QueryRowContext(ctx, `
		SELECT f.date, COUNT(*)
		FROM flights f
		WHERE f.user_id = $1`+s.filter+`
		GROUP BY f.date
		ORDER BY COUNT(*) DESC, f.date DESC
		LIMIT 1`,
		s.args...,
	).Scan(&day, &flights)
	if err != nil && err != sql.ErrNoRows {
		return nil, 0, err
	}
	if !day.Valid {
		return nil, 0, nil
	}
	d := day.Time
	return &d, flights, nil
}

func (r *reportsRepository) LastFlightDate(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	var last sql.NullTime
	if err := r.db.QueryRowContext(ctx,
		`SELECT MAX(f.date) FROM flights f WHERE f.user_id = $1 AND NOT f.is_simulator`, userID,
	).Scan(&last); err != nil {
		return nil, err
	}
	if !last.Valid {
		return nil, nil
	}
	l := last.Time
	return &l, nil
}
