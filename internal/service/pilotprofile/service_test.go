package pilotprofile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type memProfiles struct {
	rows    map[uuid.UUID]*models.PilotProfile
	upserts int
}

func (m *memProfiles) Get(_ context.Context, userID uuid.UUID) (*models.PilotProfile, error) {
	p, ok := m.rows[userID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *p
	cp.Disciplines = map[models.Discipline]models.DisciplineSetting{}
	for d, s := range p.Disciplines {
		cp.Disciplines[d] = s
	}
	return &cp, nil
}

func (m *memProfiles) Upsert(_ context.Context, p *models.PilotProfile) error {
	m.upserts++
	m.rows[p.UserID] = p
	return nil
}

type stubEvidence struct {
	groups map[uuid.UUID][]models.DisciplineFlightGroup
}

func (s stubEvidence) GetDisciplineFlightGroups(_ context.Context, userID uuid.UUID) ([]models.DisciplineFlightGroup, error) {
	return s.groups[userID], nil
}

type stubLicences struct {
	repository.LicenseRepository
	byUser map[uuid.UUID][]*models.License
}

func (s stubLicences) GetByUserID(_ context.Context, userID uuid.UUID, _ *time.Time) ([]*models.License, error) {
	return s.byUser[userID], nil
}

type stubRatings struct {
	byLicence map[uuid.UUID][]*models.ClassRating
}

func (s stubRatings) GetByLicenseID(_ context.Context, id uuid.UUID) ([]*models.ClassRating, error) {
	return s.byLicence[id], nil
}

type stubAircraft struct {
	repository.AircraftRepository
	byUser map[uuid.UUID][]*models.Aircraft
}

func (s stubAircraft) GetByUserID(_ context.Context, userID uuid.UUID, _ *time.Time) ([]*models.Aircraft, error) {
	return s.byUser[userID], nil
}

func newTestService(userID uuid.UUID) (*Service, *memProfiles) {
	spl := &models.License{ID: uuid.New(), UserID: userID, LicenseType: "SPL", RegulatoryAuthority: "LBA", LicenseNumber: "12345"}
	profiles := &memProfiles{rows: map[uuid.UUID]*models.PilotProfile{}}
	svc := NewService(profiles,
		stubEvidence{groups: map[uuid.UUID][]models.DisciplineFlightGroup{}},
		stubLicences{byUser: map[uuid.UUID][]*models.License{userID: {spl}}},
		stubRatings{byLicence: map[uuid.UUID][]*models.ClassRating{}},
		stubAircraft{byUser: map[uuid.UUID][]*models.Aircraft{}},
	)
	svc.SetClock(func() time.Time { return testNow })
	return svc, profiles
}

func statusOf(p *models.DerivedPilotProfile, d models.Discipline) models.DisciplineStatus {
	return stateOf(p.Disciplines, d).Status
}

func TestService_GetDefaultsWithoutCreatingRow(t *testing.T) {
	userID := uuid.New()
	svc, profiles := newTestService(userID)

	p, err := svc.Get(context.Background(), userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Mode != models.ModeAdaptive || len(p.Disciplines) != 10 {
		t.Errorf("profile = %+v", p)
	}
	if statusOf(p, models.DisciplineSailplane) != models.StatusActive {
		t.Errorf("SAILPLANE = %s, want active", statusOf(p, models.DisciplineSailplane))
	}
	if len(p.PendingAcknowledgement) != 1 || p.PendingAcknowledgement[0] != models.DisciplineSailplane {
		t.Errorf("pending = %v", p.PendingAcknowledgement)
	}
	if profiles.upserts != 0 {
		t.Errorf("GET stored a row")
	}
}

func TestService_Update(t *testing.T) {
	sailplane, ifr := models.DisciplineSailplane, models.DisciplineIFR
	everything := models.ModeEverything
	bogusMode := models.PilotProfileMode("sometimes")

	tests := []struct {
		name    string
		updates []Update
		wantErr bool
		check   func(t *testing.T, p *models.DerivedPilotProfile, stored *models.PilotProfile)
	}{
		{name: "acknowledge clears pending", updates: []Update{{Acknowledge: []models.Discipline{sailplane}}},
			check: func(t *testing.T, p *models.DerivedPilotProfile, stored *models.PilotProfile) {
				if len(p.PendingAcknowledgement) != 0 {
					t.Errorf("pending = %v", p.PendingAcknowledgement)
				}
				if s := stateOf(p.Disciplines, sailplane); s.AcknowledgedAt == nil || !s.AcknowledgedAt.Equal(testNow) {
					t.Errorf("acknowledgedAt = %v", s.AcknowledgedAt)
				}
			}},
		{name: "explicit intent clears pending", updates: []Update{{Intents: map[models.Discipline]models.DisciplineIntent{sailplane: models.IntentOn}}},
			check: func(t *testing.T, p *models.DerivedPilotProfile, _ *models.PilotProfile) {
				if len(p.PendingAcknowledgement) != 0 {
					t.Errorf("pending = %v", p.PendingAcknowledgement)
				}
			}},
		{name: "off beats licence", updates: []Update{{Intents: map[models.Discipline]models.DisciplineIntent{sailplane: models.IntentOff}}},
			check: func(t *testing.T, p *models.DerivedPilotProfile, _ *models.PilotProfile) {
				if statusOf(p, sailplane) != models.StatusOff {
					t.Errorf("SAILPLANE = %s", statusOf(p, sailplane))
				}
			}},
		{name: "partial merge keeps other intents", updates: []Update{
			{Intents: map[models.Discipline]models.DisciplineIntent{ifr: models.IntentGoal}},
			{Mode: &everything},
			{Intents: map[models.Discipline]models.DisciplineIntent{sailplane: models.IntentOff}},
		},
			check: func(t *testing.T, p *models.DerivedPilotProfile, stored *models.PilotProfile) {
				if p.Mode != models.ModeEverything || statusOf(p, ifr) != models.StatusTraining || statusOf(p, sailplane) != models.StatusOff {
					t.Errorf("profile = %+v", p)
				}
				if len(stored.Disciplines) != 2 {
					t.Errorf("stored = %+v", stored.Disciplines)
				}
			}},
		{name: "auto intent compacts the stored entry", updates: []Update{
			{Intents: map[models.Discipline]models.DisciplineIntent{ifr: models.IntentGoal}},
			{Intents: map[models.Discipline]models.DisciplineIntent{ifr: models.IntentAuto}},
		},
			check: func(t *testing.T, _ *models.DerivedPilotProfile, stored *models.PilotProfile) {
				if len(stored.Disciplines) != 0 {
					t.Errorf("stored = %+v", stored.Disciplines)
				}
			}},
		{name: "acknowledge twice keeps first time", updates: []Update{
			{Acknowledge: []models.Discipline{sailplane}},
			{Acknowledge: []models.Discipline{sailplane}},
		},
			check: func(t *testing.T, p *models.DerivedPilotProfile, _ *models.PilotProfile) {
				if s := stateOf(p.Disciplines, sailplane); s.AcknowledgedAt == nil || !s.AcknowledgedAt.Equal(testNow) {
					t.Errorf("acknowledgedAt = %v", s.AcknowledgedAt)
				}
			}},
		{name: "unknown discipline intent rejected", updates: []Update{{Intents: map[models.Discipline]models.DisciplineIntent{"BALLOON": models.IntentOn}}}, wantErr: true},
		{name: "unknown intent rejected", updates: []Update{{Intents: map[models.Discipline]models.DisciplineIntent{sailplane: "maybe"}}}, wantErr: true},
		{name: "unknown acknowledge rejected", updates: []Update{{Acknowledge: []models.Discipline{"BALLOON"}}}, wantErr: true},
		{name: "unknown mode rejected", updates: []Update{{Mode: &bogusMode}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			svc, profiles := newTestService(userID)
			var p *models.DerivedPilotProfile
			var err error
			for _, u := range tt.updates {
				if p, err = svc.Update(context.Background(), userID, u); err != nil {
					break
				}
			}
			if tt.wantErr {
				if !errors.Is(err, models.ErrInvalidPilotProfile) {
					t.Fatalf("err = %v, want ErrInvalidPilotProfile", err)
				}
				if profiles.upserts != 0 {
					t.Errorf("invalid update stored a row")
				}
				return
			}
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			tt.check(t, p, profiles.rows[userID])
		})
	}
}

func TestService_UsersAreIndependent(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	svc, _ := newTestService(a)
	if _, err := svc.Update(context.Background(), a, Update{Intents: map[models.Discipline]models.DisciplineIntent{models.DisciplineIFR: models.IntentOn}}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	p, err := svc.Get(context.Background(), b)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if statusOf(p, models.DisciplineIFR) != models.StatusOff || statusOf(p, models.DisciplineSailplane) != models.StatusOff {
		t.Errorf("user B sees user A's profile: %+v", p.Disciplines)
	}
}

func TestService_ReplaceAndSettings(t *testing.T) {
	userID := uuid.New()
	svc, _ := newTestService(userID)
	ctx := context.Background()

	if s, err := svc.Settings(ctx, userID); err != nil || s != nil {
		t.Fatalf("Settings before any write = %v, %v", s, err)
	}
	err := svc.Replace(ctx, userID, &models.PilotProfile{Mode: models.ModeEverything, Disciplines: map[models.Discipline]models.DisciplineSetting{
		models.DisciplineSailplane: {Intent: models.IntentGoal},
		models.DisciplineIFR:       {Intent: models.IntentAuto},
	}})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	s, err := svc.Settings(ctx, userID)
	if err != nil || s == nil || s.Mode != models.ModeEverything || len(s.Disciplines) != 1 {
		t.Fatalf("Settings = %+v, %v", s, err)
	}
	if err := svc.Replace(ctx, userID, &models.PilotProfile{Mode: "sometimes"}); !errors.Is(err, models.ErrInvalidPilotProfile) {
		t.Errorf("Replace(bad mode) err = %v", err)
	}
	if err := svc.Replace(ctx, userID, &models.PilotProfile{Mode: models.ModeAdaptive, Disciplines: map[models.Discipline]models.DisciplineSetting{
		models.DisciplineIFR: {Intent: "maybe"},
	}}); !errors.Is(err, models.ErrInvalidPilotProfile) {
		t.Errorf("Replace(bad intent) err = %v", err)
	}
}

type stubPrivileges struct {
	list []*models.LicencePrivilege
	err  error
}

func (s stubPrivileges) ListByUser(context.Context, uuid.UUID) ([]*models.LicencePrivilege, error) {
	return s.list, s.err
}

func TestService_PrivilegeSource(t *testing.T) {
	tests := []struct {
		name    string
		src     *stubPrivileges
		want    models.DisciplineStatus
		wantErr bool
	}{
		{name: "no source wired", src: nil, want: models.StatusOff},
		{name: "P4 FI(S) privilege activates INSTRUCTOR", src: &stubPrivileges{list: []*models.LicencePrivilege{{ID: uuid.New(), Kind: models.PrivilegeFIS}}}, want: models.StatusActive},
		{name: "read failure is an error", src: &stubPrivileges{err: errors.New("boom")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			svc, _ := newTestService(userID)
			if tt.src != nil {
				svc.SetPrivilegeSource(*tt.src)
			}
			p, err := svc.Get(context.Background(), userID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := statusOf(p, models.DisciplineInstructor); got != tt.want {
				t.Errorf("INSTRUCTOR = %s, want %s", got, tt.want)
			}
		})
	}
}
