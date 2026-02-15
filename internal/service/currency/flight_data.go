package currency

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
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
			COALESCE(SUM(f.total_time), 0) as total_hours,
			COALESCE(SUM(f.pic_time), 0) as pic_hours,
			COALESCE(SUM(f.ifr_time), 0) as ifr_hours,
			COALESCE(SUM(f.dual_time), 0) as instructor_hours,
			COALESCE(SUM(f.night_time), 0) as night_hours,
			COALESCE(SUM(f.landings_day + f.landings_night), 0) as landings,
			COALESCE(SUM(f.landings_day), 0) as day_landings,
			COALESCE(SUM(f.landings_night), 0) as night_landings
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND a.aircraft_class = $2 AND f.date >= $3
	`

	progress := &Progress{}
	err := p.db.QueryRowContext(ctx, query, userID, string(classType), since).Scan(
		&progress.Flights,
		&progress.TotalHours,
		&progress.PICHours,
		&progress.IFRHours,
		&progress.InstructorHours,
		&progress.NightHours,
		&progress.Landings,
		&progress.DayLandings,
		&progress.NightLandings,
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
			COALESCE(SUM(total_time), 0) as total_hours,
			COALESCE(SUM(pic_time), 0) as pic_hours,
			COALESCE(SUM(ifr_time), 0) as ifr_hours,
			COALESCE(SUM(dual_time), 0) as instructor_hours,
			COALESCE(SUM(night_time), 0) as night_hours,
			COALESCE(SUM(landings_day + landings_night), 0) as landings,
			COALESCE(SUM(landings_day), 0) as day_landings,
			COALESCE(SUM(landings_night), 0) as night_landings
		FROM flights
		WHERE user_id = $1 AND date >= $2
	`

	progress := &Progress{}
	err := p.db.QueryRowContext(ctx, query, userID, since).Scan(
		&progress.Flights,
		&progress.TotalHours,
		&progress.PICHours,
		&progress.IFRHours,
		&progress.InstructorHours,
		&progress.NightHours,
		&progress.Landings,
		&progress.DayLandings,
		&progress.NightLandings,
	)
	if err != nil {
		return nil, err
	}
	return progress, nil
}
