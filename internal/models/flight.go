package models

import (
	"time"

	"github.com/google/uuid"
)

// Flight represents a flight log entry
type Flight struct {
	ID     uuid.UUID `json:"id"`
	UserID uuid.UUID `json:"userId"`
	Date   time.Time `json:"date"`

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

	// Flight times (in minutes)
	TotalTime int  `json:"totalTime"`
	IsPIC     bool `json:"isPic"`
	IsDual    bool `json:"isDual"`
	PICTime   int  `json:"picTime"`
	DualTime  int  `json:"dualTime"`
	NightTime int  `json:"nightTime"`
	IFRTime   int  `json:"ifrTime"`

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
	SoloTime         int     `json:"soloTime"`         // Auto-calculated when not dual and not PIC with crew
	CrossCountryTime int     `json:"crossCountryTime"` // Auto-calculated when departure ≠ arrival
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
	SICTime             int `json:"sicTime"`
	DualGivenTime       int `json:"dualGivenTime"`
	SimulatedFlightTime int `json:"simulatedFlightTime"`
	GroundTrainingTime  int `json:"groundTrainingTime"`

	// Instrument tracking
	ActualInstrumentTime    int  `json:"actualInstrumentTime"`
	SimulatedInstrumentTime int  `json:"simulatedInstrumentTime"`
	Holds                   int  `json:"holds"`
	ApproachesCount         int  `json:"approachesCount"`
	IsIPC                   bool `json:"isIpc"`
	IsFlightReview          bool `json:"isFlightReview"`
	IsProficiencyCheck      bool `json:"isProficiencyCheck"`

	// SPL / Glider
	LaunchMethod *string `json:"launchMethod,omitempty"` // winch, aerotow, self-launch

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
	TotalFlights        int
	TotalMinutes        int
	PICMinutes          int
	DualMinutes         int
	NightMinutes        int
	IFRMinutes          int
	SoloMinutes         int
	CrossCountryMinutes int
	LandingsDay         int
	LandingsNight       int
	SICMinutes          int
	DualGivenMinutes    int
}

// CurrencyData holds landing/flight counts for currency calculation
type CurrencyData struct {
	Flights       int
	TotalLandings int
	DayLandings   int
	NightLandings int
}
