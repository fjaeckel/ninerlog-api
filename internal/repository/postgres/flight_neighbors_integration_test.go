//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func TestFlightNeighborsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)

	user := testutil.CreateTestUser("neighbors-test@example.com", "Neighbors User", "hashedpass")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	day := func(d int) time.Time { return time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC) }
	at := func(hhmm string) *string { return &hhmm }
	// a: Aug 1; b and c: Aug 5, b off-block earlier; d: Aug 9, other aircraft.
	a := &models.Flight{UserID: user.ID, Date: day(1), AircraftReg: "D-EAAA", AircraftType: "C172", TotalTime: 60}
	b := &models.Flight{UserID: user.ID, Date: day(5), AircraftReg: "D-EAAA", AircraftType: "C172", TotalTime: 120, OffBlockTime: at("08:00:00")}
	c := &models.Flight{UserID: user.ID, Date: day(5), AircraftReg: "D-EAAA", AircraftType: "C172", TotalTime: 30, OffBlockTime: at("14:00:00")}
	d := &models.Flight{UserID: user.ID, Date: day(9), AircraftReg: "D-EBBB", AircraftType: "PA28", TotalTime: 90}
	for _, f := range []*models.Flight{a, b, c, d} {
		if err := flightRepo.Create(ctx, f); err != nil {
			t.Fatalf("Failed to create flight: %v", err)
		}
	}

	locate := func(t *testing.T, id uuid.UUID, opts *repository.FlightQueryOptions) *models.FlightNeighbors {
		t.Helper()
		n, err := flightRepo.NeighborsByUserID(ctx, user.ID, id, opts)
		if err != nil {
			t.Fatalf("NeighborsByUserID: %v", err)
		}
		return n
	}
	is := func(got *uuid.UUID, want *models.Flight) bool {
		if want == nil {
			return got == nil
		}
		return got != nil && *got == want.ID
	}

	t.Run("follows the list order, ties broken by off-block time", func(t *testing.T) {
		// date desc: d, c, b, a
		n := locate(t, c.ID, &repository.FlightQueryOptions{SortBy: "date", SortOrder: "desc"})
		if !is(n.PreviousID, d) || !is(n.NextID, b) || n.Position == nil || *n.Position != 2 || n.Total != 4 {
			t.Errorf("c in date desc = %+v, want prev d, next b, position 2 of 4", n)
		}
	})

	t.Run("has no neighbour past either end", func(t *testing.T) {
		opts := &repository.FlightQueryOptions{SortBy: "date", SortOrder: "desc"}
		if first := locate(t, d.ID, opts); first.PreviousID != nil || !is(first.NextID, c) {
			t.Errorf("first = %+v, want no previous, next c", first)
		}
		if last := locate(t, a.ID, opts); last.NextID != nil || !is(last.PreviousID, b) || *last.Position != 4 {
			t.Errorf("last = %+v, want previous b, no next, position 4", last)
		}
	})

	t.Run("follows another sort", func(t *testing.T) {
		// totalTime asc: c(30), a(60), d(90), b(120)
		n := locate(t, a.ID, &repository.FlightQueryOptions{SortBy: "totalTime", SortOrder: "asc"})
		if !is(n.PreviousID, c) || !is(n.NextID, d) || *n.Position != 2 {
			t.Errorf("a in totalTime asc = %+v, want prev c, next d, position 2", n)
		}
	})

	t.Run("follows the filters", func(t *testing.T) {
		reg := "D-EAAA"
		n := locate(t, b.ID, &repository.FlightQueryOptions{SortBy: "date", SortOrder: "desc", AircraftReg: &reg})
		if !is(n.PreviousID, c) || !is(n.NextID, a) || *n.Position != 2 || n.Total != 3 {
			t.Errorf("b filtered = %+v, want prev c, next a, position 2 of 3", n)
		}
	})

	t.Run("a flight outside the filters has no position", func(t *testing.T) {
		reg := "D-EAAA"
		n := locate(t, d.ID, &repository.FlightQueryOptions{AircraftReg: &reg})
		if n.Position != nil || n.PreviousID != nil || n.NextID != nil || n.Total != 3 {
			t.Errorf("d outside filter = %+v, want no position or neighbours, total 3", n)
		}
	})
}
