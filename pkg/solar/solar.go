// Package solar provides a thin wrapper around github.com/mstephenholl/go-solar
// for computing sunrise/sunset and determining whether a given UTC instant is
// during daytime or nighttime.
package solar

import (
	"errors"
	"time"

	gosolar "github.com/mstephenholl/go-solar"
)

// SunTimes holds sunrise and sunset times for a given date and location.
type SunTimes struct {
	Sunrise time.Time
	Sunset  time.Time
}

// Calculate computes sunrise and sunset UTC times for a given date, latitude, and longitude.
// Computation is delegated to github.com/mstephenholl/go-solar.
//
// Edge cases:
//   - Polar night (sun never rises): both Sunrise and Sunset are set to the
//     start of the UTC day; the whole day counts as night.
//   - Midnight sun (sun never sets): Sunrise is set to the start of the UTC
//     day and Sunset to 23:59:59 UTC; the whole day counts as day.
func Calculate(date time.Time, latitude, longitude float64) SunTimes {
	loc := gosolar.NewLocation(latitude, longitude)
	t := gosolar.NewTime(date.Year(), date.Month(), date.Day())

	midnight := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	sunrise, sunset, err := gosolar.SunriseSunset(loc, t)
	if err != nil {
		switch {
		case errors.Is(err, gosolar.ErrSunNeverRises):
			return SunTimes{Sunrise: midnight, Sunset: midnight}
		case errors.Is(err, gosolar.ErrSunNeverSets):
			return SunTimes{
				Sunrise: midnight,
				Sunset:  time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, time.UTC),
			}
		default:
			return SunTimes{Sunrise: midnight, Sunset: midnight}
		}
	}
	return SunTimes{Sunrise: sunrise, Sunset: sunset}
}

// TwilightTimes holds the morning (Dawn) and evening (Dusk) civil twilight
// boundaries for a given UTC date and location.
type TwilightTimes struct {
	Dawn time.Time // beginning of morning civil twilight (sun at -6° rising)
	Dusk time.Time // end of evening civil twilight (sun at -6° setting)
}

// CivilTwilight computes the morning-civil-twilight start (Dawn) and
// evening-civil-twilight end (Dusk) in UTC for the given date and location.
// These are the EASA "night" boundaries: any time between Dusk and the next
// day's Dawn is night.
//
// Edge cases: polar night collapses the window to zero width (the entire UTC
// day is night); midnight sun spans the whole UTC day (the entire day is day).
func CivilTwilight(date time.Time, latitude, longitude float64) TwilightTimes {
	loc := gosolar.NewLocation(latitude, longitude)
	t := gosolar.NewTime(date.Year(), date.Month(), date.Day())

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, time.UTC)

	dawn, dusk := gosolar.DawnDusk(loc, t, gosolar.Civil)

	// Disambiguate polar night vs midnight sun by inspecting noon elevation.
	if dawn.IsZero() || dusk.IsZero() {
		noon := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC)
		elev := gosolar.Elevation(loc, noon)
		if elev <= -6 {
			// Polar night: whole day counts as night.
			return TwilightTimes{Dawn: startOfDay, Dusk: startOfDay}
		}
		// Midnight sun above civil twilight: whole day is "day".
		return TwilightTimes{Dawn: startOfDay, Dusk: endOfDay}
	}
	return TwilightTimes{Dawn: dawn, Dusk: dusk}
}

// IsNight returns true if the given UTC time falls in the aeronautical
// definition of night: before the morning civil twilight begins, or after
// the evening civil twilight ends, at the supplied latitude and longitude.
func IsNight(t time.Time, latitude, longitude float64) bool {
	tw := CivilTwilight(t, latitude, longitude)
	return t.Before(tw.Dawn) || t.After(tw.Dusk)
}
