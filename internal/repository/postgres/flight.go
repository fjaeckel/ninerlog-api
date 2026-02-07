package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

type flightRepository struct {
	db *sql.DB
}

// NewFlightRepository creates a new flight repository
func NewFlightRepository(db *sql.DB) repository.FlightRepository {
	return &flightRepository{db: db}
}

// timeToString converts a *time.Time (from a PostgreSQL TIME column) to a *string in HH:MM:SS format.
func timeToString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("15:04:05")
	return &s
}

func (r *flightRepository) Create(ctx context.Context, flight *models.Flight) error {
	query := `
		INSERT INTO flights (
			user_id, license_id, date, aircraft_reg, aircraft_type,
			departure_icao, arrival_icao, off_block_time, on_block_time,
			departure_time, arrival_time,
			total_time, pic_time, dual_time, solo_time, night_time, ifr_time,
			landings_day, landings_night, remarks
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING id, created_at, updated_at
	`

	return r.db.QueryRowContext(
		ctx, query,
		flight.UserID,
		flight.LicenseID,
		flight.Date,
		flight.AircraftReg,
		flight.AircraftType,
		flight.DepartureICAO,
		flight.ArrivalICAO,
		flight.OffBlockTime,
		flight.OnBlockTime,
		flight.DepartureTime,
		flight.ArrivalTime,
		flight.TotalTime,
		flight.PICTime,
		flight.DualTime,
		flight.SoloTime,
		flight.NightTime,
		flight.IFRTime,
		flight.LandingsDay,
		flight.LandingsNight,
		flight.Remarks,
	).Scan(&flight.ID, &flight.CreatedAt, &flight.UpdatedAt)
}

func (r *flightRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Flight, error) {
	query := `
		SELECT id, user_id, license_id, date, aircraft_reg, aircraft_type,
		       departure_icao, arrival_icao, off_block_time, on_block_time,
		       departure_time, arrival_time,
		       total_time, pic_time, dual_time, solo_time, night_time, ifr_time,
		       landings_day, landings_night, remarks, created_at, updated_at
		FROM flights
		WHERE id = $1
	`

	flight := &models.Flight{}
	var offBlock, onBlock, depTime, arrTime *time.Time
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&flight.ID,
		&flight.UserID,
		&flight.LicenseID,
		&flight.Date,
		&flight.AircraftReg,
		&flight.AircraftType,
		&flight.DepartureICAO,
		&flight.ArrivalICAO,
		&offBlock,
		&onBlock,
		&depTime,
		&arrTime,
		&flight.TotalTime,
		&flight.PICTime,
		&flight.DualTime,
		&flight.SoloTime,
		&flight.NightTime,
		&flight.IFRTime,
		&flight.LandingsDay,
		&flight.LandingsNight,
		&flight.Remarks,
		&flight.CreatedAt,
		&flight.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	flight.OffBlockTime = timeToString(offBlock)
	flight.OnBlockTime = timeToString(onBlock)
	flight.DepartureTime = timeToString(depTime)
	flight.ArrivalTime = timeToString(arrTime)

	return flight, nil
}

func (r *flightRepository) GetByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) ([]*models.Flight, error) {
	query, args := r.buildQuery("user_id = $1", userID, opts)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanFlights(rows)
}

func (r *flightRepository) GetByLicenseID(ctx context.Context, licenseID uuid.UUID, opts *repository.FlightQueryOptions) ([]*models.Flight, error) {
	query, args := r.buildQuery("license_id = $1", licenseID, opts)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanFlights(rows)
}

