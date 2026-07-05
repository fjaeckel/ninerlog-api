package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type flightSessionRepository struct {
	db *sql.DB
}

// NewFlightSessionRepository returns a postgres-backed FlightSessionRepository.
func NewFlightSessionRepository(db *sql.DB) repository.FlightSessionRepository {
	return &flightSessionRepository{db: db}
}

const flightSessionColumns = `
	id, user_id, status, aircraft_reg, departure_icao, arrival_icao,
	off_block_at, takeoff_at, landing_at, on_block_at, flight_id,
	created_at, updated_at
`

func scanFlightSession(row interface{ Scan(...interface{}) error }) (*models.FlightSession, error) {
	s := &models.FlightSession{}
	if err := row.Scan(
		&s.ID,
		&s.UserID,
		&s.Status,
		&s.AircraftReg,
		&s.DepartureICAO,
		&s.ArrivalICAO,
		&s.OffBlockAt,
		&s.TakeoffAt,
		&s.LandingAt,
		&s.OnBlockAt,
		&s.FlightID,
		&s.CreatedAt,
		&s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	// CHAR(4) columns come back space-padded for shorter values
	trimICAO(&s.DepartureICAO)
	trimICAO(&s.ArrivalICAO)
	return s, nil
}

func trimICAO(v **string) {
	if *v != nil {
		trimmed := strings.TrimSpace(**v)
		if trimmed == "" {
			*v = nil
		} else {
			*v = &trimmed
		}
	}
}

func (r *flightSessionRepository) Create(ctx context.Context, s *models.FlightSession) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.Status == "" {
		s.Status = models.FlightSessionStatusOpen
	}
	query := `
		INSERT INTO flight_sessions (
			id, user_id, status, aircraft_reg, departure_icao, arrival_icao,
			off_block_at, takeoff_at, landing_at, on_block_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		s.ID,
		s.UserID,
		s.Status,
		s.AircraftReg,
		s.DepartureICAO,
		s.ArrivalICAO,
		s.OffBlockAt,
		s.TakeoffAt,
		s.LandingAt,
		s.OnBlockAt,
	).Scan(&s.CreatedAt, &s.UpdatedAt)
	if err != nil && strings.Contains(err.Error(), "flight_sessions_one_open_per_user") {
		return repository.ErrDuplicate
	}
	return err
}

func (r *flightSessionRepository) GetOpenByUserID(ctx context.Context, userID uuid.UUID) (*models.FlightSession, error) {
	query := `SELECT ` + flightSessionColumns + `
		FROM flight_sessions
		WHERE user_id = $1 AND status = 'open'`
	s, err := scanFlightSession(r.db.QueryRowContext(ctx, query, userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *flightSessionRepository) Update(ctx context.Context, s *models.FlightSession) error {
	query := `
		UPDATE flight_sessions SET
			status = $2,
			aircraft_reg = $3,
			departure_icao = $4,
			arrival_icao = $5,
			off_block_at = $6,
			takeoff_at = $7,
			landing_at = $8,
			on_block_at = $9,
			flight_id = $10,
			updated_at = now()
		WHERE id = $1
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		s.ID,
		s.Status,
		s.AircraftReg,
		s.DepartureICAO,
		s.ArrivalICAO,
		s.OffBlockAt,
		s.TakeoffAt,
		s.LandingAt,
		s.OnBlockAt,
		s.FlightID,
	).Scan(&s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	return err
}
