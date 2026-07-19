package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// Mock flight session repository
type mockFlightSessionRepo struct {
	sessions map[uuid.UUID]*models.FlightSession
}

func newMockFlightSessionRepo() *mockFlightSessionRepo {
	return &mockFlightSessionRepo{sessions: make(map[uuid.UUID]*models.FlightSession)}
}

func (m *mockFlightSessionRepo) Create(ctx context.Context, s *models.FlightSession) error {
	for _, existing := range m.sessions {
		if existing.UserID == s.UserID && existing.Status == models.FlightSessionStatusOpen {
			return repository.ErrDuplicate
		}
	}
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	clone := *s
	m.sessions[s.ID] = &clone
	return nil
}

func (m *mockFlightSessionRepo) GetOpenByUserID(ctx context.Context, userID uuid.UUID) (*models.FlightSession, error) {
	for _, s := range m.sessions {
		if s.UserID == userID && s.Status == models.FlightSessionStatusOpen {
			clone := *s
			return &clone, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockFlightSessionRepo) Update(ctx context.Context, s *models.FlightSession) error {
	if _, exists := m.sessions[s.ID]; !exists {
		return repository.ErrNotFound
	}
	s.UpdatedAt = time.Now()
	clone := *s
	m.sessions[s.ID] = &clone
	return nil
}

// Minimal aircraft repository mock for session tests (the fuller
// mockAircraftRepo lives in the external service_test package).
type sessionAircraftRepo struct {
	aircraft []*models.Aircraft
}

func (m *sessionAircraftRepo) Create(ctx context.Context, a *models.Aircraft) error {
	a.ID = uuid.New()
	m.aircraft = append(m.aircraft, a)
	return nil
}

func (m *sessionAircraftRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Aircraft, error) {
	for _, a := range m.aircraft {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *sessionAircraftRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Aircraft, error) {
	var result []*models.Aircraft
	for _, a := range m.aircraft {
		if a.UserID == userID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *sessionAircraftRepo) Update(ctx context.Context, a *models.Aircraft) error { return nil }
func (m *sessionAircraftRepo) UpdateWithFlightRename(ctx context.Context, a *models.Aircraft, oldRegistration string) (int, error) {
	return 0, nil
}
func (m *sessionAircraftRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *sessionAircraftRepo) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	return len(m.aircraft), nil
}
func (m *sessionAircraftRepo) GetStatsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftStats, error) {
	return nil, nil
}
func (m *sessionAircraftRepo) GetTypeStatsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftTypeStats, error) {
	return nil, nil
}
func (m *sessionAircraftRepo) GetRecencyRowsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftRecencyRow, error) {
	return nil, nil
}

func newTestFlightSessionService(t *testing.T) (*FlightSessionService, *mockFlightSessionRepo, *mockFlightRepo, *sessionAircraftRepo) {
	t.Helper()
	sessionRepo := newMockFlightSessionRepo()
	flightRepo := newMockFlightRepo()
	aircraftRepo := &sessionAircraftRepo{}
	flightService := NewFlightService(flightRepo, &mockBaselineRepo{})
	svc := NewFlightSessionService(sessionRepo, aircraftRepo, flightService)
	svc.nearestAirport = func(lat, lon float64) *airports.AirportInfo { return nil }
	return svc, sessionRepo, flightRepo, aircraftRepo
}

func TestFlightSessionOffBlockOpensSession(t *testing.T) {
	svc, _, _, _ := newTestFlightSessionService(t)
	userID := uuid.New()

	session, created, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:        models.FlightSessionEventOffBlock,
		AircraftReg: strPtr("d-efgh"),
		ICAO:        strPtr("eddf"),
	})
	if err != nil {
		t.Fatalf("RecordEvent(offblock) failed: %v", err)
	}
	if !created {
		t.Error("expected a new session to be created")
	}
	if session.Status != models.FlightSessionStatusOpen {
		t.Errorf("status = %q, want open", session.Status)
	}
	if session.OffBlockAt == nil {
		t.Fatal("OffBlockAt not set")
	}
	if session.AircraftReg == nil || *session.AircraftReg != "D-EFGH" {
		t.Errorf("AircraftReg = %v, want D-EFGH (upper-cased)", session.AircraftReg)
	}
	if session.DepartureICAO == nil || *session.DepartureICAO != "EDDF" {
		t.Errorf("DepartureICAO = %v, want EDDF", session.DepartureICAO)
	}
}

