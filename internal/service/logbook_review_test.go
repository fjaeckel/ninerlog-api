package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

func revisionSnapshot(id uuid.UUID, date string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"id": id, "date": date, "aircraft_reg": "D-EABC", "aircraft_type": "C172",
		"departure_icao": "EDDF", "arrival_icao": "EDDM", "total_time": 60, "remarks": nil,
	})
	return b
}

func TestLogbookImport_RejectsUnreadableRevisions(t *testing.T) {
	flightID := uuid.New()
	good := revisionSnapshot(flightID, "2026-03-01")
	tests := []struct {
		name     string
		snapshot json.RawMessage
		crew     json.RawMessage
	}{
		{"null snapshot", json.RawMessage("null"), json.RawMessage("[]")},
		{"empty snapshot", nil, json.RawMessage("[]")},
		{"array snapshot", json.RawMessage("[]"), json.RawMessage("[]")},
		{"string snapshot", json.RawMessage(`"x"`), json.RawMessage("[]")},
		{"missing id", json.RawMessage(`{"date":"2026-03-01"}`), json.RawMessage("[]")},
		{"invalid id", json.RawMessage(`{"id":"nope","date":"2026-03-01"}`), json.RawMessage("[]")},
		{"missing date", json.RawMessage(`{"id":"` + flightID.String() + `"}`), json.RawMessage("[]")},
		{"date with time", json.RawMessage(`{"id":"` + flightID.String() + `","date":"2026-03-01T00:00:00Z"}`), json.RawMessage("[]")},
		{"total_time of the wrong type", json.RawMessage(`{"id":"` + flightID.String() + `","date":"2026-03-01","total_time":"x"}`), json.RawMessage("[]")},
		{"aircraft_reg of the wrong type", json.RawMessage(`{"id":"` + flightID.String() + `","date":"2026-03-01","aircraft_reg":5}`), json.RawMessage("[]")},
		{"crew snapshot is an object", good, json.RawMessage(`{}`)},
		{"crew entry of the wrong type", good, json.RawMessage(`[{"name":1,"role":"PIC"}]`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newLogbookEnv(t, false)
			in := LogbookImport{
				Strict: true,
				Revisions: []models.FlightRevision{
					{FlightID: flightID, RevisionNo: 1, Kind: "update", Snapshot: good, CrewSnapshot: json.RawMessage("[]"), Source: "user_edit"},
					{FlightID: flightID, RevisionNo: 2, Kind: "update", Snapshot: tt.snapshot, CrewSnapshot: tt.crew, Source: "user_edit"},
				},
			}
			if err := in.Validate(); !errors.Is(err, ErrInvalidLogbookImport) {
				t.Fatalf("Validate() = %v, want ErrInvalidLogbookImport", err)
			}
			if _, err := ImportLogbook(context.Background(), e.lbRepo, e.userID, in); !errors.Is(err, ErrInvalidLogbookImport) {
				t.Fatalf("ImportLogbook() = %v, want ErrInvalidLogbookImport", err)
			}
			if len(e.lbRepo.revisions) != 0 || len(e.lbRepo.events) != 0 || e.lbRepo.mode(e.userID).Strict {
				t.Fatalf("a rejected restore stored data: %d revisions, %d events", len(e.lbRepo.revisions), len(e.lbRepo.events))
			}
		})
	}
}

