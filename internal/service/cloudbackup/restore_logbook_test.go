package cloudbackup

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/testutil/restorefake"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func revisionSnapshot(t *testing.T, flightID uuid.UUID, date string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id": flightID, "user_id": uuid.New(), "date": date,
		"aircraft_reg": "D-EABC", "aircraft_type": "C172", "total_time": 55,
	})
	require.NoError(t, err)
	return raw
}

// strictPayload is samplePayload with a strict logbook history: two revisions
// of a restored flight, a void record of a deleted flight, a licence-triggered
// switch and a certification.
func strictPayload(t *testing.T) (*Payload, uuid.UUID, uuid.UUID) {
	t.Helper()
	p := samplePayload()
	since := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	licenseID := p.Licenses[0].License.ID
	kept := p.Flights[0].ID
	deleted := uuid.New()
	crew := json.RawMessage(`[{"name":"Sam Carter","role":"Instructor","contactId":"` + uuid.NewString() + `"}]`)

	p.LogbookMode = &LogbookMode{Strict: true, Since: &since, Events: []LogbookModeEvent{
		{Strict: true, Trigger: models.LogbookTriggerLicence, LicenseID: &licenseID, CreatedAt: since},
	}}
	p.FlightRevisions = []FlightRevision{
		{FlightID: kept, RevisionNo: 1, Kind: models.RevisionKindUpdate, Snapshot: revisionSnapshot(t, kept, "2026-01-01"),
			CrewSnapshot: crew, Source: "user_edit", Reason: ptr("typo"), IsCorrection: true, ChangedAt: since.Add(time.Hour)},
		{FlightID: kept, RevisionNo: 2, Kind: models.RevisionKindUpdate, Snapshot: revisionSnapshot(t, kept, "2026-01-01"),
			CrewSnapshot: crew, Source: "something-new", ChangedAt: since.Add(2 * time.Hour)},
		{FlightID: deleted, RevisionNo: 1, Kind: models.RevisionKindUpdate, Snapshot: revisionSnapshot(t, deleted, "2026-01-03"),
			CrewSnapshot: json.RawMessage(`[]`), Source: "recalc", ChangedAt: since.Add(3 * time.Hour)},
		{FlightID: deleted, RevisionNo: 2, Kind: models.RevisionKindDelete, Snapshot: revisionSnapshot(t, deleted, "2026-01-03"),
			CrewSnapshot: json.RawMessage(`[]`), Source: "user_edit", Reason: ptr("logged twice"), ChangedAt: since.Add(4 * time.Hour)},
	}
	p.LogbookCertifications = []LogbookCertification{{CertifiedThrough: "2026-01-31", CertifiedAt: since.AddDate(0, 1, 0)}}
	return p, kept, deleted
}

func TestRestore_LogbookHistoryFollowsRestoredIDs(t *testing.T) {
	p, kept, deleted := strictPayload(t)
	userID := uuid.New()
	store := restorefake.New()

	res, err := NewRestorer(store).Restore(context.Background(), userID, p, RestoreModeRestore)
	require.NoError(t, err)
	assert.Equal(t, 4, res.FlightRevisionsImported)
	assert.Equal(t, 1, res.LogbookCertificationsImported)

	st := store.State
	require.Len(t, st.Flights, len(p.Flights))
	restoredFlight := st.Flights[0].ID
	assert.Equal(t, kept, restoredFlight)

	byFlight := map[uuid.UUID][]models.FlightRevision{}
	for _, r := range st.Revisions {
		byFlight[r.FlightID] = append(byFlight[r.FlightID], r)
		assert.Equal(t, userID, r.UserID)
		var snap map[string]any
		require.NoError(t, json.Unmarshal(r.Snapshot, &snap))
		assert.Equal(t, r.FlightID.String(), snap["id"])
		assert.Equal(t, userID.String(), snap["user_id"])
		assert.NotContains(t, string(r.CrewSnapshot), "contactId")
	}
	require.Len(t, byFlight, 2, "restored flight plus the deleted flight")
	require.Len(t, byFlight[deleted], 2, "a deleted flight keeps its id")
	require.Len(t, byFlight[restoredFlight], 2)
	assert.Equal(t, "unknown", byFlight[restoredFlight][1].Source)
	assert.Equal(t, 1, byFlight[restoredFlight][0].RevisionNo)
	assert.True(t, byFlight[restoredFlight][0].IsCorrection)

	assert.True(t, st.Strict)
	require.NotNil(t, st.StrictSince)
	assert.True(t, st.StrictSince.Equal(*p.LogbookMode.Since))
	require.Len(t, st.ModeEvents, 2)
	assert.Equal(t, models.LogbookTriggerLicence, st.ModeEvents[0].Trigger)
	require.NotNil(t, st.ModeEvents[0].LicenseID)
	assert.Equal(t, st.Licenses[0].ID, *st.ModeEvents[0].LicenseID)
	assert.Equal(t, models.LogbookTriggerImport, st.ModeEvents[1].Trigger)

	require.Len(t, st.Certifications, 1)
	assert.Equal(t, "2026-01-31", st.Certifications[0].CertifiedThrough.Format("2006-01-02"))
}

