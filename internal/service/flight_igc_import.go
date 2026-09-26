package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/google/uuid"
)

// ErrIGCInvalidFlight is returned when an IGC file does not describe a flight
// the logbook accepts; see IGCInvalidFlightError for the reason.
var ErrIGCInvalidFlight = errors.New("the IGC file does not describe a valid flight")

// ErrIGCImportUnavailable is returned by ImportAsNewFlight before
// SetImportDependencies has been called.
var ErrIGCImportUnavailable = errors.New("igc import is not configured")

// IGCInvalidFlightError carries the validation reason; Reason is empty when
// the flight service rejected the flight.
type IGCInvalidFlightError struct {
	Reason string
}

func (e *IGCInvalidFlightError) Error() string {
	if e.Reason == "" {
		return ErrIGCInvalidFlight.Error()
	}
	return ErrIGCInvalidFlight.Error() + ": " + e.Reason
}

func (e *IGCInvalidFlightError) Unwrap() error { return ErrIGCInvalidFlight }

// IGCImport is a flight created from an IGC file.
type IGCImport struct {
	Flight  *models.Flight
	File    *models.FlightFile
	Preview *IGCPreview
}

// SetImportDependencies wires the flight and aircraft services
// ImportAsNewFlight creates rows through.
func (s *FlightFileService) SetImportDependencies(flights *FlightService, aircraft *AircraftService) {
	s.flights = flights
	s.aircraft = aircraft
}

// ImportAsNewFlight creates a flight from an IGC file and stores the file on
// it. The glider is looked up in, or added to, the user's fleet. userName
// feeds the auto-calculations as in POST /flights.
//
// Errors: the parse errors of Preview, ErrIGCNoRegistration,
// *IGCDuplicateError when the user already stores the file,
// *IGCInvalidFlightError, and the storage errors of Attach. No flight is
// left behind when storing the file fails.
func (s *FlightFileService) ImportAsNewFlight(ctx context.Context, userID uuid.UUID, userName, filename string, data []byte) (*IGCImport, error) {
	if s.flights == nil || s.aircraft == nil {
		return nil, ErrIGCImportUnavailable
	}
	preview, err := s.Preview(ctx, userID, data)
	if err != nil {
		return nil, err
	}
	if preview.GliderRegistration == nil {
		return nil, ErrIGCNoRegistration
	}
	stored, err := s.StoredFlightFor(ctx, userID, data)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		return nil, &IGCDuplicateError{FlightID: *stored}
	}

	aircraft := s.ensureIGCAircraft(ctx, userID, *preview.GliderRegistration, preview.GliderType)
	flight, err := igcFlight(preview, userID, aircraft)
	if err != nil {
		return nil, err
	}
	flightcalc.ApplyAutoCalculations(flight, userName, s.aircraft.AircraftFactsFor(ctx, userID, flight.AircraftReg))
	if flight.PICName == nil {
		flight.PICName = flightrules.ResolvePICNameForSave(flight, userName)
	}
	if err := s.flights.CreateFlight(ctx, flight); err != nil {
		slog.Debug("igc import: flight rejected", "error", err)
		return nil, &IGCInvalidFlightError{}
	}
	file, err := s.Attach(ctx, userID, flight.ID, filename, data)
	if err != nil {
		if derr := s.flights.DeleteFlight(ctx, flight.ID, userID); derr != nil {
			slog.Error("igc import: failed to remove flight after file store failed", "flightId", flight.ID, "error", derr)
		}
		return nil, err
	}
	return &IGCImport{Flight: flight, File: file, Preview: preview}, nil
}

// ensureIGCAircraft returns the user's aircraft with this registration,
// creating it as the importers do when absent. Returns nil when it can
// neither be found nor created.
func (s *FlightFileService) ensureIGCAircraft(ctx context.Context, userID uuid.UUID, reg string, gliderType *string) *models.Aircraft {
	fleet, err := s.aircraft.ListAircraft(ctx, userID)
	if err == nil {
		for _, a := range fleet {
			if registration.Canonical(a.Registration) == reg {
				return a
			}
		}
	}
	typeCode := reg
	if gliderType != nil && *gliderType != "" {
		typeCode = *gliderType
	}
	a := &models.Aircraft{
		UserID:        userID,
		Registration:  reg,
		Type:          typeCode,
		Make:          typeCode,
		Model:         typeCode,
		IsActive:      true,
		AircraftClass: models.InferImportedAircraftClass("", reg, true),
	}
	if err := s.aircraft.CreateAircraft(ctx, a); err != nil {
		slog.Warn("igc import: failed to create aircraft", "registration", reg, "error", err)
		return nil
	}
	return a
}

// igcFlight builds the flight an IGC preview describes, before
// auto-calculation.
func igcFlight(p *IGCPreview, userID uuid.UUID, aircraft *models.Aircraft) (*models.Flight, error) {
	reg := *p.GliderRegistration
	aircraftType := reg
	if aircraft != nil && aircraft.Type != "" {
		aircraftType = aircraft.Type
	} else if p.GliderType != nil {
		aircraftType = *p.GliderType
	}
	dep, arr := p.Departure.Label(), p.Arrival.Label()
	takeoff, landing := p.TakeoffTime, p.LandingTime
	clocks := models.FlightClocks{Takeoff: &takeoff, Landing: &landing}
	total, _, err := clocks.TotalMinutes()
	if err != nil {
		return nil, &IGCInvalidFlightError{Reason: "Invalid take-off/landing times format"}
	}
	flight := &models.Flight{
		UserID:        userID,
		Date:          p.Date,
		AircraftReg:   reg,
		AircraftType:  aircraftType,
		DepartureICAO: &dep,
		ArrivalICAO:   &arr,
		DepartureTime: &takeoff,
		ArrivalTime:   &landing,
		TotalTime:     total,
		AllLandings:   1,
		IsPIC:         true,
		PICTime:       total,
		IsOutlanding:  p.Outlanding,
	}
	var class *string
	var kind *models.ULKind
	if aircraft != nil {
		class, kind = aircraft.AircraftClass, aircraft.ULKind
	}
	if p.LaunchMethod != "unknown" && models.IsValidLaunchMethod(p.LaunchMethod) && models.LaunchMethodApplies(class, kind) {
		lm := p.LaunchMethod
		flight.LaunchMethod = &lm
		flight.ReleaseHeightM = p.ReleaseHeightM
	}
	return flight, nil
}
