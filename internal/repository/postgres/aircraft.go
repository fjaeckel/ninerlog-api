package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
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
		                      is_complex, is_high_performance, is_tailwheel, notes, is_active, aircraft_class)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
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
		       aircraft_class, created_at, updated_at
		FROM aircraft WHERE id = $1
	`
	a := &models.Aircraft{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&a.ID, &a.UserID, &a.Registration, &a.Type, &a.Make, &a.Model,
		&a.IsComplex, &a.IsHighPerformance, &a.IsTailwheel,
		&a.Notes, &a.IsActive, &a.AircraftClass, &a.CreatedAt, &a.UpdatedAt,
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
		       aircraft_class, created_at, updated_at
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
			&a.Notes, &a.IsActive, &a.AircraftClass, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		aircraft = append(aircraft, a)
	}
	return aircraft, rows.Err()
}

func (r *aircraftRepository) Update(ctx context.Context, aircraft *models.Aircraft) error {
	query := `
		UPDATE aircraft
		SET registration = $1, type = $2, make = $3, model = $4,
		    is_complex = $5, is_high_performance = $6,
		    is_tailwheel = $7, notes = $8, is_active = $9, aircraft_class = $10,
		    updated_at = $11
		WHERE id = $12
	`
	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		aircraft.Registration, aircraft.Type, aircraft.Make, aircraft.Model,
		aircraft.IsComplex, aircraft.IsHighPerformance, aircraft.IsTailwheel,
		aircraft.Notes, aircraft.IsActive, aircraft.AircraftClass, now, aircraft.ID,
	)
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
