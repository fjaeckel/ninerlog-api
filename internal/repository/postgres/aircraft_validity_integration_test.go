//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
)

func TestAircraftRegistrationValidityIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	aircraftRepo := postgres.NewAircraftRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	reportsRepo := postgres.NewReportsRepository(db)

	user := testutil.CreateTestUser("validity@example.com", "Validity User", "hash")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	day := func(y int, m time.Month, d int) *time.Time {
		v := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &v
	}
	sep, mep := "SEP_LAND", "MEP_LAND"
	cessna := &models.Aircraft{
		UserID: user.ID, Registration: "N123AB", Type: "C172", Make: "Cessna", Model: "172",
		AircraftClass: &sep, IsActive: true, ValidTo: day(2019, 12, 31),
	}
	other := &models.Aircraft{
		UserID: user.ID, Registration: "N123AB", Type: "PA16", Make: "Piper", Model: "Clipper",
		AircraftClass: &mep, IsTailwheel: true, IsActive: true, ValidFrom: day(2020, 1, 1),
	}
	if err := aircraftRepo.Create(ctx, cessna); err != nil {
		t.Fatalf("create first N123AB: %v", err)
	}
	if err := aircraftRepo.Create(ctx, other); err != nil {
		t.Fatalf("create second N123AB with a disjoint range: %v", err)
	}

	t.Run("validity dates round-trip", func(t *testing.T) {
		got, err := aircraftRepo.GetByID(ctx, other.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ValidTo != nil || got.ValidFrom == nil || got.ValidFrom.Format(time.DateOnly) != "2020-01-01" {
			t.Errorf("validity = %v..%v, want 2020-01-01..open", got.ValidFrom, got.ValidTo)
		}
	})

	t.Run("overlapping range is rejected", func(t *testing.T) {
		dup := &models.Aircraft{
			UserID: user.ID, Registration: "N123AB", Type: "PA18", Make: "Piper", Model: "Super Cub",
			ValidFrom: day(2019, 12, 31),
		}
		if err := aircraftRepo.Create(ctx, dup); !errors.Is(err, repository.ErrDuplicateRegistration) {
			t.Errorf("Create overlapping = %v, want ErrDuplicateRegistration", err)
		}
	})

	t.Run("inverted range is rejected", func(t *testing.T) {
		bad := &models.Aircraft{
			UserID: user.ID, Registration: "D-EINV", Type: "C152", Make: "Cessna", Model: "152",
			ValidFrom: day(2020, 1, 2), ValidTo: day(2020, 1, 1),
		}
		if err := aircraftRepo.Create(ctx, bad); !errors.Is(err, models.ErrInvalidAircraftValidity) {
			t.Errorf("Create inverted = %v, want ErrInvalidAircraftValidity", err)
		}
	})

	flights := []*models.Flight{
		{UserID: user.ID, Date: *day(2018, 5, 1), AircraftReg: "N123AB", AircraftType: "C172", TotalTime: 60, PICTime: 60, LandingsDay: 1, AllLandings: 1},
		{UserID: user.ID, Date: *day(2019, 12, 31), AircraftReg: "N123AB", AircraftType: "C172", TotalTime: 30, PICTime: 30, LandingsDay: 1, AllLandings: 1},
		{UserID: user.ID, Date: *day(2020, 1, 1), AircraftReg: "N123AB", AircraftType: "PA16", TotalTime: 45, PICTime: 45, LandingsDay: 1, AllLandings: 1},
	}
	for _, f := range flights {
		if err := flightRepo.Create(ctx, f); err != nil {
			t.Fatalf("create flight: %v", err)
		}
	}

	t.Run("each flight joins exactly one fleet entry", func(t *testing.T) {
		rows, err := reportsRepo.StatsByClass(ctx, user.ID, 0)
		if err != nil {
			t.Fatalf("StatsByClass: %v", err)
		}
		got := map[string]int{}
		total := 0
		for _, r := range rows {
			got[r.Class] = r.Minutes
			total += r.Flights
		}
		if total != 3 {
			t.Errorf("flights across classes = %d, want 3 (no double counting)", total)
		}
		if got[sep] != 90 || got[mep] != 45 {
			t.Errorf("minutes by class = %v, want SEP_LAND 90, MEP_LAND 45", got)
		}
	})

	t.Run("capabilities follow the airframe of the day", func(t *testing.T) {
		rows, err := reportsRepo.StatsByCategory(ctx, user.ID, 0)
		if err != nil {
			t.Fatalf("StatsByCategory: %v", err)
		}
		for _, r := range rows {
			if r.Category == "Tailwheel" && (r.Flights != 1 || r.PICMinutes != 45) {
				t.Errorf("tailwheel = %+v, want 1 flight / 45 min", r)
			}
		}
	})

	t.Run("rename touches only flights in the aircraft's range", func(t *testing.T) {
		other.Registration = "N123AC"
		n, err := aircraftRepo.UpdateWithFlightRename(ctx, other, "N123AB")
		if err != nil {
			t.Fatalf("UpdateWithFlightRename: %v", err)
		}
		if n != 1 {
			t.Errorf("flights renamed = %d, want 1", n)
		}
	})
}
