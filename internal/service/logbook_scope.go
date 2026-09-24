package service

import (
	"context"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/google/uuid"
)

// LogbookScope restricts a flight query to one licence's logbook: flights on
// aircraft whose class matches the licence's class ratings.
type LogbookScope struct {
	classRatings *ClassRatingService
	aircraft     *AircraftService
}

// NewLogbookScope creates a logbook scope resolver.
func NewLogbookScope(classRatings *ClassRatingService, aircraft *AircraftService) *LogbookScope {
	return &LogbookScope{classRatings: classRatings, aircraft: aircraft}
}

// Apply sets the registration filter on opts for the licence's logbook. A
// licence without class ratings leaves opts unfiltered. Returns
// ErrLicenseNotFound or ErrUnauthorizedAccess when the licence is not the
// user's.
func (s *LogbookScope) Apply(ctx context.Context, userID, licenseID uuid.UUID, opts *repository.FlightQueryOptions) error {
	classRatings, err := s.classRatings.ListClassRatings(ctx, licenseID, userID)
	if err != nil {
		return err
	}
	if len(classRatings) == 0 {
		return nil
	}
	allowedClasses := make(map[string]bool, len(classRatings))
	for _, cr := range classRatings {
		allowedClasses[string(cr.ClassType)] = true
	}
	aircraftList, err := s.aircraft.ListAircraft(ctx, userID)
	if err != nil {
		return err
	}
	regs := make([]string, 0, len(aircraftList))
	for _, ac := range aircraftList {
		if ac.AircraftClass != nil && allowedClasses[*ac.AircraftClass] {
			regs = append(regs, registration.Canonical(ac.Registration))
		}
	}
	opts.FilterByRegistrations = true
	opts.AircraftRegistrations = regs
	return nil
}
