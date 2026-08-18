//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

// TestDeletionTombstoneTriggersIntegration covers the AFTER DELETE tombstone
// triggers against real Postgres.
func TestDeletionTombstoneTriggersIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	deletionRepo := postgres.NewDeletionRepository(db)

	newUser := func(t *testing.T) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("tomb-"+uuid.NewString()+"@example.com", "Tomb User", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}

	// epoch is before anything this test creates.
	epoch := time.Now().Add(-time.Hour)

	listAll := func(t *testing.T, userID uuid.UUID) []*models.Deletion {
		t.Helper()
		got, err := deletionRepo.ListSince(ctx, userID, epoch, nil, 500, 0)
		if err != nil {
			t.Fatalf("list deletions: %v", err)
		}
		return got
	}

	t.Run("every entity type records a tombstone on delete", func(t *testing.T) {
		userID := newUser(t)

		flightRepo := postgres.NewFlightRepository(db)
		flight := deltaTestFlight(userID, "D-ETMB")
		if err := flightRepo.Create(ctx, flight); err != nil {
			t.Fatalf("create flight: %v", err)
		}

		aircraftRepo := postgres.NewAircraftRepository(db)
		aircraft := &models.Aircraft{UserID: userID, Registration: "D-ETMB", Type: "C172", IsActive: true}
		if err := aircraftRepo.Create(ctx, aircraft); err != nil {
			t.Fatalf("create aircraft: %v", err)
		}

		contactRepo := postgres.NewContactRepository(db)
		contact := &models.Contact{UserID: userID, Name: "Tombstone Contact"}
		if err := contactRepo.Create(ctx, contact); err != nil {
			t.Fatalf("create contact: %v", err)
		}

		credentialRepo := postgres.NewCredentialRepository(db)
		credential := &models.Credential{
			UserID: userID, CredentialType: models.CredentialTypeEASAClass2Medical,
			IssueDate: time.Now().AddDate(-1, 0, 0), IssuingAuthority: "LBA",
		}
		if err := credentialRepo.Create(ctx, credential); err != nil {
			t.Fatalf("create credential: %v", err)
		}

		licenseRepo := postgres.NewLicenseRepository(db)
		license := &models.License{
			UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
			LicenseNumber: "DE-TMB-1", IssueDate: time.Now().AddDate(-2, 0, 0), IssuingAuthority: "LBA",
		}
		if err := licenseRepo.Create(ctx, license); err != nil {
			t.Fatalf("create license: %v", err)
		}

		if err := flightRepo.Delete(ctx, flight.ID); err != nil {
			t.Fatalf("delete flight: %v", err)
		}
		if err := aircraftRepo.Delete(ctx, aircraft.ID); err != nil {
			t.Fatalf("delete aircraft: %v", err)
		}
		if err := contactRepo.Delete(ctx, contact.ID); err != nil {
			t.Fatalf("delete contact: %v", err)
		}
		if err := credentialRepo.Delete(ctx, credential.ID); err != nil {
			t.Fatalf("delete credential: %v", err)
		}
		if err := licenseRepo.Delete(ctx, license.ID); err != nil {
			t.Fatalf("delete license: %v", err)
		}

		want := map[models.DeletionEntityType]uuid.UUID{
			models.DeletionEntityFlight:     flight.ID,
			models.DeletionEntityAircraft:   aircraft.ID,
			models.DeletionEntityContact:    contact.ID,
			models.DeletionEntityCredential: credential.ID,
			models.DeletionEntityLicense:    license.ID,
		}
		got := listAll(t, userID)
		if len(got) != len(want) {
			t.Fatalf("recorded %d tombstones, want %d: %+v", len(got), len(want), got)
		}
		for _, d := range got {
			id, ok := want[d.EntityType]
			if !ok {
				t.Errorf("unexpected entity type %q", d.EntityType)
				continue
			}
			if d.EntityID != id {
				t.Errorf("%s tombstone has id %s, want %s", d.EntityType, d.EntityID, id)
			}
			delete(want, d.EntityType)
		}
		for entity := range want {
			t.Errorf("no tombstone recorded for %s", entity)
		}
	})

	t.Run("raw SQL bulk deletes are recorded too", func(t *testing.T) {
		userID := newUser(t)
		flightRepo := postgres.NewFlightRepository(db)
		for _, reg := range []string{"D-EBLK1", "D-EBLK2", "D-EBLK3"} {
			if err := flightRepo.Create(ctx, deltaTestFlight(userID, reg)); err != nil {
				t.Fatalf("create flight %s: %v", reg, err)
			}
		}

		// Raw SQL delete, bypassing every repository.
		if _, err := db.ExecContext(ctx, `DELETE FROM flights WHERE user_id = $1`, userID); err != nil {
			t.Fatalf("bulk delete: %v", err)
		}

		got := listAll(t, userID)
		if len(got) != 3 {
			t.Fatalf("bulk delete recorded %d tombstones, want 3", len(got))
		}
	})

	t.Run("deleting the account leaves no tombstones behind", func(t *testing.T) {
		userID := newUser(t)
		contactRepo := postgres.NewContactRepository(db)
		contact := &models.Contact{UserID: userID, Name: "Doomed"}
		if err := contactRepo.Create(ctx, contact); err != nil {
			t.Fatalf("create contact: %v", err)
		}

		// Deleting the user cascades to contacts; the trigger must skip the
		// cascaded rows.
		if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			t.Fatalf("delete user: %v", err)
		}

		var remaining int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM deletion_tombstones WHERE user_id = $1`, userID).Scan(&remaining); err != nil {
			t.Fatalf("count tombstones: %v", err)
		}
		if remaining != 0 {
			t.Errorf("%d tombstones outlived the deleted account", remaining)
		}
	})

	t.Run("tombstones are user-scoped", func(t *testing.T) {
		ownerID := newUser(t)
		otherID := newUser(t)

		contactRepo := postgres.NewContactRepository(db)
		contact := &models.Contact{UserID: ownerID, Name: "Owner Contact"}
		if err := contactRepo.Create(ctx, contact); err != nil {
			t.Fatalf("create contact: %v", err)
		}
		if err := contactRepo.Delete(ctx, contact.ID); err != nil {
			t.Fatalf("delete contact: %v", err)
		}

		if got := listAll(t, otherID); len(got) != 0 {
			t.Errorf("another user saw %d of the owner's deletions, want 0", len(got))
		}
		if got := listAll(t, ownerID); len(got) != 1 {
			t.Errorf("the owner saw %d deletions, want 1", len(got))
		}
	})

	t.Run("the entity filter narrows the feed", func(t *testing.T) {
		userID := newUser(t)

		contactRepo := postgres.NewContactRepository(db)
		contact := &models.Contact{UserID: userID, Name: "Filtered"}
		if err := contactRepo.Create(ctx, contact); err != nil {
			t.Fatalf("create contact: %v", err)
		}
		flightRepo := postgres.NewFlightRepository(db)
		flight := deltaTestFlight(userID, "D-EFLT")
		if err := flightRepo.Create(ctx, flight); err != nil {
			t.Fatalf("create flight: %v", err)
		}
		if err := contactRepo.Delete(ctx, contact.ID); err != nil {
			t.Fatalf("delete contact: %v", err)
		}
		if err := flightRepo.Delete(ctx, flight.ID); err != nil {
			t.Fatalf("delete flight: %v", err)
		}

		onlyFlights := models.DeletionEntityFlight
		got, err := deletionRepo.ListSince(ctx, userID, epoch, &onlyFlights, 100, 0)
		if err != nil {
			t.Fatalf("filtered list: %v", err)
		}
		if len(got) != 1 || got[0].EntityID != flight.ID {
			t.Fatalf("entity filter returned %+v, want only the flight", got)
		}

		count, err := deletionRepo.CountSince(ctx, userID, epoch, &onlyFlights)
		if err != nil {
			t.Fatalf("filtered count: %v", err)
		}
		if count != 1 {
			t.Errorf("filtered count = %d, want 1", count)
		}
	})

	t.Run("paging is stable when a bulk delete shares one timestamp", func(t *testing.T) {
		userID := newUser(t)
		flightRepo := postgres.NewFlightRepository(db)
		for i := 0; i < 5; i++ {
			if err := flightRepo.Create(ctx, deltaTestFlight(userID, "D-EPG"+string(rune('A'+i)))); err != nil {
				t.Fatalf("create flight %d: %v", i, err)
			}
		}
		// One statement: every tombstone gets the same transaction NOW().
		if _, err := db.ExecContext(ctx, `DELETE FROM flights WHERE user_id = $1`, userID); err != nil {
			t.Fatalf("bulk delete: %v", err)
		}

		seen := make(map[uuid.UUID]bool)
		for offset := 0; offset < 5; offset += 2 {
			page, err := deletionRepo.ListSince(ctx, userID, epoch, nil, 2, offset)
			if err != nil {
				t.Fatalf("page at offset %d: %v", offset, err)
			}
			for _, d := range page {
				if seen[d.EntityID] {
					t.Errorf("id %s appeared on two pages — paging is not deterministic", d.EntityID)
				}
				seen[d.EntityID] = true
			}
		}
		if len(seen) != 5 {
			t.Errorf("paging surfaced %d of 5 tombstones", len(seen))
		}
	})

	t.Run("re-deleting a recreated id refreshes rather than duplicates", func(t *testing.T) {
		userID := newUser(t)
		contactRepo := postgres.NewContactRepository(db)
		contact := &models.Contact{UserID: userID, Name: "Recreated"}
		if err := contactRepo.Create(ctx, contact); err != nil {
			t.Fatalf("create contact: %v", err)
		}
		if err := contactRepo.Delete(ctx, contact.ID); err != nil {
			t.Fatalf("delete contact: %v", err)
		}
		// Reinsert the same id, then delete it again.
		if _, err := db.ExecContext(ctx,
			`INSERT INTO contacts (id, user_id, name, created_at, updated_at)
			 VALUES ($1, $2, $3, NOW(), NOW())`, contact.ID, userID, "Recreated"); err != nil {
			t.Fatalf("recreate contact: %v", err)
		}
		if err := contactRepo.Delete(ctx, contact.ID); err != nil {
			t.Fatalf("second delete: %v", err)
		}

		got := listAll(t, userID)
		if len(got) != 1 {
			t.Errorf("recorded %d tombstones for one id, want 1", len(got))
		}
	})

	t.Run("the sweeper drops tombstones past the horizon", func(t *testing.T) {
		userID := newUser(t)
		contactRepo := postgres.NewContactRepository(db)
		contact := &models.Contact{UserID: userID, Name: "Ancient"}
		if err := contactRepo.Create(ctx, contact); err != nil {
			t.Fatalf("create contact: %v", err)
		}
		if err := contactRepo.Delete(ctx, contact.ID); err != nil {
			t.Fatalf("delete contact: %v", err)
		}
		// Age the tombstone past any plausible retention window.
		if _, err := db.ExecContext(ctx,
			`UPDATE deletion_tombstones SET deleted_at = NOW() - INTERVAL '200 days' WHERE user_id = $1`,
			userID); err != nil {
			t.Fatalf("age tombstone: %v", err)
		}

		removed, err := deletionRepo.DeleteExpired(ctx, time.Now().AddDate(0, 0, -90))
		if err != nil {
			t.Fatalf("sweep: %v", err)
		}
		if removed < 1 {
			t.Errorf("sweep removed %d rows, want at least the aged one", removed)
		}

		var remaining int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM deletion_tombstones WHERE user_id = $1`, userID).Scan(&remaining); err != nil {
			t.Fatalf("count tombstones: %v", err)
		}
		if remaining != 0 {
			t.Errorf("%d aged tombstones survived the sweep", remaining)
		}
	})
}
