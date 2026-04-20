package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// Mock flight repository
type mockFlightRepo struct {
	flights map[uuid.UUID]*models.Flight
}

func newMockFlightRepo() *mockFlightRepo {
	return &mockFlightRepo{
		flights: make(map[uuid.UUID]*models.Flight),
	}
}

func (m *mockFlightRepo) Create(ctx context.Context, flight *models.Flight) error {
	flight.ID = uuid.New()
	flight.CreatedAt = time.Now()
	flight.UpdatedAt = time.Now()
	m.flights[flight.ID] = flight
	return nil
}

func (m *mockFlightRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Flight, error) {
	flight, exists := m.flights[id]
	if !exists {
		return nil, repository.ErrNotFound
	}
	return flight, nil
}

func (m *mockFlightRepo) GetByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) ([]*models.Flight, error) {
	var result []*models.Flight
	for _, flight := range m.flights {
		if flight.UserID == userID {
			result = append(result, flight)
		}
	}
	return result, nil
}

func (m *mockFlightRepo) Update(ctx context.Context, flight *models.Flight) error {
	if _, exists := m.flights[flight.ID]; !exists {
		return repository.ErrNotFound
	}
	flight.UpdatedAt = time.Now()
	m.flights[flight.ID] = flight
	return nil
}

func (m *mockFlightRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, exists := m.flights[id]; !exists {
		return repository.ErrNotFound
	}
	delete(m.flights, id)
	return nil
}

func (m *mockFlightRepo) DeleteAllByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	for id, f := range m.flights {
		if f.UserID == userID {
			delete(m.flights, id)
			count++
		}
	}
	return count, nil
}

func (m *mockFlightRepo) CountByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) (int, error) {
	count := 0
	for _, flight := range m.flights {
		if flight.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (m *mockFlightRepo) GetStatsByUserID(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time) (*models.FlightStatistics, error) {
	stats := &models.FlightStatistics{}
	for _, f := range m.flights {
		if f.UserID == userID {
			stats.TotalFlights++
			stats.TotalMinutes += f.TotalTime
			stats.PICMinutes += f.PICTime
			stats.DualMinutes += f.DualTime
			stats.NightMinutes += f.NightTime
			stats.IFRMinutes += f.IFRTime
			stats.LandingsDay += f.LandingsDay
			stats.LandingsNight += f.LandingsNight
		}
	}
	return stats, nil
}

func (m *mockFlightRepo) GetCurrencyData(ctx context.Context, userID uuid.UUID, since time.Time) (*models.CurrencyData, error) {
	data := &models.CurrencyData{}
	for _, f := range m.flights {
		if f.UserID == userID && !f.Date.Before(since) {
			data.Flights++
			data.DayLandings += f.LandingsDay
			data.NightLandings += f.LandingsNight
			data.TotalLandings += f.LandingsDay + f.LandingsNight
		}
	}
	return data, nil
}

func TestCreateFlight(t *testing.T) {
	flightRepo := newMockFlightRepo()
	service := NewFlightService(flightRepo)
	ctx := context.Background()

	userID := uuid.New()

	// Create a flight
	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		Date:         flightDate,
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    150,
		IsPIC:        true,
		PICTime:      150,
		LandingsDay:  3,
	}

	err := service.CreateFlight(ctx, flight)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if flight.ID == uuid.Nil {
		t.Error("Expected flight ID to be set")
	}
}

func TestCreateFlightInvalidTimeDistribution(t *testing.T) {
	flightRepo := newMockFlightRepo()
	service := NewFlightService(flightRepo)
	ctx := context.Background()

	userID := uuid.New()

	// Create invalid flight (isPic and isDual both true — mutually exclusive)
	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		Date:         flightDate,
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    2.0,
		IsPIC:        true,
		IsDual:       true, // Cannot be both PIC and Dual
		LandingsDay:  3,
	}

	err := service.CreateFlight(ctx, flight)
	if err != models.ErrInvalidTimeDistribution {
		t.Errorf("Expected ErrInvalidTimeDistribution, got %v", err)
	}
}

func TestUpdateFlight(t *testing.T) {
	flightRepo := newMockFlightRepo()
	service := NewFlightService(flightRepo)
	ctx := context.Background()

	userID := uuid.New()

	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		Date:         flightDate,
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    150,
		IsPIC:        true,
		PICTime:      150,
		LandingsDay:  3,
	}

	_ = service.CreateFlight(ctx, flight)

	// Update flight
	flight.TotalTime = 180
	flight.PICTime = 180
	flight.NightTime = 30

	err := service.UpdateFlight(ctx, flight, userID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify update
	updated, _ := service.GetFlight(ctx, flight.ID, userID)
	if updated.TotalTime != 180 {
		t.Errorf("Expected total time 180, got %d", updated.TotalTime)
	}
}

