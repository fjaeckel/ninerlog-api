package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func TestCustomReportAggregateSQL(t *testing.T) {
	tests := []struct {
		groupBy string
		want    []string
	}{
		{models.ReportGroupMonth, []string{`to_char(date, 'YYYY-MM') AS k`}},
		{models.ReportGroupLaunchMethod, []string{`LOWER(TRIM(COALESCE(launch_method, ''))) AS k`}},
		{models.ReportGroupAircraftClass, []string{`SELECT ac_class AS k`}},
		{models.ReportGroupULKind, []string{`CASE WHEN ac_class = 'ULTRALIGHT' THEN ac_ul_kind ELSE '' END AS k`}},
	}
	common := []string{
		"FROM (SELECT * FROM flights WHERE user_id = $1) f",
		"LEFT JOIN aircraft a ON a.registration = f.aircraft_reg AND a.user_id = f.user_id",
		launchCountSQL + " AS launch_count",
		soaringFlightSQL + " AS is_soaring",
		"COALESCE(SUM(launch_count) FILTER (WHERE is_soaring), 0)",
		"COUNT(*) FILTER (WHERE is_flight_time AND is_outlanding)",
		"COUNT(*) FILTER (WHERE is_flight_time AND is_tow_flight)",
	}
	for _, tt := range tests {
		t.Run(tt.groupBy, func(t *testing.T) {
			q, err := customReportAggregateSQL(tt.groupBy, "user_id = $1")
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range append(tt.want, common...) {
				if !strings.Contains(q, w) {
					t.Errorf("query lacks %q:\n%s", w, q)
				}
			}
		})
	}
	t.Run("every grouping has an expression", func(t *testing.T) {
		for _, g := range models.ReportGroupings {
			if _, err := customReportAggregateSQL(g, "true"); err != nil {
				t.Errorf("%s: %v", g, err)
			}
		}
	})
	t.Run("unknown grouping", func(t *testing.T) {
		if _, err := customReportAggregateSQL("pilot", "true"); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestSoaringFlightSQLScope(t *testing.T) {
	for _, w := range []string{
		"NOT f.is_simulator AND NOT f.is_passenger",
		"= 'GLIDER'",
		"= 'ULTRALIGHT' AND a.ul_kind = 'SAILPLANE'",
		"= 'TMG' AND TRIM(COALESCE(f.launch_method, '')) <> ''",
	} {
		if !strings.Contains(soaringFlightSQL, w) {
			t.Errorf("soaringFlightSQL lacks %q", w)
		}
	}
}

func TestCustomReportAggregateScansSoaringMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := []string{"k", "flights", "total", "pic", "dual", "dualGiven", "night", "ifr", "xc", "fstd", "landings", "launches", "outlandings", "tows"}
	mock.ExpectQuery("WITH scoped AS").WillReturnRows(sqlmock.NewRows(cols).
		AddRow("winch", 6, 48, 48, 0, 0, 0, 0, 0, 0, 6, 6, 0, 0).
		AddRow("aerotow", 1, 95, 95, 0, 0, 0, 0, 0, 0, 1, 1, 1, 0))

	repo := NewCustomReportRepository(db)
	groups, err := repo.Aggregate(context.Background(), uuid.New(), nil, models.ReportGroupLaunchMethod)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Totals.Launches != 6 || groups[1].Totals.Outlandings != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSoaringSeasonStatsEmptySkipsDetailQueries(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"f", "l", "m", "o"}).AddRow(0, 0, 0, 0))

	stats, err := NewSoaringRepository(db).SeasonStats(context.Background(), uuid.New(),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), 5)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Flights != 0 || stats.LongestFlight != nil || stats.Sites == nil || len(stats.Sites) != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSoaringSeasonStatsScansEverySection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := uuid.New()
	day := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"f", "l", "m", "o"}).AddRow(8, 9, 243, 1))
	mock.ExpectQuery("GROUP BY 1").WillReturnRows(sqlmock.NewRows([]string{"m", "n"}).AddRow("winch", 7).AddRow("aerotow", 1).AddRow("", 1))
	mock.ExpectQuery("ORDER BY f.total_time DESC").WillReturnRows(sqlmock.NewRows([]string{"id", "date", "t", "reg"}).AddRow(id, day, 185, "D-5678"))
	mock.ExpectQuery("LIMIT \\$4").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 5).
		WillReturnRows(sqlmock.NewRows([]string{"p", "n"}).AddRow("EDVM", 7).AddRow("EDVK", 1))

	stats, err := NewSoaringRepository(db).SeasonStats(context.Background(), uuid.New(), day, day, 5)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Launches != 9 || stats.LaunchesByMethod["winch"] != 7 || stats.LaunchesByMethod[""] != 1 {
		t.Errorf("launches = %d %v", stats.Launches, stats.LaunchesByMethod)
	}
	if stats.LongestFlight == nil || stats.LongestFlight.FlightID != id || *stats.LongestFlight.AircraftReg != "D-5678" {
		t.Errorf("longest = %+v", stats.LongestFlight)
	}
	if len(stats.Sites) != 2 || stats.Sites[0].Place != "EDVM" {
		t.Errorf("sites = %+v", stats.Sites)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
