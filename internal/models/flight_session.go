package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Flight session status values
const (
	FlightSessionStatusOpen      = "open"
	FlightSessionStatusCompleted = "completed"
	FlightSessionStatusDiscarded = "discarded"
)

// Flight session event types
const (
	FlightSessionEventOffBlock = "offblock"
	FlightSessionEventTakeoff  = "takeoff"
	FlightSessionEventLanding  = "landing"
	FlightSessionEventOnBlock  = "onblock"
)

// Flight session errors
var (
	ErrNoOpenFlightSession      = errors.New("no open flight session")
	ErrInvalidFlightSessionData = errors.New("invalid flight session data")
	ErrFlightSessionTimeOrder   = errors.New("flight session event times are out of order")
	ErrFlightSessionTooLong     = errors.New("flight session exceeds maximum duration")
	ErrFlightSessionMissingReg  = errors.New("aircraft registration is required to complete a flight session")
)

// MaxFlightSessionDuration is the longest off-block → on-block span accepted
// when completing a session. Anything longer is almost certainly a forgotten
// session that should be discarded instead.
const MaxFlightSessionDuration = 24 * time.Hour

// FlightSession represents an in-progress flight captured live via
// tap-to-log from a mobile device. Event instants are full UTC timestamps
// (not bare times of day) so flights crossing midnight stay unambiguous;
// they are split into date + HH:MM:SS when the session is converted into a
// Flight.
type FlightSession struct {
	ID     uuid.UUID `json:"id"`
	UserID uuid.UUID `json:"userId"`
	Status string    `json:"status"` // open | completed | discarded

	AircraftReg   *string `json:"aircraftReg,omitempty"`
	DepartureICAO *string `json:"departureIcao,omitempty"`
	ArrivalICAO   *string `json:"arrivalIcao,omitempty"`

	OffBlockAt *time.Time `json:"offBlockAt,omitempty"`
	TakeoffAt  *time.Time `json:"takeoffAt,omitempty"`
	LandingAt  *time.Time `json:"landingAt,omitempty"`
	OnBlockAt  *time.Time `json:"onBlockAt,omitempty"`

	// FlightID is set once the session has been completed and converted
	// into a regular flight log entry.
	FlightID *uuid.UUID `json:"flightId,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ValidateEventOrder checks that whichever event instants are present occur
// in the physical order off block ≤ takeoff ≤ landing ≤ on block.
func (s *FlightSession) ValidateEventOrder() error {
	// Collect present timestamps in expected order and verify monotonicity.
	ordered := []*time.Time{s.OffBlockAt, s.TakeoffAt, s.LandingAt, s.OnBlockAt}
	var prev *time.Time
	for _, t := range ordered {
		if t == nil {
			continue
		}
		if prev != nil && t.Before(*prev) {
			return ErrFlightSessionTimeOrder
		}
		prev = t
	}
	return nil
}

// BlockDuration returns the off-block → on-block duration, or 0 when either
// instant is missing.
func (s *FlightSession) BlockDuration() time.Duration {
	if s.OffBlockAt == nil || s.OnBlockAt == nil {
		return 0
	}
	return s.OnBlockAt.Sub(*s.OffBlockAt)
}