func TestFlightSessionOffBlockIsIdempotent(t *testing.T) {
	svc, _, _, _ := newTestFlightSessionService(t)
	userID := uuid.New()

	first, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{Type: models.FlightSessionEventOffBlock})
	if err != nil {
		t.Fatalf("first offblock failed: %v", err)
	}
	second, created, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{Type: models.FlightSessionEventOffBlock})
	if err != nil {
		t.Fatalf("second offblock failed: %v", err)
	}
	if created {
		t.Error("duplicate offblock must not create a second session")
	}
	if second.ID != first.ID {
		t.Errorf("duplicate offblock returned different session: %s != %s", second.ID, first.ID)
	}
}

func TestFlightSessionEventWithoutOpenSession(t *testing.T) {
	svc, _, _, _ := newTestFlightSessionService(t)

	for _, eventType := range []string{
		models.FlightSessionEventTakeoff,
		models.FlightSessionEventLanding,
		models.FlightSessionEventOnBlock,
	} {
		_, _, err := svc.RecordEvent(context.Background(), uuid.New(), FlightSessionEventInput{Type: eventType})
		if !errors.Is(err, models.ErrNoOpenFlightSession) {
			t.Errorf("RecordEvent(%s) error = %v, want ErrNoOpenFlightSession", eventType, err)
		}
	}
}

func TestFlightSessionRejectsFutureTimestamps(t *testing.T) {
	svc, _, _, _ := newTestFlightSessionService(t)
	future := time.Now().Add(time.Hour)

	_, _, err := svc.RecordEvent(context.Background(), uuid.New(), FlightSessionEventInput{
		Type:       models.FlightSessionEventOffBlock,
		OccurredAt: &future,
	})
	if !errors.Is(err, models.ErrInvalidFlightSessionData) {
		t.Errorf("error = %v, want ErrInvalidFlightSessionData", err)
	}
}

func TestFlightSessionRejectsOutOfOrderEvents(t *testing.T) {
	svc, _, _, _ := newTestFlightSessionService(t)
	userID := uuid.New()

	offBlock := time.Now().Add(-time.Hour)
	beforeOffBlock := offBlock.Add(-10 * time.Minute)

	if _, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:       models.FlightSessionEventOffBlock,
		OccurredAt: &offBlock,
	}); err != nil {
		t.Fatalf("offblock failed: %v", err)
	}

	_, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:       models.FlightSessionEventTakeoff,
		OccurredAt: &beforeOffBlock,
	})
	if !errors.Is(err, models.ErrFlightSessionTimeOrder) {
		t.Errorf("error = %v, want ErrFlightSessionTimeOrder", err)
	}
}

func TestFlightSessionOnBlockRequiresRegistration(t *testing.T) {
	svc, sessionRepo, _, _ := newTestFlightSessionService(t)
	userID := uuid.New()

	offBlock := time.Now().Add(-time.Hour)
	if _, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:       models.FlightSessionEventOffBlock,
		OccurredAt: &offBlock,
	}); err != nil {
		t.Fatalf("offblock failed: %v", err)
	}

	_, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{Type: models.FlightSessionEventOnBlock})
	if !errors.Is(err, models.ErrFlightSessionMissingReg) {
		t.Fatalf("error = %v, want ErrFlightSessionMissingReg", err)
	}

	// The session must stay open so the client can retry with a registration.
	if _, err := sessionRepo.GetOpenByUserID(context.Background(), userID); err != nil {
		t.Errorf("session no longer open after failed completion: %v", err)
	}

	session, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:        models.FlightSessionEventOnBlock,
		AircraftReg: strPtr("D-EFGH"),
	})
	if err != nil {
		t.Fatalf("onblock retry with reg failed: %v", err)
	}
	if session.Status != models.FlightSessionStatusCompleted {
		t.Errorf("status = %q, want completed", session.Status)
	}
}

