package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
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

func (m *mockFlightRepo) GetByLicenseID(ctx context.Context, licenseID uuid.UUID, opts *repository.FlightQueryOptions) ([]*models.Flight, error) {
	var result []*models.Flight
	for _, flight := range m.flights {
		if flight.LicenseID == licenseID {
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

func (m *mockFlightRepo) CountByUserID(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) (int, error) {
	count := 0
	for _, flight := range m.flights {
		if flight.UserID == userID {
			count++
		}
	}
	return count, nil
}

func TestCreateFlight(t *testing.T) {
	flightRepo := newMockFlightRepo()
	licenseRepo := newMockLicenseRepo()
	service := NewFlightService(flightRepo, licenseRepo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	// Create a license first
	license := &models.License{
		UserID:           userID,
		LicenseType:      models.LicenseTypeEASAPPL,
		LicenseNumber:    "PPL-123456",
		IssueDate:        issueDate,
		IssuingAuthority: "EASA",
		IsActive:         true,
	}
	_ = licenseRepo.Create(ctx, license)

	// Create a flight
	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		LicenseID:    license.ID,
		Date:         flightDate,
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    2.5,
		PICTime:      2.5,
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
	licenseRepo := newMockLicenseRepo()
	service := NewFlightService(flightRepo, licenseRepo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	license := &models.License{
		UserID:           userID,
		LicenseType:      models.LicenseTypeEASAPPL,
		LicenseNumber:    "PPL-123456",
		IssueDate:        issueDate,
		IssuingAuthority: "EASA",
		IsActive:         true,
	}
	_ = licenseRepo.Create(ctx, license)

	// Create invalid flight (times exceed total)
	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		LicenseID:    license.ID,
		Date:         flightDate,
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    2.0,
		PICTime:      2.5, // Exceeds total
		LandingsDay:  3,
	}

	err := service.CreateFlight(ctx, flight)
	if err != models.ErrInvalidTimeDistribution {
		t.Errorf("Expected ErrInvalidTimeDistribution, got %v", err)
	}
}

func TestCreateFlightSPLNoNight(t *testing.T) {
	flightRepo := newMockFlightRepo()
	licenseRepo := newMockLicenseRepo()
	service := NewFlightService(flightRepo, licenseRepo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	// Create SPL license
	license := &models.License{
		UserID:           userID,
		LicenseType:      models.LicenseTypeEASASPL,
		LicenseNumber:    "SPL-123456",
		IssueDate:        issueDate,
		IssuingAuthority: "EASA",
		IsActive:         true,
	}
	_ = licenseRepo.Create(ctx, license)

	// Try to create flight with night time
	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		LicenseID:    license.ID,
		Date:         flightDate,
		AircraftReg:  "D-KXYZ",
		AircraftType: "ASK21",
		TotalTime:    1.5,
		PICTime:      1.5,
		NightTime:    0.5, // SPL shouldn't have night time
		LandingsDay:  2,
	}

	err := service.CreateFlight(ctx, flight)
	if err == nil {
		t.Error("Expected error for SPL with night time")
	}
}

func TestUpdateFlight(t *testing.T) {
	flightRepo := newMockFlightRepo()
	licenseRepo := newMockLicenseRepo()
	service := NewFlightService(flightRepo, licenseRepo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	license := &models.License{
		UserID:           userID,
		LicenseType:      models.LicenseTypeEASAPPL,
		LicenseNumber:    "PPL-123456",
		IssueDate:        issueDate,
		IssuingAuthority: "EASA",
		IsActive:         true,
	}
	_ = licenseRepo.Create(ctx, license)

	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		LicenseID:    license.ID,
		Date:         flightDate,
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    2.5,
		PICTime:      2.5,
		LandingsDay:  3,
	}

	_ = service.CreateFlight(ctx, flight)

	// Update flight
	flight.TotalTime = 3.0
	flight.PICTime = 3.0
	flight.NightTime = 0.5

	err := service.UpdateFlight(ctx, flight, userID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify update
	updated, _ := service.GetFlight(ctx, flight.ID, userID)
	if updated.TotalTime != 3.0 {
		t.Errorf("Expected total time 3.0, got %f", updated.TotalTime)
	}
}

func TestDeleteFlight(t *testing.T) {
	flightRepo := newMockFlightRepo()
	licenseRepo := newMockLicenseRepo()
	service := NewFlightService(flightRepo, licenseRepo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	license := &models.License{
		UserID:           userID,
		LicenseType:      models.LicenseTypeEASAPPL,
		LicenseNumber:    "PPL-123456",
		IssueDate:        issueDate,
		IssuingAuthority: "EASA",
		IsActive:         true,
	}
	_ = licenseRepo.Create(ctx, license)

	flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	flight := &models.Flight{
		UserID:       userID,
		LicenseID:    license.ID,
		Date:         flightDate,
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    2.5,
		PICTime:      2.5,
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
