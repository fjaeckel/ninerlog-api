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

// deltaTick separates two writes far enough that their microsecond-precision
// updated_at values cannot collide, so "strictly after" is actually exercised.
const deltaTick = 5 * time.Millisecond

// The delta-sync filter is a SQL predicate on updated_at, and its two load-
// bearing properties — strictly-after (so a client can hand back the watermark
// it already stored without receiving that record again) and full timestamp
// precision (unlike the day-granular `date` filters) — can only be checked
// against real Postgres, where updated_at is written by a trigger or by an
// explicit UPDATE.
func TestListRepositoriesUpdatedSinceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)

	newUser := func(t *testing.T) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("delta-"+uuid.NewString()+"@example.com", "Delta User", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}

	t.Run("aircraft", func(t *testing.T) {
		repo := postgres.NewAircraftRepository(db)
		userID := newUser(t)

		first := &models.Aircraft{UserID: userID, Registration: "D-EAAA", Type: "C172", Make: "Cessna", Model: "172S", IsActive: true}
		if err := repo.Create(ctx, first); err != nil {
			t.Fatalf("create aircraft: %v", err)
		}
		watermark := storedAircraftUpdatedAt(t, repo, ctx, userID, first.ID)

		time.Sleep(deltaTick)
		second := &models.Aircraft{UserID: userID, Registration: "D-EBBB", Type: "PA28", Make: "Piper", Model: "Archer", IsActive: true}
		if err := repo.Create(ctx, second); err != nil {
			t.Fatalf("create second aircraft: %v", err)
		}

		delta, err := repo.GetByUserID(ctx, userID, &watermark)
		if err != nil {
			t.Fatalf("delta list: %v", err)
		}
		assertOnlyIDs(t, aircraftIDs(delta), second.ID)

		// Touching the first record brings it back into the same delta window.
		time.Sleep(deltaTick)
		first.Notes = ptr("club aircraft")
		if err := repo.Update(ctx, first); err != nil {
			t.Fatalf("update aircraft: %v", err)
		}
		delta, err = repo.GetByUserID(ctx, userID, &watermark)
		if err != nil {
			t.Fatalf("delta list after update: %v", err)
		}
		assertOnlyIDs(t, aircraftIDs(delta), first.ID, second.ID)

		// A nil filter is still the full listing.
		all, err := repo.GetByUserID(ctx, userID, nil)
		if err != nil {
			t.Fatalf("full list: %v", err)
		}
		if len(all) != 2 {
			t.Errorf("full list returned %d aircraft, want 2", len(all))
		}

		future := time.Now().Add(time.Hour)
		empty, err := repo.GetByUserID(ctx, userID, &future)
		if err != nil {
			t.Fatalf("future delta list: %v", err)
		}
		if len(empty) != 0 {
			t.Errorf("a future watermark returned %d aircraft, want 0", len(empty))
		}
	})

	t.Run("contacts", func(t *testing.T) {
		repo := postgres.NewContactRepository(db)
		userID := newUser(t)

		first := &models.Contact{UserID: userID, Name: "Anna Instructor"}
		if err := repo.Create(ctx, first); err != nil {
			t.Fatalf("create contact: %v", err)
		}
		stored, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("reload contact: %v", err)
		}
		watermark := stored.UpdatedAt

		time.Sleep(deltaTick)
		second := &models.Contact{UserID: userID, Name: "Ben Examiner"}
		if err := repo.Create(ctx, second); err != nil {
			t.Fatalf("create second contact: %v", err)
		}

		delta, err := repo.GetByUserID(ctx, userID, &watermark)
		if err != nil {
			t.Fatalf("delta list: %v", err)
		}
		if len(delta) != 1 || delta[0].ID != second.ID {
			t.Fatalf("delta list = %d contacts, want only the second", len(delta))
		}

		all, err := repo.GetByUserID(ctx, userID, nil)
		if err != nil {
			t.Fatalf("full list: %v", err)
		}
		if len(all) != 2 {
			t.Errorf("full list returned %d contacts, want 2", len(all))
		}
	})

	t.Run("credentials", func(t *testing.T) {
		repo := postgres.NewCredentialRepository(db)
		userID := newUser(t)

		first := &models.Credential{
			UserID: userID, CredentialType: models.CredentialTypeEASAClass2Medical,
			IssueDate: time.Now().AddDate(-1, 0, 0), IssuingAuthority: "LBA",
		}
		if err := repo.Create(ctx, first); err != nil {
			t.Fatalf("create credential: %v", err)
		}
		stored, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("reload credential: %v", err)
		}
		watermark := stored.UpdatedAt

		time.Sleep(deltaTick)
		second := &models.Credential{
			UserID: userID, CredentialType: models.CredentialTypeLangICAOLevel4,
			IssueDate: time.Now().AddDate(-1, 0, 0), IssuingAuthority: "LBA",
		}
		if err := repo.Create(ctx, second); err != nil {
			t.Fatalf("create second credential: %v", err)
		}

		delta, err := repo.GetByUserID(ctx, userID, &watermark)
		if err != nil {
			t.Fatalf("delta list: %v", err)
		}
		if len(delta) != 1 || delta[0].ID != second.ID {
			t.Fatalf("delta list = %d credentials, want only the second", len(delta))
		}
	})

	t.Run("licenses", func(t *testing.T) {
		repo := postgres.NewLicenseRepository(db)
		userID := newUser(t)

		first := &models.License{
			UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
			LicenseNumber: "DE-PPL-1", IssueDate: time.Now().AddDate(-2, 0, 0), IssuingAuthority: "LBA",
		}
		if err := repo.Create(ctx, first); err != nil {
			t.Fatalf("create license: %v", err)
		}
		watermark := first.UpdatedAt

		time.Sleep(deltaTick)
		second := &models.License{
			UserID: userID, RegulatoryAuthority: "FAA", LicenseType: "PPL",
			LicenseNumber: "US-PPL-1", IssueDate: time.Now().AddDate(-1, 0, 0), IssuingAuthority: "FAA",
		}
		if err := repo.Create(ctx, second); err != nil {
			t.Fatalf("create second license: %v", err)
		}

		delta, err := repo.GetByUserID(ctx, userID, &watermark)
		if err != nil {
			t.Fatalf("delta list: %v", err)
		}
		if len(delta) != 1 || delta[0].ID != second.ID {
			t.Fatalf("delta list = %d licenses, want only the second", len(delta))
		}
	})

	t.Run("flights", func(t *testing.T) {
		repo := postgres.NewFlightRepository(db)
		userID := newUser(t)

		// Both flights are logged on the same calendar day: the day-granular
		// `q=updatedAt>YYYY-MM-DD` workaround cannot separate them, which is
		// the gap updatedSince exists to close.
		first := deltaTestFlight(userID, "D-EAAA")
		if err := repo.Create(ctx, first); err != nil {
			t.Fatalf("create flight: %v", err)
		}
		stored, err := repo.GetByID(ctx, first.ID)
		if err != nil {
			t.Fatalf("reload flight: %v", err)
		}
		watermark := stored.UpdatedAt

		time.Sleep(deltaTick)
		second := deltaTestFlight(userID, "D-EBBB")
		if err := repo.Create(ctx, second); err != nil {
			t.Fatalf("create second flight: %v", err)
		}

		opts := &repository.FlightQueryOptions{Page: 1, PageSize: 20, UpdatedSince: &watermark}
		delta, err := repo.GetByUserID(ctx, userID, opts)
		if err != nil {
			t.Fatalf("delta list: %v", err)
		}
		if len(delta) != 1 || delta[0].ID != second.ID {
			t.Fatalf("delta list = %d flights, want only the second", len(delta))
		}

		// Counting must apply the same predicate, or pagination reports a
		// total the caller can never page to.
		count, err := repo.CountByUserID(ctx, userID, opts)
		if err != nil {
			t.Fatalf("delta count: %v", err)
		}
		if count != 1 {
			t.Errorf("delta count = %d, want 1", count)
		}

		// ANDs with the other filters rather than replacing them.
		reg := "D-EAAA"
		narrowed := &repository.FlightQueryOptions{
			Page: 1, PageSize: 20, UpdatedSince: &watermark, AircraftReg: &reg,
		}
		none, err := repo.GetByUserID(ctx, userID, narrowed)
		if err != nil {
			t.Fatalf("narrowed delta list: %v", err)
		}
		if len(none) != 0 {
			t.Errorf("updatedSince AND aircraftReg returned %d flights, want 0", len(none))
		}
	})
}

