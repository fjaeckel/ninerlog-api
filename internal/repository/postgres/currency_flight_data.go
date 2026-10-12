package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// currencyFlightDataProvider implements currency.FlightDataProvider — the
// aggregate flight reads the regulatory currency evaluators run on.
type currencyFlightDataProvider struct {
	db    *sql.DB
	until *time.Time
}

// NewCurrencyFlightDataProvider creates a currency.FlightDataProvider backed
// by PostgreSQL.
func NewCurrencyFlightDataProvider(db *sql.DB) currency.FlightDataProvider {
	return &currencyFlightDataProvider{db: db}
}

// AsOf returns a provider that reads only flights dated on or before day.
func (p *currencyFlightDataProvider) AsOf(day time.Time) currency.FlightDataProvider {
	d := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return &currencyFlightDataProvider{db: p.db, until: &d}
}

// untilArg is the as-of day query argument; NULL reads every flight.
func (p *currencyFlightDataProvider) untilArg() interface{} {
	if p.until == nil {
		return nil
	}
	return *p.until
}

// launchCountExpr is a flight's launch count over flights aliased alias, or
// the unaliased flights table for "": the stored launches, else 0 on a
// simulator or passenger row, else its take-offs with at least one.
func launchCountExpr(alias string) string {
	p := ""
	if alias != "" {
		p = alias + "."
	}
	return "COALESCE(" + p + "launches, CASE WHEN " + p + "is_simulator OR " + p + "is_passenger THEN 0 ELSE GREATEST(" +
		p + "takeoffs_day + " + p + "takeoffs_night, 1) END)"
}

// launchCountSQL is launchCountExpr over flights aliased f.
var launchCountSQL = launchCountExpr("f")

// progressSelect aggregates a Progress row over flights aliased f.
var progressSelect = `
			COUNT(*) as flights,
			COALESCE(SUM(f.total_time), 0) as total_minutes,
			COALESCE(SUM(f.pic_time), 0) as pic_minutes,
			COALESCE(SUM(f.ifr_time), 0) as ifr_minutes,
			COALESCE(SUM(f.dual_time), 0) as instructor_minutes,
			COALESCE(SUM(f.night_time), 0) as night_minutes,
			COALESCE(SUM(f.landings_day + f.landings_night), 0) as landings,
			COALESCE(SUM(f.landings_day), 0) as day_landings,
			COALESCE(SUM(f.landings_night), 0) as night_landings,
			COALESCE(SUM(f.approaches_count), 0) as approaches,
			COALESCE(SUM(f.holds), 0) as holds,
			COALESCE(SUM(` + launchCountSQL + `), 0) as launches,
			COALESCE(SUM(f.spic_time), 0) as spic_minutes,
			COUNT(*) FILTER (WHERE f.dual_time > 0) as training_flights,
			COALESCE(MAX(f.total_time) FILTER (WHERE f.dual_time > 0), 0) as longest_training_flight_minutes`

// ulKindFilter matches ULTRALIGHT aircraft a whose kind is in $2, or unset when
// $3, with an MTOM of at least $4 kg when $4 is not 0.
const ulKindFilter = `upper(trim(a.aircraft_class)) = 'ULTRALIGHT' AND (a.ul_kind = ANY($2) OR ($3 AND a.ul_kind IS NULL)) AND ($4 = 0 OR a.mtom_kg >= $4)`

// towedLaunches is the SQL list of models.TowedLaunchMethods.
var towedLaunches = sqlStringList(models.TowedLaunchMethods())

// sqlStringList renders constant strings as a parenthesised SQL literal list.
func sqlStringList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return "(" + strings.Join(quoted, ", ") + ")"
}

// scanProgress runs a query selecting progressSelect.
func (p *currencyFlightDataProvider) scanProgress(ctx context.Context, query string, args ...interface{}) (*currency.Progress, error) {
	progress := &currency.Progress{}
	err := p.db.QueryRowContext(ctx, query, args...).Scan(
		&progress.Flights,
		&progress.TotalMinutes,
		&progress.PICMinutes,
		&progress.IFRMinutes,
		&progress.InstructorMinutes,
		&progress.NightMinutes,
		&progress.Landings,
		&progress.DayLandings,
		&progress.NightLandings,
		&progress.Approaches,
		&progress.Holds,
		&progress.Launches,
		&progress.SPICMinutes,
		&progress.TrainingFlights,
		&progress.LongestTrainingFlightMinutes,
	)
	if err != nil {
		return nil, err
	}
	return progress, nil
}