func TestLogbookImport_AcceptsReadableRevisions(t *testing.T) {
	flightID := uuid.New()
	in := LogbookImport{Revisions: []models.FlightRevision{
		{FlightID: flightID, RevisionNo: 1, Kind: "update", Snapshot: revisionSnapshot(flightID, "2026-03-01"), Source: "user_edit"},
		{FlightID: flightID, RevisionNo: 2, Kind: "update", Snapshot: revisionSnapshot(flightID, "2026-03-01"),
			CrewSnapshot: json.RawMessage(`[{"name":"Pat","role":"Instructor"}]`), Source: "user_edit"},
	}}
	if err := in.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestLogbookService_ReadersSkipUnreadableSnapshots(t *testing.T) {
	ctx := context.Background()
	e := newLogbookEnv(t, true)
	goodFlight, badFlight := uuid.New(), uuid.New()
	good := revisionSnapshot(goodFlight, "2026-03-01")
	bad := json.RawMessage("null")
	crew := json.RawMessage("[]")
	changed, _ := json.Marshal(map[string]any{
		"id": goodFlight, "date": "2026-03-01", "aircraft_reg": "D-EABC", "aircraft_type": "C172",
		"departure_icao": "EDDF", "arrival_icao": "EDDM", "total_time": 75, "remarks": nil,
	})
	e.lbRepo.current[goodFlight] = [2]json.RawMessage{changed, crew}
	e.lbRepo.current[badFlight] = [2]json.RawMessage{good, crew}
	e.lbRepo.revisions = []models.FlightRevision{
		{ID: uuid.New(), UserID: e.userID, FlightID: goodFlight, RevisionNo: 1, Kind: "update", Snapshot: good, CrewSnapshot: crew, Source: "user_edit", ChangedAt: testNow},
		{ID: uuid.New(), UserID: e.userID, FlightID: badFlight, RevisionNo: 1, Kind: "update", Snapshot: bad, CrewSnapshot: crew, Source: "user_edit", ChangedAt: testNow},
		{ID: uuid.New(), UserID: e.userID, FlightID: uuid.New(), RevisionNo: 1, Kind: "delete", Snapshot: bad, CrewSnapshot: crew, Source: "user_edit", ChangedAt: testNow},
		{ID: uuid.New(), UserID: e.userID, FlightID: uuid.New(), RevisionNo: 1, Kind: "delete", Snapshot: good, CrewSnapshot: crew, Source: "user_edit", ChangedAt: testNow.Add(-time.Hour)},
	}

	t.Run("voided list leaves out the unreadable record", func(t *testing.T) {
		voided, err := e.logbook.ListVoided(ctx, e.userID)
		if err != nil {
			t.Fatalf("ListVoided() = %v", err)
		}
		if len(voided) != 1 || voided[0].AircraftReg != "D-EABC" {
			t.Fatalf("voided = %+v", voided)
		}
	})

	t.Run("revision list keeps an undiffable revision with no changes", func(t *testing.T) {
		views, err := e.logbook.ListRevisions(ctx, e.userID, badFlight)
		if err != nil {
			t.Fatalf("ListRevisions() = %v", err)
		}
		if len(views) != 1 || len(views[0].Changes) != 0 {
			t.Fatalf("views = %+v", views)
		}
	})

	t.Run("export history skips it and keeps the rest", func(t *testing.T) {
		history, err := e.logbook.ExportHistory(ctx, e.userID)
		if err != nil {
			t.Fatalf("ExportHistory() = %v", err)
		}
		if len(history.Revisions) != 1 || history.Revisions[0].FlightID != goodFlight {
			t.Fatalf("revisions = %+v", history.Revisions)
		}
		if len(history.Revisions[0].Changes) != 1 || history.Revisions[0].Changes[0].Field != "totalTime" {
			t.Errorf("changes = %+v", history.Revisions[0].Changes)
		}
		if len(history.Voided) != 1 {
			t.Errorf("voided = %d, want 1", len(history.Voided))
		}
	})
}

func TestLogbookService_ExportHistoryUsesBatchedQueries(t *testing.T) {
	ctx := context.Background()
	e := newLogbookEnv(t, true)
	crew := json.RawMessage("[]")

	const flights = 5
	for i := 0; i < flights; i++ {
		id := uuid.New()
		snap := func(total int) json.RawMessage {
			b, _ := json.Marshal(map[string]any{
				"id": id, "date": "2026-03-01", "aircraft_reg": fmt.Sprintf("D-E%03d", i), "aircraft_type": "C172",
				"departure_icao": nil, "arrival_icao": nil, "total_time": total, "remarks": nil,
			})
			return b
		}
		e.lbRepo.current[id] = [2]json.RawMessage{snap(90), crew}
		for n := 1; n <= 2; n++ {
			e.lbRepo.revisions = append(e.lbRepo.revisions, models.FlightRevision{
				ID: uuid.New(), UserID: e.userID, FlightID: id, RevisionNo: n, Kind: "update",
				Snapshot: snap(60 + 10*(n-1)), CrewSnapshot: crew, Source: "user_edit", ChangedAt: testNow,
			})
		}
	}

	history, err := e.logbook.ExportHistory(ctx, e.userID)
	if err != nil {
		t.Fatalf("ExportHistory() = %v", err)
	}
	if len(history.Revisions) != flights*2 {
		t.Fatalf("revisions = %d, want %d", len(history.Revisions), flights*2)
	}
	for _, rev := range history.Revisions {
		if len(rev.Changes) != 1 || rev.Changes[0].Field != "totalTime" {
			t.Fatalf("revision %d of %s changes = %+v", rev.RevisionNo, rev.FlightID, rev.Changes)
		}
	}
	if e.lbRepo.listRevisionsCalls != 0 || e.lbRepo.currentStateCalls != 0 {
		t.Errorf("per-flight queries: ListRevisions=%d CurrentFlightState=%d, want 0",
			e.lbRepo.listRevisionsCalls, e.lbRepo.currentStateCalls)
	}
	if e.lbRepo.currentStatesCalls != 1 || e.lbRepo.listUserRevisionCalls != 1 {
		t.Errorf("batched queries: CurrentFlightStates=%d ListUserRevisions=%d, want 1 each",
			e.lbRepo.currentStatesCalls, e.lbRepo.listUserRevisionCalls)
	}
}

func TestLogbookService_CreateCertificationUpToUTCPlus14(t *testing.T) {
	// 2026-06-15 12:00 UTC is already 2026-06-16 02:00 at UTC+14.
	tests := []struct {
		name    string
		through string
		wantErr error
	}{
		{"today UTC", "2026-06-15", nil},
		{"today at UTC+14", "2026-06-16", nil},
		{"day after today at UTC+14", "2026-06-17", ErrCertificationInFuture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newLogbookEnv(t, true)
			through, _ := time.Parse("2006-01-02", tt.through)
			_, err := e.logbook.CreateCertification(context.Background(), e.userID, through)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	t.Run("just before the UTC+14 day starts", func(t *testing.T) {
		e := newLogbookEnv(t, true)
		e.logbook.now = func() time.Time { return time.Date(2026, 6, 15, 9, 59, 0, 0, time.UTC) }
		through := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		if _, err := e.logbook.CreateCertification(context.Background(), e.userID, through); err != nil {
			t.Fatalf("same day: %v", err)
		}
		next := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
		if _, err := e.logbook.CreateCertification(context.Background(), e.userID, next); !errors.Is(err, ErrCertificationInFuture) {
			t.Fatalf("next day: error = %v, want ErrCertificationInFuture", err)
		}
	})
}

func TestFlightService_UpdateLoadsStrictStateOnce(t *testing.T) {
	ctx := context.Background()

	t.Run("non-strict user pays for the mode lookup only", func(t *testing.T) {
		e := newCorrectionEnv(t, false, "CH")
		stored := e.seed(t, 30, 30*24*time.Hour)
		crewReads := 0
		e.svc.SetCrewRepository(countingCrewRepo{fakeCrewRepo: e.crew, reads: &crewReads})
		updated := e.edited(t, stored.ID, func(f *models.Flight) { f.Remarks = strp("x"); f.NightTime = 5; f.NightTimeOverride = true })
		crew := []models.FlightCrewMember{{Name: "Pat", Role: "Instructor"}}

		if err := e.svc.UpdateFlightWith(ctx, updated, e.userID, FlightWriteOptions{Crew: &crew}); err != nil {
			t.Fatal(err)
		}
		if e.lbRepo.getModeCalls != 1 {
			t.Errorf("GetMode calls = %d, want 1", e.lbRepo.getModeCalls)
		}
		if crewReads != 0 {
			t.Errorf("crew reads = %d, want 0", crewReads)
		}
		if e.licenses.listCalls != 0 {
			t.Errorf("licence list calls = %d, want 0", e.licenses.listCalls)
		}
	})

	t.Run("strict user loads mode and licences once for both checks", func(t *testing.T) {
		e := newCorrectionEnv(t, true, "CH")
		stored := e.seed(t, 0, time.Hour)
		updated := e.edited(t, stored.ID, func(f *models.Flight) { f.Remarks = strp("x"); f.NightTime = 0; f.NightTimeOverride = true })

		if err := e.svc.UpdateFlightWith(ctx, updated, e.userID, FlightWriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if e.lbRepo.getModeCalls != 1 {
			t.Errorf("GetMode calls = %d, want 1", e.lbRepo.getModeCalls)
		}
		if e.licenses.listCalls != 1 {
			t.Errorf("licence list calls = %d, want 1", e.licenses.listCalls)
		}
	})

	t.Run("a recalculation pass does no logbook lookups", func(t *testing.T) {
		e := newCorrectionEnv(t, true, "CH")
		for i := 0; i < 3; i++ {
			stored := e.seed(t, 30, 30*24*time.Hour)
			updated := e.edited(t, stored.ID, func(f *models.Flight) { f.Distance = 12.5 })
			if err := e.svc.UpdateFlightWith(ctx, updated, e.userID, FlightWriteOptions{Source: repository.ChangeSourceRecalc}); err != nil {
				t.Fatal(err)
			}
		}
		if e.lbRepo.getModeCalls != 0 || e.licenses.listCalls != 0 {
			t.Errorf("GetMode=%d licence lists=%d, want 0", e.lbRepo.getModeCalls, e.licenses.listCalls)
		}
	})
}

type countingCrewRepo struct {
	*fakeCrewRepo
	reads *int
}

func (c countingCrewRepo) GetByFlightID(ctx context.Context, id uuid.UUID) ([]models.FlightCrewMember, error) {
	*c.reads++
	return c.fakeCrewRepo.GetByFlightID(ctx, id)
}

func TestFlightService_BeforeWriteRunsOnlyForAcceptedWrites(t *testing.T) {
	ctx := context.Background()
	crew := []models.FlightCrewMember{{Name: "New Person", Role: "Instructor"}}

	tests := []struct {
		name      string
		strict    bool
		countries []string
		daysAgo   int
		created   time.Duration
		mutate    func(*models.Flight)
		reason    string
		prepare   func(*correctionEnv, *models.Flight)
		asUser    func(*correctionEnv) uuid.UUID
		wantErr   error
		wantCalls int
	}{
		{name: "accepted edit", strict: true, daysAgo: 0, created: time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantCalls: 1},
		{name: "accepted edit of a non-strict user", strict: false, daysAgo: 9, created: 9 * 24 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantCalls: 1},
		{name: "correction without a reason", strict: true, daysAgo: 3, created: 72 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired},
		{name: "correction with a reason", strict: true, daysAgo: 3, created: 72 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, reason: "typo", wantCalls: 1},
		{name: "override above the calculated value", strict: true, countries: []string{"CH"}, daysAgo: 0, created: time.Hour,
			mutate:  func(f *models.Flight) { f.NightTime = 59; f.NightTimeOverride = true },
			wantErr: ErrOverrideExceedsCalculated},
		{name: "locked flight", strict: true, daysAgo: 0, created: time.Hour,
			mutate:  func(f *models.Flight) { f.Remarks = strp("x") },
			prepare: func(_ *correctionEnv, f *models.Flight) { id := uuid.New(); f.SignatureID = &id },
			wantErr: ErrFlightLocked},
		{name: "other user's flight", strict: true, daysAgo: 0, created: time.Hour,
			mutate:  func(f *models.Flight) { f.Remarks = strp("x") },
			asUser:  func(*correctionEnv) uuid.UUID { return uuid.New() },
			wantErr: ErrUnauthorizedFlight},
		{name: "invalid flight", strict: true, daysAgo: 0, created: time.Hour,
			mutate:  func(f *models.Flight) { f.AircraftType = "" },
			wantErr: ErrInvalidFlight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCorrectionEnv(t, tt.strict, tt.countries...)
			stored := e.seed(t, tt.daysAgo, tt.created)
			if tt.prepare != nil {
				tt.prepare(e, stored)
			}
			updated := e.edited(t, stored.ID, tt.mutate)
			userID := e.userID
			if tt.asUser != nil {
				userID = tt.asUser(e)
			}
			calls := 0
			err := e.svc.UpdateFlightWith(ctx, updated, userID, FlightWriteOptions{
				CorrectionReason: tt.reason,
				Crew:             &crew,
				BeforeWrite:      func(context.Context) error { calls++; return nil },
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("BeforeWrite calls = %d, want %d", calls, tt.wantCalls)
			}
			if tt.wantErr != nil && len(e.flights.writes) != 0 {
				t.Errorf("a rejected update wrote %d times", len(e.flights.writes))
			}
		})
	}

	t.Run("a failing BeforeWrite stops the write", func(t *testing.T) {
		e := newCorrectionEnv(t, false)
		stored := e.seed(t, 0, time.Hour)
		updated := e.edited(t, stored.ID, func(f *models.Flight) { f.Remarks = strp("x") })
		boom := errors.New("boom")
		err := e.svc.UpdateFlightWith(ctx, updated, e.userID, FlightWriteOptions{BeforeWrite: func(context.Context) error { return boom }})
		if !errors.Is(err, boom) || len(e.flights.writes) != 0 {
			t.Fatalf("error = %v, writes = %d", err, len(e.flights.writes))
		}
	})
}

func TestFlightService_CreateRunsBeforeWriteAfterOverrideCheck(t *testing.T) {
	ctx := context.Background()
	newFlight := func(e *correctionEnv, night int) *models.Flight {
		return &models.Flight{
			UserID: e.userID, Date: testNow, AircraftReg: "D-EABC", AircraftType: "C172",
			TotalTime: 60, PICTime: 60, IsPIC: true, AllLandings: 1,
			NightTime: night, NightTimeOverride: night > 0,
		}
	}

	t.Run("rejected override never reaches BeforeWrite", func(t *testing.T) {
		e := newCorrectionEnv(t, true, "CH")
		calls := 0
		err := e.svc.CreateFlightChecked(ctx, newFlight(e, 59), FlightWriteOptions{BeforeWrite: func(context.Context) error { calls++; return nil }})
		if !errors.Is(err, ErrOverrideExceedsCalculated) || calls != 0 {
			t.Fatalf("error = %v, calls = %d", err, calls)
		}
	})

	t.Run("accepted flight is written with its crew", func(t *testing.T) {
		e := newCorrectionEnv(t, true, "CH")
		calls := 0
		crew := []models.FlightCrewMember{{Name: "Pat", Role: "Instructor"}}
		err := e.svc.CreateFlightChecked(ctx, newFlight(e, 0), FlightWriteOptions{Crew: &crew, BeforeWrite: func(context.Context) error { calls++; return nil }})
		if err != nil || calls != 1 {
			t.Fatalf("error = %v, calls = %d", err, calls)
		}
	})
}
