package currency

import (
	"context"
	"fmt"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// ClassRatingRepo is the interface needed by the currency service
type ClassRatingRepo interface {
	GetByLicenseID(ctx context.Context, licenseID uuid.UUID) ([]*models.ClassRating, error)
}

// Service evaluates currency across all class ratings for a user
type Service struct {
	registry        *Registry
	licenseRepo     repository.LicenseRepository
	classRatingRepo ClassRatingRepo
	flightData      FlightDataProvider
	fallback        Evaluator
}

// NewService creates a new currency service
func NewService(
	registry *Registry,
	licenseRepo repository.LicenseRepository,
	classRatingRepo ClassRatingRepo,
	flightData FlightDataProvider,
) *Service {
	return &Service{
		registry:        registry,
		licenseRepo:     licenseRepo,
		classRatingRepo: classRatingRepo,
		flightData:      flightData,
		fallback:        NewOtherEvaluator(),
	}
}

// EvaluateAll evaluates currency for all class ratings across all of a user's licenses.
// Returns a two-tier response:
//   - Tier 1 (Ratings): Rating/license currency — can you fly at all?
//   - Tier 2 (PassengerCurrency): Passenger currency — can you carry passengers?
func (s *Service) EvaluateAll(ctx context.Context, userID uuid.UUID) (*CurrencyStatusResponse, error) {
	// Get all user licenses
	licenses, err := s.licenseRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get licenses: %w", err)
	}

	var ratings []ClassRatingCurrency
	var passengerCurrency []PassengerCurrency
	var flightReview *FlightReviewStatus
	seenPassengerClasses := make(map[string]bool) // avoid duplicate passenger currency for same class across licenses
	flightReviewEvaluated := false

	for _, license := range licenses {
		// Get class ratings for this license
		classRatings, err := s.classRatingRepo.GetByLicenseID(ctx, license.ID)
		if err != nil {
			continue // skip on error
		}

		// Find the evaluator for this license's authority
		eval := s.registry.Get(license.RegulatoryAuthority)
		if eval == nil {
			eval = s.fallback
		}

		for _, cr := range classRatings {
			// Tier 1: Rating currency
			result := eval.Evaluate(ctx, cr, license, s.flightData)
			ratings = append(ratings, result)

			// Tier 2: Passenger currency (if evaluator supports it)
			// Skip IR ratings — passenger currency doesn't apply to instrument ratings
			if cr.ClassType == models.ClassTypeIR {
				continue
			}
			passengerKey := string(cr.ClassType) + ":" + license.RegulatoryAuthority
			if seenPassengerClasses[passengerKey] {
				continue
			}
			seenPassengerClasses[passengerKey] = true

			if paxEval, ok := eval.(PassengerCurrencyEvaluator); ok {
				pax := paxEval.EvaluatePassengerCurrency(ctx, cr.ClassType, license, s.flightData)
				passengerCurrency = append(passengerCurrency, pax)
			}
		}

		// Flight review (FAA §61.56) — evaluate once per authority, not per rating
		if !flightReviewEvaluated {
			if frEval, ok := eval.(FlightReviewEvaluator); ok {
				flightReview = frEval.EvaluateFlightReview(ctx, userID, s.flightData)
				flightReviewEvaluated = true
			}
		}
	}

	if ratings == nil {
		ratings = []ClassRatingCurrency{}
	}
	if passengerCurrency == nil {
		passengerCurrency = []PassengerCurrency{}
	}

	return &CurrencyStatusResponse{
		Ratings:           ratings,
		PassengerCurrency: passengerCurrency,
		FlightReview:      flightReview,
	}, nil
}
