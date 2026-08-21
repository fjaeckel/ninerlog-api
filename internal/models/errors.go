package models

import "errors"

var (
	// Flight validation errors
	ErrInvalidTimeDistribution = errors.New("PIC and dual time are mutually exclusive")
	ErrInvalidNightTime        = errors.New("night time exceeds total time")
	ErrInvalidIFRTime          = errors.New("IFR time exceeds total time")
	ErrNegativeTime            = errors.New("flight times cannot be negative")
	ErrNegativeLandings        = errors.New("landings cannot be negative")
	ErrInvalidFunctionTime     = errors.New("PIC, co-pilot and dual time together exceed total time")
	ErrInvalidDualGivenTime    = errors.New("instructor time exceeds total time")

	// FSTD session validation errors
	ErrFSTDTypeRequired = errors.New("fstdType is required for a simulator session")
	ErrFSTDSessionTime  = errors.New("a simulator session requires a positive session duration")
	ErrFSTDFlightTime   = errors.New("a simulator session cannot carry flight time")

	// Aircraft validation errors
	ErrAircraftRegistrationRequired = errors.New("aircraft registration is required")
	ErrAircraftTypeRequired         = errors.New("aircraft type is required")
	ErrAircraftMakeRequired         = errors.New("aircraft make is required")
	ErrAircraftModelRequired        = errors.New("aircraft model is required")
	ErrAircraftInvalidEngineType    = errors.New("invalid engine type")
)
