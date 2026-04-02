package flightcalc

import (
	"math"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/pkg/solar"
)

// ApplyAutoCalculations computes all auto-calculated fields on a flight.
// Fields with manual override flags set are not overwritten.
func ApplyAutoCalculations(flight *models.Flight) {
	// 0. Auto-determine PIC/Dual from crew: always PIC unless Instructor is on board
	calculatePICDual(flight)

	// 1. Night time — auto-calculate from departure/arrival times + sunset/sunrise
	calculateNightTime(flight)

	// 2. Landing day/night split from total landings
	if !flight.LandingsDayOverride && !flight.LandingsNightOverride {
		calculateLandingSplit(flight)
	}
	flight.AllLandings = flight.LandingsDay + flight.LandingsNight

	// 3. Solo time
	calculateSoloTime(flight)

	// 4. Cross-country time
	calculateCrossCountryTime(flight)

	// 5. Distance from airport coordinates
	calculateDistance(flight)

	// 6. Day/night takeoff split
	if !flight.TakeoffsDayOverride && !flight.TakeoffsNightOverride {
		calculateTakeoffSplit(flight)
	}

	// 7. SIC time: auto-calculated when a crew member has SIC role
	calculateSICTime(flight)

	// 8. Dual given: auto-calculated when a crew member has Instructor role
	calculateDualGivenTime(flight)
}

// calculatePICDual auto-determines PIC/Dual based on crew.
// Always PIC unless an Instructor is on board (crew member with Instructor role),
// in which case it's Dual (instruction received).
func calculatePICDual(flight *models.Flight) {
	hasInstructor := false
	for _, m := range flight.CrewMembers {
		if m.Role == models.CrewRoleInstructor {
			hasInstructor = true
			break
		}
	}
	if hasInstructor {
		flight.IsPIC = false
		flight.IsDual = true
		flight.DualTime = flight.TotalTime
		flight.PICTime = 0
	} else {
		flight.IsPIC = true
		flight.IsDual = false
		flight.PICTime = flight.TotalTime
		flight.DualTime = 0
	}
}

// calculateNightTime computes night time from the flight's departure/arrival
// times and the sunset/sunrise at a representative airport (departure).
// The night portion is the fraction of total time between sunset and sunrise.
func calculateNightTime(flight *models.Flight) {
	dep := normalizeICAO(flight.DepartureICAO)
	if dep == "" || flight.DepartureTime == nil || flight.ArrivalTime == nil {
		return
	}

	depAP := airports.Lookup(dep)
	if depAP == nil {
		return
	}

	depTime, err := parseTimeOfDay(flight.Date, *flight.DepartureTime)
	if err != nil {
		return
	}
	arrTime, err := parseTimeOfDay(flight.Date, *flight.ArrivalTime)
	if err != nil {
		return
	}
	// Handle overnight flights
	if arrTime.Before(depTime) {
		arrTime = arrTime.Add(24 * time.Hour)
	}

	sun := solar.Calculate(flight.Date, depAP.Latitude, depAP.Longitude)
	sunset := sun.Sunset
	sunrise := sun.Sunrise
	// Next day sunrise for overnight flights
	nextSunrise := solar.Calculate(flight.Date.AddDate(0, 0, 1), depAP.Latitude, depAP.Longitude).Sunrise

	totalMinutes := arrTime.Sub(depTime).Minutes()
	if totalMinutes <= 0 {
		flight.NightTime = 0
		return
	}

	nightMinutes := 0.0
	// Walk through flight time in 1-minute increments
	current := depTime
	for current.Before(arrTime) {
		isNight := current.Before(sunrise) || current.After(sunset)
		// Also check next-day sunrise for overnight flights
		if current.After(sunset) && current.Before(nextSunrise) {
			isNight = true
		}
		if isNight {
			nightMinutes++
		}
		current = current.Add(time.Minute)
	}

	nightHours := math.Round(nightMinutes/60.0*10) / 10
	if nightHours > flight.TotalTime {
		nightHours = flight.TotalTime
	}
	flight.NightTime = nightHours
}

func calculateSoloTime(flight *models.Flight) {
	if flight.IsPIC && !flight.IsDual {
		flight.SoloTime = flight.TotalTime
	} else {
		flight.SoloTime = 0
	}
}

func calculateCrossCountryTime(flight *models.Flight) {
	dep := normalizeICAO(flight.DepartureICAO)
	arr := normalizeICAO(flight.ArrivalICAO)
	if dep != "" && arr != "" && dep != arr {
		flight.CrossCountryTime = flight.TotalTime
	} else {
		flight.CrossCountryTime = 0
	}
}

