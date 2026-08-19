package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// mockDeletionRepo is an in-memory stand-in that applies the same predicate as
// the SQL: this user, strictly after the watermark, optionally one entity type,
// oldest first.
type mockDeletionRepo struct {
	tombstones []*models.Deletion
	// lastLimit / lastOffset record what the service asked for.
	lastLimit  int
	lastOffset int
}

func (m *mockDeletionRepo) matching(userID uuid.UUID, since time.Time, entity *models.DeletionEntityType) []*models.Deletion {
	var out []*models.Deletion
	for _, d := range m.tombstones {
		if d.UserID != userID || !d.DeletedAt.After(since) {
			continue
		}
		if entity != nil && d.EntityType != *entity {
			continue
		}
		out = append(out, d)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].DeletedAt.Before(out[j-1].DeletedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (m *mockDeletionRepo) ListSince(_ context.Context, userID uuid.UUID, since time.Time,
	entity *models.DeletionEntityType, limit, offset int,
) ([]*models.Deletion, error) {
	m.lastLimit, m.lastOffset = limit, offset
	all := m.matching(userID, since, entity)
	if offset >= len(all) {
		return nil, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (m *mockDeletionRepo) CountSince(_ context.Context, userID uuid.UUID, since time.Time,
	entity *models.DeletionEntityType,
) (int, error) {
	return len(m.matching(userID, since, entity)), nil
}

func (m *mockDeletionRepo) DeleteExpired(_ context.Context, before time.Time) (int64, error) {
	kept := m.tombstones[:0]
	var removed int64
	for _, d := range m.tombstones {
		if d.DeletedAt.Before(before) {
			removed++
			continue
		}
		kept = append(kept, d)
	}
	m.tombstones = kept
	return removed, nil
}

func tombstone(userID uuid.UUID, entity models.DeletionEntityType, at time.Time) *models.Deletion {
	return &models.Deletion{UserID: userID, EntityType: entity, EntityID: uuid.New(), DeletedAt: at}
}

func TestDeletionServiceListDeletions(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	otherUserID := uuid.New()
	now := time.Now()

	newSvc := func(tombstones ...*models.Deletion) (*service.DeletionService, *mockDeletionRepo) {
		repo := &mockDeletionRepo{tombstones: tombstones}
		return service.NewDeletionService(repo, service.DefaultTombstoneRetention), repo
	}

	t.Run("returns only this user's deletions after the watermark", func(t *testing.T) {
		watermark := now.Add(-time.Hour)
		svc, _ := newSvc(
			tombstone(userID, models.DeletionEntityFlight, now.Add(-2*time.Hour)), // before
			tombstone(userID, models.DeletionEntityFlight, now.Add(-30*time.Minute)),
			tombstone(userID, models.DeletionEntityAircraft, now.Add(-10*time.Minute)),
			tombstone(otherUserID, models.DeletionEntityFlight, now.Add(-5*time.Minute)),
		)

		got, err := svc.ListDeletions(ctx, userID, watermark, nil, 1, 100)
		if err != nil {
			t.Fatalf("ListDeletions: %v", err)
		}
		if got.Total != 2 {
			t.Errorf("total = %d, want 2", got.Total)
		}
		if len(got.Deletions) != 2 {
			t.Fatalf("returned %d deletions, want 2", len(got.Deletions))
		}
		// Oldest first, so a client can advance its watermark as it pages.
		if got.Deletions[0].DeletedAt.After(got.Deletions[1].DeletedAt) {
			t.Error("deletions are not oldest-first")
		}
		for _, d := range got.Deletions {
			if d.UserID != userID {
				t.Errorf("leaked a deletion belonging to %s", d.UserID)
			}
		}
	})

	t.Run("a deletion exactly at the watermark is excluded", func(t *testing.T) {
		at := now.Add(-time.Hour)
		svc, _ := newSvc(tombstone(userID, models.DeletionEntityFlight, at))

		got, err := svc.ListDeletions(ctx, userID, at, nil, 1, 100)
		if err != nil {
			t.Fatalf("ListDeletions: %v", err)
		}
		if got.Total != 0 {
			t.Errorf("total = %d, want 0 — strictly-after matches updatedSince", got.Total)
		}
	})

	t.Run("filters by entity", func(t *testing.T) {
		watermark := now.Add(-time.Hour)
		svc, _ := newSvc(
			tombstone(userID, models.DeletionEntityFlight, now.Add(-30*time.Minute)),
			tombstone(userID, models.DeletionEntityContact, now.Add(-20*time.Minute)),
		)

		contact := models.DeletionEntityContact
		got, err := svc.ListDeletions(ctx, userID, watermark, &contact, 1, 100)
		if err != nil {
			t.Fatalf("ListDeletions: %v", err)
		}
		if got.Total != 1 || got.Deletions[0].EntityType != models.DeletionEntityContact {
			t.Errorf("entity filter returned %+v, want only the contact", got.Deletions)
		}
	})

	t.Run("an unknown entity is an error, not an empty feed", func(t *testing.T) {
		svc, _ := newSvc()
		bogus := models.DeletionEntityType("spaceship")
		_, err := svc.ListDeletions(ctx, userID, now.Add(-time.Hour), &bogus, 1, 100)
		if !errors.Is(err, service.ErrInvalidDeletionEntity) {
			t.Errorf("err = %v, want ErrInvalidDeletionEntity — an empty feed reads as 'nothing was deleted'", err)
		}
	})

	t.Run("pages", func(t *testing.T) {
		watermark := now.Add(-time.Hour)
		var seeds []*models.Deletion
		for i := 0; i < 5; i++ {
			seeds = append(seeds, tombstone(userID, models.DeletionEntityFlight, now.Add(time.Duration(-30+i)*time.Minute)))
		}
		svc, _ := newSvc(seeds...)

		first, err := svc.ListDeletions(ctx, userID, watermark, nil, 1, 2)
		if err != nil {
			t.Fatalf("page 1: %v", err)
		}
		if first.Total != 5 || len(first.Deletions) != 2 {
			t.Fatalf("page 1 = %d of %d, want 2 of 5", len(first.Deletions), first.Total)
		}
		third, err := svc.ListDeletions(ctx, userID, watermark, nil, 3, 2)
		if err != nil {
			t.Fatalf("page 3: %v", err)
		}
		if len(third.Deletions) != 1 {
			t.Errorf("page 3 held %d deletions, want the remaining 1", len(third.Deletions))
		}
	})

	t.Run("clamps the page size to the maximum", func(t *testing.T) {
		svc, repo := newSvc()
		if _, err := svc.ListDeletions(ctx, userID, now.Add(-time.Hour), nil, 1, 10_000); err != nil {
			t.Fatalf("ListDeletions: %v", err)
		}
		if repo.lastLimit != service.MaxDeletionPageSize {
			t.Errorf("limit reaching the query = %d, want the %d cap", repo.lastLimit, service.MaxDeletionPageSize)
		}
	})

	t.Run("never returns a nil slice", func(t *testing.T) {
		svc, _ := newSvc()
		got, err := svc.ListDeletions(ctx, userID, now, nil, 1, 100)
		if err != nil {
			t.Fatalf("ListDeletions: %v", err)
		}
		if got.Deletions == nil {
			t.Error("Deletions is nil; an empty feed must serialise as [] not null")
		}
	})
}

// A watermark older than retention reports WatermarkExpired.
func TestDeletionServiceWatermarkExpiry(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	retention := 90 * 24 * time.Hour
	svc := service.NewDeletionService(&mockDeletionRepo{}, retention)

	fresh, err := svc.ListDeletions(ctx, userID, time.Now().Add(-retention/2), nil, 1, 100)
	if err != nil {
		t.Fatalf("fresh watermark: %v", err)
	}
	if fresh.WatermarkExpired {
		t.Error("a watermark inside the retention window must not be flagged expired")
	}

	stale, err := svc.ListDeletions(ctx, userID, time.Now().Add(-retention-time.Hour), nil, 1, 100)
	if err != nil {
		t.Fatalf("stale watermark: %v", err)
	}
	if !stale.WatermarkExpired {
		t.Error("a watermark older than retention must be flagged expired")
	}

	if svc.Retention() != retention {
		t.Errorf("Retention() = %v, want %v", svc.Retention(), retention)
	}
}

func TestDeletionServiceDefaultsRetention(t *testing.T) {
	svc := service.NewDeletionService(&mockDeletionRepo{}, 0)
	if svc.Retention() != service.DefaultTombstoneRetention {
		t.Errorf("Retention() = %v, want the %v default", svc.Retention(), service.DefaultTombstoneRetention)
	}
}

func TestDeletionEntityTypeValidation(t *testing.T) {
	for _, e := range models.ValidDeletionEntityTypes() {
		if !e.IsValid() {
			t.Errorf("%q should be valid", e)
		}
	}
	for _, e := range []models.DeletionEntityType{"", "flights", "Flight", "user"} {
		if e.IsValid() {
			t.Errorf("%q should not be valid", e)
		}
	}
}
