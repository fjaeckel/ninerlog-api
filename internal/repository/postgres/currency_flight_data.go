package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// currencyFlightDataProvider implements currency.FlightDataProvider — the
// aggregate flight reads the regulatory currency evaluators run on.
type currencyFlightDataProvider struct {
	db *sql.DB
}

// NewCurrencyFlightDataProvider creates a currency.FlightDataProvider backed
// by PostgreSQL.
func NewCurrencyFlightDataProvider(db *sql.DB) currency.FlightDataProvider {
	return &currencyFlightDataProvider{db: db}
}

func (p *currencyFlightDataProvider) GetProgressByAircraftClass(ctx context.Context, userID uuid.UUID, classTypes []models.ClassType, includeTowed bool, since time.Time) (*currency.Progress, error) {
	query := `
		SELECT
			COUNT(*) as flights,
			COALESCE(SUM(f.total_time), 0) as total_minutes,
			COALESCE(SUM(f.pic_time), 0) as pic_minutes,
			COALESCE(SUM(f.ifr_time), 0) as ifr_minutes,
			COALESCE(SUM(f.dual_time), 0) as instructor_minutes,
			COALESCE(SUM(f.night_time), 0) as night_minutes,
			COALESCE(SUM(f.landings_day + f.landings_night), 0) as landings,
			COALESCE(SUM(f.landings_day), 0) as day_landings,
			COALESCE(SUM(f.landings_night), 0) as night_landings,
			COALESCE(SUM(f.approaches_count), 0) as approaches,
			COALESCE(SUM(f.holds), 0) as holds
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = ANY($2) AND f.date >= $3
			AND ($4 OR COALESCE(f.launch_method, '') NOT IN ('winch', 'aerotow'))
	`

	progress := &currency.Progress{}
	err := p.db.QueryRowContext(ctx, query, userID, classTypeArray(classTypes), since, includeTowed).Scan(
		&progress.Flights,
		&progress.TotalMinutes,
		&progress.PICMinutes,
		&progress.IFRMinutes,
		&progress.InstructorMinutes,
		&progress.NightMinutes,
		&progress.Landings,
		&progress.DayLandings,
		&progress.NightLandings,
		&progress.Approaches,
		&progress.Holds,
	)
	if err != nil {
		return nil, err
	}
	return progress, nil
}

func (p *currencyFlightDataProvider) GetProgressAll(ctx context.Context, userID uuid.UUID, since time.Time) (*currency.Progress, error) {
	query := `
		SELECT
			COUNT(*) as flights,
			COALESCE(SUM(total_time), 0) as total_minutes,
			COALESCE(SUM(pic_time), 0) as pic_minutes,
			COALESCE(SUM(ifr_time), 0) as ifr_minutes,
			COALESCE(SUM(dual_time), 0) as instructor_minutes,
			COALESCE(SUM(night_time), 0) as night_minutes,
			COALESCE(SUM(landings_day + landings_night), 0) as landings,
			COALESCE(SUM(landings_day), 0) as day_landings,
			COALESCE(SUM(landings_night), 0) as night_landings,
			COALESCE(SUM(approaches_count), 0) as approaches,
			COALESCE(SUM(holds), 0) as holds
		FROM flights
		WHERE user_id = $1 AND NOT is_simulator AND NOT is_passenger AND date >= $2
	`

	progress := &currency.Progress{}
	err := p.db.QueryRowContext(ctx, query, userID, since).Scan(
		&progress.Flights,
		&progress.TotalMinutes,
		&progress.PICMinutes,
		&progress.IFRMinutes,
		&progress.InstructorMinutes,
		&progress.NightMinutes,
		&progress.Landings,
		&progress.DayLandings,
		&progress.NightLandings,
		&progress.Approaches,
		&progress.Holds,
	)
	if err != nil {
		return nil, err
	}
	return progress, nil
}

func (p *currencyFlightDataProvider) GetLastFlightReview(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	query := `
		SELECT date FROM flights
		WHERE user_id = $1 AND NOT is_simulator AND NOT is_passenger AND is_flight_review = true
		ORDER BY date DESC
		LIMIT 1
	`
	var reviewDate time.Time
	err := p.db.QueryRowContext(ctx, query, userID).Scan(&reviewDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reviewDate, nil
}

func (p *currencyFlightDataProvider) GetLastProficiencyCheck(ctx context.Context, userID uuid.UUID, classTypes []models.ClassType, since time.Time) (*time.Time, error) {
	var query string
	var args []interface{}

	// IR skips the aircraft-class filter (FCL.625.A is cross-class).
	if len(classTypes) == 1 && classTypes[0] == models.ClassTypeIR {
		query = `
			SELECT date FROM flights
			WHERE user_id = $1 AND NOT is_simulator AND NOT is_passenger AND is_proficiency_check = true AND date >= $2
			ORDER BY date DESC
			LIMIT 1
		`
		args = []interface{}{userID, since}
	} else {
		query = `
			SELECT f.date FROM flights f
			INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = ANY($2) AND f.is_proficiency_check = true AND f.date >= $3
				AND COALESCE(f.launch_method, '') NOT IN ('winch', 'aerotow')
			ORDER BY f.date DESC
			LIMIT 1
		`
		args = []interface{}{userID, classTypeArray(classTypes), since}
	}

	var checkDate time.Time
	err := p.db.QueryRowContext(ctx, query, args...).Scan(&checkDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &checkDate, nil
}

func (p *currencyFlightDataProvider) GetLandingDaysByAircraftClass(ctx context.Context, userID uuid.UUID, classType models.ClassType, includeTowed bool, since time.Time) ([]currency.LandingDay, error) {
	query := `
		SELECT
			f.date,
			COALESCE(SUM(f.landings_day), 0) as day_landings,
			COALESCE(SUM(f.landings_night), 0) as night_landings
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = $2 AND f.date >= $3
			AND ($4 OR COALESCE(f.launch_method, '') NOT IN ('winch', 'aerotow'))
		GROUP BY f.date
		HAVING SUM(f.landings_day + f.landings_night) > 0
		ORDER BY f.date DESC
	`

	rows, err := p.db.QueryContext(ctx, query, userID, string(classType), since, includeTowed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []currency.LandingDay
	for rows.Next() {
		var d currency.LandingDay
		if err := rows.Scan(&d.Date, &d.DayLandings, &d.NightLandings); err != nil {
			return nil, err
		}
		d.Date = time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), 0, 0, 0, 0, time.UTC)
		days = append(days, d)
	}
	return days, rows.Err()
}

func (p *currencyFlightDataProvider) GetLaunchCounts(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) (map[string]int, error) {
	query := `
		SELECT f.launch_method, COUNT(*) as launches
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = $2 AND f.date >= $3
			AND f.launch_method IS NOT NULL AND f.launch_method != ''
		GROUP BY f.launch_method
	`
	rows, err := p.db.QueryContext(ctx, query, userID, string(classType), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var method string
		var count int
		if err := rows.Scan(&method, &count); err != nil {
			return nil, err
		}
		counts[method] = count
	}
	return counts, rows.Err()
}

// classTypeArray converts class types to a Postgres text array.
func classTypeArray(classTypes []models.ClassType) interface{} {
	out := make([]string, len(classTypes))
	for i, ct := range classTypes {
		out[i] = string(ct)
	}
	return pq.Array(out)
}
