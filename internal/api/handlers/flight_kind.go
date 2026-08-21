package handlers

import (
	"errors"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
)

// Conditionally-required field checks for a FlightCreate body, which
// describes either a flight or an FSTD session.

var (
	errFlightFieldsMissing = errors.New("departureIcao, arrivalIcao, offBlockTime, onBlockTime, aircraftReg and landings are required for a flight")
	errSessionFieldsUnused = errors.New("departureIcao, arrivalIcao, offBlockTime, onBlockTime, aircraftReg and landings are not valid for a simulator session")
	errSessionFieldsNeeded = errors.New("fstdType and a positive simulatedFlightTime are required for a simulator session")
)

// isSimulatorCreate reports whether the body describes an FSTD session.
func isSimulatorCreate(req *generated.FlightCreate) bool {
	return req.IsSimulator != nil && *req.IsSimulator
}

// validateCreateShape checks the conditionally-required fields of a create
// body. Returns nil when the body is well formed for its kind.
func validateCreateShape(req *generated.FlightCreate) error {
	flightFieldsPresent := req.DepartureIcao != nil || req.ArrivalIcao != nil ||
		req.OffBlockTime != nil || req.OnBlockTime != nil ||
		req.AircraftReg != nil || req.Landings != nil

	if isSimulatorCreate(req) {
		if flightFieldsPresent {
			return errSessionFieldsUnused
		}
		if req.FstdType == nil || *req.FstdType == "" ||
			req.SimulatedFlightTime == nil || *req.SimulatedFlightTime <= 0 {
			return errSessionFieldsNeeded
		}
		return nil
	}

	if req.DepartureIcao == nil || req.ArrivalIcao == nil ||
		req.OffBlockTime == nil || req.OnBlockTime == nil ||
		req.AircraftReg == nil || req.Landings == nil {
		return errFlightFieldsMissing
	}
	return nil
}
