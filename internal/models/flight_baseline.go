package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// FlightBaseline represents a per-user "initial hours snapshot": a carried-forward
// total of pre-existing flying experience that is added on top of logged flights
// when computing aggregated user-level statistics.
type FlightBaseline struct {
	UserID              uuid.UUID
	BaselineDate        time.Time
	TotalFlights        int
	TotalMinutes        int
	PICMinutes          int
	SICMinutes          int
	DualMinutes         int
	DualGivenMinutes    int
	MultiPilotMinutes   int
	NightMinutes        int
	IFRMinutes          int
	SoloMinutes         int
	CrossCountryMinutes int
	LandingsDay         int
	LandingsNight       int
	Notes               *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ErrInvalidFlightBaseline indicates a baseline failed validation.
var ErrInvalidFlightBaseline = errors.New("invalid flight baseline")

// Validate enforces the basic invariants that aren't covered by DB CHECK
// constraints (non-negative values are enforced in the database, but we still
// fail fast at the API boundary).
func (b *FlightBaseline) Validate() error {
	if b.BaselineDate.IsZero() {
		return errors.Join(ErrInvalidFlightBaseline, errors.New("baselineDate is required"))
	}
	// Reject baselines dated in the future to avoid nonsensical totals.
	if b.BaselineDate.After(time.Now().UTC().AddDate(0, 0, 1)) {
		return errors.Join(ErrInvalidFlightBaseline, errors.New("baselineDate cannot be in the future"))
	}
	values := []int{
		b.TotalFlights,
		b.TotalMinutes,
		b.PICMinutes,
		b.SICMinutes,
		b.DualMinutes,
		b.DualGivenMinutes,
		b.MultiPilotMinutes,
		b.NightMinutes,
		b.IFRMinutes,
		b.SoloMinutes,
		b.CrossCountryMinutes,
		b.LandingsDay,
		b.LandingsNight,
	}
	for _, v := range values {
		if v < 0 {
			return errors.Join(ErrInvalidFlightBaseline, errors.New("baseline values must be non-negative"))
		}
	}
	if b.Notes != nil && len(*b.Notes) > 1000 {
		return errors.Join(ErrInvalidFlightBaseline, errors.New("notes too long (max 1000 chars)"))
	}
	return nil
}
