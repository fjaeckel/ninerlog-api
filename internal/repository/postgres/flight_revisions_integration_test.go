//go:build integration

package postgres_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

// TestFlightRevisionCaptureIntegration covers the revision triggers against
// real Postgres: one revision per logical change, none for no-ops or users
// outside strict mode, and the change source taken from the context.
func TestFlightRevisionCaptureIntegration(t *testing.T) {
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

	newUser := func(t *testing.T, strict bool) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("rev-"+uuid.NewString()+"@example.com", "Rev User", "hash")
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
		dep, arr := "EDDF", "EDDM"
		f := &models.Flight{
			UserID:        userID,
			Date:          time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			AircraftReg:   "D-EABC",
			AircraftType:  "C172",
			DepartureICAO: &dep,
			ArrivalICAO:   &arr,
			TotalTime:     60,
			PICTime:       60,
			IsPIC:         true,
			AllLandings:   1,
			LandingsDay:   1,
			TakeoffsDay:   1,
		}
		if err := flightRepo.CreateWithCrew(ctx, f, []models.FlightCrewMember{
			{Name: "Pat Instructor", Role: models.CrewRole("Instructor")},
		}); err != nil {
			t.Fatalf("create flight: %v", err)
		}
		return f
	}

	revisions := func(t *testing.T, userID, flightID uuid.UUID) []models.FlightRevision {
		t.Helper()
		revs, err := logbookRepo.ListRevisions(ctx, userID, flightID)
		if err != nil {
			t.Fatalf("list revisions: %v", err)
		}
		return revs
	}

	userEdit := func(reason string, correction bool) context.Context {
		return repository.WithChangeContext(ctx, repository.ChangeContext{
			Source: repository.ChangeSourceUserEdit, Reason: &reason, IsCorrection: correction,
		})
	}

	t.Run("creating a flight with crew records nothing", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)
		if got := revisions(t, userID, f.ID); len(got) != 0 {
			t.Fatalf("revisions after create = %d, want 0", len(got))
		}
	})

	t.Run("strict user edit yields exactly one revision with source and reason", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)

		f.TotalTime = 75
		f.PICTime = 75
		if err := flightRepo.Update(userEdit("wrong block time", true), f); err != nil {
			t.Fatalf("update: %v", err)
		}

		revs := revisions(t, userID, f.ID)
		if len(revs) != 1 {
			t.Fatalf("revisions = %d, want 1", len(revs))
		}
		r := revs[0]
		if r.RevisionNo != 1 || r.Kind != models.RevisionKindUpdate {
			t.Errorf("revision = no %d kind %s", r.RevisionNo, r.Kind)
		}
		if r.Source != repository.ChangeSourceUserEdit {
			t.Errorf("source = %q, want user_edit", r.Source)
		}
		if r.Reason == nil || *r.Reason != "wrong block time" {
			t.Errorf("reason = %v", r.Reason)
		}
		if !r.IsCorrection {
			t.Error("is_correction = false, want true")
		}
	})

	t.Run("flight update and crew replacement in one transaction yield one revision", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)

		f.Remarks = strPtr("night leg")
		crew := []models.FlightCrewMember{{Name: "Sam Student", Role: models.CrewRole("Student")}}
		if err := flightRepo.UpdateWithCrew(userEdit("", false), f, crew); err != nil {
			t.Fatalf("update with crew: %v", err)
		}
		if got := revisions(t, userID, f.ID); len(got) != 1 {
			t.Fatalf("revisions = %d, want 1", len(got))
		}
	})

	t.Run("crew-only change yields one revision", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)

		err := crewRepo.SetCrewMembers(userEdit("", false), f.ID, []models.FlightCrewMember{
			{Name: "Other Instructor", Role: models.CrewRole("Instructor")},
		})
		if err != nil {
			t.Fatalf("set crew: %v", err)
		}
		revs := revisions(t, userID, f.ID)
		if len(revs) != 1 {
			t.Fatalf("revisions = %d, want 1", len(revs))
		}
		if revs[0].Source != repository.ChangeSourceUserEdit {
			t.Errorf("source = %q", revs[0].Source)
		}
	})

	t.Run("no-op save yields no revision", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)

		same := []models.FlightCrewMember{{Name: "Pat Instructor", Role: models.CrewRole("Instructor")}}
		if err := flightRepo.UpdateWithCrew(userEdit("", false), f, same); err != nil {
			t.Fatalf("update with crew: %v", err)
		}
		if got := revisions(t, userID, f.ID); len(got) != 0 {
			t.Fatalf("revisions = %d, want 0", len(got))
		}
	})

	t.Run("non-strict user yields no revision", func(t *testing.T) {
		userID := newUser(t, false)
		f := newFlight(t, userID)

		f.TotalTime = 90
		f.PICTime = 90
		if err := flightRepo.Update(userEdit("edit", false), f); err != nil {
			t.Fatalf("update: %v", err)
		}
		if err := flightRepo.Delete(userEdit("", false), f.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if got := revisions(t, userID, f.ID); len(got) != 0 {
			t.Fatalf("revisions = %d, want 0", len(got))
		}
	})

	t.Run("recalc source is recorded, not unknown", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)

		f.Distance = 123.4
		f.NightTime = 10
		if err := flightRepo.Update(repository.WithChangeSource(ctx, repository.ChangeSourceRecalc), f); err != nil {
			t.Fatalf("update: %v", err)
		}
		revs := revisions(t, userID, f.ID)
		if len(revs) != 1 {
			t.Fatalf("revisions = %d, want 1", len(revs))
		}
		if revs[0].Source != repository.ChangeSourceRecalc {
			t.Errorf("source = %q, want recalc", revs[0].Source)
		}
	})

	t.Run("a write without change context is stored as unknown", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)

		f.TotalTime = 61
		f.PICTime = 61
		if err := flightRepo.Update(ctx, f); err != nil {
			t.Fatalf("update: %v", err)
		}
		revs := revisions(t, userID, f.ID)
		if len(revs) != 1 || revs[0].Source != "unknown" {
			t.Fatalf("revisions = %+v, want one unknown", revs)
		}
	})

	t.Run("delete keeps a void record with the reason", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)

		if err := flightRepo.Delete(userEdit("duplicate entry", false), f.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		revs := revisions(t, userID, f.ID)
		if len(revs) != 1 || revs[0].Kind != models.RevisionKindDelete {
			t.Fatalf("revisions = %+v, want one delete", revs)
		}
		if revs[0].Reason == nil || *revs[0].Reason != "duplicate entry" {
			t.Errorf("reason = %v", revs[0].Reason)
		}
		voided, err := logbookRepo.ListVoided(ctx, userID)
		if err != nil || len(voided) != 1 {
			t.Fatalf("voided = %d, err = %v", len(voided), err)
		}
	})

	t.Run("delete-all records one void record per flight", func(t *testing.T) {
		userID := newUser(t, true)
		a := newFlight(t, userID)
		b := newFlight(t, userID)

		n, err := flightRepo.DeleteAllByUserID(repository.WithChangeSource(ctx, repository.ChangeSourceUserEdit), userID)
		if err != nil || n != 2 {
			t.Fatalf("delete all = %d, err = %v", n, err)
		}
		for _, id := range []uuid.UUID{a.ID, b.ID} {
			if revs := revisions(t, userID, id); len(revs) != 1 || revs[0].Kind != models.RevisionKindDelete {
				t.Fatalf("revisions for %s = %+v", id, revs)
			}
		}
	})

	t.Run("revisions are append-only", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)
		f.TotalTime = 70
		f.PICTime = 70
		if err := flightRepo.Update(userEdit("fix", false), f); err != nil {
			t.Fatalf("update: %v", err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE flight_revisions SET reason = 'x' WHERE flight_id = $1`, f.ID); err == nil {
			t.Error("update of a revision succeeded")
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM flight_revisions WHERE flight_id = $1`, f.ID); err == nil {
			t.Error("delete of a revision succeeded")
		}
	})

	t.Run("aircraft re-registration records aircraft_rename", func(t *testing.T) {
		userID := newUser(t, true)
		f := newFlight(t, userID)
		aircraftRepo := postgres.NewAircraftRepository(db)
		ac := &models.Aircraft{UserID: userID, Registration: "D-EABC", Type: "C172", Make: "Cessna", Model: "172S", IsActive: true}
		if err := aircraftRepo.Create(ctx, ac); err != nil {
			t.Fatalf("create aircraft: %v", err)
		}
		ac.Registration = "D-EXYZ"
		if _, err := aircraftRepo.UpdateWithFlightRename(ctx, ac, "D-EABC"); err != nil {
			t.Fatalf("rename: %v", err)
		}
		revs := revisions(t, userID, f.ID)
		if len(revs) != 1 || revs[0].Source != repository.ChangeSourceAircraftRename {
			t.Fatalf("revisions = %+v, want one aircraft_rename", revs)
		}
	})
}

func strPtr(s string) *string { return &s }

// TestRevisionDiffFieldsMatchFlightSchema checks that every flights column a
// revision diff can report maps to a field of the Flight API schema.
func TestRevisionDiffFieldsMatchFlightSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	apiFields := map[string]bool{}
	ft := reflect.TypeOf(generated.Flight{})
	for i := 0; i < ft.NumField(); i++ {
		name := strings.Split(ft.Field(i).Tag.Get("json"), ",")[0]
		apiFields[name] = true
	}

	rows, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name = 'flights'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	excluded := map[string]bool{"id": true, "user_id": true, "created_at": true, "updated_at": true, "signature_id": true}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		if excluded[column] {
			continue
		}
		if field := service.CamelField(column); !apiFields[field] {
			t.Errorf("flights.%s maps to %q, which is not a Flight schema field", column, field)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
