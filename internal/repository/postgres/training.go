package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type trainingRepository struct {
	db *sql.DB
}

// NewTrainingRepository creates a Postgres-backed training progress repository.
func NewTrainingRepository(db *sql.DB) repository.TrainingRepository {
	return &trainingRepository{db: db}
}

const trainingFlightsQuery = `
	SELECT
		UPPER(TRIM(a.aircraft_class)),
		CASE WHEN UPPER(TRIM(a.aircraft_class)) = 'ULTRALIGHT' THEN a.ul_kind END,
		f.dual_time, f.pic_time, f.spic_time,
		` + launchCountSQL + `,
		f.all_landings, f.cross_country_time, COALESCE(f.distance, 0),
		f.signature_id IS NOT NULL
	FROM flights f
	JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
	WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger
		AND UPPER(TRIM(a.aircraft_class)) IN ('GLIDER', 'TMG', 'ULTRALIGHT')
	ORDER BY f.date, f.id`

// ListTrainingFlights returns the user's crewed GLIDER, TMG and ULTRALIGHT flights.
func (r *trainingRepository) ListTrainingFlights(ctx context.Context, userID uuid.UUID) ([]repository.TrainingFlight, error) {
	rows, err := r.db.QueryContext(ctx, trainingFlightsQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("training flights: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []repository.TrainingFlight{}
	for rows.Next() {
		var f repository.TrainingFlight
		var kind sql.NullString
		if err := rows.Scan(&f.AircraftClass, &kind, &f.DualMinutes, &f.PICMinutes, &f.SPICMinutes,
			&f.Launches, &f.Landings, &f.CrossCountryMinutes, &f.DistanceNM, &f.Signed); err != nil {
			return nil, fmt.Errorf("scan training flight: %w", err)
		}
		if kind.Valid {
			k := models.ULKind(kind.String)
			f.ULKind = &k
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("training flights: %w", err)
	}
	return out, nil
}

// OtherCategoryPICMinutes sums PIC minutes on aircraft that are not GLIDER, TMG or ULTRALIGHT.
func (r *trainingRepository) OtherCategoryPICMinutes(ctx context.Context, userID uuid.UUID) (int, error) {
	var minutes int
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(f.pic_time), 0)
		FROM flights f
		LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger
			AND UPPER(TRIM(COALESCE(a.aircraft_class, ''))) NOT IN ('GLIDER', 'TMG', 'ULTRALIGHT')`,
		userID).Scan(&minutes)
	if err != nil {
		return 0, fmt.Errorf("other category PIC minutes: %w", err)
	}
	return minutes, nil
}
