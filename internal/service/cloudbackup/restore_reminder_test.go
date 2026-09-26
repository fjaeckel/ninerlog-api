package cloudbackup

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestore_AircraftReminders(t *testing.T) {
	due := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	reminder := func(aircraftID uuid.UUID, reg string, kind models.AircraftReminderKind) *models.AircraftReminder {
		return &models.AircraftReminder{ID: uuid.New(), AircraftID: aircraftID, AircraftRegistration: reg, Kind: kind, DueDate: due}
	}

	t.Run("attached by registration; orphans and repeats skipped", func(t *testing.T) {
		store := restorefake.New()
		p := samplePayload()
		backupAircraftID := uuid.New()
		p.Aircraft[0].ID = backupAircraftID
		p.AircraftReminders = []*models.AircraftReminder{
			reminder(backupAircraftID, "", models.ReminderKindAnnualInspection),
			reminder(backupAircraftID, "", models.ReminderKindAnnualInspection),
			reminder(uuid.New(), "D-MISS", models.ReminderKindInsurance),
		}

		res, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, "")
		require.NoError(t, err)
		assert.Equal(t, 1, res.AircraftRemindersImported)
		assert.Equal(t, 2, res.AircraftRemindersSkipped)
		require.Len(t, store.State.Reminders, 1)
		assert.Equal(t, store.State.Aircraft[0].ID, store.State.Reminders[0].AircraftID)
	})

	t.Run("an invalid reminder fails the restore", func(t *testing.T) {
		store := restorefake.New()
		p := samplePayload()
		p.AircraftReminders = []*models.AircraftReminder{reminder(uuid.New(), "D-EABC", models.ReminderKindCustom)}

		_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, "")
		require.Error(t, err)
		assert.Zero(t, store.Commits)
	})
}
