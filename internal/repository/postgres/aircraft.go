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
		INSERT INTO aircraft (user_id, registration, type, make, model, category, engine_type,
		                      is_complex, is_high_performance, is_tailwheel, notes, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at
	`
	var engineType *string
	if aircraft.EngineType != nil {
		s := string(*aircraft.EngineType)
		engineType = &s
	}
	err := r.db.QueryRowContext(ctx, query,
		aircraft.UserID,
		aircraft.Registration,
		aircraft.Type,
		aircraft.Make,
		aircraft.Model,
		aircraft.Category,
		engineType,
		aircraft.IsComplex,
		aircraft.IsHighPerformance,
		aircraft.IsTailwheel,
		aircraft.Notes,
		aircraft.IsActive,
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
		SELECT id, user_id, registration, type, make, model, category, engine_type,
		       is_complex, is_high_performance, is_tailwheel, notes, is_active,
		       created_at, updated_at
		FROM aircraft WHERE id = $1
	`
	a := &models.Aircraft{}
	var engineType *string
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&a.ID, &a.UserID, &a.Registration, &a.Type, &a.Make, &a.Model,
		&a.Category, &engineType,
		&a.IsComplex, &a.IsHighPerformance, &a.IsTailwheel,
		&a.Notes, &a.IsActive, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if engineType != nil {
		et := models.EngineType(*engineType)
		a.EngineType = &et
	}
	return a, nil
}

func (r *aircraftRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Aircraft, error) {
	query := `
		SELECT id, user_id, registration, type, make, model, category, engine_type,
		       is_complex, is_high_performance, is_tailwheel, notes, is_active,
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
		var engineType *string
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.Registration, &a.Type, &a.Make, &a.Model,
			&a.Category, &engineType,
			&a.IsComplex, &a.IsHighPerformance, &a.IsTailwheel,
			&a.Notes, &a.IsActive, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if engineType != nil {
			et := models.EngineType(*engineType)
			a.EngineType = &et
		}
		aircraft = append(aircraft, a)
	}
	return aircraft, rows.Err()
}

func (r *aircraftRepository) Update(ctx context.Context, aircraft *models.Aircraft) error {
	query := `
		UPDATE aircraft
		SET registration = $1, type = $2, make = $3, model = $4, category = $5,
		    engine_type = $6, is_complex = $7, is_high_performance = $8,
		    is_tailwheel = $9, notes = $10, is_active = $11, updated_at = $12
		WHERE id = $13
	`
	var engineType *string
	if aircraft.EngineType != nil {
		s := string(*aircraft.EngineType)
		engineType = &s
	}
	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		aircraft.Registration, aircraft.Type, aircraft.Make, aircraft.Model,
		aircraft.Category, engineType,
		aircraft.IsComplex, aircraft.IsHighPerformance, aircraft.IsTailwheel,
		aircraft.Notes, aircraft.IsActive, now, aircraft.ID,
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
