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
	"github.com/google/uuid"
)

// TestAircraftReminderRepositoryIntegration covers CRUD, due-date ordering,
// the due-date bound, the registration join, cross-user isolation and the
// cascade from aircraft against real Postgres.
func TestAircraftReminderRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	aircraftRepo := postgres.NewAircraftRepository(db)
	repo := postgres.NewAircraftReminderRepository(db)

	newUser := func(t *testing.T) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("rem-"+uuid.NewString()+"@example.com", "Reminder User", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}
	newAircraft := func(t *testing.T, userID uuid.UUID, reg string) uuid.UUID {
		t.Helper()
		a := &models.Aircraft{UserID: userID, Registration: reg, Type: "C42", Make: "Comco Ikarus", Model: "C42", IsActive: true}
		if err := aircraftRepo.Create(ctx, a); err != nil {
			t.Fatalf("create aircraft: %v", err)
		}
		return a.ID
	}
	date := func(s string) time.Time {
		d, _ := time.Parse("2006-01-02", s)
		return d
	}

	mehmet := newUser(t)
	other := newUser(t)
	c42 := newAircraft(t, mehmet, "D-MXYZ")
	trike := newAircraft(t, mehmet, "D-MTRK")
	otherAC := newAircraft(t, other, "D-EFGH")

	interval := 12
	label := "Hull policy"
	notes := "Allianz"
	annual := &models.AircraftReminder{UserID: mehmet, AircraftID: c42, Kind: models.ReminderKindAnnualInspection, DueDate: date("2026-10-15"), IntervalMonths: &interval}
	insurance := &models.AircraftReminder{UserID: mehmet, AircraftID: trike, Kind: models.ReminderKindInsurance, Label: &label, Notes: &notes, DueDate: date("2026-09-01")}
	elt := &models.AircraftReminder{UserID: mehmet, AircraftID: c42, Kind: models.ReminderKindELTBattery, DueDate: date("2028-01-01")}
	theirs := &models.AircraftReminder{UserID: other, AircraftID: otherAC, Kind: models.ReminderKindARC, DueDate: date("2026-09-10")}
	for _, r := range []*models.AircraftReminder{annual, insurance, elt, theirs} {
		if err := repo.Create(ctx, r); err != nil {
			t.Fatalf("create reminder: %v", err)
		}
		if r.ID == uuid.Nil || r.CreatedAt.IsZero() {
			t.Fatal("create must assign id and timestamps")
		}
	}

	t.Run("get joins the registration and round-trips every field", func(t *testing.T) {
		got, err := repo.GetByID(ctx, insurance.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AircraftRegistration != "D-MTRK" || got.Label == nil || *got.Label != label ||
			got.Notes == nil || *got.Notes != notes || got.DueDate.Format("2006-01-02") != "2026-09-01" ||
			got.IntervalMonths != nil || got.LastDoneOn != nil {
			t.Errorf("unexpected reminder: %+v", got)
		}
	})

	t.Run("list by aircraft is ordered by due date", func(t *testing.T) {
		got, err := repo.ListByAircraft(ctx, c42)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != annual.ID || got[1].ID != elt.ID {
			t.Fatalf("unexpected list: %+v", got)
		}
	})

	t.Run("list by user spans aircraft and excludes other users", func(t *testing.T) {
		got, err := repo.ListByUser(ctx, mehmet, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got[0].ID != insurance.ID || got[1].ID != annual.ID || got[2].ID != elt.ID {
			t.Fatalf("unexpected list: %+v", got)
		}
	})

	t.Run("due bound is inclusive", func(t *testing.T) {
		bound := date("2026-10-15")
		got, err := repo.ListByUser(ctx, mehmet, &bound)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d reminders, want 2", len(got))
		}
	})

	t.Run("update writes the mutable fields", func(t *testing.T) {
		done := date("2026-10-02")
		annual.LastDoneOn = &done
		annual.DueDate = date("2027-10-02")
		annual.IntervalMonths = nil
		if err := repo.Update(ctx, annual); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetByID(ctx, annual.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.DueDate.Format("2006-01-02") != "2027-10-02" || got.LastDoneOn == nil ||
			got.LastDoneOn.Format("2006-01-02") != "2026-10-02" || got.IntervalMonths != nil {
			t.Errorf("unexpected reminder after update: %+v", got)
		}
	})

	t.Run("custom without label is rejected by the schema", func(t *testing.T) {
		bad := &models.AircraftReminder{UserID: mehmet, AircraftID: c42, Kind: models.ReminderKindCustom, DueDate: date("2027-01-01")}
		if err := repo.Create(ctx, bad); err == nil {
			t.Fatal("expected a check-constraint violation")
		}
	})

	t.Run("delete and not found", func(t *testing.T) {
		if err := repo.Delete(ctx, elt.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetByID(ctx, elt.ID); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("GetByID after delete = %v, want ErrNotFound", err)
		}
		if err := repo.Delete(ctx, elt.ID); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("second Delete = %v, want ErrNotFound", err)
		}
	})

	t.Run("deleting the aircraft cascades", func(t *testing.T) {
		if err := aircraftRepo.Delete(ctx, trike); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetByID(ctx, insurance.ID); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("reminder survived its aircraft: %v", err)
		}
	})
}
