package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type customReportRepository struct {
	db *sql.DB
}

// NewCustomReportRepository creates a Postgres-backed custom report repository.
func NewCustomReportRepository(db *sql.DB) repository.CustomReportRepository {
	return &customReportRepository{db: db}
}

const customReportColumns = `id, user_id, name, definition, position, created_at, updated_at`

func scanCustomReport(s interface {
	Scan(dest ...interface{}) error
}) (*models.CustomReport, error) {
	r := &models.CustomReport{}
	if err := s.Scan(&r.ID, &r.UserID, &r.Name, &r.Definition, &r.Position, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *customReportRepository) Create(ctx context.Context, report *models.CustomReport) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO custom_reports (user_id, name, definition, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM custom_reports WHERE user_id = $1))
		RETURNING id, position, created_at, updated_at`,
		report.UserID, report.Name, report.Definition,
	).Scan(&report.ID, &report.Position, &report.CreatedAt, &report.UpdatedAt)
}

func (r *customReportRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.CustomReport, error) {
	rep, err := scanCustomReport(r.db.QueryRowContext(ctx,
		`SELECT `+customReportColumns+` FROM custom_reports WHERE id = $1`, id))
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	return rep, err
}

func (r *customReportRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]*models.CustomReport, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+customReportColumns+` FROM custom_reports WHERE user_id = $1 ORDER BY position ASC, created_at ASC, id ASC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reports := []*models.CustomReport{}
	for rows.Next() {
		rep, err := scanCustomReport(rows)
		if err != nil {
			return nil, err
		}
		reports = append(reports, rep)
	}
	return reports, rows.Err()
}

func (r *customReportRepository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_reports WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

func (r *customReportRepository) Update(ctx context.Context, report *models.CustomReport) error {
	err := r.db.QueryRowContext(ctx, `
		UPDATE custom_reports SET name = $1, definition = $2
		WHERE id = $3
		RETURNING updated_at`,
		report.Name, report.Definition, report.ID,
	).Scan(&report.UpdatedAt)
	if err == sql.ErrNoRows {
		return repository.ErrNotFound
	}
	return err
}

func (r *customReportRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM custom_reports WHERE id = $1`, id)
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

func (r *customReportRepository) SetPositions(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE custom_reports SET position = $1 WHERE id = $2 AND user_id = $3`,
			i, id, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// reportGroupExpr maps a grouping to its SQL key expression over `scoped`.
var reportGroupExpr = map[string]string{
	models.ReportGroupMonth:        `to_char(date, 'YYYY-MM')`,
	models.ReportGroupYear:         `to_char(date, 'YYYY')`,
	models.ReportGroupDayOfWeek:    `EXTRACT(ISODOW FROM date)::int::text`,
	models.ReportGroupAircraftType: `UPPER(TRIM(COALESCE(aircraft_type, '')))`,
	models.ReportGroupRegistration: `UPPER(TRIM(COALESCE(aircraft_reg, '')))`,
	models.ReportGroupDeparture:    `UPPER(TRIM(COALESCE(departure_icao, '')))`,
	models.ReportGroupArrival:      `UPPER(TRIM(COALESCE(arrival_icao, '')))`,
	models.ReportGroupRoute: `CASE WHEN TRIM(COALESCE(departure_icao, '')) = '' AND TRIM(COALESCE(arrival_icao, '')) = '' THEN ''
		ELSE UPPER(TRIM(COALESCE(departure_icao, ''))) || '-' || UPPER(TRIM(COALESCE(arrival_icao, ''))) END`,
}

func (r *customReportRepository) Aggregate(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions, groupBy string) ([]repository.CustomReportGroup, error) {
	expr, ok := reportGroupExpr[groupBy]
	if !ok {
		return nil, fmt.Errorf("unknown grouping %q", groupBy)
	}

	where, args, _ := appendFlightFilters("user_id = $1", []interface{}{userID}, 2, opts)
	query := `
		WITH scoped AS (
			SELECT *, NOT is_simulator AND NOT is_passenger AS is_flight_time
			FROM flights WHERE ` + where + `
		)
		SELECT ` + expr + ` AS k,
			COUNT(*) FILTER (WHERE is_flight_time),
			COALESCE(SUM(total_time) FILTER (WHERE is_flight_time), 0),
			COALESCE(SUM(pic_time) FILTER (WHERE is_flight_time), 0),
			COALESCE(SUM(dual_time) FILTER (WHERE is_flight_time), 0),
			COALESCE(SUM(dual_given_time) FILTER (WHERE is_flight_time), 0),
			COALESCE(SUM(night_time) FILTER (WHERE is_flight_time), 0),
			COALESCE(SUM(ifr_time) FILTER (WHERE is_flight_time), 0),
			COALESCE(SUM(cross_country_time) FILTER (WHERE is_flight_time), 0),
			COALESCE(SUM(simulated_flight_time) FILTER (WHERE is_simulator), 0),
			COALESCE(SUM(all_landings) FILTER (WHERE is_flight_time), 0)
		FROM scoped
		GROUP BY 1
		ORDER BY 1`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := []repository.CustomReportGroup{}
	for rows.Next() {
		var g repository.CustomReportGroup
		t := &g.Totals
		if err := rows.Scan(&g.Key, &t.Flights, &t.TotalTime, &t.PicTime, &t.DualTime, &t.DualGivenTime,
			&t.NightTime, &t.IfrTime, &t.CrossCountryTime, &t.FstdTime, &t.Landings); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}
