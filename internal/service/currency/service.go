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
	licenses, err := s.licenseRepo.GetByUserID(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get licenses: %w", err)
	}

	var ratings []ClassRatingCurrency
	var passengerCurrency []PassengerCurrency
	var flightReview *FlightReviewStatus
	seenPassengerClasses := make(map[string]bool) // one passenger currency per class across licenses
	flightReviewEvaluated := false

	ratingsByLicense := make(map[uuid.UUID][]*models.ClassRating, len(licenses))
	var heldRatings []*models.ClassRating
	for _, license := range licenses {
		classRatings, err := s.classRatingRepo.GetByLicenseID(ctx, license.ID)
		if err != nil {
			continue // skip on error
		}
		ratingsByLicense[license.ID] = classRatings
		heldRatings = append(heldRatings, classRatings...)
	}
	partFCLTMG := s.holdsPartFCLTMG(licenses, ratingsByLicense)

	for _, license := range licenses {
		classRatings, ok := ratingsByLicense[license.ID]
		if !ok {
			continue
		}

		// Find the evaluator for this license's authority
		eval := s.evaluatorFor(license)

		for _, cr := range classRatings {
			// Tier 1: Rating currency
			var result ClassRatingCurrency
			holderEval, holderAware := eval.(HolderAwareEvaluator)
			if holderAware {
				result = holderEval.EvaluateForHolder(ctx, cr, license, classRatings, heldRatings, s.flightData)
			} else if pe, ok := eval.(PeerAwareEvaluator); ok {
				result = pe.EvaluateWithPeers(ctx, cr, license, classRatings, s.flightData)
			} else {
				result = eval.Evaluate(ctx, cr, license, s.flightData)
			}
			if partFCLTMG && result.RuleDescriptionKey == easaSPLTMGRule.displayKey {
				applySFCLTMGExemption(&result)
			}
			ratings = append(ratings, result)

			// Tier 2: Passenger currency (if evaluator supports it).
			// IR ratings, ULTRALIGHT outside the German UL evaluator, and
			// ULTRALIGHT with no kind are skipped.
			if cr.ClassType == models.ClassTypeIR {
				continue
			}
			if _, germanUL := eval.(*GermanULEvaluator); cr.ClassType == models.ClassTypeUL && (!germanUL || cr.ULKind == nil) {
				continue
			}
			passengerKey := string(cr.ClassType) + ":" + license.RegulatoryAuthority
			if cr.ClassType == models.ClassTypeUL {
				passengerKey += ":" + string(*cr.ULKind)
			}
			if seenPassengerClasses[passengerKey] {
				continue
			}
			seenPassengerClasses[passengerKey] = true

			if holderAware {
				passengerCurrency = append(passengerCurrency, holderEval.EvaluateRatingPassengerCurrencyForHolder(ctx, cr, license, classRatings, heldRatings, s.flightData))
			} else if paxEval, ok := eval.(RatingPassengerCurrencyEvaluator); ok {
				passengerCurrency = append(passengerCurrency, paxEval.EvaluateRatingPassengerCurrency(ctx, cr, license, classRatings, s.flightData))
			} else if paxEval, ok := eval.(PassengerCurrencyEvaluator); ok {
				pax := paxEval.EvaluatePassengerCurrency(ctx, cr.ClassType, license, classRatings, s.flightData)
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

// evaluatorFor returns the evaluator for a license's authority.
func (s *Service) evaluatorFor(license *models.License) Evaluator {
	if eval := s.registry.Get(license.RegulatoryAuthority); eval != nil {
		return eval
	}
	return s.fallback
}

// holdsPartFCLTMG reports whether the user holds a TMG class rating on a
// Part-FCL licence — one evaluated under EASA rules that is neither a
// sailplane nor an ultralight licence (SFCL.160(c)).
func (s *Service) holdsPartFCLTMG(licenses []*models.License, ratingsByLicense map[uuid.UUID][]*models.ClassRating) bool {
	for _, license := range licenses {
		switch s.evaluatorFor(license).(type) {
		case *EASAEvaluator, *GermanULEvaluator:
		default:
			continue
		}
		if isEASASailplane(license.LicenseType) || isULLicenceType(license.LicenseType) {
			continue
		}
		if hasClass(ratingsByLicense[license.ID], models.ClassTypeTMG) {
			return true
		}
	}
	return false
}
