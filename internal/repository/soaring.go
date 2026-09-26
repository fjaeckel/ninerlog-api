package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SoaringLongestFlight is the longest soaring flight of a season.
type SoaringLongestFlight struct {
	FlightID    uuid.UUID
	Date        time.Time
	Minutes     int
	AircraftReg *string
}

// SoaringSite is a departure place and its soaring flight count.
type SoaringSite struct {
	Place   string
	Flights int
}

// SoaringSeasonStats aggregates soaring flights over a date range.
// LaunchesByMethod is keyed by the stored launch method, "" when unset.
type SoaringSeasonStats struct {
	Flights          int
	Launches         int
	TotalMinutes     int
	Outlandings      int
	LaunchesByMethod map[string]int
	LongestFlight    *SoaringLongestFlight
	Sites            []SoaringSite
}

// SoaringRepository reads soaring statistics. It is read-only.
type SoaringRepository interface {
	// SeasonStats aggregates the user's soaring flights dated from..to
	// inclusive, with the topSites most-flown departure places.
	SeasonStats(ctx context.Context, userID uuid.UUID, from, to time.Time, topSites int) (*SoaringSeasonStats, error)
}
