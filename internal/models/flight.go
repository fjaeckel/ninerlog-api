package models

import (
	"time"

	"github.com/google/uuid"
)

// Flight represents a flight log entry
type Flight struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"userId"`
	LicenseID uuid.UUID `json:"licenseId"`
	Date      time.Time `json:"date"`

	// Aircraft information
	AircraftReg  string `json:"aircraftReg"`
	AircraftType string `json:"aircraftType"`

	// Route information
	DepartureICAO *string `json:"departureIcao,omitempty"`
	ArrivalICAO   *string `json:"arrivalIcao,omitempty"`
	OffBlockTime  *string `json:"offBlockTime,omitempty"`  // HH:MM:SS format - chocks off / engine start (UTC)
	OnBlockTime   *string `json:"onBlockTime,omitempty"`   // HH:MM:SS format - chocks on / engine shutdown (UTC)
	DepartureTime *string `json:"departureTime,omitempty"` // HH:MM:SS format - takeoff time (UTC)
	ArrivalTime   *string `json:"arrivalTime,omitempty"`   // HH:MM:SS format - landing time (UTC)

	// Flight times (in decimal hours)
	TotalTime float64 `json:"totalTime"`
	IsPIC     bool    `json:"isPic"`
	IsDual    bool    `json:"isDual"`
	PICTime   float64 `json:"picTime"`
	DualTime  float64 `json:"dualTime"`
	NightTime float64 `json:"nightTime"`
	IFRTime   float64 `json:"ifrTime"`

	// Landings
	LandingsDay   int `json:"landingsDay"`
	LandingsNight int `json:"landingsNight"`
	AllLandings   int `json:"allLandings"` // Auto-calculated: day + night

	// Takeoffs
	TakeoffsDay   int `json:"takeoffsDay"`   // Auto-calculated from sunset/sunrise at departure (with manual override)
	TakeoffsNight int `json:"takeoffsNight"` // Auto-calculated from sunset/sunrise at departure (with manual override)

	// Route
	Route *string `json:"route,omitempty"` // Comma-separated ICAO waypoints

	// Auto-calculated fields
	SoloTime         float64 `json:"soloTime"`         // Auto-calculated when not dual and not PIC with crew
	CrossCountryTime float64 `json:"crossCountryTime"` // Auto-calculated when departure ≠ arrival
	Distance         float64 `json:"distance"`         // Auto-calculated from airport coordinates (NM)

	// Manual override flags
	TakeoffsDayOverride   bool `json:"-"` // When true, takeoffsDay is not auto-calculated
	TakeoffsNightOverride bool `json:"-"` // When true, takeoffsNight is not auto-calculated
	LandingsDayOverride   bool `json:"-"` // When true, landingsDay is not auto-calculated
	LandingsNightOverride bool `json:"-"` // When true, landingsNight is not auto-calculated

	// Instructor & comments
	InstructorName     *string `json:"instructorName,omitempty"`
	InstructorComments *string `json:"instructorComments,omitempty"`

	// Multi-crew / advanced times
	SICTime             float64 `json:"sicTime"`
	DualGivenTime       float64 `json:"dualGivenTime"`
	SimulatedFlightTime float64 `json:"simulatedFlightTime"`
	GroundTrainingTime  float64 `json:"groundTrainingTime"`

	// Crew members on board (populated from flight_crew_members table)
	CrewMembers []FlightCrewMember `json:"crewMembers,omitempty"`

	// Additional information
	Remarks *string `json:"remarks,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IsValid checks if all required fields are set
func (f *Flight) IsValid() bool {
	return f.UserID != uuid.Nil &&
		f.LicenseID != uuid.Nil &&
		!f.Date.IsZero() &&
		f.AircraftReg != "" &&
		f.AircraftType != "" &&
		f.TotalTime > 0
}

// ValidateTimeDistribution checks if time distribution is valid
func (f *Flight) ValidateTimeDistribution() error {
	// isPic and isDual are mutually exclusive
	if f.IsPIC && f.IsDual {
		return ErrInvalidTimeDistribution
	}

	// Night time should not exceed total time
	if f.NightTime > f.TotalTime {
		return ErrInvalidNightTime
	}

	// IFR time should not exceed total time
	if f.IFRTime > f.TotalTime {
		return ErrInvalidIFRTime
	}

	// All times must be non-negative
	if f.TotalTime < 0 || f.NightTime < 0 || f.IFRTime < 0 {
		return ErrNegativeTime
	}

	// Landings must be non-negative
	if f.LandingsDay < 0 || f.LandingsNight < 0 {
		return ErrNegativeLandings
	}

	return nil
}

// FlightStatistics holds aggregated flight statistics for a license
type FlightStatistics struct {
	LicenseID         uuid.UUID
	TotalFlights      int
	TotalHours        float64
	PICHours          float64
	DualHours         float64
	NightHours        float64
	IFRHours          float64
	SoloHours         float64
	CrossCountryHours float64
	LandingsDay       int
	LandingsNight     int
	SICHours          float64
	DualGivenHours    float64
}

// CurrencyData holds landing/flight counts for currency calculation
type CurrencyData struct {
	Flights       int
	TotalLandings int
	DayLandings   int
	NightLandings int
}