func (p *currencyFlightDataProvider) GetProgressByAircraftClass(ctx context.Context, userID uuid.UUID, classTypes []models.ClassType, includeTowed bool, since time.Time) (*currency.Progress, error) {
	query := `
		SELECT` + progressSelect + `
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = ANY($2) AND f.date >= $3
			AND ($4 OR COALESCE(f.launch_method, '') NOT IN ` + towedLaunches + `)
			AND ($5::date IS NULL OR f.date <= $5::date)
	`
	return p.scanProgress(ctx, query, userID, classTypeArray(classTypes), since, includeTowed, p.untilArg())
}

func (p *currencyFlightDataProvider) GetProgressByULKind(ctx context.Context, userID uuid.UUID, sel currency.ULSelector, includeTowed bool, since time.Time) (*currency.Progress, error) {
	query := `
		SELECT` + progressSelect + `
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND ` + ulKindFilter + ` AND f.date >= $5
			AND ($6 OR COALESCE(f.launch_method, '') NOT IN ` + towedLaunches + `)
			AND ($7::date IS NULL OR f.date <= $7::date)
	`
	return p.scanProgress(ctx, query, userID, ulKindArray(sel.Kinds), sel.IncludeUnspecified, sel.MinMTOMKg, since, includeTowed, p.untilArg())
}

func (p *currencyFlightDataProvider) GetLastProficiencyCheckByULKind(ctx context.Context, userID uuid.UUID, sel currency.ULSelector, since time.Time) (*time.Time, error) {
	query := `
		SELECT f.date FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND ` + ulKindFilter + ` AND f.is_proficiency_check = true AND f.date >= $5
			AND ($6::date IS NULL OR f.date <= $6::date)
		ORDER BY f.date DESC
		LIMIT 1
	`
	var checkDate time.Time
	err := p.db.QueryRowContext(ctx, query, userID, ulKindArray(sel.Kinds), sel.IncludeUnspecified, sel.MinMTOMKg, since, p.untilArg()).Scan(&checkDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &checkDate, nil
}

func (p *currencyFlightDataProvider) GetLandingDaysByULKind(ctx context.Context, userID uuid.UUID, sel currency.ULSelector, includeTowed bool, since time.Time) ([]currency.LandingDay, error) {
	query := `
		SELECT
			f.date,
			COALESCE(SUM(f.landings_day), 0) as day_landings,
			COALESCE(SUM(f.landings_night), 0) as night_landings,
			COALESCE(SUM(GREATEST(f.takeoffs_day + f.takeoffs_night, 1)), 0) as takeoffs
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND ` + ulKindFilter + ` AND f.date >= $5
			AND ($6 OR COALESCE(f.launch_method, '') NOT IN ` + towedLaunches + `)
			AND ($7::date IS NULL OR f.date <= $7::date)
		GROUP BY f.date
		ORDER BY f.date DESC
	`
	rows, err := p.db.QueryContext(ctx, query, userID, ulKindArray(sel.Kinds), sel.IncludeUnspecified, sel.MinMTOMKg, since, includeTowed, p.untilArg())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var days []currency.LandingDay
	for rows.Next() {
		var d currency.LandingDay
		if err := rows.Scan(&d.Date, &d.DayLandings, &d.NightLandings, &d.Takeoffs); err != nil {
			return nil, err
		}
		d.Date = time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), 0, 0, 0, 0, time.UTC)
		days = append(days, d)
	}
	return days, rows.Err()
}

func (p *currencyFlightDataProvider) GetProgressAll(ctx context.Context, userID uuid.UUID, since time.Time) (*currency.Progress, error) {
	query := `
		SELECT` + progressSelect + `
		FROM flights f
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND f.date >= $2
			AND ($3::date IS NULL OR f.date <= $3::date)
	`
	return p.scanProgress(ctx, query, userID, since, p.untilArg())
}

