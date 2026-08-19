package flightrules

import (
	"time"

	"github.com/fjaeckel/ninerlog-api/pkg/solar"
)

// IsNightAt is the single point of truth for "is this instant night?" at a
// given latitude/longitude; it delegates to pkg/solar and every caller goes
// through here.
func IsNightAt(t time.Time, lat, lon float64) bool {
	return solar.IsNight(t, lat, lon)
}
