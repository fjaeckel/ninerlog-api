package handlers

import "strings"

// foreFlightAircraftClass maps a ForeFlight Aircraft Table Class and
// EngineType to an aircraft class, or nil for a non-aeroplane class.
//
//   - Piston, diesel, electric, unknown or empty engine type: SEP_* / MEP_*
//   - Turboprop or turbine: SET_* / MET_*
//   - Turbofan, turbojet or jet: SEJ_LAND / MEJ_LAND on land, SET_SEA / MET_SEA on water
func foreFlightAircraftClass(ffClass, ffEngineType string) *string {
	var engines, surface string
	switch strings.ToLower(strings.TrimSpace(ffClass)) {
	case "airplane_single_engine_land":
		engines, surface = "SE", "LAND"
	case "airplane_single_engine_sea":
		engines, surface = "SE", "SEA"
	case "airplane_multi_engine_land":
		engines, surface = "ME", "LAND"
	case "airplane_multi_engine_sea":
		engines, surface = "ME", "SEA"
	default:
		return nil
	}

	power := "P"
	switch strings.ToLower(strings.TrimSpace(ffEngineType)) {
	case "turboprop", "turbine":
		power = "T"
	case "turbofan", "turbojet", "jet":
		power = "J"
		if surface == "SEA" {
			power = "T"
		}
	}

	c := engines + power + "_" + surface
	return &c
}
