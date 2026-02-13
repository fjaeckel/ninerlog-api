package solar

import (
	"math"
	"time"
)

// SunTimes holds sunrise and sunset times for a given date and location.
type SunTimes struct {
	Sunrise time.Time
	Sunset  time.Time
}

// Calculate computes sunrise and sunset UTC times for a given date, latitude, and longitude.
// Uses the NOAA Solar Calculator algorithm. Zenith = 90.833 (official sunrise/sunset with refraction).
func Calculate(date time.Time, latitude, longitude float64) SunTimes {
	jd := toJulianDay(date)
	jc := (jd - 2451545.0) / 36525.0

	sunMeanLong := math.Mod(280.46646+jc*(36000.76983+0.0003032*jc), 360.0)
	sunMeanAnomaly := 357.52911 + jc*(35999.05029-0.0001537*jc)

	sinM := math.Sin(degToRad(sunMeanAnomaly))
	sin2M := math.Sin(degToRad(2.0 * sunMeanAnomaly))
	sin3M := math.Sin(degToRad(3.0 * sunMeanAnomaly))
	eqCenter := sinM*(1.9146-jc*(0.004817+0.000014*jc)) + sin2M*(0.019993-0.000101*jc) + sin3M*0.000289

	sunTrueLong := sunMeanLong + eqCenter
	omega := 125.04 - 1934.136*jc
	sunApparentLong := sunTrueLong - 0.00569 - 0.00478*math.Sin(degToRad(omega))

	meanObliq := 23.0 + (26.0+(21.448-jc*(46.815+jc*(0.00059-jc*0.001813)))/60.0)/60.0
	obliqCorr := meanObliq + 0.00256*math.Cos(degToRad(omega))

	sinDecl := math.Sin(degToRad(obliqCorr)) * math.Sin(degToRad(sunApparentLong))
	decl := math.Asin(sinDecl)

	eqTime := eqOfTime(jc, sunMeanLong, sunMeanAnomaly, obliqCorr)

	zenith := 90.833
	latRad := degToRad(latitude)
	cosHA := math.Cos(degToRad(zenith))/(math.Cos(latRad)*math.Cos(decl)) - math.Tan(latRad)*math.Tan(decl)

	// Polar night
	if cosHA > 1.0 {
		midnight := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
		return SunTimes{Sunrise: midnight, Sunset: midnight}
	}
	// Midnight sun
	if cosHA < -1.0 {
		return SunTimes{
			Sunrise: time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC),
			Sunset:  time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, time.UTC),
		}
	}

	ha := radToDeg(math.Acos(cosHA))
	solarNoon := 720.0 - 4.0*longitude - eqTime
	sunriseMin := solarNoon - ha*4.0
	sunsetMin := solarNoon + ha*4.0

	base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	sunrise := base.Add(time.Duration(sunriseMin * float64(time.Minute)))
	sunset := base.Add(time.Duration(sunsetMin * float64(time.Minute)))

	return SunTimes{Sunrise: sunrise, Sunset: sunset}
}

// IsNight returns true if the given UTC time is before sunrise or after sunset.
func IsNight(t time.Time, latitude, longitude float64) bool {
	sun := Calculate(t, latitude, longitude)
	return t.Before(sun.Sunrise) || t.After(sun.Sunset)
}

func eqOfTime(jc, sunMeanLong, sunMeanAnomaly, obliqCorr float64) float64 {
	y := math.Tan(degToRad(obliqCorr / 2.0))
	y *= y

	sl2 := math.Sin(2.0 * degToRad(sunMeanLong))
	sa := math.Sin(degToRad(sunMeanAnomaly))
	sa2 := math.Sin(2.0 * degToRad(sunMeanAnomaly))
	sl4 := math.Sin(4.0 * degToRad(sunMeanLong))

	ecc := 0.016708634 - jc*(0.000042037+0.0000001267*jc)

	et := 4.0 * radToDeg(
		y*sl2-2.0*ecc*sa+4.0*ecc*y*sa*math.Cos(2.0*degToRad(sunMeanLong))-
			0.5*y*y*sl4-1.25*ecc*ecc*sa2,
	)

	if et > 20.0 || et < -20.0 {
		return 0
	}
	return et
}

func toJulianDay(t time.Time) float64 {
	y := float64(t.Year())
	m := float64(t.Month())
	d := float64(t.Day())
	if m <= 2 {
		y--
		m += 12
	}
	a := math.Floor(y / 100.0)
	b := 2.0 - a + math.Floor(a/4.0)
	return math.Floor(365.25*(y+4716.0)) + math.Floor(30.6001*(m+1.0)) + d + b - 1524.5
}

func degToRad(d float64) float64 { return d * math.Pi / 180.0 }
func radToDeg(r float64) float64 { return r * 180.0 / math.Pi }
