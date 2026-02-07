package models

import "errors"

var (
	// Flight validation errors
	ErrInvalidTimeDistribution = errors.New("PIC and dual time are mutually exclusive")
	ErrInvalidNightTime        = errors.New("night time exceeds total time")
	ErrInvalidIFRTime          = errors.New("IFR time exceeds total time")
	ErrNegativeTime            = errors.New("flight times cannot be negative")
	ErrNegativeLandings        = errors.New("landings cannot be negative")
)