func TestFlightSessionFullFlowCreatesFlight(t *testing.T) {
	svc, _, flightRepo, aircraftRepo := newTestFlightSessionService(t)
	userID := uuid.New()

	if err := aircraftRepo.Create(context.Background(), &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "C172",
	}); err != nil {
		t.Fatalf("seeding aircraft failed: %v", err)
	}

	// 2h10m block time crossing midnight UTC, airborne 14 minutes after off block
	offBlock := time.Date(2026, 7, 4, 23, 5, 0, 0, time.UTC)
	takeoff := offBlock.Add(14 * time.Minute)
	landing := offBlock.Add(2 * time.Hour)
	onBlock := offBlock.Add(2*time.Hour + 10*time.Minute)

	steps := []FlightSessionEventInput{
		{Type: models.FlightSessionEventOffBlock, OccurredAt: &offBlock, AircraftReg: strPtr("D-EFGH"), ICAO: strPtr("EDDF")},
		{Type: models.FlightSessionEventTakeoff, OccurredAt: &takeoff},
		{Type: models.FlightSessionEventLanding, OccurredAt: &landing, ICAO: strPtr("EDDH")},
		{Type: models.FlightSessionEventOnBlock, OccurredAt: &onBlock},
	}

	var session *models.FlightSession
	var err error
	for _, step := range steps {
		session, _, err = svc.RecordEvent(context.Background(), userID, step)
		if err != nil {
			t.Fatalf("RecordEvent(%s) failed: %v", step.Type, err)
		}
	}

	if session.Status != models.FlightSessionStatusCompleted {
		t.Fatalf("status = %q, want completed", session.Status)
	}
	if session.FlightID == nil {
		t.Fatal("FlightID not set on completed session")
	}

	flight, err := flightRepo.GetByID(context.Background(), *session.FlightID)
	if err != nil {
		t.Fatalf("created flight not found: %v", err)
	}
	if got := flight.Date.Format("2006-01-02"); got != "2026-07-04" {
		t.Errorf("flight date = %s, want 2026-07-04 (UTC date of off block)", got)
	}
	if flight.TotalTime != 130 {
		t.Errorf("TotalTime = %d, want 130", flight.TotalTime)
	}
	if flight.AircraftType != "C172" {
		t.Errorf("AircraftType = %q, want C172 (resolved from user's aircraft)", flight.AircraftType)
	}
	if flight.OffBlockTime == nil || *flight.OffBlockTime != "23:05:00" {
		t.Errorf("OffBlockTime = %v, want 23:05:00", flight.OffBlockTime)
	}
	if flight.OnBlockTime == nil || *flight.OnBlockTime != "01:15:00" {
		t.Errorf("OnBlockTime = %v, want 01:15:00", flight.OnBlockTime)
	}
	if flight.DepartureTime == nil || *flight.DepartureTime != "23:19:00" {
		t.Errorf("DepartureTime = %v, want 23:19:00", flight.DepartureTime)
	}
	if flight.DepartureICAO == nil || *flight.DepartureICAO != "EDDF" {
		t.Errorf("DepartureICAO = %v, want EDDF", flight.DepartureICAO)
	}
	if flight.ArrivalICAO == nil || *flight.ArrivalICAO != "EDDH" {
		t.Errorf("ArrivalICAO = %v, want EDDH", flight.ArrivalICAO)
	}
	if !flight.IsPIC {
		t.Error("quick-logged flight should default to PIC")
	}
	if flight.AllLandings != 1 {
		t.Errorf("AllLandings = %d, want 1", flight.AllLandings)
	}

	// A new offblock after completion starts a fresh session.
	next, created, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{Type: models.FlightSessionEventOffBlock})
	if err != nil {
		t.Fatalf("offblock after completion failed: %v", err)
	}
	if !created || next.ID == session.ID {
		t.Error("offblock after completion should open a fresh session")
	}
}

