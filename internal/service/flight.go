package service

import (
	"context"
	"errors"
	"time"

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
	flightRepo repository.FlightRepository
}

func NewFlightService(flightRepo repository.FlightRepository) *FlightService {
	return &FlightService{
		flightRepo: flightRepo,
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

// GetStatsByUserID returns aggregated statistics for a user
func (s *FlightService) GetStatsByUserID(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time) (*models.FlightStatistics, error) {
	return s.flightRepo.GetStatsByUserID(ctx, userID, startDate, endDate)
}

// CurrencyResult holds the calculated currency status
type CurrencyResult struct {
	IsCurrent     bool
	DaysCurrent   bool
	NightsCurrent bool
	Flights90Days int
	TotalLandings int
	DayLandings   int
	NightLandings int
	RequiredDay   int
	RequiredNight int
}

// GetCurrency calculates currency status for a user based on EASA/FAA rules.
// FAA 14 CFR 61.57: 3 takeoffs and landings in the preceding 90 days
// EASA FCL.060: Recent experience — 3 takeoffs and landings in the preceding 90 days (simplified)
func (s *FlightService) GetCurrency(ctx context.Context, userID uuid.UUID) (*CurrencyResult, error) {
	// Query last 90 days
	since := time.Now().AddDate(0, 0, -90)
	data, err := s.flightRepo.GetCurrencyData(ctx, userID, since)
	if err != nil {
		return nil, err
	}

	// Both EASA and FAA require 3 landings in 90 days for passenger currency
	requiredDay := 3
	requiredNight := 3

	daysCurrent := data.DayLandings >= requiredDay
	nightsCurrent := data.NightLandings >= requiredNight

	// Overall current = day current (night is separate per FAA)
	isCurrent := daysCurrent

	return &CurrencyResult{
		IsCurrent:     isCurrent,
		DaysCurrent:   daysCurrent,
		NightsCurrent: nightsCurrent,
		Flights90Days: data.Flights,
		TotalLandings: data.TotalLandings,
		DayLandings:   data.DayLandings,
		NightLandings: data.NightLandings,
		RequiredDay:   requiredDay,
		RequiredNight: requiredNight,
	}, nil
}
