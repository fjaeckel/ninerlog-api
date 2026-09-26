package repository

import (
	"context"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// TrainingFlight is one crewed flight on a GLIDER, TMG or ULTRALIGHT aircraft, reduced to
// the values training progress counts. AircraftClass is trimmed and upper-cased; ULKind is
// set for ULTRALIGHT aircraft only. DistanceNM is 0 when unknown.
type TrainingFlight struct {
	AircraftClass       string
	ULKind              *models.ULKind
	DualMinutes         int
	PICMinutes          int
	SPICMinutes         int
	Launches            int
	Landings            int
	CrossCountryMinutes int
	DistanceNM          float64
	Signed              bool
}

// TrainingRepository reads the flight data training progress is computed from. It is read-only.
type TrainingRepository interface {
	// ListTrainingFlights returns the user's crewed flights on GLIDER, TMG and ULTRALIGHT
	// aircraft (matched by registration in the user's fleet).
	ListTrainingFlights(ctx context.Context, userID uuid.UUID) ([]TrainingFlight, error)
	// OtherCategoryPICMinutes sums the PIC minutes of crewed flights on aircraft that are
	// not GLIDER, TMG or ULTRALIGHT, including flights on registrations with no class.
	OtherCategoryPICMinutes(ctx context.Context, userID uuid.UUID) (int, error)
}
