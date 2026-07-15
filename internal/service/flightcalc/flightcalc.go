package flightcalc

import (
	"math"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/solar"
)

// Role enum + classification moved to internal/service/flightrules so the
// read path (handlers, exporters, PDF) and the write path (this package)
// cannot disagree about who is PIC. The flightcalc package keeps a thin
// alias to avoid touching every internal call site.
type userPilotRole = flightrules.Role

const (
	rolePIC           = flightrules.RolePIC
	roleDualReceiving = flightrules.RoleDualReceiving
	roleDualGiving    = flightrules.RoleDualGiving
	roleSIC           = flightrules.RoleSIC
)

// ApplyAutoCalculations computes all auto-calculated fields on a flight.
// Fields with manual override flags set are not overwritten.
//
// userName is the authenticated user's display name. It is used to decide
// whether an Instructor crew member refers to the user themselves (→ Dual
// given) or to a third party (→ Dual received), and likewise whether an
// Examiner is a third party (→ Dual received; the examiner is PIC of record
// on a check ride) or the user themselves (→ PIC). When userName is empty,
// any Instructor or Examiner crew member is conservatively treated as a
// third party (Dual received), preserving prior behaviour for callers that
// do not yet have the user context (e.g. some legacy tests).
func ApplyAutoCalculations(flight *models.Flight, userName string) {
	role := determineUserRole(flight, userName)

	// 0. Auto-determine PIC/Dual from crew + user role
	calculatePICDual(flight, role)

	// 1. Night time — auto-calculate from departure/arrival times + sunset/sunrise
	calculateNightTime(flight)

	// 2. Landing day/night split from total landings
	if !flight.LandingsDayOverride && !flight.LandingsNightOverride {
		calculateLandingSplit(flight)
	}
	flight.AllLandings = flight.LandingsDay + flight.LandingsNight

	// 3. Solo time
	calculateSoloTime(flight, userName)

	// 4. Cross-country time
	calculateCrossCountryTime(flight)

	// 5. Distance from airport coordinates
	calculateDistance(flight)

	// 6. Day/night takeoff split
	if !flight.TakeoffsDayOverride && !flight.TakeoffsNightOverride {
		calculateTakeoffSplit(flight)
	}

	// 7. SIC time: auto-calculated when the user is the co-pilot
	calculateSICTime(flight, role)

	// 8. Dual given: only when the user is acting as instructor
	calculateDualGivenTime(flight, role)

	// 8b. Multi-pilot time: auto-filled when the crew indicates a
	//     multi-pilot operation (user is SIC, or a SIC crew member exists)
	calculateMultiPilotTime(flight, role)

	// 9. IFR time: if user did not set it explicitly, derive from
	//    Actual + Simulated instrument (capped at TotalTime).
	flight.IFRTime = flightrules.EffectiveIFRTime(flight)
}

// determineUserRole is a thin wrapper over flightrules.DetermineRole kept
// for backward compatibility with this package's internal call sites.
func determineUserRole(flight *models.Flight, userName string) userPilotRole {
	return flightrules.DetermineRole(flight, userName)
}

// calculatePICDual sets PIC/Dual flags and times based on the resolved user
// role. A user giving instruction is also PIC of the flight. A user flying
// as co-pilot (SIC) on a multi-pilot operation logs neither PIC nor Dual —
// only the designated PIC logs PIC time (AMC1 FCL.050).
func calculatePICDual(flight *models.Flight, role userPilotRole) {
	switch role {
	case roleDualReceiving:
		flight.IsPIC = false
		flight.IsDual = true
		flight.DualTime = flight.TotalTime
		flight.PICTime = 0
	case roleSIC:
		flight.IsPIC = false
		flight.IsDual = false
		flight.PICTime = 0
		flight.DualTime = 0
	default:
		// rolePIC and roleDualGiving — user is PIC.
		flight.IsPIC = true
		flight.IsDual = false
		flight.PICTime = flight.TotalTime
		flight.DualTime = 0
	}
}

