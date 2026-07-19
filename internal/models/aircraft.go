package models

import (
	"time"

	"github.com/google/uuid"
)

// Aircraft represents an aircraft in a user's fleet
type Aircraft struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"userId"`
	Registration      string    `json:"registration"`
	Type              string    `json:"type"`
	Make              string    `json:"make"`
	Model             string    `json:"model"`
	IsComplex         bool      `json:"isComplex"`
	IsHighPerformance bool      `json:"isHighPerformance"`
	IsTailwheel       bool      `json:"isTailwheel"`
	Notes             *string   `json:"notes,omitempty"`
	IsActive          bool      `json:"isActive"`
	AircraftClass     *string   `json:"aircraftClass,omitempty"`
	// Logging defaults, prefilled when a flight with this aircraft is logged
	DefaultDepartureICAO *string   `json:"defaultDepartureIcao,omitempty"`
	DefaultArrivalICAO   *string   `json:"defaultArrivalIcao,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

// AircraftStats holds flight statistics aggregated per aircraft registration
type AircraftStats struct {
	Registration    string     `json:"registration"`
	TotalFlights    int        `json:"totalFlights"`
	TotalMinutes    int        `json:"totalMinutes"`
	LandingsDay     int        `json:"landingsDay"`
	LandingsNight   int        `json:"landingsNight"`
	FirstFlightDate *time.Time `json:"firstFlightDate,omitempty"`
	LastFlightDate  *time.Time `json:"lastFlightDate,omitempty"`
	// Informational 90-day recency (EASA FCL.060(b)-style day recency:
	// 3 landings in the preceding 90 days)
	LandingsLast90Days int        `json:"landingsLast90Days"`
	RecencyLapsesOn    *time.Time `json:"recencyLapsesOn,omitempty"`
}

// AircraftTypeStats holds flight statistics aggregated per aircraft type
type AircraftTypeStats struct {
	AircraftType       string     `json:"aircraftType"`
	TotalFlights       int        `json:"totalFlights"`
	TotalMinutes       int        `json:"totalMinutes"`
	LandingsDay        int        `json:"landingsDay"`
	LandingsNight      int        `json:"landingsNight"`
	FirstFlightDate    *time.Time `json:"firstFlightDate,omitempty"`
	LastFlightDate     *time.Time `json:"lastFlightDate,omitempty"`
	LandingsLast90Days int        `json:"landingsLast90Days"`
	RecencyLapsesOn    *time.Time `json:"recencyLapsesOn,omitempty"`
}

// AircraftRecencyRow is one day of landings for one registration/type pair,
// used to derive 90-day recency counts and lapse dates
type AircraftRecencyRow struct {
	Registration string
	AircraftType string
	Date         time.Time
	Landings     int
}

// Validate checks basic Aircraft field requirements
func (a *Aircraft) Validate() error {
	if a.Registration == "" {
		return ErrAircraftRegistrationRequired
	}
	if a.Type == "" {
		return ErrAircraftTypeRequired
	}
	if a.Make == "" {
		return ErrAircraftMakeRequired
	}
	if a.Model == "" {
		return ErrAircraftModelRequired
	}
	return nil
}
