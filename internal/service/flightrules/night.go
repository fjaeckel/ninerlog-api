package flightrules

import (
	"time"

	"github.com/fjaeckel/ninerlog-api/pkg/solar"
)

// IsNightAt is the single point of truth for "is this instant night?" at a
// given latitude/longitude. It delegates to pkg/solar but every caller
// (flightcalc takeoff/landing splits, night-minute walk, future stats) goes
// through here so we can swap in civil-twilight or AMC FCL.010 rules in one
// place if the spec is ever clarified.
func IsNightAt(t time.Time, lat, lon float64) bool {
	return solar.IsNight(t, lat, lon)
}
