package postgres_test

import (
	"context"
	"testing"

	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/fjaeckel/pilotlog-api/internal/repository/postgres"
	"github.com/fjaeckel/pilotlog-api/internal/testutil"
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

		err = tokenRepo.RevokeByTokenHash(ctx, token.TokenHash)
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
