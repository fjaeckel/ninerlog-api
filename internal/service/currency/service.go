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

// EvaluateAll evaluates currency for all class ratings across all of a user's licenses
func (s *Service) EvaluateAll(ctx context.Context, userID uuid.UUID) (*CurrencyStatusResponse, error) {
	// Get all user licenses
	licenses, err := s.licenseRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get licenses: %w", err)
	}

	var ratings []ClassRatingCurrency

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
			result := eval.Evaluate(ctx, cr, license, s.flightData)
			ratings = append(ratings, result)
		}
	}

	if ratings == nil {
		ratings = []ClassRatingCurrency{}
	}

	return &CurrencyStatusResponse{Ratings: ratings}, nil
}
