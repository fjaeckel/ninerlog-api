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
// evening-civil-twilight end (Dusk) in UTC of the local solar day go-solar
// associates with the given UTC date; either may fall on an adjacent UTC
// date. Classify instants with IsNight or DaylightBetween, not with one pair.
//
// Edge cases: polar night collapses the window to zero width at the start of
// the UTC day; midnight sun spans the whole UTC day.
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

// Interval is a closed UTC time range [Start, End].
type Interval struct {
	Start time.Time
	End   time.Time
}

// Contains reports whether t lies in [Start, End].
func (iv Interval) Contains(t time.Time) bool {
	return !t.Before(iv.Start) && !t.After(iv.End)
}

// Daylight is a set of civil-twilight day intervals at one location.
type Daylight []Interval

// IsNight reports whether t lies outside every interval in d.
func (d Daylight) IsNight(t time.Time) bool {
	for _, iv := range d {
		if iv.Contains(t) {
			return false
		}
	}
	return true
}

// solarDay returns the civil-twilight day interval of the local solar day
// that go-solar associates with the given UTC date: [dawn, dusk] on a normal
// day, the whole solar day (mean solar noon ± 12h) under midnight sun, and
// ok=false under polar night.
func solarDay(date time.Time, latitude, longitude float64) (Interval, bool) {
	loc := gosolar.NewLocation(latitude, longitude)
	t := gosolar.NewTime(date.Year(), date.Month(), date.Day())

	dawn, dusk := gosolar.DawnDusk(loc, t, gosolar.Civil)
	if !dawn.IsZero() && !dusk.IsZero() {
		return Interval{Start: dawn, End: dusk}, true
	}

	noon := gosolar.MeanSolarNoon(loc, t)
	if gosolar.Elevation(loc, noon) <= -6 {
		return Interval{}, false
	}
	return Interval{Start: noon.Add(-12 * time.Hour), End: noon.Add(12 * time.Hour)}, true
}

// DaylightBetween returns the civil-twilight day intervals at the location
// for every UTC date from the day before from's date to the day after to's
// date. Every instant in [from, to] is day iff the result contains it.
func DaylightBetween(from, to time.Time, latitude, longitude float64) Daylight {
	from, to = from.UTC(), to.UTC()
	first := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	last := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

	var d Daylight
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		if iv, ok := solarDay(day, latitude, longitude); ok {
			d = append(d, iv)
		}
	}
	return d
}

// IsNight returns true if the given instant falls in the aeronautical
// definition of night at the supplied latitude and longitude: outside every
// civil-twilight day interval of the local solar days around t's UTC date.
func IsNight(t time.Time, latitude, longitude float64) bool {
	return DaylightBetween(t, t, latitude, longitude).IsNight(t)
}
