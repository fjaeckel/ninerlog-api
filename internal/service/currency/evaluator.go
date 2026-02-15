package currency

import (
	"context"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/google/uuid"
)

// FlightDataProvider provides aggregated flight data for currency evaluation
type FlightDataProvider interface {
	// GetProgressByAircraftClass returns aggregated flight stats for a user's flights
	// on aircraft of the given class, since the given date.
	GetProgressByAircraftClass(ctx context.Context, userID uuid.UUID, classType models.ClassType, since time.Time) (*Progress, error)

	// GetProgressAll returns aggregated flight stats for all flights regardless of aircraft class
	GetProgressAll(ctx context.Context, userID uuid.UUID, since time.Time) (*Progress, error)
}

// Evaluator evaluates currency for a class rating based on the regulatory authority
type Evaluator interface {
	// Authority returns the regulatory authority this evaluator handles (e.g., "EASA", "FAA")
	Authority() string

	// Evaluate evaluates currency for a class rating, using the data provider to query flight data
	Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dataProvider FlightDataProvider) ClassRatingCurrency
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

// Get returns the evaluator for an authority, or nil if not found
func (r *Registry) Get(authority string) Evaluator {
	return r.evaluators[authority]
}

// HasEvaluator checks if an evaluator exists for an authority
func (r *Registry) HasEvaluator(authority string) bool {
	_, ok := r.evaluators[authority]
	return ok
}
