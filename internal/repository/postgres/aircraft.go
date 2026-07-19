package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type aircraftRepository struct {
	db *sql.DB
}

func NewAircraftRepository(db *sql.DB) repository.AircraftRepository {
	return &aircraftRepository{db: db}
}

func (r *aircraftRepository) Create(ctx context.Context, aircraft *models.Aircraft) error {
	query := `
		INSERT INTO aircraft (user_id, registration, type, make, model,
		                      is_complex, is_high_performance, is_tailwheel, notes, is_active, aircraft_class,
		                      default_departure_icao, default_arrival_icao)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		aircraft.UserID,
		aircraft.Registration,
		aircraft.Type,
		aircraft.Make,
		aircraft.Model,
		aircraft.IsComplex,
		aircraft.IsHighPerformance,
		aircraft.IsTailwheel,
		aircraft.Notes,
		aircraft.IsActive,
		aircraft.AircraftClass,
		aircraft.DefaultDepartureICAO,
		aircraft.DefaultArrivalICAO,
	).Scan(&aircraft.ID, &aircraft.CreatedAt, &aircraft.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "idx_aircraft_user_registration") {
			return repository.ErrDuplicateRegistration
		}
		return err
	}
	return nil
}

func (r *aircraftRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Aircraft, error) {
	query := `
		SELECT id, user_id, registration, type, make, model,
		       is_complex, is_high_performance, is_tailwheel, notes, is_active,
		       aircraft_class, default_departure_icao, default_arrival_icao,
		       created_at, updated_at
		FROM aircraft WHERE id = $1
	`
	a := &models.Aircraft{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&a.ID, &a.UserID, &a.Registration, &a.Type, &a.Make, &a.Model,
		&a.IsComplex, &a.IsHighPerformance, &a.IsTailwheel,
		&a.Notes, &a.IsActive, &a.AircraftClass,
		&a.DefaultDepartureICAO, &a.DefaultArrivalICAO,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *aircraftRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Aircraft, error) {
	query := `
		SELECT id, user_id, registration, type, make, model,
		       is_complex, is_high_performance, is_tailwheel, notes, is_active,
		       aircraft_class, default_departure_icao, default_arrival_icao,
		       created_at, updated_at
		FROM aircraft WHERE user_id = $1
		ORDER BY registration ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aircraft []*models.Aircraft
	for rows.Next() {
		a := &models.Aircraft{}
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Registration, &a.Type, &a.Make, &a.Model,
			&a.IsComplex, &a.IsHighPerformance, &a.IsTailwheel,
			&a.Notes, &a.IsActive, &a.AircraftClass,
			&a.DefaultDepartureICAO, &a.DefaultArrivalICAO,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		aircraft = append(aircraft, a)
	}
	return aircraft, rows.Err()
}

const aircraftUpdateQuery = `
	UPDATE aircraft
	SET registration = $1, type = $2, make = $3, model = $4,
	    is_complex = $5, is_high_performance = $6,
	    is_tailwheel = $7, notes = $8, is_active = $9, aircraft_class = $10,
	    default_departure_icao = $11, default_arrival_icao = $12,
	    updated_at = $13
	WHERE id = $14
`

func aircraftUpdateArgs(aircraft *models.Aircraft, now time.Time) []any {
	return []any{
		aircraft.Registration, aircraft.Type, aircraft.Make, aircraft.Model,
		aircraft.IsComplex, aircraft.IsHighPerformance, aircraft.IsTailwheel,
		aircraft.Notes, aircraft.IsActive, aircraft.AircraftClass,
		aircraft.DefaultDepartureICAO, aircraft.DefaultArrivalICAO,
		now, aircraft.ID,
	}
}

func (r *aircraftRepository) Update(ctx context.Context, aircraft *models.Aircraft) error {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, aircraftUpdateQuery, aircraftUpdateArgs(aircraft, now)...)
	if err != nil {
		if strings.Contains(err.Error(), "idx_aircraft_user_registration") {
			return repository.ErrDuplicateRegistration
		}
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return repository.ErrNotFound
	}
	aircraft.UpdatedAt = now
	return nil
}

func (r *aircraftRepository) UpdateWithFlightRename(ctx context.Context, aircraft *models.Aircraft, oldRegistration string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now()
	result, err := tx.ExecContext(ctx, aircraftUpdateQuery, aircraftUpdateArgs(aircraft, now)...)
	if err != nil {
		if strings.Contains(err.Error(), "idx_aircraft_user_registration") {
			return 0, repository.ErrDuplicateRegistration
		}
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows == 0 {
		return 0, repository.ErrNotFound
	}

	flightsResult, err := tx.ExecContext(ctx,
		`UPDATE flights SET aircraft_reg = $1, updated_at = NOW() WHERE user_id = $2 AND aircraft_reg = $3`,
		aircraft.Registration, aircraft.UserID, oldRegistration,
	)
	if err != nil {
		return 0, err
	}
	flightsUpdated, err := flightsResult.RowsAffected()
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE flight_sessions SET aircraft_reg = $1, updated_at = NOW() WHERE user_id = $2 AND aircraft_reg = $3`,
		aircraft.Registration, aircraft.UserID, oldRegistration,
	); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	aircraft.UpdatedAt = now
	return int(flightsUpdated), nil
}

func (r *aircraftRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM aircraft WHERE id = $1", id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *aircraftRepository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aircraft WHERE user_id = $1", userID).Scan(&count)
	return count, err
}

func (r *aircraftRepository) GetStatsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftStats, error) {
	query := `
		SELECT aircraft_reg,
		       COUNT(*),
		       COALESCE(SUM(total_time), 0),
		       COALESCE(SUM(landings_day), 0),
		       COALESCE(SUM(landings_night), 0),
		       MIN(date),
		       MAX(date)
		FROM flights
		WHERE user_id = $1
		GROUP BY aircraft_reg
		ORDER BY aircraft_reg ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []*models.AircraftStats
	for rows.Next() {
		s := &models.AircraftStats{}
		if err := rows.Scan(
			&s.Registration, &s.TotalFlights, &s.TotalMinutes,
			&s.LandingsDay, &s.LandingsNight,
			&s.FirstFlightDate, &s.LastFlightDate,
		); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

func (r *aircraftRepository) GetTypeStatsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftTypeStats, error) {
	query := `
		SELECT aircraft_type,
		       COUNT(*),
		       COALESCE(SUM(total_time), 0),
		       COALESCE(SUM(landings_day), 0),
		       COALESCE(SUM(landings_night), 0),
		       MIN(date),
		       MAX(date)
		FROM flights
		WHERE user_id = $1
		GROUP BY aircraft_type
		ORDER BY aircraft_type ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []*models.AircraftTypeStats
	for rows.Next() {
		s := &models.AircraftTypeStats{}
		if err := rows.Scan(
			&s.AircraftType, &s.TotalFlights, &s.TotalMinutes,
			&s.LandingsDay, &s.LandingsNight,
			&s.FirstFlightDate, &s.LastFlightDate,
		); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

func (r *aircraftRepository) GetRecencyRowsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftRecencyRow, error) {
	query := `
		SELECT aircraft_reg, aircraft_type, date,
		       COALESCE(SUM(landings_day + landings_night), 0)
		FROM flights
		WHERE user_id = $1 AND date >= CURRENT_DATE - INTERVAL '90 days'
		GROUP BY aircraft_reg, aircraft_type, date
		ORDER BY date DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.AircraftRecencyRow
	for rows.Next() {
		row := &models.AircraftRecencyRow{}
		if err := rows.Scan(&row.Registration, &row.AircraftType, &row.Date, &row.Landings); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