func TestDeleteFlight(t *testing.T) {
	flightRepo := newMockFlightRepo()
	service := NewFlightService(flightRepo)
	ctx := context.Background()

	userID := uuid.New()

	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		Date:         flightDate,
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    150,
		IsPIC:        true,
		PICTime:      150,
		LandingsDay:  3,
	}

	_ = service.CreateFlight(ctx, flight)

	err := service.DeleteFlight(ctx, flight.ID, userID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify deletion
	_, err = service.GetFlight(ctx, flight.ID, userID)
	if err != ErrFlightNotFound {
		t.Errorf("Expected ErrFlightNotFound, got %v", err)
	}
}

func TestGetFlight_Success(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	flight := &models.Flight{
		UserID:       userID,
		Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    90,
		IsPIC:        true,
		PICTime:      90,
		LandingsDay:  1,
	}
	_ = svc.CreateFlight(ctx, flight)

	got, err := svc.GetFlight(ctx, flight.ID, userID)
	if err != nil {
		t.Fatalf("GetFlight() error = %v", err)
	}
	if got.ID != flight.ID {
		t.Errorf("GetFlight() ID = %v, want %v", got.ID, flight.ID)
	}
}

func TestGetFlight_NotFound(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()

	_, err := svc.GetFlight(ctx, uuid.New(), uuid.New())
	if err != ErrFlightNotFound {
		t.Errorf("GetFlight(nonexistent) error = %v, want ErrFlightNotFound", err)
	}
}

func TestGetFlight_WrongUser(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	ownerID := uuid.New()
	otherID := uuid.New()

	flight := &models.Flight{
		UserID:       ownerID,
		Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    90,
		IsPIC:        true,
		PICTime:      90,
		LandingsDay:  1,
	}
	_ = svc.CreateFlight(ctx, flight)

	_, err := svc.GetFlight(ctx, flight.ID, otherID)
	if err != ErrUnauthorizedFlight {
		t.Errorf("GetFlight(wrong user) error = %v, want ErrUnauthorizedFlight", err)
	}
}

func TestListFlights(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	// Create two flights
	for i := 0; i < 2; i++ {
		flight := &models.Flight{
			UserID:       userID,
			Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
			AircraftReg:  "D-EFGH",
			AircraftType: "C172",
			TotalTime:    60,
			IsPIC:        true,
			PICTime:      60,
			LandingsDay:  1,
		}
		_ = svc.CreateFlight(ctx, flight)
	}

	flights, err := svc.ListFlights(ctx, userID, nil)
	if err != nil {
		t.Fatalf("ListFlights() error = %v", err)
	}
	if len(flights) != 2 {
		t.Errorf("ListFlights() count = %d, want 2", len(flights))
	}
}

func TestListFlights_EmptyForOtherUser(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()
	otherID := uuid.New()

	flight := &models.Flight{
		UserID:       userID,
		Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    60,
		IsPIC:        true,
		PICTime:      60,
		LandingsDay:  1,
	}
	_ = svc.CreateFlight(ctx, flight)

	flights, err := svc.ListFlights(ctx, otherID, nil)
	if err != nil {
		t.Fatalf("ListFlights() error = %v", err)
	}
	if len(flights) != 0 {
		t.Errorf("ListFlights(otherUser) count = %d, want 0", len(flights))
	}
}

func TestUpdateFlight_NotFound(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()

	flight := &models.Flight{
		ID:           uuid.New(),
		UserID:       uuid.New(),
		Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    60,
		IsPIC:        true,
		PICTime:      60,
		LandingsDay:  1,
	}

	err := svc.UpdateFlight(ctx, flight, flight.UserID)
	if err != ErrFlightNotFound {
		t.Errorf("UpdateFlight(nonexistent) error = %v, want ErrFlightNotFound", err)
	}
}

func TestUpdateFlight_WrongUser(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	ownerID := uuid.New()
	otherID := uuid.New()

	flight := &models.Flight{
		UserID:       ownerID,
		Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    90,
		IsPIC:        true,
		PICTime:      90,
		LandingsDay:  1,
	}
	_ = svc.CreateFlight(ctx, flight)

	flight.TotalTime = 120
	flight.PICTime = 120
	err := svc.UpdateFlight(ctx, flight, otherID)
	if err != ErrUnauthorizedFlight {
		t.Errorf("UpdateFlight(wrong user) error = %v, want ErrUnauthorizedFlight", err)
	}
}

func TestDeleteFlight_NotFound(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()

	err := svc.DeleteFlight(ctx, uuid.New(), uuid.New())
	if err != ErrFlightNotFound {
		t.Errorf("DeleteFlight(nonexistent) error = %v, want ErrFlightNotFound", err)
	}
}

