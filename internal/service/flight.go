package service

import (
	"context"
	"errors"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrFlightNotFound     = errors.New("flight not found")
	ErrInvalidFlight      = errors.New("invalid flight data")
	ErrUnauthorizedFlight = errors.New("unauthorized access to flight")
)

type FlightService struct {
	flightRepo  repository.FlightRepository
	licenseRepo repository.LicenseRepository
}

func NewFlightService(flightRepo repository.FlightRepository, licenseRepo repository.LicenseRepository) *FlightService {
	return &FlightService{
		flightRepo:  flightRepo,
		licenseRepo: licenseRepo,
	}
}

// CreateFlight creates a new flight log entry
func (s *FlightService) CreateFlight(ctx context.Context, flight *models.Flight) error {
	// Validate basic fields
	if !flight.IsValid() {
		return ErrInvalidFlight
	}

	// Validate time distribution
	if err := flight.ValidateTimeDistribution(); err != nil {
		return err
	}

	// Verify license ownership
	license, err := s.licenseRepo.GetByID(ctx, flight.LicenseID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errors.New("license not found")
		}
		return err
	}

	if license.UserID != flight.UserID {
		return ErrUnauthorizedFlight
	}

	// Validate SPL-specific rules (no night flying for sailplane licenses)
	if license.LicenseType == models.LicenseTypeEASASPL || license.LicenseType == models.LicenseTypeFAASport {
		if flight.NightTime > 0 {
			return errors.New("sailplane/sport licenses do not allow night flying")
		}
	}

	return s.flightRepo.Create(ctx, flight)
}

// GetFlight retrieves a flight by ID and verifies user ownership
func (s *FlightService) GetFlight(ctx context.Context, flightID, userID uuid.UUID) (*models.Flight, error) {
	flight, err := s.flightRepo.GetByID(ctx, flightID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrFlightNotFound
		}
		return nil, err
	}

	// Verify ownership
	if flight.UserID != userID {
		return nil, ErrUnauthorizedFlight
	}

	return flight, nil
}

// ListFlights retrieves flights for a user with optional filters
func (s *FlightService) ListFlights(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) ([]*models.Flight, error) {
	return s.flightRepo.GetByUserID(ctx, userID, opts)
}

// UpdateFlight updates a flight and verifies user ownership
func (s *FlightService) UpdateFlight(ctx context.Context, flight *models.Flight, userID uuid.UUID) error {
	// Verify ownership
	existing, err := s.flightRepo.GetByID(ctx, flight.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrFlightNotFound
		}
		return err
	}

	if existing.UserID != userID {
		return ErrUnauthorizedFlight
	}

	// Validate basic fields
	if !flight.IsValid() {
		return ErrInvalidFlight
	}

	// Validate time distribution
	if err := flight.ValidateTimeDistribution(); err != nil {
		return err
	}

	// Verify license ownership
	license, err := s.licenseRepo.GetByID(ctx, flight.LicenseID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errors.New("license not found")
		}
		return err
	}

	if license.UserID != userID {
		return ErrUnauthorizedFlight
	}

	// Validate SPL-specific rules
	if license.LicenseType == models.LicenseTypeEASASPL || license.LicenseType == models.LicenseTypeFAASport {
		if flight.NightTime > 0 {
			return errors.New("sailplane/sport licenses do not allow night flying")
		}
	}

	// Keep original IDs and timestamps
	flight.ID = existing.ID
	flight.UserID = existing.UserID
	flight.CreatedAt = existing.CreatedAt

	return s.flightRepo.Update(ctx, flight)
}

// DeleteFlight deletes a flight and verifies user ownership
func (s *FlightService) DeleteFlight(ctx context.Context, flightID, userID uuid.UUID) error {
	// Verify ownership
	flight, err := s.flightRepo.GetByID(ctx, flightID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrFlightNotFound
		}
		return err
	}

	if flight.UserID != userID {
		return ErrUnauthorizedFlight
	}

	return s.flightRepo.Delete(ctx, flightID)
}

// CountFlights counts flights for a user with optional filters
func (s *FlightService) CountFlights(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) (int, error) {
	return s.flightRepo.CountByUserID(ctx, userID, opts)
}
