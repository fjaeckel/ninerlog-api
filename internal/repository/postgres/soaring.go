package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// soaringFlightSQL matches soaring flights over flights f LEFT JOINed to
// aircraft a: GLIDER aircraft, ULTRALIGHT aircraft of kind SAILPLANE, and TMG
// flights with a launch method; FSTD sessions and passenger flights never match.
const soaringFlightSQL = `(NOT f.is_simulator AND NOT f.is_passenger AND (
	UPPER(TRIM(COALESCE(a.aircraft_class, ''))) = 'GLIDER'
	OR (UPPER(TRIM(COALESCE(a.aircraft_class, ''))) = 'ULTRALIGHT' AND a.ul_kind = 'SAILPLANE')
	OR (UPPER(TRIM(COALESCE(a.aircraft_class, ''))) = 'TMG' AND TRIM(COALESCE(f.launch_method, '')) <> '')))`

// soaringFrom is the FROM clause soaringFlightSQL is written against.
const soaringFrom = `FROM flights f
		LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id`

type soaringRepository struct {
	db *sql.DB
}

// NewSoaringRepository creates a Postgres-backed soaring statistics repository.
func NewSoaringRepository(db *sql.DB) repository.SoaringRepository {
	return &soaringRepository{db: db}
}

// SeasonStats aggregates the user's soaring flights dated from..to inclusive.
func (r *soaringRepository) SeasonStats(ctx context.Context, userID uuid.UUID, from, to time.Time, topSites int) (*repository.SoaringSeasonStats, error) {
	const scope = `WHERE f.user_id = $1 AND f.date >= $2 AND f.date <= $3 AND ` + soaringFlightSQL
	stats := &repository.SoaringSeasonStats{LaunchesByMethod: map[string]int{}, Sites: []repository.SoaringSite{}}

	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
			COALESCE(SUM(`+launchCountSQL+`), 0),
			COALESCE(SUM(f.total_time), 0),
			COUNT(*) FILTER (WHERE f.is_outlanding)
		`+soaringFrom+`
		`+scope, userID, from, to,
	).Scan(&stats.Flights, &stats.Launches, &stats.TotalMinutes, &stats.Outlandings); err != nil {
		return nil, err
	}
	if stats.Flights == 0 {
		return stats, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT LOWER(TRIM(COALESCE(f.launch_method, ''))), COALESCE(SUM(`+launchCountSQL+`), 0)
		`+soaringFrom+`
		`+scope+`
		GROUP BY 1`, userID, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var method string
		var n int
		if err := rows.Scan(&method, &n); err != nil {
			_ = rows.Close()
			return nil, err
		}
		stats.LaunchesByMethod[method] += n
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}

	var (
		longest repository.SoaringLongestFlight
		reg     sql.NullString
	)
	if err := r.db.QueryRowContext(ctx, `
		SELECT f.id, f.date, f.total_time, f.aircraft_reg
		`+soaringFrom+`
		`+scope+`
		ORDER BY f.total_time DESC, f.date DESC, f.id
		LIMIT 1`, userID, from, to,
	).Scan(&longest.FlightID, &longest.Date, &longest.Minutes, &reg); err != nil {
		return nil, err
	}
	if reg.Valid && reg.String != "" {
		v := reg.String
		longest.AircraftReg = &v
	}
	stats.LongestFlight = &longest

	siteRows, err := r.db.QueryContext(ctx, `
		SELECT MIN(TRIM(f.departure_icao)), COUNT(*)
		`+soaringFrom+`
		`+scope+` AND TRIM(COALESCE(f.departure_icao, '')) <> ''
		GROUP BY UPPER(TRIM(f.departure_icao))
		ORDER BY COUNT(*) DESC, 1
		LIMIT $4`, userID, from, to, topSites)
	if err != nil {
		return nil, err
	}
	defer func() { _ = siteRows.Close() }()
	for siteRows.Next() {
		var s repository.SoaringSite
		if err := siteRows.Scan(&s.Place, &s.Flights); err != nil {
			return nil, err
		}
		stats.Sites = append(stats.Sites, s)
	}
	return stats, siteRows.Err()
}
