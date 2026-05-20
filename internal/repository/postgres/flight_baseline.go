package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type flightBaselineRepository struct {
	db *sql.DB
}

// NewFlightBaselineRepository returns a postgres-backed FlightBaselineRepository.
func NewFlightBaselineRepository(db *sql.DB) repository.FlightBaselineRepository {
	return &flightBaselineRepository{db: db}
}

const flightBaselineColumns = `
	user_id, baseline_date, total_flights, total_minutes,
	pic_minutes, sic_minutes, dual_minutes, dual_given_minutes,
	multi_pilot_minutes, night_minutes, ifr_minutes, solo_minutes,
	cross_country_minutes, landings_day, landings_night,
	notes, created_at, updated_at
`

func scanFlightBaseline(row interface{ Scan(...interface{}) error }) (*models.FlightBaseline, error) {
	b := &models.FlightBaseline{}
	if err := row.Scan(
		&b.UserID,
		&b.BaselineDate,
		&b.TotalFlights,
		&b.TotalMinutes,
		&b.PICMinutes,
		&b.SICMinutes,
		&b.DualMinutes,
		&b.DualGivenMinutes,
		&b.MultiPilotMinutes,
		&b.NightMinutes,
		&b.IFRMinutes,
		&b.SoloMinutes,
		&b.CrossCountryMinutes,
		&b.LandingsDay,
		&b.LandingsNight,
		&b.Notes,
		&b.CreatedAt,
		&b.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return b, nil
}

func (r *flightBaselineRepository) Get(ctx context.Context, userID uuid.UUID) (*models.FlightBaseline, error) {
	query := `SELECT ` + flightBaselineColumns + ` FROM flight_baselines WHERE user_id = $1`
	row := r.db.QueryRowContext(ctx, query, userID)
	b, err := scanFlightBaseline(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return b, nil
}

func (r *flightBaselineRepository) Upsert(ctx context.Context, b *models.FlightBaseline) error {
	query := `
		INSERT INTO flight_baselines (
			user_id, baseline_date, total_flights, total_minutes,
			pic_minutes, sic_minutes, dual_minutes, dual_given_minutes,
			multi_pilot_minutes, night_minutes, ifr_minutes, solo_minutes,
			cross_country_minutes, landings_day, landings_night, notes
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)
		ON CONFLICT (user_id) DO UPDATE SET
			baseline_date = EXCLUDED.baseline_date,
			total_flights = EXCLUDED.total_flights,
			total_minutes = EXCLUDED.total_minutes,
			pic_minutes = EXCLUDED.pic_minutes,
			sic_minutes = EXCLUDED.sic_minutes,
			dual_minutes = EXCLUDED.dual_minutes,
			dual_given_minutes = EXCLUDED.dual_given_minutes,
			multi_pilot_minutes = EXCLUDED.multi_pilot_minutes,
			night_minutes = EXCLUDED.night_minutes,
			ifr_minutes = EXCLUDED.ifr_minutes,
			solo_minutes = EXCLUDED.solo_minutes,
			cross_country_minutes = EXCLUDED.cross_country_minutes,
			landings_day = EXCLUDED.landings_day,
			landings_night = EXCLUDED.landings_night,
			notes = EXCLUDED.notes
		RETURNING created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		b.UserID,
		b.BaselineDate,
		b.TotalFlights,
		b.TotalMinutes,
		b.PICMinutes,
		b.SICMinutes,
		b.DualMinutes,
		b.DualGivenMinutes,
		b.MultiPilotMinutes,
		b.NightMinutes,
		b.IFRMinutes,
		b.SoloMinutes,
		b.CrossCountryMinutes,
		b.LandingsDay,
		b.LandingsNight,
		b.Notes,
	).Scan(&b.CreatedAt, &b.UpdatedAt)
}

func (r *flightBaselineRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM flight_baselines WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}