func TestRestore_NonStrictBackupLeavesTheAccountNonStrict(t *testing.T) {
	p, _, _ := strictPayload(t)
	p.LogbookMode.Strict = false
	store := restorefake.New()

	_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
	require.NoError(t, err)
	assert.False(t, store.State.Strict)
	for _, e := range store.State.ModeEvents {
		assert.NotEqual(t, models.LogbookTriggerImport, e.Trigger)
	}
}

func TestRestore_UnreadableLogbookEntryRejectsTheWholeRestore(t *testing.T) {
	tests := map[string]struct {
		mutate  func(p *Payload)
		section string
	}{
		"snapshot not an object": {func(p *Payload) { p.FlightRevisions[1].Snapshot = json.RawMessage(`[1]`) }, "flightRevisions"},
		"crew not an array":      {func(p *Payload) { p.FlightRevisions[0].CrewSnapshot = json.RawMessage(`{"a":1}`) }, "flightRevisions"},
		"unknown kind":           {func(p *Payload) { p.FlightRevisions[2].Kind = "rewrite" }, "flightRevisions"},
		"revision number":        {func(p *Payload) { p.FlightRevisions[0].RevisionNo = 0 }, "flightRevisions"},
		"unknown trigger":        {func(p *Payload) { p.LogbookMode.Events[0].Trigger = "admin" }, "logbookMode.events"},
		"certification date":     {func(p *Payload) { p.LogbookCertifications[0].CertifiedThrough = "31.01.2026" }, "logbookCertifications"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, _, _ := strictPayload(t)
			tc.mutate(p)
			store := restorefake.New()

			_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
			var invalid *InvalidBackupError
			require.True(t, errors.As(err, &invalid), "error = %v", err)
			assert.Contains(t, invalid.Message, tc.section)
			assert.Zero(t, store.Commits)
			assert.Empty(t, store.State.Flights)
			assert.Empty(t, store.State.Revisions)
			assert.False(t, store.State.Strict)
		})
	}
}

func TestRestore_RevisionCap(t *testing.T) {
	p := samplePayload()
	p.FlightRevisions = make([]FlightRevision, MaxRestoreRevisions+1)
	_, err := NewRestorer(restorefake.New()).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
	var invalid *InvalidBackupError
	require.True(t, errors.As(err, &invalid))
	assert.Contains(t, invalid.Message, "flight revisions")
}

func TestRestore_RunsWithImportChangeContext(t *testing.T) {
	store := restorefake.New()
	_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), samplePayload(), RestoreModeRestore)
	require.NoError(t, err)
	assert.Equal(t, repository.ChangeSourceImport, store.ChangeContext.Source)
	assert.Nil(t, store.ChangeContext.Reason)
}

