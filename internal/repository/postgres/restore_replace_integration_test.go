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
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func TestReplaceWithOwnBackupIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	crewRepo := postgres.NewFlightCrewRepository(db)
	logbookRepo := postgres.NewLogbookRepository(db)
	restorer := cloudbackup.NewRestorer(postgres.NewRestoreStore(db, time.Minute))
	userEdit := repository.WithChangeSource(ctx, repository.ChangeSourceUserEdit)

	userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	crew := []models.FlightCrewMember{{Name: "Pat", Role: models.CrewRoleInstructor}}

	var live, voided []*models.Flight
	for i := 0; i < 5; i++ {
		f := reviewFlight(userID, today.AddDate(0, 0, -i), time.Time{})
		if err := flightRepo.CreateWithCrew(ctx, f, crew); err != nil {
			t.Fatalf("create flight %d: %v", i, err)
		}
		if i < 3 {
			voided = append(voided, f)
		} else {
			live = append(live, f)
		}
	}
	for _, f := range voided {
		if err := flightRepo.Delete(userEdit, f.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
	}
	edited := *live[0]
	edited.Remarks = ptrTo("edited")
	if err := flightRepo.Update(userEdit, &edited); err != nil {
		t.Fatalf("update: %v", err)
	}

	voidedCount := func() int {
		return countRows(t, db, `SELECT COUNT(*) FROM flight_revisions WHERE user_id = $1 AND kind = 'delete'`, userID)
	}
	revisionCount := func() int {
		return countRows(t, db, `SELECT COUNT(*) FROM flight_revisions WHERE user_id = $1`, userID)
	}
	if voidedCount() != 3 || revisionCount() != 4 {
		t.Fatalf("setup: voided = %d, revisions = %d", voidedCount(), revisionCount())
	}

	gather := func() *cloudbackup.Payload {
		t.Helper()
		flights, err := flightRepo.GetByUserID(ctx, userID, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]uuid.UUID, len(flights))
		for i, f := range flights {
			ids[i] = f.ID
		}
		crews, err := crewRepo.GetByFlightIDs(ctx, ids)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range flights {
			f.CrewMembers = crews[f.ID]
		}
		mode, err := logbookRepo.GetMode(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		revs, err := logbookRepo.ListUserRevisions(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		p := &cloudbackup.Payload{
			Format: cloudbackup.PayloadFormat, Version: cloudbackup.PayloadVersion, Flights: flights,
			LogbookMode: &cloudbackup.LogbookMode{Strict: mode.Strict, Since: mode.Since, Events: []cloudbackup.LogbookModeEvent{}},
		}
		for _, r := range revs {
			p.FlightRevisions = append(p.FlightRevisions, cloudbackup.FlightRevision{
				FlightID: r.FlightID, RevisionNo: r.RevisionNo, Kind: r.Kind, Snapshot: r.Snapshot,
				CrewSnapshot: r.CrewSnapshot, Source: r.Source, Reason: r.Reason, IsCorrection: r.IsCorrection, ChangedAt: r.ChangedAt,
			})
		}
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var out cloudbackup.Payload
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return &out
	}

	backup := gather()
	if len(backup.Flights) != 2 {
		t.Fatalf("backup flights = %d, want 2", len(backup.Flights))
	}
	historyOf := func(id uuid.UUID) []models.FlightRevision {
		revs, err := logbookRepo.ListRevisions(ctx, userID, id)
		if err != nil {
			t.Fatal(err)
		}
		return revs
	}
	before := historyOf(live[0].ID)

	for round := 1; round <= 2; round++ {
		res, err := restorer.Restore(ctx, userID, backup, cloudbackup.RestoreModeReplace)
		if err != nil {
			t.Fatalf("replace %d: %v", round, err)
		}
		if res.FlightsImported != 2 || res.FlightRevisionsImported != 0 {
			t.Errorf("replace %d: result = %+v", round, res)
		}
		if n := voidedCount(); n != 3 {
			t.Errorf("replace %d: void records = %d, want 3", round, n)
		}
		if n := revisionCount(); n != 4 {
			t.Errorf("replace %d: revisions = %d, want 4 (no new void records)", round, n)
		}
		for _, f := range live {
			got, err := flightRepo.GetByID(ctx, f.ID)
			if err != nil || got.UserID != userID {
				t.Fatalf("replace %d: flight %s kept its id: %v", round, f.ID, err)
			}
		}
		if after := historyOf(live[0].ID); len(after) != len(before) || after[0].ID != before[0].ID {
			t.Errorf("replace %d: history of a live flight changed: %+v", round, after)
		}
		voidedList, err := logbookRepo.ListVoided(ctx, userID)
		if err != nil || len(voidedList) != 3 {
			t.Errorf("replace %d: voided list = %d, %v", round, len(voidedList), err)
		}
	}

	t.Run("flights absent from the backup get void records classified by the delete rules", func(t *testing.T) {
		restorer.SetLogbookService(service.NewLogbookService(
			logbookRepo, postgres.NewLicenseRepository(db), flightRepo, postgres.NewFlightSignatureRepository(db)))
		current := reviewFlight(userID, today, time.Time{})
		old := reviewFlight(userID, today.AddDate(0, 0, -30), time.Time{})
		for _, f := range []*models.Flight{current, old} {
			if err := flightRepo.CreateWithCrew(ctx, f, crew); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := restorer.Restore(ctx, userID, backup, cloudbackup.RestoreModeReplace); err != nil {
			t.Fatalf("replace: %v", err)
		}
		for f, wantCorrection := range map[*models.Flight]bool{current: false, old: true} {
			revs := historyOf(f.ID)
			if len(revs) != 1 || revs[0].Kind != models.RevisionKindDelete ||
				revs[0].Source != repository.ChangeSourceImport || revs[0].Reason == nil || *revs[0].Reason != cloudbackup.ReplaceReason ||
				revs[0].IsCorrection != wantCorrection {
				t.Fatalf("void record of %s = %+v, want correction %v", f.Date.Format("2006-01-02"), revs, wantCorrection)
			}
		}
		if voidedCount() != 5 {
			t.Errorf("void records = %d, want 5", voidedCount())
		}
	})
}

func TestFlightIDUsageIntegration(t *testing.T) {
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
	userEdit := repository.WithChangeSource(ctx, repository.ChangeSourceUserEdit)

	owner := newReviewUser(t, ctx, userRepo, logbookRepo, true)
	other := newReviewUser(t, ctx, userRepo, logbookRepo, true)
	now := time.Now().UTC()

	existing := reviewFlight(other, now, time.Time{})
	foreignVoided := reviewFlight(other, now, time.Time{})
	ownVoided := reviewFlight(owner, now, time.Time{})
	for _, f := range []*models.Flight{existing, foreignVoided, ownVoided} {
		if err := flightRepo.Create(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []*models.Flight{foreignVoided, ownVoided} {
		if err := flightRepo.Delete(userEdit, f.ID); err != nil {
			t.Fatal(err)
		}
	}
	free := uuid.New()

	var usage repository.FlightIDUsage
	err := store.InRestoreTx(ctx, owner, func(tx repository.RestoreTx) error {
		var err error
		usage, err = tx.FlightIDUsage(ctx, owner, []uuid.UUID{existing.ID, foreignVoided.ID, ownVoided.ID, free})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]uuid.UUID{"existing flight": existing.ID, "other user's void record": foreignVoided.ID} {
		if !usage.Taken[id] {
			t.Errorf("%s is not taken", name)
		}
	}
	if usage.Taken[ownVoided.ID] || usage.Taken[free] || !usage.Voided[ownVoided.ID] || usage.Voided[free] {
		t.Errorf("usage = %+v", usage)
	}
}
