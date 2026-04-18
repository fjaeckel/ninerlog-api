package currency

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// postgresFlightDataProvider implements FlightDataProvider using PostgreSQL
type postgresFlightDataProvider struct {
	db *sql.DB
}

// NewFlightDataProvider creates a FlightDataProvider backed by PostgreSQL
func NewFlightDataProvider(db *sql.DB) FlightDataProvider {
	return &postgresFlightDataProvider{db: db}
}

func (p *postgresFlightDataProvider) GetProgressByAircraftClass(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) (*Progress, error) {
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
		WHERE f.user_id = $1 AND a.aircraft_class = $2 AND f.date >= $3
	`

	progress := &Progress{}
	err := p.db.QueryRowContext(ctx, query, userID, string(classType), since).Scan(
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

func (p *postgresFlightDataProvider) GetProgressAll(ctx context.Context, userID uuid.UUID, since time.Time) (*Progress, error) {
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
		WHERE user_id = $1 AND date >= $2
	`

	progress := &Progress{}
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

func (p *postgresFlightDataProvider) GetLastFlightReview(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	query := `
		SELECT date FROM flights
		WHERE user_id = $1 AND is_flight_review = true
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

func (p *postgresFlightDataProvider) GetLastProficiencyCheck(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) (*time.Time, error) {
	var query string
	var args []interface{}

	// IR is a rating type, not an aircraft class — prof checks for IR can be on any aircraft.
	// Skip the aircraft class filter for IR (FCL.625.A is cross-class).
	if classType == models.ClassTypeIR {
		query = `
			SELECT date FROM flights
			WHERE user_id = $1 AND is_proficiency_check = true AND date >= $2
			ORDER BY date DESC
			LIMIT 1
		`
		args = []interface{}{userID, since}
	} else {
		query = `
			SELECT f.date FROM flights f
			INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND a.aircraft_class = $2 AND f.is_proficiency_check = true AND f.date >= $3
			ORDER BY f.date DESC
			LIMIT 1
		`
		args = []interface{}{userID, string(classType), since}
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

func (p *postgresFlightDataProvider) GetLaunchCounts(ctx context.Context, userID uuid.UUID, since time.Time) (map[string]int, error) {
	query := `
		SELECT launch_method, COUNT(*) as launches
		FROM flights
		WHERE user_id = $1 AND date >= $2 AND launch_method IS NOT NULL AND launch_method != ''
		GROUP BY launch_method
	`
	rows, err := p.db.QueryContext(ctx, query, userID, since)
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