func TestRestore_ReplaceKeepsHistoryAndRecordsTheWipe(t *testing.T) {
	store := restorefake.New()
	userID := uuid.New()
	_, err := NewRestorer(store).Restore(context.Background(), userID, samplePayload(), RestoreModeRestore)
	require.NoError(t, err)

	since := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	existingRevision := models.FlightRevision{ID: uuid.New(), UserID: userID, FlightID: uuid.New(), RevisionNo: 1,
		Kind: models.RevisionKindDelete, Source: "user_edit", ChangedAt: since}
	store.State.Strict = true
	store.State.StrictSince = &since
	store.State.ModeEvents = []models.LogbookModeEvent{{ID: uuid.New(), UserID: userID, Strict: true, Trigger: "manual", CreatedAt: since}}
	store.State.Revisions = []models.FlightRevision{existingRevision}
	store.State.Certifications = []models.LogbookCertification{{ID: uuid.New(), UserID: userID, CertifiedThrough: since, CertifiedAt: since}}

	p, _, _ := strictPayload(t)
	res, err := NewRestorer(store).Restore(context.Background(), userID, p, RestoreModeReplace)
	require.NoError(t, err)

	assert.Equal(t, repository.ChangeSourceImport, store.ChangeContext.Source)
	require.NotNil(t, store.ChangeContext.Reason)
	assert.Equal(t, ReplaceReason, *store.ChangeContext.Reason)

	st := store.State
	assert.Len(t, st.Flights, len(p.Flights))
	assert.Len(t, st.Revisions, 1+len(p.FlightRevisions))
	assert.Equal(t, existingRevision.ID, st.Revisions[0].ID)
	assert.Equal(t, 4, res.FlightRevisionsImported)
	assert.Len(t, st.Certifications, 2)
	require.Len(t, st.ModeEvents, 2, "existing switch kept, backup switch added, no import event for a strict account")
	assert.Equal(t, models.LogbookTriggerManual, st.ModeEvents[0].Trigger)
	assert.True(t, st.StrictSince.Equal(since), "an already strict account keeps its since")
}

func TestRestore_FlightKeepsItsCreatedAt(t *testing.T) {
	p := samplePayload()
	created := time.Date(2025, 12, 31, 7, 0, 0, 0, time.UTC)
	p.Flights[0].CreatedAt = created
	p.Flights[1].CreatedAt = time.Now().Add(72 * time.Hour)
	store := restorefake.New()

	before := time.Now()
	_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
	require.NoError(t, err)
	assert.True(t, store.State.Flights[0].CreatedAt.Equal(created))
	assert.False(t, store.State.Flights[1].CreatedAt.Before(before))
	assert.False(t, store.State.Flights[1].CreatedAt.After(time.Now()), "a future createdAt is clamped to the restore time")
}

