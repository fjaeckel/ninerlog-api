//go:build integration

package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func hashHandle(handle string) []byte {
	sum := sha256.Sum256([]byte(handle))
	return sum[:]
}

// newSession builds a session row for the given handle, valid for one hour
// unless expiresAt is overridden by the caller.
func newSession(handle string, userID *uuid.UUID, ceremony string) *models.WebAuthnSession {
	return &models.WebAuthnSession{
		IDHash:    hashHandle(handle),
		UserID:    userID,
		Ceremony:  ceremony,
		Data:      []byte(`{"challenge":"abc"}`),
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	}
}

func TestWebAuthnSessionRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewWebAuthnSessionRepository(db)
	ctx := context.Background()

	user := testutil.CreateTestUser("webauthn-session@example.com", "Session User", "hash")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Run("Consume returns the row and deletes it", func(t *testing.T) {
		s := newSession("handle-roundtrip", &user.ID, models.WebAuthnCeremonyRegistration)
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}

		got, err := repo.Consume(ctx, hashHandle("handle-roundtrip"), models.WebAuthnCeremonyRegistration)
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if got.UserID == nil || *got.UserID != user.ID {
			t.Errorf("expected user id %s, got %v", user.ID, got.UserID)
		}
		if got.Ceremony != models.WebAuthnCeremonyRegistration {
			t.Errorf("expected registration ceremony, got %q", got.Ceremony)
		}
		// Compare the JSON semantically.
		var decoded map[string]string
		if err := json.Unmarshal(got.Data, &decoded); err != nil {
			t.Fatalf("decode data: %v", err)
		}
		if decoded["challenge"] != "abc" {
			t.Errorf("unexpected data payload: %s", got.Data)
		}

		// The row must be gone, not merely marked used.
		var count int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM webauthn_sessions WHERE id_hash = $1`,
			hashHandle("handle-roundtrip")).Scan(&count); err != nil {
			t.Fatalf("count rows: %v", err)
		}
		if count != 0 {
			t.Errorf("expected row to be deleted, found %d", count)
		}
	})

	t.Run("Second consume of the same handle returns ErrNotFound", func(t *testing.T) {
		s := newSession("handle-single-use", &user.ID, models.WebAuthnCeremonyLogin)
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}
		if _, err := repo.Consume(ctx, hashHandle("handle-single-use"), models.WebAuthnCeremonyLogin); err != nil {
			t.Fatalf("first consume: %v", err)
		}
		_, err := repo.Consume(ctx, hashHandle("handle-single-use"), models.WebAuthnCeremonyLogin)
		if err != repository.ErrNotFound {
			t.Errorf("expected ErrNotFound on replay, got %v", err)
		}
	})

	t.Run("Concurrent consume: exactly one succeeds", func(t *testing.T) {
		const goroutines = 8
		for attempt := 0; attempt < 20; attempt++ {
			handle := "handle-race-" + uuid.NewString()
			s := newSession(handle, &user.ID, models.WebAuthnCeremonyLogin)
			if err := repo.Create(ctx, s); err != nil {
				t.Fatalf("create session: %v", err)
			}

			var (
				wg       sync.WaitGroup
				mu       sync.Mutex
				wins     int
				notFound int
			)
			start := make(chan struct{})
			for i := 0; i < goroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start // maximise contention
					_, err := repo.Consume(ctx, hashHandle(handle), models.WebAuthnCeremonyLogin)
					mu.Lock()
					defer mu.Unlock()
					switch {
					case err == nil:
						wins++
					case err == repository.ErrNotFound:
						notFound++
					default:
						t.Errorf("unexpected consume error: %v", err)
					}
				}()
			}
			close(start)
			wg.Wait()

			if wins != 1 {
				t.Fatalf("attempt %d: expected exactly 1 successful consume, got %d (notFound=%d)",
					attempt, wins, notFound)
			}
		}
	})

	t.Run("Expired row is not returned even though it is still present", func(t *testing.T) {
		s := newSession("handle-expired", &user.ID, models.WebAuthnCeremonyLogin)
		s.ExpiresAt = time.Now().Add(-time.Minute).UTC()
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}

		_, err := repo.Consume(ctx, hashHandle("handle-expired"), models.WebAuthnCeremonyLogin)
		if err != repository.ErrNotFound {
			t.Errorf("expected ErrNotFound for expired session, got %v", err)
		}

		// The expired row is still present.
		var count int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM webauthn_sessions WHERE id_hash = $1`,
			hashHandle("handle-expired")).Scan(&count); err != nil {
			t.Fatalf("count rows: %v", err)
		}
		if count != 1 {
			t.Errorf("expected expired row to still be present, found %d", count)
		}
	})

	t.Run("Wrong ceremony returns ErrNotFound and leaves the row intact", func(t *testing.T) {
		s := newSession("handle-ceremony", &user.ID, models.WebAuthnCeremonyRegistration)
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}

		_, err := repo.Consume(ctx, hashHandle("handle-ceremony"), models.WebAuthnCeremonyLogin)
		if err != repository.ErrNotFound {
			t.Errorf("expected ErrNotFound for ceremony mismatch, got %v", err)
		}

		// A mismatched attempt must not burn the legitimate session.
		if _, err := repo.Consume(ctx, hashHandle("handle-ceremony"), models.WebAuthnCeremonyRegistration); err != nil {
			t.Errorf("registration consume after mismatched attempt: %v", err)
		}
	})

	t.Run("DeleteExpired removes only expired rows and reports the count", func(t *testing.T) {
		expired := newSession("handle-sweep-old", &user.ID, models.WebAuthnCeremonyLogin)
		expired.ExpiresAt = time.Now().Add(-time.Hour).UTC()
		live := newSession("handle-sweep-live", &user.ID, models.WebAuthnCeremonyLogin)
		for _, s := range []*models.WebAuthnSession{expired, live} {
			if err := repo.Create(ctx, s); err != nil {
				t.Fatalf("create session: %v", err)
			}
		}

		n, err := repo.DeleteExpired(ctx)
		if err != nil {
			t.Fatalf("delete expired: %v", err)
		}
		if n < 1 {
			t.Errorf("expected at least one expired row deleted, got %d", n)
		}
		if _, err := repo.Consume(ctx, hashHandle("handle-sweep-live"), models.WebAuthnCeremonyLogin); err != nil {
			t.Errorf("live session should have survived the sweep: %v", err)
		}
	})

	t.Run("DeleteOldestForUser keeps the newest and is scoped to one user", func(t *testing.T) {
		other := testutil.CreateTestUser("webauthn-other@example.com", "Other User", "hash")
		if err := userRepo.Create(ctx, other); err != nil {
			t.Fatalf("create other user: %v", err)
		}

		// Distinct created_at values.
		base := time.Now().Add(-time.Hour)
		handles := []string{"cap-1", "cap-2", "cap-3", "cap-4", "cap-5"}
		for i, h := range handles {
			s := newSession(h, &user.ID, models.WebAuthnCeremonyLogin)
			s.CreatedAt = base.Add(time.Duration(i) * time.Minute).UTC()
			if err := repo.Create(ctx, s); err != nil {
				t.Fatalf("create session %s: %v", h, err)
			}
		}
		otherSession := newSession("cap-other", &other.ID, models.WebAuthnCeremonyLogin)
		if err := repo.Create(ctx, otherSession); err != nil {
			t.Fatalf("create other session: %v", err)
		}

		deleted, err := repo.DeleteOldestForUser(ctx, user.ID, 2)
		if err != nil {
			t.Fatalf("evict: %v", err)
		}
		if deleted != 3 {
			t.Errorf("expected 3 evicted, got %d", deleted)
		}

		// The two newest survive; the three oldest are gone.
		for _, h := range []string{"cap-4", "cap-5"} {
			if _, err := repo.Consume(ctx, hashHandle(h), models.WebAuthnCeremonyLogin); err != nil {
				t.Errorf("expected %s to survive eviction, got %v", h, err)
			}
		}
		for _, h := range []string{"cap-1", "cap-2", "cap-3"} {
			if _, err := repo.Consume(ctx, hashHandle(h), models.WebAuthnCeremonyLogin); err != repository.ErrNotFound {
				t.Errorf("expected %s to be evicted, got %v", h, err)
			}
		}

		// A second user's ceremonies must be untouched.
		if _, err := repo.Consume(ctx, hashHandle("cap-other"), models.WebAuthnCeremonyLogin); err != nil {
			t.Errorf("other user's session was evicted: %v", err)
		}
	})

	t.Run("Deleting the user cascades to their sessions", func(t *testing.T) {
		doomed := testutil.CreateTestUser("webauthn-cascade@example.com", "Cascade User", "hash")
		if err := userRepo.Create(ctx, doomed); err != nil {
			t.Fatalf("create user: %v", err)
		}
		s := newSession("handle-cascade", &doomed.ID, models.WebAuthnCeremonyRegistration)
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}

		if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, doomed.ID); err != nil {
			t.Fatalf("delete user: %v", err)
		}

		_, err := repo.Consume(ctx, hashHandle("handle-cascade"), models.WebAuthnCeremonyRegistration)
		if err != repository.ErrNotFound {
			t.Errorf("expected session to cascade away with the user, got %v", err)
		}
	})

	t.Run("Discoverable login sessions have no user and survive eviction", func(t *testing.T) {
		s := newSession("handle-discoverable", nil, models.WebAuthnCeremonyLogin)
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create session: %v", err)
		}

		// The per-user cap must not touch NULL-user rows.
		if _, err := repo.DeleteOldestForUser(ctx, user.ID, 0); err != nil {
			t.Fatalf("evict: %v", err)
		}

		got, err := repo.Consume(ctx, hashHandle("handle-discoverable"), models.WebAuthnCeremonyLogin)
		if err != nil {
			t.Fatalf("consume discoverable session: %v", err)
		}
		if got.UserID != nil {
			t.Errorf("expected nil user id for discoverable login, got %v", *got.UserID)
		}
	})
}