func TestFlightSessionUnknownAircraftFallsBackToUnknownType(t *testing.T) {
	svc, _, flightRepo, _ := newTestFlightSessionService(t)
	userID := uuid.New()

	offBlock := time.Now().Add(-90 * time.Minute)
	if _, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:       models.FlightSessionEventOffBlock,
		OccurredAt: &offBlock,
	}); err != nil {
		t.Fatalf("offblock failed: %v", err)
	}
	session, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:        models.FlightSessionEventOnBlock,
		AircraftReg: strPtr("N12345"),
	})
	if err != nil {
		t.Fatalf("onblock failed: %v", err)
	}
	flight, err := flightRepo.GetByID(context.Background(), *session.FlightID)
	if err != nil {
		t.Fatalf("created flight not found: %v", err)
	}
	if flight.AircraftType != "UNKNOWN" {
		t.Errorf("AircraftType = %q, want UNKNOWN", flight.AircraftType)
	}
}

func TestFlightSessionRejectsOverlongSession(t *testing.T) {
	svc, _, _, _ := newTestFlightSessionService(t)
	userID := uuid.New()

	offBlock := time.Now().Add(-30 * time.Hour)
	if _, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:       models.FlightSessionEventOffBlock,
		OccurredAt: &offBlock,
	}); err != nil {
		t.Fatalf("offblock failed: %v", err)
	}
	_, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type:        models.FlightSessionEventOnBlock,
		AircraftReg: strPtr("D-EFGH"),
	})
	if !errors.Is(err, models.ErrFlightSessionTooLong) {
		t.Errorf("error = %v, want ErrFlightSessionTooLong", err)
	}
}

func TestFlightSessionGPSResolvesNearestAirport(t *testing.T) {
	svc, _, _, _ := newTestFlightSessionService(t)
	svc.nearestAirport = func(lat, lon float64) *airports.AirportInfo {
		if lat == 50.03 && lon == 8.57 {
			return &airports.AirportInfo{ICAO: "EDDF"}
		}
		return nil
	}
	userID := uuid.New()
	lat, lon := 50.03, 8.57

	session, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{
		Type: models.FlightSessionEventOffBlock,
		Lat:  &lat,
		Lon:  &lon,
	})
	if err != nil {
		t.Fatalf("offblock failed: %v", err)
	}
	if session.DepartureICAO == nil || *session.DepartureICAO != "EDDF" {
		t.Errorf("DepartureICAO = %v, want EDDF resolved from coordinates", session.DepartureICAO)
	}
}

func TestFlightSessionDiscard(t *testing.T) {
	svc, _, _, _ := newTestFlightSessionService(t)
	userID := uuid.New()

	if err := svc.Discard(context.Background(), userID); !errors.Is(err, models.ErrNoOpenFlightSession) {
		t.Errorf("Discard without session: error = %v, want ErrNoOpenFlightSession", err)
	}

	if _, _, err := svc.RecordEvent(context.Background(), userID, FlightSessionEventInput{Type: models.FlightSessionEventOffBlock}); err != nil {
		t.Fatalf("offblock failed: %v", err)
	}
	if err := svc.Discard(context.Background(), userID); err != nil {
		t.Fatalf("Discard failed: %v", err)
	}
	if _, err := svc.GetCurrent(context.Background(), userID); !errors.Is(err, models.ErrNoOpenFlightSession) {
		t.Errorf("session still open after discard: %v", err)
	}
}
