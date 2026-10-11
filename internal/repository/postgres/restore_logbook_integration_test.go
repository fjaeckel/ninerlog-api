//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func TestRestoreLogbookIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	logbookRepo := postgres.NewLogbookRepository(db)
	store := postgres.NewRestoreStore(db, time.Minute)

	newUser := func(t *testing.T, strict bool) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("restore-lb-"+uuid.NewString()+"@example.com", "Restore Logbook", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		if strict {
			if _, err := logbookRepo.SetStrict(ctx, u.ID, true, models.LogbookTriggerManual, nil); err != nil {
				t.Fatalf("switch strict mode on: %v", err)
			}
		}
		return u.ID
	}

	newFlight := func(t *testing.T, userID uuid.UUID) *models.Flight {
		t.Helper()
		f := fullFlight(userID, 1)
		f.ID = uuid.Nil
		if err := flightRepo.Create(ctx, f); err != nil {
			t.Fatalf("create flight: %v", err)
		}
		return f
	}

	snapshot := func(t *testing.T, flightID uuid.UUID) json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"id": flightID, "date": "2026-01-02", "aircraft_reg": "D-EOLD", "total_time": 45})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	replaceContext := func() context.Context {
		reason := "Replaced by backup restore"
		return repository.WithChangeContext(ctx, repository.ChangeContext{Source: repository.ChangeSourceImport, Reason: &reason})
	}

	t.Run("a replace wipe leaves a void record per flight and keeps existing history", func(t *testing.T) {
		userID := newUser(t, true)
		edited := newFlight(t, userID)
		edited.Remarks = ptrTo("edited")
		if err := flightRepo.Update(repository.WithChangeSource(ctx, repository.ChangeSourceUserEdit), edited); err != nil {
			t.Fatalf("update: %v", err)
		}
		untouched := newFlight(t, userID)

		created := time.Date(2025, 11, 3, 9, 30, 0, 0, time.UTC)
		restored := fullFlight(userID, 2)
		restored.CreatedAt = created
		restoredRevision := models.FlightRevision{
			UserID: userID, FlightID: restored.ID, RevisionNo: 1, Kind: models.RevisionKindUpdate,
			Snapshot: snapshot(t, restored.ID), CrewSnapshot: json.RawMessage(`[]`), Source: "user_edit",
			ChangedAt: created.Add(time.Hour),
		}

		err := store.InRestoreTx(replaceContext(), userID, func(tx repository.RestoreTx) error {
			if err := tx.WipeContent(ctx, userID); err != nil {
				return err
			}
			if err := tx.InsertFlights(ctx, []*models.Flight{restored}); err != nil {
				return err
			}
			if err := tx.InsertCrewMembers(ctx, []models.FlightCrewMember{
				{ID: uuid.New(), FlightID: restored.ID, Name: "Pat", Role: models.CrewRoleInstructor},
			}); err != nil {
				return err
			}
			if n, err := tx.ImportRevisions(ctx, []models.FlightRevision{restoredRevision}); err != nil || n != 1 {
				t.Errorf("ImportRevisions = %d, %v", n, err)
			}
			switched, err := tx.SetStrict(ctx, userID, true, models.LogbookTriggerImport, nil)
			if err != nil {
				return err
			}
			if switched {
				t.Error("an already strict account was switched again")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("restore: %v", err)
		}

		for _, f := range []*models.Flight{edited, untouched} {
			revs, err := logbookRepo.ListRevisions(ctx, userID, f.ID)
			if err != nil {
				t.Fatal(err)
			}
			last := revs[len(revs)-1]
			if last.Kind != models.RevisionKindDelete || last.Source != repository.ChangeSourceImport ||
				last.Reason == nil || *last.Reason != "Replaced by backup restore" || last.IsCorrection {
				t.Errorf("void record of %s = %+v", f.ID, last)
			}
		}
		if revs, _ := logbookRepo.ListRevisions(ctx, userID, edited.ID); len(revs) != 2 || revs[0].Source != repository.ChangeSourceUserEdit {
			t.Errorf("revisions of the edited flight = %+v, want the earlier edit kept before the void record", revs)
		}

		revs, err := logbookRepo.ListRevisions(ctx, userID, restored.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(revs) != 1 || revs[0].Source != "user_edit" {
			t.Errorf("revisions of the restored flight = %+v, want only the imported one", revs)
		}

		got, err := flightRepo.GetByID(ctx, restored.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.CreatedAt.Equal(created) {
			t.Errorf("restored createdAt = %v, want %v", got.CreatedAt, created)
		}
	})

	t.Run("a strict backup switches strict mode on inside the transaction", func(t *testing.T) {
		userID := newUser(t, false)
		since := time.Date(2025, 1, 5, 6, 0, 0, 0, time.UTC)
		err := store.InRestoreTx(repository.WithChangeSource(ctx, repository.ChangeSourceImport), userID, func(tx repository.RestoreTx) error {
			if _, err := tx.ImportModeEvent(ctx, &models.LogbookModeEvent{
				UserID: userID, Strict: true, Trigger: models.LogbookTriggerManual, CreatedAt: since,
			}); err != nil {
				return err
			}
			switched, err := tx.SetStrict(ctx, userID, true, models.LogbookTriggerImport, nil)
			if err != nil || !switched {
				t.Errorf("SetStrict = %v, %v", switched, err)
			}
			if err := tx.RestoreStrictSince(ctx, userID, since); err != nil {
				return err
			}
			_, err = tx.ImportCertification(ctx, &models.LogbookCertification{
				UserID: userID, CertifiedThrough: since, CertifiedAt: since,
			})
			return err
		})
		if err != nil {
			t.Fatalf("restore: %v", err)
		}
		mode, err := logbookRepo.GetMode(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if !mode.Strict || mode.Since == nil || !mode.Since.Equal(since) {
			t.Errorf("mode = %+v", mode)
		}
		events, err := logbookRepo.ListModeEvents(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 2 || events[0].Trigger != models.LogbookTriggerManual || events[1].Trigger != models.LogbookTriggerImport {
			t.Errorf("events = %+v", events)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM logbook_certifications WHERE user_id = $1`, userID); n != 1 {
			t.Errorf("certifications = %d", n)
		}
	})

	t.Run("a failed restore rolls the logbook writes back", func(t *testing.T) {
		userID := newUser(t, false)
		err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			if _, err := tx.SetStrict(ctx, userID, true, models.LogbookTriggerImport, nil); err != nil {
				return err
			}
			return context.Canceled
		})
		if err == nil {
			t.Fatal("restore succeeded")
		}
		if mode, _ := logbookRepo.GetMode(ctx, userID); mode.Strict {
			t.Error("strict mode survived the rollback")
		}
	})

	t.Run("revisions are inserted in batches and a duplicate number is skipped", func(t *testing.T) {
		userID := newUser(t, false)
		flightID := uuid.New()
		revs := make([]models.FlightRevision, 0, 1201)
		for i := 1; i <= 1200; i++ {
			revs = append(revs, models.FlightRevision{
				UserID: userID, FlightID: flightID, RevisionNo: i, Kind: models.RevisionKindUpdate,
				Snapshot: snapshot(t, flightID), CrewSnapshot: json.RawMessage(`[]`), Source: "recalc",
				ChangedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute),
			})
		}
		revs = append(revs, revs[0])
		var stored int
		err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			var err error
			stored, err = tx.ImportRevisions(ctx, revs)
			return err
		})
		if err != nil {
			t.Fatalf("restore: %v", err)
		}
		if stored != 1200 {
			t.Errorf("stored = %d, want 1200", stored)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM flight_revisions WHERE flight_id = $1`, flightID); n != 1200 {
			t.Errorf("rows = %d", n)
		}
	})
}
