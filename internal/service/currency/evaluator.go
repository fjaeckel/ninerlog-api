package currency

import (
	"context"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// FlightDataProvider provides aggregated flight data for currency evaluation
type FlightDataProvider interface {
	// GetProgressByAircraftClass returns aggregated flight stats for a user's flights
	// on aircraft of the given class, since the given date.
	GetProgressByAircraftClass(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) (*Progress, error)

	// GetProgressAll returns aggregated flight stats for all flights regardless of aircraft class
	GetProgressAll(ctx context.Context, userID uuid.UUID, since time.Time) (*Progress, error)

	// GetLastFlightReview returns the date of the most recent flight with is_flight_review = true
	// for the given user. Returns nil if no flight review found.
	GetLastFlightReview(ctx context.Context, userID uuid.UUID) (*time.Time, error)

	// GetLastProficiencyCheck returns the date of the most recent flight with is_proficiency_check = true
	// for the given user and class type. Returns nil if none found.
	GetLastProficiencyCheck(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) (*time.Time, error)

	// GetLaunchCounts returns per-launch-method counts for SPL currency (FCL.140.S(b)(1)).
	// Groups flights by launch_method and returns a map of method → launch count.
	GetLaunchCounts(ctx context.Context, userID uuid.UUID, since time.Time) (map[string]int, error)
}

// Evaluator evaluates currency for a class rating based on the regulatory authority
type Evaluator interface {
	// Authority returns the regulatory authority this evaluator handles (e.g., "EASA", "FAA")
	Authority() string

	// Evaluate evaluates currency for a class rating, using the data provider to query flight data
	Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dataProvider FlightDataProvider) ClassRatingCurrency
}

// PassengerCurrencyEvaluator is an optional interface that evaluators can implement
// to provide Tier 2 passenger currency evaluation (FCL.060(b) / §61.57(a)/(b)).
// This is separate from Tier 1 rating currency.
type PassengerCurrencyEvaluator interface {
	EvaluatePassengerCurrency(ctx context.Context, classType models.ClassType, license *models.License, dp FlightDataProvider) PassengerCurrency
}

// FlightReviewEvaluator is an optional interface for evaluators that support
// flight review tracking (FAA §61.56 — 24 calendar months).
type FlightReviewEvaluator interface {
	EvaluateFlightReview(ctx context.Context, userID uuid.UUID, dp FlightDataProvider) *FlightReviewStatus
}

// Registry holds all registered currency evaluators, dispatching by authority
type Registry struct {
	evaluators map[string]Evaluator
}

// NewRegistry creates a new evaluator registry
func NewRegistry() *Registry {
	return &Registry{
		evaluators: make(map[string]Evaluator),
	}
}

// Register adds an evaluator for an authority
func (r *Registry) Register(eval Evaluator) {
	r.evaluators[eval.Authority()] = eval
}

// RegisterMulti registers an evaluator for multiple authority strings.
// Used for national evaluators that may be referenced by different authority names
// (e.g., German UL can be "LBA", "DULV", or "DAeC").
func (r *Registry) RegisterMulti(eval Evaluator, authorities ...string) {
	for _, auth := range authorities {
		r.evaluators[auth] = eval
	}
}

// Get returns the evaluator for an authority, or nil if not found
func (r *Registry) Get(authority string) Evaluator {
	return r.evaluators[authority]
}

// HasEvaluator checks if an evaluator exists for an authority
func (r *Registry) HasEvaluator(authority string) bool {
	_, ok := r.evaluators[authority]
	return ok
}
