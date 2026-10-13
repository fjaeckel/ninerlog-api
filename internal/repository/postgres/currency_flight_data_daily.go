package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
)

var _ currency.DailyFlightDataProvider = (*currencyFlightDataProvider)(nil)

// scanDailyProgress scans (date, progressSelect) rows.
func scanDailyProgress(rows *sql.Rows) ([]currency.DailyProgress, error) {
	defer rows.Close()
	var out []currency.DailyProgress
	for rows.Next() {
		var d currency.DailyProgress
		p := &d.Progress
		if err := rows.Scan(
			&d.Date,
			&p.Flights,
			&p.TotalMinutes,
			&p.PICMinutes,
			&p.IFRMinutes,
			&p.InstructorMinutes,
			&p.NightMinutes,
			&p.Landings,
			&p.DayLandings,
			&p.NightLandings,
			&p.Approaches,
			&p.Holds,
			&p.Launches,
			&p.SPICMinutes,
			&p.TrainingFlights,
			&p.LongestTrainingFlightMinutes,
		); err != nil {
			return nil, err
		}
		d.Date = time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), 0, 0, 0, 0, time.UTC)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *currencyFlightDataProvider) GetDailyProgressByAircraftClass(ctx context.Context, userID uuid.UUID, classTypes []models.ClassType, includeTowed bool, since time.Time) ([]currency.DailyProgress, error) {
	query := `
		SELECT f.date,` + progressSelect + `
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = ANY($2) AND f.date >= $3
			AND ($4 OR COALESCE(f.launch_method, '') NOT IN ` + towedLaunches + `)
		GROUP BY f.date
		ORDER BY f.date
	`
	rows, err := p.db.QueryContext(ctx, query, userID, classTypeArray(classTypes), since, includeTowed)
	if err != nil {
		return nil, err
	}
	return scanDailyProgress(rows)
}

func (p *currencyFlightDataProvider) GetDailyProgressByULKind(ctx context.Context, userID uuid.UUID, sel currency.ULSelector, includeTowed bool, since time.Time) ([]currency.DailyProgress, error) {
	query := `
		SELECT f.date,` + progressSelect + `
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND ` + ulKindFilter + ` AND f.date >= $5
			AND ($6 OR COALESCE(f.launch_method, '') NOT IN ` + towedLaunches + `)
		GROUP BY f.date
		ORDER BY f.date
	`
	rows, err := p.db.QueryContext(ctx, query, userID, ulKindArray(sel.Kinds), sel.IncludeUnspecified, sel.MinMTOMKg, since, includeTowed)
	if err != nil {
		return nil, err
	}
	return scanDailyProgress(rows)
}

func (p *currencyFlightDataProvider) GetDailyProgressAll(ctx context.Context, userID uuid.UUID, since time.Time) ([]currency.DailyProgress, error) {
	query := `
		SELECT f.date,` + progressSelect + `
		FROM flights f
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND f.date >= $2
		GROUP BY f.date
		ORDER BY f.date
	`
	rows, err := p.db.QueryContext(ctx, query, userID, since)
	if err != nil {
		return nil, err
	}
	return scanDailyProgress(rows)
}

func (p *currencyFlightDataProvider) GetDailyLaunchCounts(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) ([]currency.DailyLaunches, error) {
	query := `
		SELECT f.date, f.launch_method, SUM(` + launchCountSQL + `) as launches
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = $2 AND f.date >= $3
			AND f.launch_method IS NOT NULL AND f.launch_method != ''
		GROUP BY f.date, f.launch_method
		ORDER BY f.date
	`
	rows, err := p.db.QueryContext(ctx, query, userID, string(classType), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []currency.DailyLaunches
	for rows.Next() {
		var d currency.DailyLaunches
		if err := rows.Scan(&d.Date, &d.Method, &d.Launches); err != nil {
			return nil, err
		}
		d.Date = time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), 0, 0, 0, 0, time.UTC)
		out = append(out, d)
	}
	return out, rows.Err()
}
