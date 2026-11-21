package service

import (
	"context"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/google/uuid"
)

// FleetIndex resolves a registration and flight date to the fleet entry whose
// validity range covers that date.
type FleetIndex struct {
	byReg map[string][]*models.Aircraft
}

// NewFleetIndex indexes fleet by upper-cased registration.
func NewFleetIndex(fleet []*models.Aircraft) FleetIndex {
	idx := FleetIndex{byReg: make(map[string][]*models.Aircraft, len(fleet))}
	for _, a := range fleet {
		if a == nil {
			continue
		}
		key := NormalizeRegistrationKey(a.Registration)
		idx.byReg[key] = append(idx.byReg[key], a)
	}
	return idx
}

// Resolve returns the aircraft registered as registration on date, or nil.
// A zero date resolves against today.
func (f FleetIndex) Resolve(registration string, date time.Time) *models.Aircraft {
	if date.IsZero() {
		date = time.Now()
	}
	for _, a := range f.byReg[NormalizeRegistrationKey(registration)] {
		if a.CoversDate(date) {
			return a
		}
	}
	return nil
}

// Has reports whether any fleet entry carries registration.
func (f FleetIndex) Has(registration string) bool {
	return len(f.byReg[NormalizeRegistrationKey(registration)]) > 0
}

// AircraftFactsIndex is the fleet facts lookup that flight derivation reads.
type AircraftFactsIndex struct {
	fleet FleetIndex
}

// NewAircraftFactsIndex builds the facts lookup for fleet.
func NewAircraftFactsIndex(fleet []*models.Aircraft) AircraftFactsIndex {
	return AircraftFactsIndex{fleet: NewFleetIndex(fleet)}
}

// For returns the fleet facts for registration on date, or nil when no fleet
// entry matches.
func (i AircraftFactsIndex) For(registration string, date time.Time) *flightrules.AircraftFacts {
	a := i.fleet.Resolve(registration, date)
	if a == nil {
		return nil
	}
	return &flightrules.AircraftFacts{
		Registration: a.Registration,
		IsMultiPilot: a.IsMultiPilot,
	}
}

// NormalizeRegistrationKey returns the lookup key for a registration.
func NormalizeRegistrationKey(registration string) string {
	return strings.ToUpper(strings.TrimSpace(registration))
}

// AircraftFactsFor resolves the fleet facts for one registration on date,
// returning nil when the user's fleet holds no matching aircraft.
func AircraftFactsFor(ctx context.Context, repo repository.AircraftRepository, userID uuid.UUID, registration string, date time.Time) *flightrules.AircraftFacts {
	if NormalizeRegistrationKey(registration) == "" || repo == nil {
		return nil
	}
	fleet, err := repo.GetByUserID(ctx, userID, nil)
	if err != nil {
		return nil
	}
	return NewAircraftFactsIndex(fleet).For(registration, date)
}

// AircraftFactsFor resolves the fleet facts for one registration on date,
// returning nil when the user's fleet holds no matching aircraft.
func (s *AircraftService) AircraftFactsFor(ctx context.Context, userID uuid.UUID, registration string, date time.Time) *flightrules.AircraftFacts {
	if s == nil {
		return nil
	}
	return AircraftFactsFor(ctx, s.aircraftRepo, userID, registration, date)
}

// AircraftFactsIndexFor returns the fleet facts lookup for every aircraft the
// user owns.
func (s *AircraftService) AircraftFactsIndexFor(ctx context.Context, userID uuid.UUID) AircraftFactsIndex {
	if s == nil || s.aircraftRepo == nil {
		return NewAircraftFactsIndex(nil)
	}
	fleet, err := s.aircraftRepo.GetByUserID(ctx, userID, nil)
	if err != nil {
		return NewAircraftFactsIndex(nil)
	}
	return NewAircraftFactsIndex(fleet)
}