func (r *flightRepository) Update(ctx context.Context, flight *models.Flight) error {
	query := `
		UPDATE flights
		SET date = $1, aircraft_reg = $2, aircraft_type = $3,
		    departure_icao = $4, arrival_icao = $5,
		    off_block_time = $6, on_block_time = $7,
		    departure_time = $8, arrival_time = $9,
		    total_time = $10, pic_time = $11, dual_time = $12, solo_time = $13,
		    night_time = $14, ifr_time = $15, landings_day = $16, landings_night = $17,
		    remarks = $18, updated_at = $19
		WHERE id = $20
	`

	result, err := r.db.ExecContext(
		ctx, query,
		flight.Date,
		flight.AircraftReg,
		flight.AircraftType,
		flight.DepartureICAO,
		flight.ArrivalICAO,
		flight.OffBlockTime,
		flight.OnBlockTime,
		flight.DepartureTime,
		flight.ArrivalTime,
		flight.TotalTime,
		flight.PICTime,
		flight.DualTime,
		flight.SoloTime,
		flight.NightTime,
		flight.IFRTime,
		flight.LandingsDay,
		flight.LandingsNight,
		flight.Remarks,
		time.Now(),
		flight.ID,
	)
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

func (r *flightRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM flights WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
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

func (r *flightRepository) CountByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) (int, error) {
	query := "SELECT COUNT(*) FROM flights WHERE user_id = $1"
	args := []interface{}{userID}
	argNum := 2

	if opts != nil {
		if opts.LicenseID != nil {
			query += fmt.Sprintf(" AND license_id = $%d", argNum)
			args = append(args, *opts.LicenseID)
			argNum++
		}
		if opts.StartDate != nil {
			query += fmt.Sprintf(" AND date >= $%d", argNum)
			args = append(args, *opts.StartDate)
			argNum++
		}
		if opts.EndDate != nil {
			query += fmt.Sprintf(" AND date <= $%d", argNum)
			args = append(args, *opts.EndDate)
			argNum++
		}
		if opts.AircraftReg != nil {
			query += fmt.Sprintf(" AND aircraft_reg = $%d", argNum)
			args = append(args, *opts.AircraftReg)
			argNum++
		}
	}

	var count int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (r *flightRepository) buildQuery(baseCondition string, baseValue interface{}, opts *repository.FlightQueryOptions) (string, []interface{}) {
	query := `
		SELECT id, user_id, license_id, date, aircraft_reg, aircraft_type,
		       departure_icao, arrival_icao, off_block_time, on_block_time,
		       departure_time, arrival_time,
		       total_time, pic_time, dual_time, solo_time, night_time, ifr_time,
		       landings_day, landings_night, remarks, created_at, updated_at
		FROM flights
		WHERE ` + baseCondition

	args := []interface{}{baseValue}
	argNum := 2

	if opts != nil {
		if opts.LicenseID != nil {
			query += fmt.Sprintf(" AND license_id = $%d", argNum)
			args = append(args, *opts.LicenseID)
			argNum++
		}
		if opts.StartDate != nil {
			query += fmt.Sprintf(" AND date >= $%d", argNum)
			args = append(args, *opts.StartDate)
			argNum++
		}
		if opts.EndDate != nil {
			query += fmt.Sprintf(" AND date <= $%d", argNum)
			args = append(args, *opts.EndDate)
			argNum++
		}
		if opts.AircraftReg != nil {
			query += fmt.Sprintf(" AND aircraft_reg = $%d", argNum)
			args = append(args, *opts.AircraftReg)
			argNum++
		}
	}

	// Sorting
	sortBy := "date"
	sortOrder := "DESC"
	if opts != nil {
		if opts.SortBy != "" {
			sortBy = opts.SortBy
		}
		if opts.SortOrder != "" {
			sortOrder = strings.ToUpper(opts.SortOrder)
		}
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	// Pagination
	if opts != nil && opts.PageSize > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, opts.PageSize)
		argNum++

		if opts.Page > 1 {
			offset := (opts.Page - 1) * opts.PageSize
			query += fmt.Sprintf(" OFFSET $%d", argNum)
			args = append(args, offset)
		}
	}

	return query, args
}

func (r *flightRepository) scanFlights(rows *sql.Rows) ([]*models.Flight, error) {
	var flights []*models.Flight

	for rows.Next() {
		flight := &models.Flight{}
		var offBlock, onBlock, depTime, arrTime *time.Time
		err := rows.Scan(
			&flight.ID,
			&flight.UserID,
			&flight.LicenseID,
			&flight.Date,
			&flight.AircraftReg,
			&flight.AircraftType,
			&flight.DepartureICAO,
			&flight.ArrivalICAO,
			&offBlock,
			&onBlock,
			&depTime,
			&arrTime,
			&flight.TotalTime,
			&flight.PICTime,
			&flight.DualTime,
			&flight.SoloTime,
			&flight.NightTime,
			&flight.IFRTime,
			&flight.LandingsDay,
			&flight.LandingsNight,
			&flight.Remarks,
			&flight.CreatedAt,
			&flight.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		flight.OffBlockTime = timeToString(offBlock)
		flight.OnBlockTime = timeToString(onBlock)
		flight.DepartureTime = timeToString(depTime)
		flight.ArrivalTime = timeToString(arrTime)
		flights = append(flights, flight)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return flights, nil
}
