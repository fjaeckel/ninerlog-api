package service

import (
	"context"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/google/uuid"
)

// AircraftFactsIndex builds the registration → fleet facts lookup that flight
// derivation reads, keyed by upper-cased registration.
func AircraftFactsIndex(fleet []*models.Aircraft) map[string]*flightrules.AircraftFacts {
	index := make(map[string]*flightrules.AircraftFacts, len(fleet))
	for _, a := range fleet {
		if a == nil {
			continue
		}
		index[NormalizeRegistrationKey(a.Registration)] = &flightrules.AircraftFacts{
			Registration: a.Registration,
			IsMultiPilot: a.IsMultiPilot,
		}
	}
	return index
}

// NormalizeRegistrationKey returns the lookup key for a registration.
func NormalizeRegistrationKey(registration string) string {
	return strings.ToUpper(strings.TrimSpace(registration))
}

// AircraftFactsFor resolves the fleet facts for one registration, returning
// nil when the user's fleet holds no matching aircraft.
func AircraftFactsFor(ctx context.Context, repo repository.AircraftRepository, userID uuid.UUID, registration string) *flightrules.AircraftFacts {
	key := NormalizeRegistrationKey(registration)
	if key == "" || repo == nil {
		return nil
	}
	fleet, err := repo.GetByUserID(ctx, userID, nil)
	if err != nil {
		return nil
	}
	return AircraftFactsIndex(fleet)[key]
}

// AircraftFactsFor resolves the fleet facts for one registration, returning
// nil when the user's fleet holds no matching aircraft.
func (s *AircraftService) AircraftFactsFor(ctx context.Context, userID uuid.UUID, registration string) *flightrules.AircraftFacts {
	if s == nil {
		return nil
	}
	return AircraftFactsFor(ctx, s.aircraftRepo, userID, registration)
}

// AircraftFactsIndexFor returns the fleet facts for every aircraft the user
// owns, keyed by upper-cased registration.
func (s *AircraftService) AircraftFactsIndexFor(ctx context.Context, userID uuid.UUID) map[string]*flightrules.AircraftFacts {
	if s == nil || s.aircraftRepo == nil {
		return map[string]*flightrules.AircraftFacts{}
	}
	fleet, err := s.aircraftRepo.GetByUserID(ctx, userID, nil)
	if err != nil {
		return map[string]*flightrules.AircraftFacts{}
	}
	return AircraftFactsIndex(fleet)
}
