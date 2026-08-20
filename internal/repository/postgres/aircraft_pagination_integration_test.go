//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

// TestAircraftGetPageByUserIDIntegration covers the SQL-bounded aircraft page
// query — LIMIT/OFFSET, total count, ordering, updatedSince and cross-user
// isolation — against real Postgres.
func TestAircraftGetPageByUserIDIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	aircraftRepo := postgres.NewAircraftRepository(db)

	newUser := func(t *testing.T) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("page-"+uuid.NewString()+"@example.com", "Page User", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}

	userID := newUser(t)
	otherID := newUser(t)

	const fleetSize = 250
	for i := 0; i < fleetSize; i++ {
		a := &models.Aircraft{
			UserID:       userID,
			Registration: fmt.Sprintf("D-E%03d", i),
			Type:         "C172", Make: "Cessna", Model: "172", IsActive: true,
		}
		if err := aircraftRepo.Create(ctx, a); err != nil {
			t.Fatalf("create aircraft %d: %v", i, err)
		}
	}
	other := &models.Aircraft{
		UserID: otherID, Registration: "D-ZZZZ", Type: "C152", Make: "Cessna", Model: "152", IsActive: true,
	}
	if err := aircraftRepo.Create(ctx, other); err != nil {
		t.Fatalf("create other user's aircraft: %v", err)
	}

	t.Run("total ignores the limit and other users", func(t *testing.T) {
		page, total, err := aircraftRepo.GetPageByUserID(ctx, userID, nil, 10, 0)
		if err != nil {
			t.Fatalf("GetPageByUserID: %v", err)
		}
		if len(page) != 10 {
			t.Errorf("page length = %d, want 10", len(page))
		}
		if total != fleetSize {
			t.Errorf("total = %d, want %d", total, fleetSize)
		}
	})

	t.Run("offset walks the fleet in registration order without gaps", func(t *testing.T) {
		seen := make([]string, 0, fleetSize)
		for offset := 0; offset < fleetSize; offset += 100 {
			page, _, err := aircraftRepo.GetPageByUserID(ctx, userID, nil, 100, offset)
			if err != nil {
				t.Fatalf("offset %d: %v", offset, err)
			}
			for _, a := range page {
				seen = append(seen, a.Registration)
			}
		}
		if len(seen) != fleetSize {
			t.Fatalf("walked %d aircraft, want %d", len(seen), fleetSize)
		}
		for i := 1; i < len(seen); i++ {
			if seen[i-1] >= seen[i] {
				t.Fatalf("registrations out of order at %d: %s then %s", i, seen[i-1], seen[i])
			}
		}
		if seen[0] != "D-E000" || seen[fleetSize-1] != fmt.Sprintf("D-E%03d", fleetSize-1) {
			t.Errorf("walk spans %s..%s, want D-E000..D-E%03d", seen[0], seen[fleetSize-1], fleetSize-1)
		}
	})

	t.Run("offset past the end returns no rows and the real total", func(t *testing.T) {
		page, total, err := aircraftRepo.GetPageByUserID(ctx, userID, nil, 100, fleetSize+500)
		if err != nil {
			t.Fatalf("GetPageByUserID: %v", err)
		}
		if len(page) != 0 {
			t.Errorf("page length = %d, want 0", len(page))
		}
		if total != fleetSize {
			t.Errorf("total = %d, want %d", total, fleetSize)
		}
	})

	t.Run("another user's fleet is not visible", func(t *testing.T) {
		page, total, err := aircraftRepo.GetPageByUserID(ctx, otherID, nil, 100, 0)
		if err != nil {
			t.Fatalf("GetPageByUserID: %v", err)
		}
		if total != 1 || len(page) != 1 || page[0].Registration != "D-ZZZZ" {
			t.Errorf("got %d of %d for the other user, want 1 of 1 (D-ZZZZ)", len(page), total)
		}
	})

	t.Run("updatedSince narrows both the page and the total", func(t *testing.T) {
		watermark := time.Now()
		time.Sleep(5 * time.Millisecond)

		touched := &models.Aircraft{
			UserID:       userID,
			Registration: "D-NEW1",
			Type:         "PA28", Make: "Piper", Model: "Cherokee", IsActive: true,
		}
		if err := aircraftRepo.Create(ctx, touched); err != nil {
			t.Fatalf("create post-watermark aircraft: %v", err)
		}

		page, total, err := aircraftRepo.GetPageByUserID(ctx, userID, &watermark, 100, 0)
		if err != nil {
			t.Fatalf("GetPageByUserID: %v", err)
		}
		if total != 1 {
			t.Fatalf("total = %d, want 1 aircraft changed after the watermark", total)
		}
		if len(page) != 1 || page[0].Registration != "D-NEW1" {
			t.Errorf("page = %v, want just D-NEW1", page)
		}
	})
}
