package models

import "errors"

var (
	// Flight validation errors
	ErrInvalidTimeDistribution = errors.New("PIC and dual time are mutually exclusive")
	ErrInvalidNightTime        = errors.New("night time exceeds total time")
	ErrInvalidIFRTime          = errors.New("IFR time exceeds total time")
	ErrNegativeTime            = errors.New("flight times cannot be negative")
	ErrNegativeLandings        = errors.New("landings cannot be negative")

	// Aircraft validation errors
	ErrAircraftRegistrationRequired = errors.New("aircraft registration is required")
	ErrAircraftTypeRequired         = errors.New("aircraft type is required")
	ErrAircraftMakeRequired         = errors.New("aircraft make is required")
	ErrAircraftModelRequired        = errors.New("aircraft model is required")
	ErrAircraftInvalidEngineType    = errors.New("invalid engine type")
)
