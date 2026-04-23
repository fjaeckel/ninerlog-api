package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
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
	approachesJSON, err := json.Marshal(flight.Approaches)
	if err != nil {
		approachesJSON = []byte("[]")
	}

	query := `
		INSERT INTO flights (
			user_id, date, aircraft_reg, aircraft_type,
			departure_icao, arrival_icao, off_block_time, on_block_time,
			departure_time, arrival_time,
			total_time, is_pic, is_dual, pic_time, dual_time, night_time, ifr_time,
			landings_day, landings_night, all_landings,
			takeoffs_day, takeoffs_night,
			route, solo_time, cross_country_time, distance,
			takeoffs_day_override, takeoffs_night_override,
			landings_day_override, landings_night_override,
			remarks,
			instructor_name, instructor_comments,
			sic_time, dual_given_time, simulated_flight_time, ground_training_time,
			actual_instrument_time, simulated_instrument_time, holds, approaches_count, is_ipc, is_flight_review, is_proficiency_check,
			launch_method,
			pic_name, multi_pilot_time, fstd_type, approaches, endorsements
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46, $47, $48, $49, $50)
		RETURNING id, created_at, updated_at
	`

	return r.db.QueryRowContext(
		ctx, query,
		flight.UserID,
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
		flight.IsPIC,
		flight.IsDual,
		flight.PICTime,
		flight.DualTime,
		flight.NightTime,
		flight.IFRTime,
		flight.LandingsDay,
		flight.LandingsNight,
		flight.AllLandings,
		flight.TakeoffsDay,
		flight.TakeoffsNight,
		flight.Route,
		flight.SoloTime,
		flight.CrossCountryTime,
		flight.Distance,
		flight.TakeoffsDayOverride,
		flight.TakeoffsNightOverride,
		flight.LandingsDayOverride,
		flight.LandingsNightOverride,
		flight.Remarks,
		flight.InstructorName,
		flight.InstructorComments,
		flight.SICTime,
		flight.DualGivenTime,
		flight.SimulatedFlightTime,
		flight.GroundTrainingTime,
		flight.ActualInstrumentTime,
		flight.SimulatedInstrumentTime,
		flight.Holds,
		flight.ApproachesCount,
		flight.IsIPC,
		flight.IsFlightReview,
		flight.IsProficiencyCheck,
		flight.LaunchMethod,
		flight.PICName,
		flight.MultiPilotTime,
		flight.FSTDType,
		approachesJSON,
		flight.Endorsements,
	).Scan(&flight.ID, &flight.CreatedAt, &flight.UpdatedAt)
}

