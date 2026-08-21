package models

import (
	"time"

	"github.com/google/uuid"
)

// ApproachEntry represents a single instrument approach with type and location
type ApproachEntry struct {
	Type    string  `json:"type"`
	Airport *string `json:"airport,omitempty"`
	Runway  *string `json:"runway,omitempty"`
}

// ValidApproachTypes lists all valid approach type values
var ValidApproachTypes = map[string]bool{
	"ILS": true, "LOC": true, "VOR": true, "RNAV/GPS": true, "NDB": true,
	"LDA": true, "SDF": true, "PAR": true, "ASR": true, "Visual": true,
	"Circling": true, "Other": true, "Unknown": true,
}

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
	TakeoffsDayOverride    bool `json:"-"` // When true, takeoffsDay is not auto-calculated
	TakeoffsNightOverride  bool `json:"-"` // When true, takeoffsNight is not auto-calculated
	LandingsDayOverride    bool `json:"-"` // When true, landingsDay is not auto-calculated
	LandingsNightOverride  bool `json:"-"` // When true, landingsNight is not auto-calculated
	SICTimeOverride        bool `json:"-"` // When true, sicTime was declared by the pilot rather than derived
	MultiPilotTimeOverride bool `json:"-"` // When true, multiPilotTime was declared by the pilot rather than derived

	// Instructor & comments
	InstructorName     *string `json:"instructorName,omitempty"`
	InstructorComments *string `json:"instructorComments,omitempty"`

	// PIC name (EASA AMC1 FCL.050 Col 12)
	PICName *string `json:"picName,omitempty"`

	// Multi-crew / advanced times
	SICTime             int `json:"sicTime"`
	DualGivenTime       int `json:"dualGivenTime"`
	SimulatedFlightTime int `json:"simulatedFlightTime"`
	GroundTrainingTime  int `json:"groundTrainingTime"`
	MultiPilotTime      int `json:"multiPilotTime"` // EASA AMC1 FCL.050 Col 10

	// FSTD type designation (EASA AMC1 FCL.050 Col 22, FAA §61.51(b)(1)(iv))
	FSTDType *string `json:"fstdType,omitempty"`

	// IsSimulator marks the row as an FSTD session rather than a flight.
	// Session duration lives in SimulatedFlightTime; TotalTime is 0 and the
	// row contributes nothing to flight totals.
	IsSimulator bool `json:"isSimulator"`

	// IsPassenger marks a flight the user was carried on rather than crewed.
	// The row keeps its route and block times as a record of the trip;
	// TotalTime is 0 and it contributes nothing to flight totals.
	IsPassenger bool `json:"isPassenger"`

	// Instrument tracking
	ActualInstrumentTime    int             `json:"actualInstrumentTime"`
	SimulatedInstrumentTime int             `json:"simulatedInstrumentTime"`
	Holds                   int             `json:"holds"`
	ApproachesCount         int             `json:"approachesCount"`
	Approaches              []ApproachEntry `json:"approaches,omitempty"` // Structured approach data (FAA §61.51(g)(3))
	IsIPC                   bool            `json:"isIpc"`
	IsFlightReview          bool            `json:"isFlightReview"`
	IsProficiencyCheck      bool            `json:"isProficiencyCheck"`

	// Endorsements (EASA AMC1 FCL.050 Col 24, FAA §61.51(h))
	Endorsements *string `json:"endorsements,omitempty"`

	// SPL / Glider
	LaunchMethod *string `json:"launchMethod,omitempty"` // winch, aerotow, self-launch

	// Crew members on board (populated from flight_crew_members table)
	CrewMembers []FlightCrewMember `json:"crewMembers,omitempty"`

	// Additional information
	Remarks *string `json:"remarks,omitempty"`

	// Instructor sign-off lock. Non-nil iff the flight is locked by a
	// completed, non-voided FlightSignature (see flight_signature.go).
	SignatureID *uuid.UUID `json:"signatureId,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IsValid checks if all required fields are set. An FSTD session identifies
// the device instead of an aircraft, so it carries no registration and no
// block time. A passenger flight identifies the aircraft but logs no flight
// time.
func (f *Flight) IsValid() bool {
	if f.UserID == uuid.Nil || f.Date.IsZero() || f.AircraftType == "" {
		return false
	}
	if f.IsSimulator {
		return f.FSTDType != nil && *f.FSTDType != "" && f.SimulatedFlightTime > 0
	}
	if f.IsPassenger {
		return f.AircraftReg != ""
	}
	return f.AircraftReg != "" && f.TotalTime > 0
}

// ValidateTimeDistribution checks that the recorded times are coherent.
//
// TotalTime is block time (EASA AMC1 FCL.050 Col 9). The pilot function
// columns decompose it: PIC + co-pilot + dual received must not exceed it.
// Instructor time (DualGivenTime) overlays the function time rather than
// adding to it — an instructor logs it alongside PIC time, or alone when
// the student acts as PIC — so it is bounded by TotalTime only.
func (f *Flight) ValidateTimeDistribution() error {
	if f.IsSimulator {
		return f.validateSessionTimes()
	}

	// isPic and isDual are mutually exclusive
	if f.IsPIC && f.IsDual {
		return ErrInvalidTimeDistribution
	}

	// All times must be non-negative
	if f.TotalTime < 0 || f.NightTime < 0 || f.IFRTime < 0 ||
		f.PICTime < 0 || f.DualTime < 0 || f.SICTime < 0 || f.DualGivenTime < 0 {
		return ErrNegativeTime
	}

	// Night time should not exceed total time
	if f.NightTime > f.TotalTime {
		return ErrInvalidNightTime
	}

	// IFR time should not exceed total time
	if f.IFRTime > f.TotalTime {
		return ErrInvalidIFRTime
	}

	// Pilot function time decomposes total time
	if f.PICTime+f.SICTime+f.DualTime > f.TotalTime {
		return ErrInvalidFunctionTime
	}

	// Instructor time overlays the function time it is flown under
	if f.DualGivenTime > f.TotalTime {
		return ErrInvalidDualGivenTime
	}

	// A passenger is not a crew member and logs no flight time
	if f.IsPassenger && (f.TotalTime != 0 || f.PICTime != 0 || f.SICTime != 0 ||
		f.DualTime != 0 || f.DualGivenTime != 0 || f.MultiPilotTime != 0 ||
		f.SoloTime != 0) {
		return ErrPassengerFunctionTime
	}

	// Landings must be non-negative
	if f.LandingsDay < 0 || f.LandingsNight < 0 {
		return ErrNegativeLandings
	}

	return nil
}

// validateSessionTimes enforces the FSTD invariants: a session carries a
// positive duration in SimulatedFlightTime and nothing in any flight-time
// column (AMC1 FCL.050 — session time is never summed with flight time).
func (f *Flight) validateSessionTimes() error {
	if f.FSTDType == nil || *f.FSTDType == "" {
		return ErrFSTDTypeRequired
	}
	if f.SimulatedFlightTime <= 0 {
		return ErrFSTDSessionTime
	}
	if f.TotalTime != 0 || f.PICTime != 0 || f.DualTime != 0 || f.SICTime != 0 ||
		f.DualGivenTime != 0 || f.MultiPilotTime != 0 || f.SoloTime != 0 ||
		f.CrossCountryTime != 0 || f.NightTime != 0 || f.IFRTime != 0 {
		return ErrFSTDFlightTime
	}
	if f.SimulatedInstrumentTime > f.SimulatedFlightTime {
		return ErrInvalidIFRTime
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
