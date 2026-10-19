//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func TestUserRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	t.Run("Create and retrieve user", func(t *testing.T) {
		user := testutil.CreateTestUser("test@example.com", "Test User", "hashedpassword")

		err := repo.Create(ctx, user)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		retrieved, err := repo.GetByEmail(ctx, user.Email)
		if err != nil {
			t.Fatalf("Failed to retrieve user: %v", err)
		}

		if retrieved.Email != user.Email {
			t.Errorf("Expected email %s, got %s", user.Email, retrieved.Email)
		}
		if retrieved.Name != user.Name {
			t.Errorf("Expected name %s, got %s", user.Name, retrieved.Name)
		}
	})

	t.Run("Get non-existent user", func(t *testing.T) {
		_, err := repo.GetByEmail(ctx, "nonexistent@example.com")
		if err != repository.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Duplicate email", func(t *testing.T) {
		user1 := testutil.CreateTestUser("duplicate@example.com", "User 1", "hash1")
		err := repo.Create(ctx, user1)
		if err != nil {
			t.Fatalf("Failed to create first user: %v", err)
		}

		user2 := testutil.CreateTestUser("duplicate@example.com", "User 2", "hash2")
		err = repo.Create(ctx, user2)
		if err != repository.ErrDuplicateEmail {
			t.Errorf("Expected ErrDuplicateEmail, got %v", err)
		}
	})
}

func TestRefreshTokenRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	userRepo := postgres.NewUserRepository(db)
	tokenRepo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	user := testutil.CreateTestUser("tokenuser@example.com", "Token User", "hash")
	err := userRepo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	t.Run("Create and retrieve token", func(t *testing.T) {
		token := testutil.CreateTestRefreshToken(user.ID, "tokenhash")

		err := tokenRepo.Create(ctx, token)
		if err != nil {
			t.Fatalf("Failed to create token: %v", err)
		}

		retrieved, err := tokenRepo.GetByTokenHash(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("Failed to retrieve token: %v", err)
		}

		if retrieved.TokenHash != token.TokenHash {
			t.Errorf("Expected token hash %s, got %s", token.TokenHash, retrieved.TokenHash)
		}
		if retrieved.Revoked {
			t.Error("Token should not be revoked")
		}
	})

	t.Run("Revoke token", func(t *testing.T) {
		token := testutil.CreateTestRefreshToken(user.ID, "revokehash")
		err := tokenRepo.Create(ctx, token)
		if err != nil {
			t.Fatalf("Failed to create token: %v", err)
		}

		err = tokenRepo.RevokeSession(ctx, user.ID, token.SessionID)
		if err != nil {
			t.Fatalf("Failed to revoke token: %v", err)
		}

		retrieved, err := tokenRepo.GetByTokenHash(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("Failed to retrieve token: %v", err)
		}

		if !retrieved.Revoked {
			t.Error("Token should be revoked")
		}
	})

	t.Run("CreateInLiveSession adds a token to a live session", func(t *testing.T) {
		first := testutil.CreateTestRefreshToken(user.ID, "livefirst")
		if err := tokenRepo.Create(ctx, first); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		next := testutil.CreateTestRefreshToken(user.ID, "livenext")
		next.SessionID = first.SessionID
		if err := tokenRepo.CreateInLiveSession(ctx, next); err != nil {
			t.Fatalf("CreateInLiveSession() error = %v, want nil", err)
		}
		if _, err := tokenRepo.GetByTokenHash(ctx, next.TokenHash); err != nil {
			t.Errorf("GetByTokenHash() for the new token error = %v", err)
		}
	})

	t.Run("CreateInLiveSession refuses a revoked session", func(t *testing.T) {
		first := testutil.CreateTestRefreshToken(user.ID, "deadfirst")
		if err := tokenRepo.Create(ctx, first); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := tokenRepo.RevokeSession(ctx, user.ID, first.SessionID); err != nil {
			t.Fatalf("RevokeSession() error = %v", err)
		}

		next := testutil.CreateTestRefreshToken(user.ID, "deadnext")
		next.SessionID = first.SessionID
		if err := tokenRepo.CreateInLiveSession(ctx, next); err != repository.ErrNotFound {
			t.Fatalf("CreateInLiveSession() error = %v, want ErrNotFound", err)
		}
		if _, err := tokenRepo.GetByTokenHash(ctx, next.TokenHash); err != repository.ErrNotFound {
			t.Errorf("GetByTokenHash() for the refused token error = %v, want ErrNotFound", err)
		}
	})

	t.Run("CreateInLiveSession refuses a session holding only rotated tokens", func(t *testing.T) {
		first := testutil.CreateTestRefreshToken(user.ID, "rotatedfirst")
		if err := tokenRepo.Create(ctx, first); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := tokenRepo.MarkRotated(ctx, first.TokenHash); err != nil {
			t.Fatalf("MarkRotated() error = %v", err)
		}

		next := testutil.CreateTestRefreshToken(user.ID, "rotatednext")
		next.SessionID = first.SessionID
		if err := tokenRepo.CreateInLiveSession(ctx, next); err != repository.ErrNotFound {
			t.Errorf("CreateInLiveSession() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("RevokeSession waits for an in-flight rotation and revokes its token", func(t *testing.T) {
		first := testutil.CreateTestRefreshToken(user.ID, "racefirst")
		if err := tokenRepo.Create(ctx, first); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx() error = %v", err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(ctx, `SELECT 1 FROM users WHERE id = $1 FOR SHARE`, user.ID); err != nil {
			t.Fatalf("lock user error = %v", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, session_id)
			VALUES ($1, $2, 'racenext', NOW() + INTERVAL '1 day', $3)`,
			uuid.New(), user.ID, first.SessionID); err != nil {
			t.Fatalf("insert in rotation tx error = %v", err)
		}

		done := make(chan error, 1)
		go func() { done <- tokenRepo.RevokeSession(ctx, user.ID, first.SessionID) }()

		select {
		case err := <-done:
			t.Fatalf("RevokeSession() returned %v before the rotation committed", err)
		case <-time.After(200 * time.Millisecond):
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if err := <-done; err != nil {
			t.Fatalf("RevokeSession() error = %v", err)
		}

		next, err := tokenRepo.GetByTokenHash(ctx, "racenext")
		if err != nil {
			t.Fatalf("GetByTokenHash() error = %v", err)
		}
		if !next.Revoked {
			t.Error("RevokeSession() left the token written by the concurrent rotation live")
		}
	})
}

func TestPasswordResetTokenRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	userRepo := postgres.NewUserRepository(db)
	resetRepo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	user := testutil.CreateTestUser("reset@example.com", "Reset User", "hash")
	err := userRepo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	t.Run("Create and retrieve reset token", func(t *testing.T) {
		token := testutil.CreateTestPasswordResetToken(user.ID, "resethash")

		err := resetRepo.Create(ctx, token)
		if err != nil {
			t.Fatalf("Failed to create reset token: %v", err)
		}

		retrieved, err := resetRepo.GetByTokenHash(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("Failed to retrieve reset token: %v", err)
		}

		if retrieved.TokenHash != token.TokenHash {
			t.Errorf("Expected token hash %s, got %s", token.TokenHash, retrieved.TokenHash)
		}
		if retrieved.Used {
			t.Error("Token should not be marked as used")
		}
	})

	t.Run("Mark token as used", func(t *testing.T) {
		token := testutil.CreateTestPasswordResetToken(user.ID, "usedhash")
		err := resetRepo.Create(ctx, token)
		if err != nil {
			t.Fatalf("Failed to create reset token: %v", err)
		}

		err = resetRepo.MarkAsUsed(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("Failed to mark token as used: %v", err)
		}

		retrieved, err := resetRepo.GetByTokenHash(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("Failed to retrieve token: %v", err)
		}

		if !retrieved.Used {
			t.Error("Token should be marked as used")
		}
	})
}
