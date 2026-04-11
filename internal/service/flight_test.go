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
