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

// IsNight returns true if the given UTC time is before sunrise or after sunset
// at the supplied latitude and longitude.
func IsNight(t time.Time, latitude, longitude float64) bool {
	sun := Calculate(t, latitude, longitude)
	return t.Before(sun.Sunrise) || t.After(sun.Sunset)
}
