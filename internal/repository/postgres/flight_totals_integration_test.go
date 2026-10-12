//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/flightsearch"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
)

func TestFlightTotalsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)

	user := testutil.CreateTestUser("totals-test@example.com", "Totals User", "hashedpass")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	other := testutil.CreateTestUser("totals-other@example.com", "Other User", "hashedpass")
	if err := userRepo.Create(ctx, other); err != nil {
		t.Fatalf("Failed to create other user: %v", err)
	}

	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	flights := []*models.Flight{
		{UserID: user.ID, Date: day(7, 3), AircraftReg: "D-EAAA", AircraftType: "C172", TotalTime: 60, PICTime: 60, NightTime: 20, LandingsDay: 1, TakeoffsDay: 1, Holds: 1, ApproachesCount: 2, Distance: 40},
		{UserID: user.ID, Date: day(8, 1), AircraftReg: "D-EAAA", AircraftType: "C172", TotalTime: 90, PICTime: 90, IFRTime: 30, LandingsDay: 2, LandingsNight: 1, TakeoffsDay: 3, Distance: 60.5},
		{UserID: user.ID, Date: day(8, 20), AircraftReg: "D-EBBB", AircraftType: "PA28", TotalTime: 45, DualTime: 45, LandingsDay: 1, TakeoffsDay: 1},
		{UserID: other.ID, Date: day(8, 5), AircraftReg: "D-EAAA", AircraftType: "C172", TotalTime: 500, PICTime: 500},
	}
	for _, f := range flights {
		if err := flightRepo.Create(ctx, f); err != nil {
			t.Fatalf("Failed to create flight: %v", err)
		}
	}

	t.Run("sums every column over the user's flights", func(t *testing.T) {
		totals, err := flightRepo.TotalsByUserID(ctx, user.ID, nil)
		if err != nil {
			t.Fatalf("TotalsByUserID: %v", err)
		}
		if totals.Flights != 3 || totals.TotalTime != 195 || totals.PICTime != 150 || totals.DualTime != 45 {
			t.Errorf("counts/times = %+v", totals)
		}
		if totals.NightTime != 20 || totals.IFRTime != 30 {
			t.Errorf("night/ifr = %d/%d, want 20/30", totals.NightTime, totals.IFRTime)
		}
		if totals.LandingsDay != 4 || totals.LandingsNight != 1 || totals.TakeoffsDay != 5 {
			t.Errorf("landings/takeoffs = %+v", totals)
		}
		if totals.Approaches != 2 || totals.Holds != 1 || totals.Distance != 100.5 {
			t.Errorf("approaches/holds/distance = %d/%d/%v", totals.Approaches, totals.Holds, totals.Distance)
		}
	})

	t.Run("honours the list filters", func(t *testing.T) {
		reg := "D-EAAA"
		totals, err := flightRepo.TotalsByUserID(ctx, user.ID, &repository.FlightQueryOptions{AircraftReg: &reg})
		if err != nil {
			t.Fatalf("TotalsByUserID: %v", err)
		}
		if totals.Flights != 2 || totals.TotalTime != 150 {
			t.Errorf("filtered totals = %d flights, %d min; want 2, 150", totals.Flights, totals.TotalTime)
		}
	})

	t.Run("honours a free-text search", func(t *testing.T) {
		search := "PA28"
		totals, err := flightRepo.TotalsByUserID(ctx, user.ID, &repository.FlightQueryOptions{Search: &search})
		if err != nil {
			t.Fatalf("TotalsByUserID: %v", err)
		}
		if totals.Flights != 1 || totals.TotalTime != 45 {
			t.Errorf("search totals = %d flights, %d min; want 1, 45", totals.Flights, totals.TotalTime)
		}
	})

	t.Run("honours an advanced query, per month too", func(t *testing.T) {
		q, err := flightsearch.Parse("night>0 OR ifr>0")
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		opts := &repository.FlightQueryOptions{Query: q}
		totals, err := flightRepo.TotalsByUserID(ctx, user.ID, opts)
		if err != nil {
			t.Fatalf("TotalsByUserID: %v", err)
		}
		if totals.Flights != 2 || totals.TotalTime != 150 {
			t.Errorf("query totals = %d flights, %d min; want 2, 150", totals.Flights, totals.TotalTime)
		}
		months, err := flightRepo.MonthTotalsByUserID(ctx, user.ID, opts, []time.Time{day(7, 1), day(8, 1)})
		if err != nil {
			t.Fatalf("MonthTotalsByUserID: %v", err)
		}
		if len(months) != 2 || months[1].Flights != 1 || months[1].TotalTime != 90 {
			t.Errorf("query month totals = %+v, want July and August with one flight each", months)
		}
	})

	t.Run("returns zeros when nothing matches", func(t *testing.T) {
		reg := "D-NONE"
		totals, err := flightRepo.TotalsByUserID(ctx, user.ID, &repository.FlightQueryOptions{AircraftReg: &reg})
		if err != nil {
			t.Fatalf("TotalsByUserID: %v", err)
		}
		if totals.Flights != 0 || totals.TotalTime != 0 || totals.Distance != 0 {
			t.Errorf("empty totals = %+v", totals)
		}
	})

	t.Run("sums per month, only for the months asked", func(t *testing.T) {
		months, err := flightRepo.MonthTotalsByUserID(ctx, user.ID, nil, []time.Time{day(8, 20), day(9, 1)})
		if err != nil {
			t.Fatalf("MonthTotalsByUserID: %v", err)
		}
		if len(months) != 1 {
			t.Fatalf("got %d months, want 1 (August only)", len(months))
		}
		aug := months[0]
		if aug.Month.Format("2006-01") != "2026-08" || aug.Flights != 2 || aug.TotalTime != 135 {
			t.Errorf("August = %s, %d flights, %d min; want 2026-08, 2, 135", aug.Month.Format("2006-01"), aug.Flights, aug.TotalTime)
		}
	})

	t.Run("no months asked, no rows", func(t *testing.T) {
		months, err := flightRepo.MonthTotalsByUserID(ctx, user.ID, nil, nil)
		if err != nil {
			t.Fatalf("MonthTotalsByUserID: %v", err)
		}
		if len(months) != 0 {
			t.Errorf("got %d months, want none", len(months))
		}
	})
}
