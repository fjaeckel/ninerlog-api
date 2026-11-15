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

func TestPreservedSourceAndOverridePersistence(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ctx := context.Background()
	u := testutil.CreateTestUser("refinement@example.test", "Pilot", "hashed")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	start, end, entered := 15234, 15247, 90
	repo := postgres.NewFlightRepository(db)
	f := &models.Flight{UserID: u.ID, Date: time.Now(), AircraftReg: "N483QA", AircraftType: "C172", FlightTimeMethod: "hobbs", HobbsStartTenths: &start, HobbsEndTenths: &end, ManualTotalTime: &entered, TotalTime: 90, PICTime: 90, LandingsDay: 1}
	require.NoError(t, repo.Create(ctx, f))
	saved, err := repo.GetByID(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, &entered, saved.ManualTotalTime)
	require.Equal(t, &start, saved.HobbsStartTenths)
	saved.FlightTimeMethod = "manual"
	require.NoError(t, repo.Update(ctx, saved))
	saved, err = repo.GetByID(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, &end, saved.HobbsEndTenths)
	require.Equal(t, 90, saved.TotalTime)
	// Downgrade must refuse to erase retained evidence or an expanded override.
	down, err := os.ReadFile(filepath.Join("..", "..", "..", "db", "migrations", "000081_preserve_flight_time_sources.down.sql"))
	require.NoError(t, err)
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(down))
	require.ErrorContains(t, err, "Preserved source observations")
	require.NoError(t, tx.Rollback())
	saved, err = repo.GetByID(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, 90, saved.TotalTime)
}

func TestSourceConstraintMigrationPreservesLegacyTotals(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ctx := context.Background()
	u := testutil.CreateTestUser("legacy-refinement@example.test", "Pilot", "hashed")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	f := &models.Flight{UserID: u.ID, Date: time.Now(), AircraftReg: "N483QA", AircraftType: "C172", TotalTime: 83, PICTime: 83}
	repo := postgres.NewFlightRepository(db)
	require.NoError(t, repo.Create(ctx, f))
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for _, direction := range []string{"down", "up"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "db", "migrations", "000081_preserve_flight_time_sources."+direction+".sql"))
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(sql))
		require.NoError(t, err)
	}
	var total int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT total_time FROM flights WHERE id=$1`, f.ID).Scan(&total))
	require.Equal(t, 83, total)
}
