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
	var windows []repository.RegistrationWindow
	for _, ac := range aircraftList {
		if ac.AircraftClass == nil || !allowedClasses[*ac.AircraftClass] {
			continue
		}
		reg := registration.Canonical(ac.Registration)
		if ac.ValidFrom == nil && ac.ValidTo == nil {
			regs = append(regs, reg)
			continue
		}
		windows = append(windows, repository.RegistrationWindow{Registration: reg, From: ac.ValidFrom, To: ac.ValidTo})
	}
	opts.FilterByRegistrations = true
	opts.AircraftRegistrations = regs
	opts.RegistrationWindows = windows
	return nil
}
