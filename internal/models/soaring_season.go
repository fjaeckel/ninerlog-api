package models

import (
	"time"

	"github.com/google/uuid"
)

// SoaringLaunchesByMethod counts a season's launches per launch method;
// Unspecified holds soaring flights without one.
type SoaringLaunchesByMethod struct {
	Winch       int `json:"winch"`
	Aerotow     int `json:"aerotow"`
	SelfLaunch  int `json:"selfLaunch"`
	Car         int `json:"car"`
	Bungee      int `json:"bungee"`
	Unspecified int `json:"unspecified"`
}

// Add counts n launches of method m; unknown methods count as unspecified.
func (l *SoaringLaunchesByMethod) Add(m string, n int) {
	switch m {
	case LaunchMethodWinch:
		l.Winch += n
	case LaunchMethodAerotow:
		l.Aerotow += n
	case LaunchMethodSelfLaunch:
		l.SelfLaunch += n
	case LaunchMethodCar:
		l.Car += n
	case LaunchMethodBungee:
		l.Bungee += n
	default:
		l.Unspecified += n
	}
}

// SoaringSeasonFlight is the longest flight of a soaring season.
type SoaringSeasonFlight struct {
	FlightID    uuid.UUID `json:"flightId"`
	Date        string    `json:"date"`
	Minutes     int       `json:"minutes"`
	AircraftReg *string   `json:"aircraftReg"`
}

// SoaringSeasonSite is a departure place and its flight count.
type SoaringSeasonSite struct {
	Place   string `json:"place"`
	Flights int    `json:"flights"`
}

// SoaringSeason summarises one calendar year of soaring flights.
// Durations are minutes.
type SoaringSeason struct {
	Year                 int                     `json:"year"`
	Flights              int                     `json:"flights"`
	Launches             int                     `json:"launches"`
	LaunchesByMethod     SoaringLaunchesByMethod `json:"launchesByMethod"`
	TotalMinutes         int                     `json:"totalMinutes"`
	LongestFlight        *SoaringSeasonFlight    `json:"longestFlight,omitempty"`
	Outlandings          int                     `json:"outlandings"`
	AverageFlightMinutes int                     `json:"averageFlightMinutes"`
	Sites                []SoaringSeasonSite     `json:"sites"`
}

// SoaringSeasonTopSites is how many departure places a season lists.
const SoaringSeasonTopSites = 5

// SoaringSeasonMinYear is the earliest season year accepted.
const SoaringSeasonMinYear = 1900

// ValidSoaringSeasonYear reports whether year lies in
// SoaringSeasonMinYear..now's year + 1.
func ValidSoaringSeasonYear(year int, now time.Time) bool {
	return year >= SoaringSeasonMinYear && year <= now.Year()+1
}