func (r *flightRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Flight, error) {
	query := `
		SELECT id, user_id, date, aircraft_reg, aircraft_type,
		       departure_icao, arrival_icao, off_block_time, on_block_time,
		       departure_time, arrival_time,
		       total_time, is_pic, is_dual, pic_time, dual_time, night_time, ifr_time,
		       landings_day, landings_night, all_landings,
		       takeoffs_day, takeoffs_night,
		       route, solo_time, cross_country_time, distance,
		       takeoffs_day_override, takeoffs_night_override,
		       landings_day_override, landings_night_override,
		       remarks, created_at, updated_at,
		       instructor_name, instructor_comments,
		       sic_time, dual_given_time, simulated_flight_time, ground_training_time,
		       actual_instrument_time, simulated_instrument_time, holds, approaches_count, is_ipc, is_flight_review, is_proficiency_check,
		       launch_method,
		       pic_name, multi_pilot_time, fstd_type, approaches, endorsements
		FROM flights
		WHERE id = $1
	`

	flight := &models.Flight{}
	var offBlock, onBlock, depTime, arrTime *time.Time
	var approachesJSON []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&flight.ID,
		&flight.UserID,
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
		&flight.IsPIC,
		&flight.IsDual,
		&flight.PICTime,
		&flight.DualTime,
		&flight.NightTime,
		&flight.IFRTime,
		&flight.LandingsDay,
		&flight.LandingsNight,
		&flight.AllLandings,
		&flight.TakeoffsDay,
		&flight.TakeoffsNight,
		&flight.Route,
		&flight.SoloTime,
		&flight.CrossCountryTime,
		&flight.Distance,
		&flight.TakeoffsDayOverride,
		&flight.TakeoffsNightOverride,
		&flight.LandingsDayOverride,
		&flight.LandingsNightOverride,
		&flight.Remarks,
		&flight.CreatedAt,
		&flight.UpdatedAt,
		&flight.InstructorName,
		&flight.InstructorComments,
		&flight.SICTime,
		&flight.DualGivenTime,
		&flight.SimulatedFlightTime,
		&flight.GroundTrainingTime,
		&flight.ActualInstrumentTime,
		&flight.SimulatedInstrumentTime,
		&flight.Holds,
		&flight.ApproachesCount,
		&flight.IsIPC,
		&flight.IsFlightReview,
		&flight.IsProficiencyCheck,
		&flight.LaunchMethod,
		&flight.PICName,
		&flight.MultiPilotTime,
		&flight.FSTDType,
		&approachesJSON,
		&flight.Endorsements,
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

	if len(approachesJSON) > 0 {
		if err := json.Unmarshal(approachesJSON, &flight.Approaches); err != nil {
			flight.Approaches = nil
		}
	}

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

func (r *flightRepository) Update(ctx context.Context, flight *models.Flight) error {
	approachesJSON, err := json.Marshal(flight.Approaches)
	if err != nil {
		approachesJSON = []byte("[]")
	}

	query := `
		UPDATE flights
		SET date = $1, aircraft_reg = $2, aircraft_type = $3,
		    departure_icao = $4, arrival_icao = $5,
		    off_block_time = $6, on_block_time = $7,
		    departure_time = $8, arrival_time = $9,
		    total_time = $10, is_pic = $11, is_dual = $12,
		    pic_time = $13, dual_time = $14,
		    night_time = $15, ifr_time = $16, landings_day = $17, landings_night = $18,
		    all_landings = $19, takeoffs_day = $20, takeoffs_night = $21,
		    route = $22, solo_time = $23, cross_country_time = $24, distance = $25,
		    takeoffs_day_override = $26, takeoffs_night_override = $27,
		    landings_day_override = $28, landings_night_override = $29,
		    remarks = $30,
		    instructor_name = $31, instructor_comments = $32,
		    sic_time = $33, dual_given_time = $34, simulated_flight_time = $35, ground_training_time = $36,
		    actual_instrument_time = $37, simulated_instrument_time = $38, holds = $39, approaches_count = $40, is_ipc = $41, is_flight_review = $42, is_proficiency_check = $43,
		    launch_method = $44,
		    pic_name = $45, multi_pilot_time = $46, fstd_type = $47, approaches = $48, endorsements = $49,
		    updated_at = $50
		WHERE id = $51
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
		flight.IsPIC,
		flight.IsDual,
		flight.PICTime,
		flight.DualTime,
		flight.NightTime,
		flight.IFRTime,
		flight.LandingsDay,
		flight.LandingsNight,
		flight.AllLandings,
		flight.TakeoffsDay,
		flight.TakeoffsNight,
		flight.Route,
		flight.SoloTime,
		flight.CrossCountryTime,
		flight.Distance,
		flight.TakeoffsDayOverride,
		flight.TakeoffsNightOverride,
		flight.LandingsDayOverride,
		flight.LandingsNightOverride,
		flight.Remarks,
		flight.InstructorName,
		flight.InstructorComments,
		flight.SICTime,
		flight.DualGivenTime,
		flight.SimulatedFlightTime,
		flight.GroundTrainingTime,
		flight.ActualInstrumentTime,
		flight.SimulatedInstrumentTime,
		flight.Holds,
		flight.ApproachesCount,
		flight.IsIPC,
		flight.IsFlightReview,
		flight.IsProficiencyCheck,
		flight.LaunchMethod,
		flight.PICName,
		flight.MultiPilotTime,
		flight.FSTDType,
		approachesJSON,
		flight.Endorsements,
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

func (r *flightRepository) DeleteAllByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	// Crew members cascade via FK, but delete explicitly for safety
	_, _ = r.db.ExecContext(ctx, `DELETE FROM flight_crew_members WHERE flight_id IN (SELECT id FROM flights WHERE user_id = $1)`, userID)

	result, err := r.db.ExecContext(ctx, `DELETE FROM flights WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *flightRepository) CountByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) (int, error) {
	query := "SELECT COUNT(*) FROM flights WHERE user_id = $1"
	args := []interface{}{userID}
	argNum := 2

	if opts != nil {
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
			query += fmt.Sprintf(" AND UPPER(aircraft_reg) = UPPER($%d)", argNum)
			args = append(args, *opts.AircraftReg)
			argNum++
		}
		if opts.DepartureICAO != nil {
			query += fmt.Sprintf(" AND UPPER(departure_icao) = UPPER($%d)", argNum)
			args = append(args, *opts.DepartureICAO)
			argNum++
		}
		if opts.ArrivalICAO != nil {
			query += fmt.Sprintf(" AND UPPER(arrival_icao) = UPPER($%d)", argNum)
			args = append(args, *opts.ArrivalICAO)
			argNum++
		}
		if opts.IsPIC != nil {
			query += fmt.Sprintf(" AND is_pic = $%d", argNum)
			args = append(args, *opts.IsPIC)
			argNum++
		}
		if opts.IsDual != nil {
			query += fmt.Sprintf(" AND is_dual = $%d", argNum)
			args = append(args, *opts.IsDual)
			argNum++
		}
		if opts.Search != nil && *opts.Search != "" {
			searchPattern := "%" + *opts.Search + "%"
			query += fmt.Sprintf(
				" AND (UPPER(aircraft_reg) LIKE UPPER($%d) OR UPPER(aircraft_type) LIKE UPPER($%d) OR UPPER(departure_icao) LIKE UPPER($%d) OR UPPER(arrival_icao) LIKE UPPER($%d) OR UPPER(COALESCE(remarks, '')) LIKE UPPER($%d))",
				argNum, argNum, argNum, argNum, argNum,
			)
			args = append(args, searchPattern)
			argNum++
		}
	}

	var count int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (r *flightRepository) GetStatsByUserID(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time) (*models.FlightStatistics, error) {
	query := `
		SELECT
			COUNT(*) as total_flights,
			COALESCE(SUM(total_time), 0) as total_minutes,
			COALESCE(SUM(pic_time), 0) as pic_minutes,
			COALESCE(SUM(dual_time), 0) as dual_minutes,
			COALESCE(SUM(night_time), 0) as night_minutes,
			COALESCE(SUM(ifr_time), 0) as ifr_minutes,
			COALESCE(SUM(solo_time), 0) as solo_minutes,
			COALESCE(SUM(cross_country_time), 0) as cross_country_minutes,
			COALESCE(SUM(landings_day), 0) as landings_day,
			COALESCE(SUM(landings_night), 0) as landings_night,
			COALESCE(SUM(sic_time), 0) as sic_minutes,
			COALESCE(SUM(dual_given_time), 0) as dual_given_minutes
		FROM flights
		WHERE user_id = $1
	`
	args := []interface{}{userID}
	argNum := 2

	if startDate != nil {
		query += fmt.Sprintf(" AND date >= $%d", argNum)
		args = append(args, *startDate)
		argNum++
	}
	if endDate != nil {
		query += fmt.Sprintf(" AND date <= $%d", argNum)
		args = append(args, *endDate)
	}

	stats := &models.FlightStatistics{}
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&stats.TotalFlights,
		&stats.TotalMinutes,
		&stats.PICMinutes,
		&stats.DualMinutes,
		&stats.NightMinutes,
		&stats.IFRMinutes,
		&stats.SoloMinutes,
		&stats.CrossCountryMinutes,
		&stats.LandingsDay,
		&stats.LandingsNight,
		&stats.SICMinutes,
		&stats.DualGivenMinutes,
	)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *flightRepository) GetCurrencyData(ctx context.Context, userID uuid.UUID, since time.Time) (*models.CurrencyData, error) {
	query := `
		SELECT
			COUNT(*) as flights,
			COALESCE(SUM(landings_day + landings_night), 0) as total_landings,
			COALESCE(SUM(landings_day), 0) as day_landings,
			COALESCE(SUM(landings_night), 0) as night_landings
		FROM flights
		WHERE user_id = $1 AND date >= $2
	`

	data := &models.CurrencyData{}
	err := r.db.QueryRowContext(ctx, query, userID, since).Scan(
		&data.Flights,
		&data.TotalLandings,
		&data.DayLandings,
		&data.NightLandings,
	)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (r *flightRepository) buildQuery(baseCondition string, baseValue interface{}, opts *repository.FlightQueryOptions) (string, []interface{}) {
	query := `
		SELECT id, user_id, date, aircraft_reg, aircraft_type,
		       departure_icao, arrival_icao, off_block_time, on_block_time,
		       departure_time, arrival_time,
		       total_time, is_pic, is_dual, pic_time, dual_time, night_time, ifr_time,
		       landings_day, landings_night, all_landings,
		       takeoffs_day, takeoffs_night,
		       route, solo_time, cross_country_time, distance,
		       takeoffs_day_override, takeoffs_night_override,
		       landings_day_override, landings_night_override,
		       remarks, created_at, updated_at,
		       instructor_name, instructor_comments,
		       sic_time, dual_given_time, simulated_flight_time, ground_training_time,
		       actual_instrument_time, simulated_instrument_time, holds, approaches_count, is_ipc, is_flight_review, is_proficiency_check,
		       launch_method,
		       pic_name, multi_pilot_time, fstd_type, approaches, endorsements
		FROM flights
		WHERE ` + baseCondition

	args := []interface{}{baseValue}
	argNum := 2

	if opts != nil {
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
			query += fmt.Sprintf(" AND UPPER(aircraft_reg) = UPPER($%d)", argNum)
			args = append(args, *opts.AircraftReg)
			argNum++
		}
		if opts.DepartureICAO != nil {
			query += fmt.Sprintf(" AND UPPER(departure_icao) = UPPER($%d)", argNum)
			args = append(args, *opts.DepartureICAO)
			argNum++
		}
		if opts.ArrivalICAO != nil {
			query += fmt.Sprintf(" AND UPPER(arrival_icao) = UPPER($%d)", argNum)
			args = append(args, *opts.ArrivalICAO)
			argNum++
		}
		if opts.IsPIC != nil {
			query += fmt.Sprintf(" AND is_pic = $%d", argNum)
			args = append(args, *opts.IsPIC)
			argNum++
		}
		if opts.IsDual != nil {
			query += fmt.Sprintf(" AND is_dual = $%d", argNum)
			args = append(args, *opts.IsDual)
			argNum++
		}
		if opts.Search != nil && *opts.Search != "" {
			searchPattern := "%" + *opts.Search + "%"
			query += fmt.Sprintf(
				" AND (UPPER(aircraft_reg) LIKE UPPER($%d) OR UPPER(aircraft_type) LIKE UPPER($%d) OR UPPER(departure_icao) LIKE UPPER($%d) OR UPPER(arrival_icao) LIKE UPPER($%d) OR UPPER(COALESCE(remarks, '')) LIKE UPPER($%d))",
				argNum, argNum, argNum, argNum, argNum,
			)
			args = append(args, searchPattern)
			argNum++
		}
	}

	// Sorting — map camelCase API field names to snake_case DB columns
	sortColumn := "date"
	sortDirection := "DESC"
	if opts != nil {
		switch opts.SortBy {
		case "date":
			sortColumn = "date"
		case "totalTime":
			sortColumn = "total_time"
		case "createdAt":
			sortColumn = "created_at"
		}
		if strings.EqualFold(opts.SortOrder, "asc") {
			sortDirection = "ASC"
		}
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortColumn, sortDirection)

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
		var approachesJSON []byte
		err := rows.Scan(
			&flight.ID,
			&flight.UserID,
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
			&flight.IsPIC,
			&flight.IsDual,
			&flight.PICTime,
			&flight.DualTime,
			&flight.NightTime,
			&flight.IFRTime,
			&flight.LandingsDay,
			&flight.LandingsNight,
			&flight.AllLandings,
			&flight.TakeoffsDay,
			&flight.TakeoffsNight,
			&flight.Route,
			&flight.SoloTime,
			&flight.CrossCountryTime,
			&flight.Distance,
			&flight.TakeoffsDayOverride,
			&flight.TakeoffsNightOverride,
			&flight.LandingsDayOverride,
			&flight.LandingsNightOverride,
			&flight.Remarks,
			&flight.CreatedAt,
			&flight.UpdatedAt,
			&flight.InstructorName,
			&flight.InstructorComments,
			&flight.SICTime,
			&flight.DualGivenTime,
			&flight.SimulatedFlightTime,
			&flight.GroundTrainingTime,
			&flight.ActualInstrumentTime,
			&flight.SimulatedInstrumentTime,
			&flight.Holds,
			&flight.ApproachesCount,
			&flight.IsIPC,
			&flight.IsFlightReview,
			&flight.IsProficiencyCheck,
			&flight.LaunchMethod,
			&flight.PICName,
			&flight.MultiPilotTime,
			&flight.FSTDType,
			&approachesJSON,
			&flight.Endorsements,
		)
		if err != nil {
			return nil, err
		}
		flight.OffBlockTime = timeToString(offBlock)
		flight.OnBlockTime = timeToString(onBlock)
		flight.DepartureTime = timeToString(depTime)
		flight.ArrivalTime = timeToString(arrTime)
		if len(approachesJSON) > 0 {
			if err := json.Unmarshal(approachesJSON, &flight.Approaches); err != nil {
				flight.Approaches = nil
			}
		}
		flights = append(flights, flight)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return flights, nil
}