// calculateNightTime computes night time from the flight's off-block /
// on-block times and the civil twilight boundaries at the departure airport.
// Per ICAO / EASA, night is the period between the end of evening civil
// twilight and the beginning of morning civil twilight.
//
// Block times are used exclusively (not takeoff/landing times): they are the
// authoritative recorded times in this logbook, populated by every import
// path and required for any flight to be valid.
func calculateNightTime(flight *models.Flight) {
	dep := normalizeICAO(flight.DepartureICAO)
	if dep == "" {
		return
	}

	if flight.OffBlockTime == nil || flight.OnBlockTime == nil ||
		strings.TrimSpace(*flight.OffBlockTime) == "" ||
		strings.TrimSpace(*flight.OnBlockTime) == "" {
		return
	}

	depAP := airports.Lookup(dep)
	if depAP == nil {
		return
	}

	depTime, err := parseTimeOfDay(flight.Date, *flight.OffBlockTime)
	if err != nil {
		return
	}
	arrTime, err := parseTimeOfDay(flight.Date, *flight.OnBlockTime)
	if err != nil {
		return
	}
	// Handle overnight flights
	if arrTime.Before(depTime) {
		arrTime = arrTime.Add(24 * time.Hour)
	}

	tw := solar.CivilTwilight(flight.Date, depAP.Latitude, depAP.Longitude)
	dusk := tw.Dusk
	dawn := tw.Dawn
	// Next day morning civil twilight for overnight flights
	nextDawn := solar.CivilTwilight(flight.Date.AddDate(0, 0, 1), depAP.Latitude, depAP.Longitude).Dawn

	totalMinutes := arrTime.Sub(depTime).Minutes()
	if totalMinutes <= 0 {
		flight.NightTime = 0
		return
	}

	nightMinutes := 0
	// Walk through flight time in 1-minute increments
	current := depTime
	for current.Before(arrTime) {
		isNight := current.Before(dawn) || current.After(dusk)
		// Also check next-day dawn for overnight flights
		if current.After(dusk) && current.Before(nextDawn) {
			isNight = true
		}
		if isNight {
			nightMinutes++
		}
		current = current.Add(time.Minute)
	}

	if nightMinutes > flight.TotalTime {
		nightMinutes = flight.TotalTime
	}
	flight.NightTime = nightMinutes
}

// calculateSoloTime sets solo time when the user is PIC and the sole
// occupant of the aircraft (FCL.050 / FOCA GM/INFO §1.6: solo flight time
// means flight time during which the pilot is the sole occupant). Any crew
// member other than the user themselves — passenger, co-pilot, safety
// pilot — means the flight is not solo.
func calculateSoloTime(flight *models.Flight, userName string) {
	if !flight.IsPIC || flight.IsDual {
		flight.SoloTime = 0
		return
	}
	for _, m := range flight.CrewMembers {
		if userName == "" || !flightrules.MatchesUser(m.Name, userName) {
			flight.SoloTime = 0
			return
		}
	}
	flight.SoloTime = flight.TotalTime
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
	if dep == "" || flight.OffBlockTime == nil || strings.TrimSpace(*flight.OffBlockTime) == "" {
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

	depTime, err := parseTimeOfDay(flight.Date, *flight.OffBlockTime)
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
	if arr == "" || flight.OnBlockTime == nil || strings.TrimSpace(*flight.OnBlockTime) == "" {
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

	arrTime, err := parseTimeOfDay(flight.Date, *flight.OnBlockTime)
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

// calculateSICTime sets SIC (co-pilot) time when the user's resolved role is
// SIC — someone else is the designated PIC and the user occupies the other
// pilot seat (AMC1 FCL.050). In all other roles the time is zeroed when crew
// context exists, so stale values do not survive a recalculation.
func calculateSICTime(flight *models.Flight, role userPilotRole) {
	if role == roleSIC {
		flight.SICTime = flight.TotalTime
		return
	}
	if len(flight.CrewMembers) > 0 {
		flight.SICTime = 0
		return
	}
	// Don't zero out if no crew members — keep manually set value
}

// calculateMultiPilotTime fills the multi-pilot column (EASA AMC1 FCL.050
// Col 10) when the crew composition indicates a multi-pilot operation: both
// the designated PIC and the co-pilot log the full flight time there. A
// manually entered non-zero value is preserved (e.g. augmented-crew ops
// where each pilot logs a fraction of block time). When crew context exists
// but does not indicate a multi-pilot operation, a stale value is zeroed;
// without any crew the manual value is kept (user-declared MP aircraft).
func calculateMultiPilotTime(flight *models.Flight, role userPilotRole) {
	if flightrules.IsMultiPilotOperation(flight, role) {
		if flight.MultiPilotTime == 0 {
			flight.MultiPilotTime = flight.TotalTime
		}
		return
	}
	if len(flight.CrewMembers) > 0 {
		flight.MultiPilotTime = 0
		return
	}
	// No crew at all — leave any manually entered value untouched.
}

// calculateDualGivenTime sets dual given time when the user is acting as
// instructor. Per determineUserRole, that means a Student is on board OR the
// user themselves is listed with the Instructor role. In all other cases this
// time is zeroed so stale values do not survive a recalculation (e.g. when a
// flight is re-saved after fixing crew roles).
func calculateDualGivenTime(flight *models.Flight, role userPilotRole) {
	if role == roleDualGiving {
		flight.DualGivenTime = flight.TotalTime
		return
	}
	if len(flight.CrewMembers) > 0 {
		// We have crew context and the user is not the instructor → force 0.
		flight.DualGivenTime = 0
		return
	}
	// No crew at all — leave any manually entered value untouched.
}
