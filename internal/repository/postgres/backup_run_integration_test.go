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

func TestBackupRunRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	userRepo := postgres.NewUserRepository(db)
	destRepo := postgres.NewBackupDestinationRepository(db)
	runRepo := postgres.NewBackupRunRepository(db)
	ctx := context.Background()

	user := testutil.CreateTestUser("runs-user@example.com", "Runs User", "hash")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	d := &models.BackupDestination{
		UserID:           user.ID,
		Provider:         "s3",
		DisplayName:      "primary",
		Config:           map[string]any{"bucket": "b", "region": "us-east-1"},
		CredentialsEnc:   []byte{1},
		CredentialsNonce: []byte{2},
		Schedule:         models.BackupScheduleDaily,
		ScheduleHourUTC:  3,
		Status:           models.BackupStatusActive,
		Enabled:          true,
		RetentionCount:   30,
	}
	if err := destRepo.Create(ctx, d); err != nil {
		t.Fatalf("create dest: %v", err)
	}

	t.Run("create + get", func(t *testing.T) {
		run := &models.BackupRun{
			DestinationID: d.ID,
			UserID:        user.ID,
			Status:        models.BackupRunStatusFailed,
			Trigger:       models.BackupRunTriggerManual,
			StartedAt:     time.Now().UTC(),
		}
		if err := runRepo.Create(ctx, run); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if run.ID == uuid.Nil {
			t.Errorf("ID not assigned")
		}

		got, err := runRepo.GetByID(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Status != models.BackupRunStatusFailed {
			t.Errorf("status: %s", got.Status)
		}
	})

	t.Run("update transitions", func(t *testing.T) {
		run := &models.BackupRun{
			DestinationID: d.ID, UserID: user.ID,
			Status:    models.BackupRunStatusFailed,
			Trigger:   models.BackupRunTriggerScheduled,
			StartedAt: time.Now().UTC(),
		}
		if err := runRepo.Create(ctx, run); err != nil {
			t.Fatalf("Create: %v", err)
		}
		// Promote to success with metrics.
		completed := time.Now().UTC()
		duration := 1234
		size := int64(4567)
		fc, ac, lc, cc := 1, 2, 3, 4
		run.Status = models.BackupRunStatusSuccess
		run.CompletedAt = &completed
		run.DurationMs = &duration
		run.SizeBytes = &size
		run.SHA256 = "abc"
		run.FlightCount = &fc
		run.AircraftCount = &ac
		run.LicenseCount = &lc
		run.CredentialCount = &cc
		run.RemotePath = "p/x.gz"
		if err := runRepo.Update(ctx, run); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, _ := runRepo.GetByID(ctx, run.ID)
		if got.Status != models.BackupRunStatusSuccess {
			t.Errorf("status not updated: %s", got.Status)
		}
		if got.SHA256 != "abc" {
			t.Errorf("sha256 not updated: %s", got.SHA256)
		}
		if got.RemotePath != "p/x.gz" {
			t.Errorf("remote path: %s", got.RemotePath)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		// Empty out then insert 5 rows.
		_ = runRepo.DeleteByDestinationID(ctx, d.ID)
		for i := 0; i < 5; i++ {
			r := &models.BackupRun{
				DestinationID: d.ID, UserID: user.ID,
				Status:    models.BackupRunStatusSuccess,
				Trigger:   models.BackupRunTriggerScheduled,
				StartedAt: time.Now().UTC().Add(time.Duration(-i) * time.Minute),
			}
			if err := runRepo.Create(ctx, r); err != nil {
				t.Fatalf("Create %d: %v", i, err)
			}
		}
		page1, total, err := runRepo.GetByDestinationID(ctx, d.ID, 2, 0)
		if err != nil {
			t.Fatalf("GetByDestinationID: %v", err)
		}
		if total != 5 {
			t.Errorf("total: %d", total)
		}
		if len(page1) != 2 {
			t.Errorf("page1 size: %d", len(page1))
		}
		page2, _, _ := runRepo.GetByDestinationID(ctx, d.ID, 2, 2)
		if len(page2) != 2 {
			t.Errorf("page2 size: %d", len(page2))
		}
		page3, _, _ := runRepo.GetByDestinationID(ctx, d.ID, 2, 4)
		if len(page3) != 1 {
			t.Errorf("page3 size: %d", len(page3))
		}
	})

	t.Run("delete by destination", func(t *testing.T) {
		if err := runRepo.DeleteByDestinationID(ctx, d.ID); err != nil {
			t.Fatalf("DeleteByDestinationID: %v", err)
		}
		_, total, _ := runRepo.GetByDestinationID(ctx, d.ID, 10, 0)
		if total != 0 {
			t.Errorf("rows remain: %d", total)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := runRepo.GetByID(ctx, uuid.Nil)
		if err != repository.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}
