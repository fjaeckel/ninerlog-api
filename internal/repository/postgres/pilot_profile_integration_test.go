//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
)

func TestPilotProfileRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ctx := context.Background()

	user := testutil.CreateTestUser("pilot-profile@example.com", "Lena", "hashedpass")
	if err := postgres.NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	repo := postgres.NewPilotProfileRepository(db)

	if _, err := repo.Get(ctx, user.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get before upsert err = %v, want ErrNotFound", err)
	}

	ack := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	p := &models.PilotProfile{UserID: user.ID, Mode: models.ModeEverything, Disciplines: map[models.Discipline]models.DisciplineSetting{
		models.DisciplineSailplane: {Intent: models.IntentOn, AcknowledgedAt: &ack},
	}}
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := repo.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	s := got.Disciplines[models.DisciplineSailplane]
	if got.Mode != models.ModeEverything || s.Intent != models.IntentOn || s.AcknowledgedAt == nil || !s.AcknowledgedAt.Equal(ack) {
		t.Errorf("round trip = %+v", got)
	}

	if _, err := db.ExecContext(ctx, `UPDATE pilot_profiles SET disciplines = disciplines || '{"BALLOON":{"intent":"on"}}' WHERE user_id = $1`, user.ID); err != nil {
		t.Fatalf("inject unknown key: %v", err)
	}
	got, err = repo.Get(ctx, user.ID)
	if err != nil || len(got.Disciplines) != 1 {
		t.Errorf("unknown discipline key must be skipped on read: %+v, %v", got, err)
	}

	p.Mode = "sometimes"
	if err := repo.Upsert(ctx, p); err == nil {
		t.Error("CHECK constraint must reject an unknown mode")
	}
}

func TestDisciplineEvidenceSourceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ctx := context.Background()

	user := testutil.CreateTestUser("discipline-evidence@example.com", "Mehmet", "hashedpass")
	if err := postgres.NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	other := testutil.CreateTestUser("discipline-evidence-other@example.com", "Other", "hashedpass")
	if err := postgres.NewUserRepository(db).Create(ctx, other); err != nil {
		t.Fatalf("create user: %v", err)
	}

	aircraftRepo := postgres.NewAircraftRepository(db)
	glider, ul := "GLIDER", "ULTRALIGHT"
	threeAxis := models.ULKindThreeAxis
	for _, a := range []*models.Aircraft{
		{UserID: user.ID, Registration: "D-1234", Type: "ASK21", Make: "Schleicher", Model: "ASK 21", AircraftClass: &glider, IsActive: true},
		{UserID: user.ID, Registration: "D-MXYZ", Type: "C42", Make: "Comco Ikarus", Model: "C42", AircraftClass: &ul, ULKind: &threeAxis, IsActive: true},
	} {
		if err := aircraftRepo.Create(ctx, a); err != nil {
			t.Fatalf("create aircraft: %v", err)
		}
	}

	flightRepo := postgres.NewFlightRepository(db)
	winch, selfLaunch := "winch", "self-launch"
	d1 := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	flights := []*models.Flight{
		{UserID: user.ID, Date: d1, AircraftReg: "D-1234", AircraftType: "ASK21", TotalTime: 8, DualTime: 8, LaunchMethod: &winch, LandingsDay: 1},
		{UserID: user.ID, Date: d2, AircraftReg: "D-1234", AircraftType: "ASK21", TotalTime: 8, PICTime: 8, LaunchMethod: &winch, LandingsDay: 1},
		{UserID: user.ID, Date: d1, AircraftReg: "D-MXYZ", AircraftType: "C42", TotalTime: 60, PICTime: 60, LaunchMethod: &selfLaunch, LandingsDay: 1, IFRTime: 10},
		{UserID: user.ID, Date: d2, AircraftReg: "D-MXYZ", AircraftType: "C42", TotalTime: 60, IsPassenger: true},
		{UserID: user.ID, Date: d2, AircraftReg: "SIM", AircraftType: "FNPT II", TotalTime: 60, IsSimulator: true, SimulatedFlightTime: 60},
		{UserID: user.ID, Date: d2, AircraftReg: "D-EXXX", AircraftType: "C172", TotalTime: 60, PICTime: 60, SICTime: 0, DualGivenTime: 60, LandingsDay: 1},
		{UserID: other.ID, Date: d1, AircraftReg: "D-1234", AircraftType: "ASK21", TotalTime: 8, PICTime: 8, LaunchMethod: &winch, LandingsDay: 1},
	}
	for _, f := range flights {
		if err := flightRepo.Create(ctx, f); err != nil {
			t.Fatalf("create flight: %v", err)
		}
	}

	groups, err := postgres.NewDisciplineEvidenceSource(db).GetDisciplineFlightGroups(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetDisciplineFlightGroups: %v", err)
	}
	byClass := map[string]models.DisciplineFlightGroup{}
	for _, g := range groups {
		byClass[g.AircraftClass] = g
	}
	if len(byClass) != 3 {
		t.Fatalf("groups = %+v", groups)
	}

	g := byClass["GLIDER"]
	if g.TowedFlights != 2 || g.TowedDualReceivedFlights != 1 || g.Flights != 0 || g.LastTowed == nil || !g.LastTowed.Equal(d1) {
		t.Errorf("glider group = %+v", g)
	}
	u := byClass["ULTRALIGHT"]
	if u.ULKind == nil || *u.ULKind != models.ULKindThreeAxis || u.Flights != 1 || u.IFRFlights != 1 ||
		u.PassengerFlights != 1 || u.TowedFlights != 0 {
		t.Errorf("UL group = %+v", u)
	}
	n := byClass[""]
	if n.SimulatorSessions != 1 || n.Flights != 1 || n.InstructingFlights != 1 || n.DualGivenMinutes != 60 {
		t.Errorf("unclassed group = %+v", n)
	}
}
