package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
)

// GetActivityDays implements currency.PrivilegeDataProvider.
func (p *currencyFlightDataProvider) GetActivityDays(ctx context.Context, userID uuid.UUID, q currency.ActivityQuery, since time.Time) ([]currency.ActivityDay, error) {
	args := []any{userID, since}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	conds := []string{"f.user_id = $1", "NOT f.is_simulator", "NOT f.is_passenger", "f.date >= $2"}
	class := "upper(trim(COALESCE(a.aircraft_class, '')))"
	switch {
	case len(q.Classes) > 0 && len(q.ULKinds) > 0:
		conds = append(conds, fmt.Sprintf("(%s = ANY(%s) OR (%s = 'ULTRALIGHT' AND a.ul_kind = ANY(%s)))",
			class, arg(classTypeArray(q.Classes)), class, arg(ulKindArray(q.ULKinds))))
	case len(q.Classes) > 0:
		conds = append(conds, fmt.Sprintf("%s = ANY(%s)", class, arg(classTypeArray(q.Classes))))
	case len(q.ULKinds) > 0:
		conds = append(conds, fmt.Sprintf("%s = 'ULTRALIGHT' AND a.ul_kind = ANY(%s)", class, arg(ulKindArray(q.ULKinds))))
	}
	if q.ExcludeUL {
		conds = append(conds, class+" <> 'ULTRALIGHT'")
	}
	if q.TowOnly {
		conds = append(conds, "f.is_tow_flight")
	}
	if q.IFROnly {
		conds = append(conds, "f.ifr_time > 0")
	}
	if q.PICOnly {
		conds = append(conds, "f.pic_time > 0")
	}
	if q.DualGivenOnly {
		conds = append(conds, "f.dual_given_time > 0")
	}
	if q.DualReceivedOnly {
		conds = append(conds, "f.dual_time > 0")
	}
	if q.CrossCountryOnly {
		conds = append(conds, "f.cross_country_time > 0")
	}
	if q.LaunchMethod != "" {
		conds = append(conds, "f.launch_method = "+arg(q.LaunchMethod))
	}
	query := `
		SELECT
			f.date,
			COUNT(*),
			COALESCE(SUM(f.pic_time), 0),
			COALESCE(SUM(f.ifr_time), 0),
			COALESCE(SUM(f.dual_given_time), 0),
			COALESCE(SUM(` + launchCountSQL + `), 0),
			COUNT(*) FILTER (WHERE f.landings_day + f.landings_night >= 2),
			COALESCE(SUM(f.distance), 0)
		FROM flights f
		LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id
		WHERE ` + strings.Join(conds, " AND ") + `
		GROUP BY f.date
		ORDER BY f.date DESC`
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []currency.ActivityDay
	for rows.Next() {
		var d currency.ActivityDay
		if err := rows.Scan(&d.Date, &d.Flights, &d.PICMinutes, &d.IFRMinutes, &d.DualGivenMinutes,
			&d.Launches, &d.MultiLandingFlights, &d.DistanceNM); err != nil {
			return nil, err
		}
		d.Date = time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), 0, 0, 0, 0, time.UTC)
		out = append(out, d)
	}
	return out, rows.Err()
}

var _ currency.PrivilegeDataProvider = (*currencyFlightDataProvider)(nil)
