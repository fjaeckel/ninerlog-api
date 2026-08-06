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

// The claim semantics live in a single ON CONFLICT statement, which is
// precisely the part sqlmock cannot verify — these run against real Postgres.
func TestIdempotencyRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewIdempotencyRepository(db)
	ctx := context.Background()

	newUser := func(t *testing.T) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("idem-"+uuid.NewString()+"@example.com", "Idem User", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}

	record := func(userID uuid.UUID, key string, at time.Time, ttl time.Duration) *models.IdempotencyRecord {
		return &models.IdempotencyRecord{
			UserID:      userID,
			Key:         key,
			RequestHash: []byte("fingerprint"),
			CreatedAt:   at.Truncate(time.Microsecond),
			ExpiresAt:   at.Add(ttl).Truncate(time.Microsecond),
		}
	}

	t.Run("second claim of a live key is refused", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()

		first := record(userID, "key-live", now, time.Hour)
		existing, err := repo.Claim(ctx, first, now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing != nil {
			t.Fatalf("first claim should have succeeded, got existing %+v", existing)
		}

		second := record(userID, "key-live", now.Add(time.Second), time.Hour)
		existing, err = repo.Claim(ctx, second, now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing == nil {
			t.Fatal("second claim of an in-progress key should have been refused")
		}
		if existing.State != models.IdempotencyStateInProgress {
			t.Errorf("state: want in_progress, got %q", existing.State)
		}
	})

	t.Run("completed response round-trips", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()
		rec := record(userID, "key-complete", now, time.Hour)
		if _, err := repo.Claim(ctx, rec, now.Add(-time.Minute)); err != nil {
			t.Fatalf("Claim: %v", err)
		}

		status := 201
		rec.ResponseStatus = &status
		rec.ResponseBody = []byte(`{"id":"flight-1"}`)
		rec.ResponseContentType = "application/json; charset=utf-8"
		if err := repo.Complete(ctx, rec); err != nil {
			t.Fatalf("Complete: %v", err)
		}

		retry := record(userID, "key-complete", now.Add(time.Second), time.Hour)
		existing, err := repo.Claim(ctx, retry, now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing == nil {
			t.Fatal("retry after completion should have been refused")
		}
		if !existing.Replayable() {
			t.Fatalf("record should be replayable, got %+v", existing)
		}
		if *existing.ResponseStatus != 201 {
			t.Errorf("status: want 201, got %d", *existing.ResponseStatus)
		}
		if string(existing.ResponseBody) != `{"id":"flight-1"}` {
			t.Errorf("body: got %q", existing.ResponseBody)
		}
		if existing.ResponseContentType != "application/json; charset=utf-8" {
			t.Errorf("content type: got %q", existing.ResponseContentType)
		}
		if string(existing.RequestHash) != "fingerprint" {
			t.Errorf("Complete must not disturb the request fingerprint, got %q", existing.RequestHash)
		}
	})

	t.Run("completed with no response is not replayable", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()
		rec := record(userID, "key-nobody", now, time.Hour)
		if _, err := repo.Claim(ctx, rec, now.Add(-time.Minute)); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if err := repo.Complete(ctx, rec); err != nil {
			t.Fatalf("Complete: %v", err)
		}

		existing, err := repo.Claim(ctx, record(userID, "key-nobody", now.Add(time.Second), time.Hour), now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing == nil || existing.Replayable() {
			t.Fatalf("want a non-replayable record, got %+v", existing)
		}
	})

	t.Run("abandoned claim is taken over after the lease", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()
		stale := record(userID, "key-stale", now.Add(-10*time.Minute), time.Hour)
		if _, err := repo.Claim(ctx, stale, now.Add(-11*time.Minute)); err != nil {
			t.Fatalf("Claim: %v", err)
		}

		// staleBefore is now-1m, so the 10-minute-old claim is fair game.
		existing, err := repo.Claim(ctx, record(userID, "key-stale", now, time.Hour), now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing != nil {
			t.Fatalf("stale claim should have been taken over, got %+v", existing)
		}
	})

	t.Run("expired record is taken over", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()
		old := record(userID, "key-expired", now.Add(-2*time.Hour), time.Hour) // expired an hour ago
		if _, err := repo.Claim(ctx, old, now.Add(-2*time.Hour-time.Minute)); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		status := 201
		old.ResponseStatus = &status
		old.ResponseBody = []byte("{}")
		if err := repo.Complete(ctx, old); err != nil {
			t.Fatalf("Complete: %v", err)
		}

		existing, err := repo.Claim(ctx, record(userID, "key-expired", now, time.Hour), now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing != nil {
			t.Fatalf("expired record should have been taken over, got %+v", existing)
		}
	})

	t.Run("keys are scoped per user", func(t *testing.T) {
		userA, userB := newUser(t), newUser(t)
		now := time.Now().UTC()

		if _, err := repo.Claim(ctx, record(userA, "shared", now, time.Hour), now.Add(-time.Minute)); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		existing, err := repo.Claim(ctx, record(userB, "shared", now, time.Hour), now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing != nil {
			t.Fatal("one user's key must not block another's")
		}
	})

	t.Run("Complete and Release ignore a claim they no longer own", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()
		rec := record(userID, "key-fenced", now, time.Hour)
		if _, err := repo.Claim(ctx, rec, now.Add(-time.Minute)); err != nil {
			t.Fatalf("Claim: %v", err)
		}

		straggler := record(userID, "key-fenced", now.Add(-time.Hour), time.Hour)
		status := 500
		straggler.ResponseStatus = &status
		if err := repo.Complete(ctx, straggler); err != nil {
			t.Fatalf("Complete: %v", err)
		}
		if err := repo.Release(ctx, userID, "key-fenced", straggler.CreatedAt); err != nil {
			t.Fatalf("Release: %v", err)
		}

		existing, err := repo.Claim(ctx, record(userID, "key-fenced", now.Add(time.Second), time.Hour), now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing == nil {
			t.Fatal("the live claim was clobbered by a straggler")
		}
		if existing.State != models.IdempotencyStateInProgress {
			t.Errorf("state: want in_progress, got %q", existing.State)
		}
	})

	t.Run("Release frees the key", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()
		rec := record(userID, "key-release", now, time.Hour)
		if _, err := repo.Claim(ctx, rec, now.Add(-time.Minute)); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if err := repo.Release(ctx, userID, "key-release", rec.CreatedAt); err != nil {
			t.Fatalf("Release: %v", err)
		}

		existing, err := repo.Claim(ctx, record(userID, "key-release", now.Add(time.Second), time.Hour), now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing != nil {
			t.Fatalf("key should be claimable after Release, got %+v", existing)
		}
	})

	t.Run("DeleteExpired sweeps only expired rows", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()

		if _, err := repo.Claim(ctx, record(userID, "sweep-old", now.Add(-3*time.Hour), time.Hour), now.Add(-4*time.Hour)); err != nil {
			t.Fatalf("Claim: %v", err)
		}
		live := record(userID, "sweep-live", now, time.Hour)
		if _, err := repo.Claim(ctx, live, now.Add(-time.Minute)); err != nil {
			t.Fatalf("Claim: %v", err)
		}

		n, err := repo.DeleteExpired(ctx, now)
		if err != nil {
			t.Fatalf("DeleteExpired: %v", err)
		}
		if n < 1 {
			t.Errorf("expected at least the expired row to be swept, got %d", n)
		}

		existing, err := repo.Claim(ctx, record(userID, "sweep-live", now.Add(time.Second), time.Hour), now.Add(-time.Minute))
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if existing == nil {
			t.Error("the live record was swept")
		}
	})

	t.Run("concurrent claims elect exactly one winner", func(t *testing.T) {
		userID := newUser(t)
		now := time.Now().UTC()
		const racers = 8

		results := make(chan bool, racers)
		start := make(chan struct{})
		for i := 0; i < racers; i++ {
			go func(i int) {
				<-start
				rec := record(userID, "key-race", now.Add(time.Duration(i)*time.Microsecond), time.Hour)
				existing, err := repo.Claim(ctx, rec, now.Add(-time.Minute))
				results <- err == nil && existing == nil
			}(i)
		}
		close(start)

		won := 0
		for i := 0; i < racers; i++ {
			if <-results {
				won++
			}
		}
		if won != 1 {
			t.Errorf("exactly one racer must claim the key, got %d", won)
		}
	})
}
