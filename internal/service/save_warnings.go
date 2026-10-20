package service

import (
	"context"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ULMTOMLimitKg is the German UL class MTOM limit.
const ULMTOMLimitKg = 600

// UL120kgClassLimitKg is the mass limit of the single-seat 120 kg class.
const UL120kgClassLimitKg = 120

// flightCheck inspects a saved flight and the fleet aircraft it names (nil
// when the fleet holds none) and returns a warning or nil.
type flightCheck func(f *models.Flight, ac *models.Aircraft) *models.Warning

// aircraftCheck inspects a saved aircraft and returns a warning or nil.
type aircraftCheck func(ac *models.Aircraft) *models.Warning

var flightChecks = []flightCheck{checkULNightFlight}

var aircraftChecks = []aircraftCheck{checkULMTOMExceeds600, checkUL120kgClass}

// FlightWarnings runs every flight check against f.
func FlightWarnings(f *models.Flight, ac *models.Aircraft) []models.Warning {
	var out []models.Warning
	for _, check := range flightChecks {
		if w := check(f, ac); w != nil {
			out = append(out, *w)
		}
	}
	return out
}

// AircraftWarnings runs every aircraft check against ac.
func AircraftWarnings(ac *models.Aircraft) []models.Warning {
	var out []models.Warning
	for _, check := range aircraftChecks {
		if w := check(ac); w != nil {
			out = append(out, *w)
		}
	}
	return out
}

// CheckAircraft returns the warnings a saved aircraft triggers.
func (s *AircraftService) CheckAircraft(ac *models.Aircraft) []models.Warning {
	return AircraftWarnings(ac)
}

// CheckFlights returns the warnings each saved flight triggers, in order.
// Flights are matched to userID's fleet by registration; without an aircraft
// repository no flight check sees an aircraft.
func (s *FlightService) CheckFlights(ctx context.Context, userID uuid.UUID, flights []*models.Flight) [][]models.Warning {
	fleet := s.fleetByRegistration(ctx, userID)
	out := make([][]models.Warning, len(flights))
	for i, f := range flights {
		if f == nil {
			continue
		}
		out[i] = FlightWarnings(f, fleet[NormalizeRegistrationKey(f.AircraftReg)])
	}
	return out
}

// fleetByRegistration indexes userID's aircraft by upper-cased registration.
func (s *FlightService) fleetByRegistration(ctx context.Context, userID uuid.UUID) map[string]*models.Aircraft {
	index := map[string]*models.Aircraft{}
	if s.aircraftRepo == nil {
		return index
	}
	fleet, err := s.aircraftRepo.GetByUserID(ctx, userID, nil)
	if err != nil {
		return index
	}
	for _, ac := range fleet {
		if ac != nil {
			index[NormalizeRegistrationKey(ac.Registration)] = ac
		}
	}
	return index
}

func checkULNightFlight(f *models.Flight, ac *models.Aircraft) *models.Warning {
	if f.IsSimulator || ac == nil || !models.IsULClass(ac.AircraftClass) {
		return nil
	}
	if f.NightTime <= 0 && f.LandingsNight <= 0 {
		return nil
	}
	params := map[string]any{
		"nightTime":     f.NightTime,
		"landingsNight": f.LandingsNight,
		"registration":  ac.Registration,
	}
	if ac.ULKind != nil {
		params["ulKind"] = string(*ac.ULKind)
	}
	return &models.Warning{Code: models.WarningULNightFlight, Severity: models.WarningSeverityWarning, Params: params}
}

func checkULMTOMExceeds600(ac *models.Aircraft) *models.Warning {
	if ac == nil || !models.IsULClass(ac.AircraftClass) || ac.MTOMKg == nil || *ac.MTOMKg <= ULMTOMLimitKg {
		return nil
	}
	return &models.Warning{
		Code:     models.WarningULMTOMExceeds600,
		Severity: models.WarningSeverityWarning,
		Params:   map[string]any{"mtomKg": *ac.MTOMKg, "limitKg": ULMTOMLimitKg},
	}
}

func checkUL120kgClass(ac *models.Aircraft) *models.Warning {
	if ac == nil || !models.IsULClass(ac.AircraftClass) || ac.MTOMKg == nil || *ac.MTOMKg > UL120kgClassLimitKg {
		return nil
	}
	return &models.Warning{
		Code:     models.WarningUL120kgClass,
		Severity: models.WarningSeverityInfo,
		Params:   map[string]any{"mtomKg": *ac.MTOMKg, "limitKg": UL120kgClassLimitKg},
	}
}