func deltaTestFlight(userID uuid.UUID, reg string) *models.Flight {
	return &models.Flight{
		UserID:       userID,
		Date:         time.Now().Truncate(24 * time.Hour),
		AircraftReg:  reg,
		AircraftType: "C172",
		TotalTime:    60,
		IsPIC:        true,
	}
}

func storedAircraftUpdatedAt(t *testing.T, repo repository.AircraftRepository, ctx context.Context, userID, id uuid.UUID) time.Time {
	t.Helper()
	all, err := repo.GetByUserID(ctx, userID, nil)
	if err != nil {
		t.Fatalf("list aircraft: %v", err)
	}
	for _, a := range all {
		if a.ID == id {
			return a.UpdatedAt
		}
	}
	t.Fatalf("aircraft %s not found", id)
	return time.Time{}
}

func aircraftIDs(list []*models.Aircraft) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(list))
	for _, a := range list {
		ids = append(ids, a.ID)
	}
	return ids
}

func assertOnlyIDs(t *testing.T, got []uuid.UUID, want ...uuid.UUID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d ids %v, want %d %v", len(got), got, len(want), want)
	}
	seen := make(map[uuid.UUID]bool, len(got))
	for _, id := range got {
		seen[id] = true
	}
	for _, id := range want {
		if !seen[id] {
			t.Errorf("expected id %s in %v", id, got)
		}
	}
}

func ptr[T any](v T) *T { return &v }
