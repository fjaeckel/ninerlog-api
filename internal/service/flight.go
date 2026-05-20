package service

import (
	"context"
	"errors"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrFlightNotFound     = errors.New("flight not found")
	ErrInvalidFlight      = errors.New("invalid flight data")
	ErrUnauthorizedFlight = errors.New("unauthorized access to flight")
)

type FlightService struct {
	flightRepo   repository.FlightRepository
	baselineRepo repository.FlightBaselineRepository
}

func NewFlightService(flightRepo repository.FlightRepository, baselineRepo repository.FlightBaselineRepository) *FlightService {
	return &FlightService{
		flightRepo:   flightRepo,
		baselineRepo: baselineRepo,
	}
}

// CreateFlight creates a new flight log entry
func (s *FlightService) CreateFlight(ctx context.Context, flight *models.Flight) error {
	// Validate text field lengths
	if err := models.ValidateFlightTextFields(flight); err != nil {
		return err
	}

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

	// Validate text field lengths
	if err := models.ValidateFlightTextFields(flight); err != nil {
		return err
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

// DeleteAllFlights deletes all flights for a user
func (s *FlightService) DeleteAllFlights(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.flightRepo.DeleteAllByUserID(ctx, userID)
}

// CountFlights counts flights for a user with optional filters
func (s *FlightService) CountFlights(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) (int, error) {
	return s.flightRepo.CountByUserID(ctx, userID, opts)
}

// GetStatsByUserID returns aggregated statistics for a user. The optional
// initial-hours snapshot (baseline) is added on top when:
//   - the user has a baseline, AND
//   - applyBaseline is true, AND
//   - startDate is nil OR startDate <= baseline_date AND
//     endDate is nil OR endDate >= baseline_date.
//
// When the baseline is applied, it is also returned so callers can surface a
// breakdown to the client. baseline is nil otherwise.
func (s *FlightService) GetStatsByUserID(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, applyBaseline bool) (*models.FlightStatistics, *models.FlightBaseline, error) {
	stats, err := s.flightRepo.GetStatsByUserID(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, nil, err
	}

	if !applyBaseline || s.baselineRepo == nil {
		return stats, nil, nil
	}

	baseline, err := s.baselineRepo.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return stats, nil, nil
		}
		return nil, nil, err
	}

	if !baselineCoversRange(baseline.BaselineDate, startDate, endDate) {
		return stats, nil, nil
	}

	stats.TotalFlights += baseline.TotalFlights
	stats.TotalMinutes += baseline.TotalMinutes
	stats.PICMinutes += baseline.PICMinutes
	stats.SICMinutes += baseline.SICMinutes
	stats.DualMinutes += baseline.DualMinutes
	stats.DualGivenMinutes += baseline.DualGivenMinutes
	stats.NightMinutes += baseline.NightMinutes
	stats.IFRMinutes += baseline.IFRMinutes
	stats.SoloMinutes += baseline.SoloMinutes
	stats.CrossCountryMinutes += baseline.CrossCountryMinutes
	stats.LandingsDay += baseline.LandingsDay
	stats.LandingsNight += baseline.LandingsNight

	return stats, baseline, nil
}

// baselineCoversRange reports whether a baseline whose cutoff is `baselineDate`
// should contribute to a statistics query bounded by [startDate, endDate].
// The baseline conceptually represents flying done on or before its cutoff;
// it is therefore excluded only when the requested window is fully after it.
func baselineCoversRange(baselineDate time.Time, startDate, endDate *time.Time) bool {
	if startDate != nil && startDate.After(baselineDate) {
		return false
	}
	_ = endDate // endDate alone never excludes the baseline
	return true
}

// GetBaseline returns the user's initial-hours snapshot, or repository.ErrNotFound.
func (s *FlightService) GetBaseline(ctx context.Context, userID uuid.UUID) (*models.FlightBaseline, error) {
	if s.baselineRepo == nil {
		return nil, repository.ErrNotFound
	}
	return s.baselineRepo.Get(ctx, userID)
}

// UpsertBaseline validates and stores the user's initial-hours snapshot.
func (s *FlightService) UpsertBaseline(ctx context.Context, baseline *models.FlightBaseline) error {
	if s.baselineRepo == nil {
		return errors.New("baseline repository not configured")
	}
	if err := baseline.Validate(); err != nil {
		return err
	}
	return s.baselineRepo.Upsert(ctx, baseline)
}

// DeleteBaseline removes the user's initial-hours snapshot.
func (s *FlightService) DeleteBaseline(ctx context.Context, userID uuid.UUID) error {
	if s.baselineRepo == nil {
		return repository.ErrNotFound
	}
	return s.baselineRepo.Delete(ctx, userID)
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
