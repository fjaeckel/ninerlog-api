//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func newReviewUser(t *testing.T, ctx context.Context, userRepo repository.UserRepository, logbookRepo repository.LogbookRepository, strict bool) uuid.UUID {
	t.Helper()
	u := testutil.CreateTestUser("review-"+uuid.NewString()+"@example.com", "Review User", "hash")
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

func reviewFlight(userID uuid.UUID, date time.Time, createdAt time.Time) *models.Flight {
	dep, arr := "EDDF", "EDDM"
	return &models.Flight{
		UserID: userID, Date: date, AircraftReg: "D-EABC", AircraftType: "C172",
		DepartureICAO: &dep, ArrivalICAO: &arr, TotalTime: 60, PICTime: 60, IsPIC: true,
		AllLandings: 1, LandingsDay: 1, TakeoffsDay: 1, CreatedAt: createdAt,
	}
}

// TestContactRenameRevisionsIntegration covers the strict-mode handling of a
// contact rename against real Postgres.
func TestContactRenameRevisionsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	logbookRepo := postgres.NewLogbookRepository(db)
	contactRepo := postgres.NewContactRepository(db)

	contactSvc := service.NewContactService(contactRepo)

	setup := func(t *testing.T, strict bool) (uuid.UUID, *models.Contact, map[string]*models.Flight) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, strict)
		contact := &models.Contact{UserID: userID, Name: "Hans Müller"}
		if err := contactRepo.Create(ctx, contact); err != nil {
			t.Fatalf("create contact: %v", err)
		}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		crew := []models.FlightCrewMember{{ContactID: &contact.ID, Name: "Hans Müller", Role: models.CrewRole("Instructor")}}

		flights := map[string]*models.Flight{
			"fresh":  reviewFlight(userID, today, time.Time{}),
			"late":   reviewFlight(userID, today.AddDate(0, 0, -5), time.Now().Add(-5*24*time.Hour)),
			"signed": reviewFlight(userID, today, time.Time{}),
		}
		for name, f := range flights {
			if err := flightRepo.CreateWithCrew(ctx, f, crew); err != nil {
				t.Fatalf("create %s flight: %v", name, err)
			}
		}
		var sigID uuid.UUID
		if err := db.QueryRowContext(ctx,
			`INSERT INTO flight_signatures (flight_id, user_id, method, status) VALUES ($1, $2, 'live', 'completed') RETURNING id`,
			flights["signed"].ID, userID).Scan(&sigID); err != nil {
			t.Fatalf("create signature: %v", err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE flights SET signature_id = $1 WHERE id = $2`, sigID, flights["signed"].ID); err != nil {
			t.Fatalf("lock flight: %v", err)
		}
		return userID, contact, flights
	}

	revisions := func(t *testing.T, userID, flightID uuid.UUID) []models.FlightRevision {
		t.Helper()
		revs, err := logbookRepo.ListRevisions(ctx, userID, flightID)
		if err != nil {
			t.Fatalf("list revisions: %v", err)
		}
		return revs
	}

	t.Run("strict rename records a contact_rename revision per flight and skips signed ones", func(t *testing.T) {
		userID, contact, flights := setup(t, true)

		renamed, err := contactSvc.UpdateContact(ctx, &models.Contact{ID: contact.ID, Name: "Hans Mueller"}, userID)
		if err != nil {
			t.Fatalf("rename: %v", err)
		}
		if renamed != 2 {
			t.Fatalf("crew rows renamed = %d, want 2", renamed)
		}

		const wantReason = "Crew contact renamed: Hans Müller → Hans Mueller"
		for _, name := range []string{"fresh", "late"} {
			revs := revisions(t, userID, flights[name].ID)
			if len(revs) != 1 {
				t.Fatalf("%s: revisions = %d, want 1", name, len(revs))
			}
			r := revs[0]
			if r.IsCorrection {
				t.Errorf("%s: a contact rename is never a correction", name)
			}
			if r.Source != repository.ChangeSourceContactRename {
				t.Errorf("%s: source = %q", name, r.Source)
			}
			if r.Reason == nil || *r.Reason != wantReason {
				t.Errorf("%s: reason = %v", name, r.Reason)
			}
		}
		if revs := revisions(t, userID, flights["signed"].ID); len(revs) != 0 {
			t.Errorf("signed flight revisions = %d, want 0", len(revs))
		}

		var name string
		if err := db.QueryRowContext(ctx, `SELECT name FROM flight_crew_members WHERE flight_id = $1`, flights["signed"].ID).Scan(&name); err != nil || name != "Hans Müller" {
			t.Errorf("signed flight crew name = %q, err = %v", name, err)
		}
	})

	t.Run("non-strict rename records nothing", func(t *testing.T) {
		userID, contact, flights := setup(t, false)
		if _, err := contactSvc.UpdateContact(ctx, &models.Contact{ID: contact.ID, Name: "Hans Mueller"}, userID); err != nil {
			t.Fatalf("rename: %v", err)
		}
		for name, f := range flights {
			if revs := revisions(t, userID, f.ID); len(revs) != 0 {
				t.Errorf("%s: revisions = %d, want 0", name, len(revs))
			}
		}
	})
}

// TestCreateWithCrewIsAtomic checks that a failing crew row leaves no flight.
func TestCreateWithCrewIsAtomic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	logbookRepo := postgres.NewLogbookRepository(db)

	userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
	f := reviewFlight(userID, time.Now().UTC(), time.Time{})
	err := flightRepo.CreateWithCrew(ctx, f, []models.FlightCrewMember{
		{Name: "Pat", Role: models.CrewRole("Instructor")},
		{Name: "Broken", Role: models.CrewRole("NotARole")},
	})
	if err == nil {
		t.Fatal("CreateWithCrew() accepted an invalid crew role")
	}
	if !errors.Is(err, repository.ErrCrewWrite) {
		t.Fatalf("CreateWithCrew() error = %v, want ErrCrewWrite", err)
	}
	if _, err := flightRepo.GetByUserID(ctx, userID, nil); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM flights WHERE user_id = $1`, userID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("flights after a failed create = %d, err = %v", n, err)
	}
}