func (p *currencyFlightDataProvider) GetLastFlightReview(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	query := `
		SELECT date FROM flights
		WHERE user_id = $1 AND NOT is_simulator AND NOT is_passenger AND is_flight_review = true
			AND ($2::date IS NULL OR date <= $2::date)
		ORDER BY date DESC
		LIMIT 1
	`
	var reviewDate time.Time
	err := p.db.QueryRowContext(ctx, query, userID, p.untilArg()).Scan(&reviewDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reviewDate, nil
}

func (p *currencyFlightDataProvider) GetLastProficiencyCheck(ctx context.Context, userID uuid.UUID, classTypes []models.ClassType, since time.Time) (*time.Time, error) {
	var query string
	var args []interface{}

	// IR skips the aircraft-class filter (FCL.625.A is cross-class).
	if len(classTypes) == 1 && classTypes[0] == models.ClassTypeIR {
		query = `
			SELECT date FROM flights
			WHERE user_id = $1 AND NOT is_simulator AND NOT is_passenger AND is_proficiency_check = true AND date >= $2
				AND ($3::date IS NULL OR date <= $3::date)
			ORDER BY date DESC
			LIMIT 1
		`
		args = []interface{}{userID, since, p.untilArg()}
	} else {
		query = `
			SELECT f.date FROM flights f
			INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
			WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = ANY($2) AND f.is_proficiency_check = true AND f.date >= $3
				AND (upper(trim(a.aircraft_class)) = 'GLIDER' OR COALESCE(f.launch_method, '') NOT IN ` + towedLaunches + `)
				AND ($4::date IS NULL OR f.date <= $4::date)
			ORDER BY f.date DESC
			LIMIT 1
		`
		args = []interface{}{userID, classTypeArray(classTypes), since, p.untilArg()}
	}

	var checkDate time.Time
	err := p.db.QueryRowContext(ctx, query, args...).Scan(&checkDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &checkDate, nil
}

func (p *currencyFlightDataProvider) GetLandingDaysByAircraftClass(ctx context.Context, userID uuid.UUID, classType models.ClassType, includeTowed, picOnly bool, since time.Time) ([]currency.LandingDay, error) {
	query := `
		SELECT
			f.date,
			COALESCE(SUM(f.landings_day), 0) as day_landings,
			COALESCE(SUM(f.landings_night), 0) as night_landings
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = $2 AND f.date >= $3
			AND ($4 OR COALESCE(f.launch_method, '') NOT IN ` + towedLaunches + `)
			AND (NOT $5 OR f.pic_time > 0)
			AND ($6::date IS NULL OR f.date <= $6::date)
		GROUP BY f.date
		HAVING SUM(f.landings_day + f.landings_night) > 0
		ORDER BY f.date DESC
	`

	rows, err := p.db.QueryContext(ctx, query, userID, string(classType), since, includeTowed, picOnly, p.untilArg())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanLandingDays(rows)
}

// scanLandingDays scans (date, day_landings, night_landings) rows.
func scanLandingDays(rows *sql.Rows) ([]currency.LandingDay, error) {
	var days []currency.LandingDay
	for rows.Next() {
		var d currency.LandingDay
		if err := rows.Scan(&d.Date, &d.DayLandings, &d.NightLandings); err != nil {
			return nil, err
		}
		d.Date = time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), 0, 0, 0, 0, time.UTC)
		days = append(days, d)
	}
	return days, rows.Err()
}

func (p *currencyFlightDataProvider) GetLaunchCounts(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) (map[string]int, error) {
	query := `
		SELECT f.launch_method, SUM(` + launchCountSQL + `) as launches
		FROM flights f
		INNER JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE f.user_id = $1 AND NOT f.is_simulator AND NOT f.is_passenger AND upper(trim(a.aircraft_class)) = $2 AND f.date >= $3
			AND f.launch_method IS NOT NULL AND f.launch_method != ''
			AND ($4::date IS NULL OR f.date <= $4::date)
		GROUP BY f.launch_method
	`
	rows, err := p.db.QueryContext(ctx, query, userID, string(classType), since, p.untilArg())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int)
	for rows.Next() {
		var method string
		var count int
		if err := rows.Scan(&method, &count); err != nil {
			return nil, err
		}
		counts[method] = count
	}
	return counts, rows.Err()
}

// classTypeArray converts class types to a Postgres text array.
func classTypeArray(classTypes []models.ClassType) interface{} {
	out := make([]string, len(classTypes))
	for i, ct := range classTypes {
		out[i] = string(ct)
	}
	return pq.Array(out)
}

// ulKindArray converts ultralight kinds to a Postgres text array.
func ulKindArray(kinds []models.ULKind) interface{} {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return pq.Array(out)
}
