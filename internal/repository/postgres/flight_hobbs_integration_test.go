//go:build integration

package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestHobbsPersistenceAndTotals(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ctx := context.Background()
	users := postgres.NewUserRepository(db)
	u := testutil.CreateTestUser("hobbs-pilot@example.test", "Pilot", "hashed")
	other := testutil.CreateTestUser("block-pilot@example.test", "Other", "hashed")
	require.NoError(t, users.Create(ctx, u))
	require.NoError(t, users.Create(ctx, other))
	u.DefaultFlightTimeMethod = models.FlightTimeHobbs
	u.TimeDisplayFormat = "hm"
	u.DateFormat = "YYYY-MM-DD"
	u.DecimalSeparator = "dot"
	require.NoError(t, users.Update(ctx, u))
	loaded, err := users.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "hobbs", loaded.DefaultFlightTimeMethod)
	loadedOther, err := users.GetByID(ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, "block", loadedOther.DefaultFlightTimeMethod)
	repo := postgres.NewFlightRepository(db)
	start, end := 15234, 15247
	f := &models.Flight{UserID: u.ID, Date: time.Now(), AircraftReg: "N12345", AircraftType: "C172", FlightTimeMethod: "hobbs", HobbsStartTenths: &start, HobbsEndTenths: &end, TotalTime: 78, PICTime: 78, LandingsDay: 1}
	require.NoError(t, repo.Create(ctx, f))
	legacy := &models.Flight{UserID: u.ID, Date: time.Now(), AircraftReg: "N12345", AircraftType: "C172", TotalTime: 83, PICTime: 83}
	require.NoError(t, repo.Create(ctx, legacy))
	saved, err := repo.GetByID(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, 78, saved.TotalTime)
	require.Equal(t, "hobbs", saved.FlightTimeMethod)
	require.Equal(t, &start, saved.HobbsStartTenths)
	require.Nil(t, saved.OffBlockTime)
	list, err := repo.GetByUserID(ctx, u.ID, nil)
	require.NoError(t, err)
	require.Len(t, list, 2)
	u.DefaultFlightTimeMethod = "block"
	require.NoError(t, users.Update(ctx, u))
	saved, err = repo.GetByID(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, 78, saved.TotalTime)
	require.Equal(t, "hobbs", saved.FlightTimeMethod)
	historical, err := repo.GetByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.Equal(t, 83, historical.TotalTime)
	var total, pic int
	require.NoError(t, db.QueryRow(`SELECT SUM(total_time), SUM(pic_time) FROM flights WHERE user_id=$1`, u.ID).Scan(&total, &pic))
	require.Equal(t, 161, total)
	require.Equal(t, 161, pic)
}

func TestHobbsMigration_H1PreservesHistoricalManualTotal(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ctx := context.Background()
	users := postgres.NewUserRepository(db)
	user := testutil.CreateTestUser("hobbs-migration@example.test", "Historical Pilot", "hashed")
	require.NoError(t, users.Create(ctx, user))
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	down, err := os.ReadFile(filepath.Join("..", "..", "..", "db", "migrations", "000080_add_hobbs_flight_time.down.sql"))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(down))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO flights (user_id,date,aircraft_reg,aircraft_type,off_block_time,on_block_time,total_time) VALUES ($1,'2026-11-14','N483HB','C172','08:00','10:00',83)`, user.ID)
	require.NoError(t, err)
	up, err := os.ReadFile(filepath.Join("..", "..", "..", "db", "migrations", "000080_add_hobbs_flight_time.up.sql"))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(up))
	require.NoError(t, err)
	var total int
	var method string
	var start, end *int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT total_time,flight_time_method,hobbs_start_tenths,hobbs_end_tenths FROM flights WHERE user_id=$1`, user.ID).Scan(&total, &method, &start, &end))
	require.Equal(t, 83, total)
	require.Equal(t, "block", method)
	require.Nil(t, start)
	require.Nil(t, end)
}

func TestManualCreditsPersistAndCurrencyAvoidsOverlap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ctx := context.Background()
	user := testutil.CreateTestUser("manual-pilot@example.test", "Pilot", "hashed")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, user))
	minutes := 90
	f := &models.Flight{UserID: user.ID, Date: time.Now(), AircraftReg: "N483MC", AircraftType: "C172", FlightTimeMethod: "manual", ManualTotalTime: &minutes, TotalTime: 90, PICTime: 66, DualTime: 90, PICTimeOverride: true, DualTimeOverride: true}
	repo := postgres.NewFlightRepository(db)
	require.NoError(t, repo.Create(ctx, f))
	saved, err := repo.GetByID(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, &minutes, saved.ManualTotalTime)
	require.True(t, saved.PICTimeOverride)
	require.True(t, saved.DualTimeOverride)
	saved.DualTime = 6
	require.NoError(t, repo.Update(ctx, saved))
	saved, err = repo.GetByID(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, 6, saved.DualTime)
	provider := postgres.NewCurrencyFlightDataProvider(db)
	progress, err := provider.GetProgressAll(ctx, user.ID, time.Now().AddDate(0, 0, -1))
	require.NoError(t, err)
	require.Equal(t, 66, progress.PICMinutes)
	require.Equal(t, 6, progress.InstructorMinutes)
	require.Equal(t, 6, progress.OverlappingPICDualMinutes)
	require.Equal(t, 6, progress.LongestTrainingFlightMinutes)
	list, err := repo.GetByUserID(ctx, user.ID, nil)
	require.NoError(t, err)
	require.Equal(t, &minutes, list[0].ManualTotalTime)
}

func TestLegacySplitPICDualCurrencyUnchanged(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ctx := context.Background()
	user := testutil.CreateTestUser("legacy-split@example.test", "Pilot", "hashed")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, user))
	f := &models.Flight{UserID: user.ID, Date: time.Now(), AircraftReg: "N483LS", AircraftType: "C172", TotalTime: 60, PICTime: 30, DualTime: 30}
	require.NoError(t, f.ValidateTimeDistribution())
	require.NoError(t, postgres.NewFlightRepository(db).Create(ctx, f))
	progress, err := postgres.NewCurrencyFlightDataProvider(db).GetProgressAll(ctx, user.ID, time.Now().AddDate(0, 0, -1))
	require.NoError(t, err)
	require.Equal(t, 30, progress.PICMinutes)
	require.Equal(t, 30, progress.InstructorMinutes)
	require.Zero(t, progress.OverlappingPICDualMinutes)
}