func TestRestoredCreatedAt(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	past := now.Add(-48 * time.Hour)
	flightDate := time.Date(2026, 3, 2, 0, 0, 0, 0, time.FixedZone("x", 3600))
	tests := []struct {
		name       string
		backup     time.Time
		flightDate time.Time
		want       time.Time
	}{
		{"past value is kept", past, flightDate, past},
		{"value equal to now is kept", now, flightDate, now},
		{"future value is clamped to now", now.Add(24 * time.Hour), flightDate, now},
		{"missing value becomes the flight date at midnight UTC", time.Time{}, flightDate, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
		{"missing value of a future flight is clamped to now", time.Time{}, now.AddDate(0, 0, 3), now},
		{"missing value and date becomes now", time.Time{}, time.Time{}, now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := restoredCreatedAt(tt.backup, tt.flightDate, now); !got.Equal(tt.want) {
				t.Errorf("restoredCreatedAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRestoreLimits(t *testing.T) {
	assert.GreaterOrEqual(t, MaxRestoreRevisions, 10*MaxRestoreFlights)
}

func idsOf(flights []*models.Flight) []uuid.UUID {
	out := make([]uuid.UUID, len(flights))
	for i, f := range flights {
		out[i] = f.ID
	}
	return out
}

func TestRestore_FlightIDs(t *testing.T) {
	t.Run("flights, crew and history keep their ids", func(t *testing.T) {
		p, kept, deleted := strictPayload(t)
		store := restorefake.New()

		_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
		require.NoError(t, err)

		assert.Equal(t, []uuid.UUID{p.Flights[0].ID, p.Flights[1].ID}, idsOf(store.State.Flights))
		require.Len(t, store.State.Crew, 1)
		assert.Equal(t, kept, store.State.Crew[0].FlightID)
		ids := map[uuid.UUID]bool{}
		for _, r := range store.State.Revisions {
			ids[r.FlightID] = true
		}
		assert.Equal(t, map[uuid.UUID]bool{kept: true, deleted: true}, ids)
	})

	t.Run("an id held by another user is replaced, with its crew and history", func(t *testing.T) {
		p, kept, deleted := strictPayload(t)
		store := restorefake.New()
		store.ForeignFlightIDs = map[uuid.UUID]bool{kept: true, deleted: true}

		_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
		require.NoError(t, err)

		newID := store.State.Flights[0].ID
		assert.NotEqual(t, kept, newID)
		assert.Equal(t, p.Flights[1].ID, store.State.Flights[1].ID)
		assert.Equal(t, newID, store.State.Crew[0].FlightID)
		ids := map[uuid.UUID]int{}
		for _, r := range store.State.Revisions {
			ids[r.FlightID]++
		}
		assert.Equal(t, 2, ids[newID])
		assert.Zero(t, ids[kept])
		assert.Zero(t, ids[deleted])
		assert.Len(t, ids, 2, "the deleted flight gets one fresh id")
	})

	t.Run("a flight with a void record of its own gets a new id", func(t *testing.T) {
		p := samplePayload()
		store := restorefake.New()
		userID := uuid.New()
		store.State.Revisions = []models.FlightRevision{{
			ID: uuid.New(), UserID: userID, FlightID: p.Flights[0].ID, RevisionNo: 1, Kind: models.RevisionKindDelete,
		}}

		_, err := NewRestorer(store).Restore(context.Background(), userID, p, RestoreModeRestore)
		require.NoError(t, err)

		assert.NotEqual(t, p.Flights[0].ID, store.State.Flights[0].ID)
		assert.Equal(t, store.State.Flights[0].ID, store.State.Crew[0].FlightID)
		assert.Equal(t, p.Flights[1].ID, store.State.Flights[1].ID)
	})

	t.Run("a repeated id in the backup keeps it once", func(t *testing.T) {
		p := samplePayload()
		p.Flights[1].ID = p.Flights[0].ID
		store := restorefake.New()

		_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
		require.NoError(t, err)

		assert.Equal(t, p.Flights[0].ID, store.State.Flights[0].ID)
		assert.NotEqual(t, p.Flights[0].ID, store.State.Flights[1].ID)
	})

	t.Run("a flight without an id gets a new one", func(t *testing.T) {
		p := samplePayload()
		p.Flights[0].ID = uuid.Nil
		store := restorefake.New()

		_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
		require.NoError(t, err)

		assert.NotEqual(t, uuid.Nil, store.State.Flights[0].ID)
		assert.Equal(t, store.State.Flights[0].ID, store.State.Crew[0].FlightID)
	})

	t.Run("replacing the account with its own backup twice stores the history once", func(t *testing.T) {
		p, _, _ := strictPayload(t)
		store := restorefake.New()
		userID := uuid.New()
		restorer := NewRestorer(store)

		_, err := restorer.Restore(context.Background(), userID, p, RestoreModeRestore)
		require.NoError(t, err)
		first := idsOf(store.State.Flights)
		revisions := len(store.State.Revisions)

		for range 2 {
			res, err := restorer.Restore(context.Background(), userID, p, RestoreModeReplace)
			require.NoError(t, err)
			assert.Zero(t, res.FlightRevisionsImported)
			assert.Equal(t, first, idsOf(store.State.Flights))
			assert.Len(t, store.State.Revisions, revisions)
		}
	})
}

// outsideTxLogbookRepo and outsideTxLicences fail any read that does not go
// through the restore transaction.
type outsideTxLogbookRepo struct{ repository.LogbookRepository }

func (outsideTxLogbookRepo) GetMode(context.Context, uuid.UUID) (*models.LogbookMode, error) {
	return nil, errors.New("strict mode read outside the restore transaction")
}

func (outsideTxLogbookRepo) LatestCertifiedThrough(context.Context, uuid.UUID) (*time.Time, error) {
	return nil, errors.New("certification read outside the restore transaction")
}

type outsideTxLicences struct{ repository.LicenseRepository }

func (outsideTxLicences) GetByUserID(context.Context, uuid.UUID, *time.Time) ([]*models.License, error) {
	return nil, errors.New("licence read outside the restore transaction")
}

func logbookServiceOutsideTx() *service.LogbookService {
	return service.NewLogbookService(outsideTxLogbookRepo{}, outsideTxLicences{}, nil, nil)
}

func TestRestore_ReplaceClassifiesTheWipedFlights(t *testing.T) {
	late := sampleFlight(1, "D-EOLD")
	late.Date = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	future := sampleFlight(2, "D-ENEW")
	future.Date = time.Now().UTC().AddDate(5, 0, 0)

	tests := []struct {
		name   string
		strict bool
		want   []uuid.UUID
	}{
		{"strict account flags flights past their window", true, []uuid.UUID{late.ID}},
		{"standard account flags nothing", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := restorefake.New()
			store.State.Strict = tt.strict
			store.State.Flights = []*models.Flight{late, future}
			restorer := NewRestorer(store)
			restorer.SetLogbookService(logbookServiceOutsideTx())

			_, err := restorer.Restore(context.Background(), uuid.New(), samplePayload(), RestoreModeReplace)
			require.NoError(t, err)
			assert.Equal(t, tt.want, store.WipeCorrections)
			assert.Positive(t, store.TxReads, "the strict-mode state is read through the transaction")
		})
	}

	t.Run("a signed flight is flagged whatever its date", func(t *testing.T) {
		signed := sampleFlight(3, "D-ESGN")
		signed.Date = time.Now().UTC().AddDate(5, 0, 0)
		sigID := uuid.New()
		signed.SignatureID = &sigID
		store := restorefake.New()
		store.State.Strict = true
		store.State.Flights = []*models.Flight{signed}
		restorer := NewRestorer(store)
		restorer.SetLogbookService(logbookServiceOutsideTx())

		_, err := restorer.Restore(context.Background(), uuid.New(), samplePayload(), RestoreModeReplace)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{signed.ID}, store.WipeCorrections)
	})

	t.Run("restore mode wipes nothing", func(t *testing.T) {
		store := restorefake.New()
		restorer := NewRestorer(store)
		restorer.SetLogbookService(logbookServiceOutsideTx())

		_, err := restorer.Restore(context.Background(), uuid.New(), samplePayload(), RestoreModeRestore)
		require.NoError(t, err)
		assert.Nil(t, store.WipeCorrections)
		assert.Zero(t, store.TxReads)
	})
}

func TestRestore_RejectsMissingLogbookTimestamps(t *testing.T) {
	tests := map[string]struct {
		mutate  func(p *Payload)
		section string
	}{
		"revision changedAt":        {func(p *Payload) { p.FlightRevisions[1].ChangedAt = time.Time{} }, "flightRevisions"},
		"mode event createdAt":      {func(p *Payload) { p.LogbookMode.Events[0].CreatedAt = time.Time{} }, "logbookMode.events"},
		"certification certifiedAt": {func(p *Payload) { p.LogbookCertifications[0].CertifiedAt = time.Time{} }, "logbookCertifications"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, _, _ := strictPayload(t)
			tc.mutate(p)
			store := restorefake.New()

			_, err := NewRestorer(store).Restore(context.Background(), uuid.New(), p, RestoreModeRestore)
			var invalid *InvalidBackupError
			require.True(t, errors.As(err, &invalid), "error = %v", err)
			assert.Contains(t, invalid.Message, tc.section)
			assert.Zero(t, store.Commits)
		})
	}
}