// TestCurrentFlightStatesIntegration covers the batched current-state read.
func TestCurrentFlightStatesIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	logbookRepo := postgres.NewLogbookRepository(db)

	userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
	otherID := newReviewUser(t, ctx, userRepo, logbookRepo, false)
	a := reviewFlight(userID, time.Now().UTC(), time.Time{})
	b := reviewFlight(userID, time.Now().UTC(), time.Time{})
	foreign := reviewFlight(otherID, time.Now().UTC(), time.Time{})
	crew := []models.FlightCrewMember{{Name: "Pat", Role: models.CrewRole("Instructor")}}
	for _, f := range []*models.Flight{a, b, foreign} {
		if err := flightRepo.CreateWithCrew(ctx, f, crew); err != nil {
			t.Fatal(err)
		}
	}

	states, err := logbookRepo.CurrentFlightStates(ctx, userID, []uuid.UUID{a.ID, b.ID, foreign.ID, uuid.New()})
	if err != nil {
		t.Fatalf("CurrentFlightStates() = %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("states = %d, want 2 (own flights that exist)", len(states))
	}
	single, singleCrew, err := logbookRepo.CurrentFlightState(ctx, userID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(states[a.ID].Snapshot) != string(single) || string(states[a.ID].Crew) != string(singleCrew) {
		t.Error("batched state differs from the single-flight state")
	}
}

// TestCrewKeyAgreesWithSQL checks that models.SameCrew and the SQL crew key
// used by the revision trigger agree on equality.
func TestCrewKeyAgreesWithSQL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	contactA, contactB := uuid.New(), uuid.New()
	member := func(name, role string, contact *uuid.UUID) models.FlightCrewMember {
		return models.FlightCrewMember{Name: name, Role: models.CrewRole(role), ContactID: contact}
	}
	tests := []struct {
		name string
		a, b []models.FlightCrewMember
		same bool
	}{
		{"both empty", nil, nil, true},
		{"same set, different order",
			[]models.FlightCrewMember{member("Pat", "Instructor", nil), member("Sam", "Student", nil), member("Alex", "Instructor", nil)},
			[]models.FlightCrewMember{member("Sam", "Student", nil), member("Alex", "Instructor", nil), member("Pat", "Instructor", nil)}, true},
		{"contact id differs",
			[]models.FlightCrewMember{member("Pat", "Instructor", &contactA)},
			[]models.FlightCrewMember{member("Pat", "Instructor", &contactB)}, true},
		{"contact id set on one side",
			[]models.FlightCrewMember{member("Pat", "Instructor", &contactA)},
			[]models.FlightCrewMember{member("Pat", "Instructor", nil)}, true},
		{"name differs",
			[]models.FlightCrewMember{member("Pat", "Instructor", nil)},
			[]models.FlightCrewMember{member("Pam", "Instructor", nil)}, false},
		{"role differs",
			[]models.FlightCrewMember{member("Pat", "Instructor", nil)},
			[]models.FlightCrewMember{member("Pat", "Student", nil)}, false},
		{"member missing",
			[]models.FlightCrewMember{member("Pat", "Instructor", nil), member("Sam", "Student", nil)},
			[]models.FlightCrewMember{member("Pat", "Instructor", nil)}, false},
		{"mixed case names are different",
			[]models.FlightCrewMember{member("pat", "Instructor", nil), member("Pat", "Instructor", nil)},
			[]models.FlightCrewMember{member("Pat", "Instructor", nil), member("pat", "Instructor", nil)}, true},
	}
	toJSON := func(members []models.FlightCrewMember) string {
		out := make([]models.RevisionCrew, 0, len(members))
		for _, m := range members {
			out = append(out, models.RevisionCrew{Name: m.Name, Role: string(m.Role), ContactID: m.ContactID})
		}
		b, _ := json.Marshal(out)
		return string(b)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sqlSame bool
			if err := db.QueryRowContext(ctx,
				`SELECT flight_revision_crew_key($1::jsonb) = flight_revision_crew_key($2::jsonb)`,
				toJSON(tt.a), toJSON(tt.b)).Scan(&sqlSame); err != nil {
				t.Fatal(err)
			}
			goSame := models.SameCrew(tt.a, tt.b)
			if goSame != tt.same || sqlSame != tt.same {
				t.Fatalf("Go = %v, SQL = %v, want %v", goSame, sqlSame, tt.same)
			}
		})
	}
}
