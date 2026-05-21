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

func TestBackupDestinationRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewBackupDestinationRepository(db)
	ctx := context.Background()

	user := testutil.CreateTestUser("backup-user@example.com", "Backup User", "hash")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Run("create and retrieve", func(t *testing.T) {
		d := &models.BackupDestination{
			UserID:          user.ID,
			Provider:        "s3",
			DisplayName:     "primary",
			Config:          map[string]any{"bucket": "test-bucket", "region": "us-east-1"},
			CredentialHint:  "•••1234",
			CredentialsEnc:  []byte{0x01, 0x02},
			CredentialsNonce: []byte{0x03, 0x04},
			Schedule:        models.BackupScheduleDaily,
			ScheduleHourUTC: 3,
			RetentionCount:  30,
			Status:          models.BackupStatusActive,
			Enabled:         true,
		}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if d.ID == uuid.Nil {
			t.Fatalf("ID not assigned")
		}

		got, err := repo.GetByID(ctx, d.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Provider != "s3" || got.Config["bucket"] != "test-bucket" {
			t.Errorf("unexpected: %+v", got)
		}
		if got.Schedule != models.BackupScheduleDaily || got.ScheduleHourUTC != 3 {
			t.Errorf("schedule wrong: %+v", got)
		}
		if string(got.CredentialsEnc) != string(d.CredentialsEnc) {
			t.Errorf("credentials_enc mismatch")
		}
	})

	t.Run("update", func(t *testing.T) {
		d := &models.BackupDestination{
			UserID: user.ID, Provider: "s3", DisplayName: "updatable",
			Config: map[string]any{"bucket": "b", "region": "us-east-1"},
			CredentialsEnc: []byte{1}, CredentialsNonce: []byte{2},
			Schedule: models.BackupScheduleManual, ScheduleHourUTC: 0,
			Status: models.BackupStatusActive, Enabled: true, RetentionCount: 5,
		}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("Create: %v", err)
		}
		d.DisplayName = "renamed"
		d.RetentionCount = 99
		if err := repo.Update(ctx, d); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, _ := repo.GetByID(ctx, d.ID)
		if got.DisplayName != "renamed" || got.RetentionCount != 99 {
			t.Errorf("update did not take: %+v", got)
		}
	})

	t.Run("delete", func(t *testing.T) {
		d := &models.BackupDestination{
			UserID: user.ID, Provider: "s3", DisplayName: "todelete",
			Config: map[string]any{"bucket": "b", "region": "us-east-1"},
			CredentialsEnc: []byte{1}, CredentialsNonce: []byte{2},
			Schedule: models.BackupScheduleManual, ScheduleHourUTC: 0,
			Status: models.BackupStatusActive, Enabled: true,
		}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.Delete(ctx, d.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := repo.GetByID(ctx, d.ID); err != repository.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("list by user", func(t *testing.T) {
		list, err := repo.GetByUserID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByUserID: %v", err)
		}
		if len(list) == 0 {
			t.Errorf("expected destinations for user")
		}
	})

	t.Run("list due daily", func(t *testing.T) {
		// Insert a daily destination at the current hour, with no last_success_at.
		now := time.Now().UTC()
		d := &models.BackupDestination{
			UserID: user.ID, Provider: "s3", DisplayName: "due-daily",
			Config: map[string]any{"bucket": "b", "region": "us-east-1"},
			CredentialsEnc: []byte{1}, CredentialsNonce: []byte{2},
			Schedule: models.BackupScheduleDaily, ScheduleHourUTC: now.Hour(),
			Status: models.BackupStatusActive, Enabled: true,
		}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("Create: %v", err)
		}
		due, err := repo.ListDueForRun(ctx, now)
		if err != nil {
			t.Fatalf("ListDueForRun: %v", err)
		}
		found := false
		for _, x := range due {
			if x.ID == d.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected daily destination to be due, got %d due", len(due))
		}
	})

	t.Run("list due skips paused", func(t *testing.T) {
		now := time.Now().UTC()
		d := &models.BackupDestination{
			UserID: user.ID, Provider: "s3", DisplayName: "paused",
			Config: map[string]any{"bucket": "b", "region": "us-east-1"},
			CredentialsEnc: []byte{1}, CredentialsNonce: []byte{2},
			Schedule: models.BackupScheduleDaily, ScheduleHourUTC: now.Hour(),
			Status: models.BackupStatusPaused, Enabled: true,
		}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("Create: %v", err)
		}
		due, _ := repo.ListDueForRun(ctx, now)
		for _, x := range due {
			if x.ID == d.ID {
				t.Errorf("paused destination should not be due")
			}
		}
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.Nil)
		if err != repository.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}