func calculateDistance(flight *models.Flight) {
	dep := normalizeICAO(flight.DepartureICAO)
	arr := normalizeICAO(flight.ArrivalICAO)
	if dep == "" || arr == "" {
		flight.Distance = 0
		return
	}
	depAP := airports.Lookup(dep)
	arrAP := airports.Lookup(arr)
	if depAP == nil || arrAP == nil {
		flight.Distance = 0
		return
	}
	flight.Distance = haversineNM(depAP.Latitude, depAP.Longitude, arrAP.Latitude, arrAP.Longitude)
}

func calculateTakeoffSplit(flight *models.Flight) {
	total := flight.TakeoffsDay + flight.TakeoffsNight
	if total == 0 {
		if flight.AllLandings > 0 || flight.LandingsDay > 0 || flight.LandingsNight > 0 {
			total = 1
		} else {
			return
		}
	}

	dep := normalizeICAO(flight.DepartureICAO)
	if dep == "" || flight.DepartureTime == nil {
		if total > 0 && flight.TakeoffsDay == 0 && flight.TakeoffsNight == 0 {
			flight.TakeoffsDay = total
		}
		return
	}

	depAP := airports.Lookup(dep)
	if depAP == nil {
		flight.TakeoffsDay = total
		return
	}

	depTime, err := parseTimeOfDay(flight.Date, *flight.DepartureTime)
	if err != nil {
		flight.TakeoffsDay = total
		return
	}

	if solar.IsNight(depTime, depAP.Latitude, depAP.Longitude) {
		flight.TakeoffsNight = total
		flight.TakeoffsDay = 0
	} else {
		flight.TakeoffsDay = total
		flight.TakeoffsNight = 0
	}
}

func calculateLandingSplit(flight *models.Flight) {
	total := flight.AllLandings
	if total == 0 {
		total = flight.LandingsDay + flight.LandingsNight
	}
	if total == 0 {
		flight.LandingsDay = 0
		flight.LandingsNight = 0
		return
	}

	arr := normalizeICAO(flight.ArrivalICAO)
	if arr == "" || flight.ArrivalTime == nil {
		// Can't determine day/night — default all landings to day
		flight.LandingsDay = total
		flight.LandingsNight = 0
		return
	}

	arrAP := airports.Lookup(arr)
	if arrAP == nil {
		// Unknown airport — default all landings to day
		flight.LandingsDay = total
		flight.LandingsNight = 0
		return
	}

	arrTime, err := parseTimeOfDay(flight.Date, *flight.ArrivalTime)
	if err != nil {
		// Can't parse time — default all landings to day
		flight.LandingsDay = total
		flight.LandingsNight = 0
		return
	}

	if solar.IsNight(arrTime, arrAP.Latitude, arrAP.Longitude) {
		flight.LandingsNight = total
		flight.LandingsDay = 0
	} else {
		flight.LandingsDay = total
		flight.LandingsNight = 0
	}
}

func haversineNM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusNM = 3440.065
	dLat := degToRad(lat2 - lat1)
	dLon := degToRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(degToRad(lat1))*math.Cos(degToRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return math.Round(earthRadiusNM*c*10) / 10
}

func degToRad(d float64) float64 {
	return d * math.Pi / 180.0
}

func normalizeICAO(icao *string) string {
	if icao == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(*icao))
}

func parseTimeOfDay(date time.Time, timeStr string) (time.Time, error) {
	t, err := time.Parse("15:04:05", timeStr)
	if err != nil {
		t, err = time.Parse("15:04", timeStr)
		if err != nil {
			return time.Time{}, err
		}
	}
	return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC), nil
}

// calculateSICTime sets SIC time when a crew member has the SIC role.
// SIC is mutually exclusive with PIC — if isPIC is true, SIC time is 0.
func calculateSICTime(flight *models.Flight) {
	if flight.IsPIC {
		flight.SICTime = 0
		return
	}
	for _, m := range flight.CrewMembers {
		if m.Role == models.CrewRoleSIC {
			flight.SICTime = flight.TotalTime
			return
		}
	}
	// Don't zero out if no crew members — keep manually set value
}

// calculateDualGivenTime sets dual given time when the user is acting as instructor
// (indicated by a crew member with Instructor role, meaning user gave instruction).
func calculateDualGivenTime(flight *models.Flight) {
	for _, m := range flight.CrewMembers {
		if m.Role == models.CrewRoleInstructor {
			flight.DualGivenTime = flight.TotalTime
			return
		}
	}
	// Don't zero out if no crew members — keep manually set value
}
