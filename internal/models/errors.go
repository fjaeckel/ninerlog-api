package models

import "errors"

var (
	// Flight validation errors
	ErrInvalidTimeDistribution = errors.New("sum of PIC, dual, and solo time exceeds total time")
	ErrInvalidNightTime        = errors.New("night time exceeds total time")
	ErrInvalidIFRTime          = errors.New("IFR time exceeds total time")
	ErrNegativeTime            = errors.New("flight times cannot be negative")
	ErrNegativeLandings        = errors.New("landings cannot be negative")
)
