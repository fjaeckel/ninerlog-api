// Package solar provides a thin wrapper around github.com/mstephenholl/go-solar
// for computing sunrise/sunset and determining whether a given UTC instant is
// during daytime or nighttime.
//
// The wrapper preserves the historical API used throughout this repository
// (Calculate, SunTimes, IsNight) while delegating the underlying astronomical
// math to go-solar.
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
//     start of the UTC day, so any time on that day is considered "night".
//   - Midnight sun (sun never sets): Sunrise is set to the start of the UTC
//     day and Sunset to 23:59:59 UTC, so any time on that day is considered "day".
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
// boundaries for a given UTC date and location. Per ICAO Annex 2 / EU
// Regulation 2018/1976 the aeronautical definition of "night" is:
//
//	"the period between the end of evening civil twilight and the beginning
//	 of morning civil twilight or such other period between sunset and
//	 sunrise as may be prescribed by the appropriate authority."
//
// Civil twilight is defined as the period during which the centre of the
// sun is between 0° and 6° below the horizon. Dawn marks the beginning of
// morning civil twilight; Dusk marks the end of evening civil twilight.
type TwilightTimes struct {
	Dawn time.Time // beginning of morning civil twilight (sun at -6° rising)
	Dusk time.Time // end of evening civil twilight (sun at -6° setting)
}

// CivilTwilight computes the morning-civil-twilight start (Dawn) and
// evening-civil-twilight end (Dusk) in UTC for the given date and location.
// These are the EASA "night" boundaries: any time between Dusk and the next
// day's Dawn is considered night.
//
// Edge cases (polar night / midnight sun): when the sun is permanently below
// or above the civil-twilight threshold, go-solar returns the zero time for
// the affected boundary. We collapse those zero values to the start of the
// UTC day for Dawn and end of the UTC day for Dusk so that:
//   - permanent polar night (no civil dawn or dusk): the entire UTC day is
//     night;
//   - midnight sun (sun stays above −6°): the entire UTC day is day.
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
			// Polar night: sun never reaches civil-twilight altitude. Make the
			// whole day count as night by collapsing the window to zero width.
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