func TestDeleteFlight_WrongUser(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	ownerID := uuid.New()
	otherID := uuid.New()

	flight := &models.Flight{
		UserID:       ownerID,
		Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    90,
		IsPIC:        true,
		PICTime:      90,
		LandingsDay:  1,
	}
	_ = svc.CreateFlight(ctx, flight)

	err := svc.DeleteFlight(ctx, flight.ID, otherID)
	if err != ErrUnauthorizedFlight {
		t.Errorf("DeleteFlight(wrong user) error = %v, want ErrUnauthorizedFlight", err)
	}
}

func TestDeleteAllFlights(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	for i := 0; i < 3; i++ {
		flight := &models.Flight{
			UserID:       userID,
			Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
			AircraftReg:  "D-EFGH",
			AircraftType: "C172",
			TotalTime:    60,
			IsPIC:        true,
			PICTime:      60,
			LandingsDay:  1,
		}
		_ = svc.CreateFlight(ctx, flight)
	}

	count, err := svc.DeleteAllFlights(ctx, userID)
	if err != nil {
		t.Fatalf("DeleteAllFlights() error = %v", err)
	}
	if count != 3 {
		t.Errorf("DeleteAllFlights() count = %d, want 3", count)
	}

	flights, _ := svc.ListFlights(ctx, userID, nil)
	if len(flights) != 0 {
		t.Errorf("After DeleteAllFlights, remaining flights = %d, want 0", len(flights))
	}
}

func TestCountFlights(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	for i := 0; i < 5; i++ {
		flight := &models.Flight{
			UserID:       userID,
			Date:         time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
			AircraftReg:  "D-EFGH",
			AircraftType: "C172",
			TotalTime:    60,
			IsPIC:        true,
			PICTime:      60,
			LandingsDay:  1,
		}
		_ = svc.CreateFlight(ctx, flight)
	}

	count, err := svc.CountFlights(ctx, userID, nil)
	if err != nil {
		t.Fatalf("CountFlights() error = %v", err)
	}
	if count != 5 {
		t.Errorf("CountFlights() = %d, want 5", count)
	}
}

func TestGetStatsByUserID(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	flight := &models.Flight{
		UserID:        userID,
		Date:          time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
		AircraftReg:   "D-EFGH",
		AircraftType:  "C172",
		TotalTime:     120,
		IsPIC:         true,
		PICTime:       120,
		NightTime:     30,
		LandingsDay:   2,
		LandingsNight: 1,
	}
	_ = svc.CreateFlight(ctx, flight)

	stats, err := svc.GetStatsByUserID(ctx, userID, nil, nil)
	if err != nil {
		t.Fatalf("GetStatsByUserID() error = %v", err)
	}
	if stats.TotalFlights != 1 {
		t.Errorf("TotalFlights = %d, want 1", stats.TotalFlights)
	}
	if stats.TotalMinutes != 120 {
		t.Errorf("TotalMinutes = %d, want 120", stats.TotalMinutes)
	}
	if stats.PICMinutes != 120 {
		t.Errorf("PICMinutes = %d, want 120", stats.PICMinutes)
	}
}

func TestGetCurrency_Current(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	// Create flights with enough recent landings
	for i := 0; i < 3; i++ {
		flight := &models.Flight{
			UserID:        userID,
			Date:          time.Now().AddDate(0, 0, -10), // recent
			AircraftReg:   "D-EFGH",
			AircraftType:  "C172",
			TotalTime:     60,
			IsPIC:         true,
			PICTime:       60,
			LandingsDay:   1,
			LandingsNight: 1,
		}
		_ = svc.CreateFlight(ctx, flight)
	}

	currency, err := svc.GetCurrency(ctx, userID)
	if err != nil {
		t.Fatalf("GetCurrency() error = %v", err)
	}
	if !currency.IsCurrent {
		t.Error("Expected IsCurrent=true with 3 day landings")
	}
	if !currency.DaysCurrent {
		t.Error("Expected DaysCurrent=true")
	}
	if !currency.NightsCurrent {
		t.Error("Expected NightsCurrent=true")
	}
}

func TestGetCurrency_NotCurrent(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	// Only 1 landing - not enough for currency
	flight := &models.Flight{
		UserID:       userID,
		Date:         time.Now().AddDate(0, 0, -10),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    60,
		IsPIC:        true,
		PICTime:      60,
		LandingsDay:  1,
	}
	_ = svc.CreateFlight(ctx, flight)

	currency, err := svc.GetCurrency(ctx, userID)
	if err != nil {
		t.Fatalf("GetCurrency() error = %v", err)
	}
	if currency.IsCurrent {
		t.Error("Expected IsCurrent=false with only 1 day landing")
	}
	if currency.DayLandings != 1 {
		t.Errorf("DayLandings = %d, want 1", currency.DayLandings)
	}
}

func TestCreateFlight_InvalidData(t *testing.T) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()

	// Empty flight should fail validation
	flight := &models.Flight{
		UserID: uuid.New(),
	}
	err := svc.CreateFlight(ctx, flight)
	if err == nil {
		t.Error("CreateFlight with empty data should return error")
	}
}
