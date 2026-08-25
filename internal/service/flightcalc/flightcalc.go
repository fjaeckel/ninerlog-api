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

// Role enum + classification live in internal/service/flightrules; this
// package keeps thin aliases for its internal call sites.
type userPilotRole = flightrules.Role

const (
	rolePIC           = flightrules.RolePIC
	roleDualReceiving = flightrules.RoleDualReceiving
	roleDualGiving    = flightrules.RoleDualGiving
	roleSIC           = flightrules.RoleSIC
	rolePassenger     = flightrules.RolePassenger
)

// ApplyAutoCalculations computes all auto-calculated fields on a flight.
// Fields with manual override flags set are not overwritten.
//
// userName is the authenticated user's display name, used to decide whether
// an Instructor crew member is the user themselves (→ Dual given) or a third
// party (→ Dual received), and likewise whether an Examiner is a third party
// (→ Dual received) or the user themselves (→ PIC). When userName is empty,
// any Instructor or Examiner crew member is treated as a third party (Dual
// received).
//
// aircraft carries the fleet facts for the registration flown; pass nil when
// the registration has no fleet entry. Co-pilot and multi-pilot time are
// derived from it, so an unknown aircraft is treated as single-pilot and
// never has co-pilot time invented for it.
func ApplyAutoCalculations(flight *models.Flight, userName string, aircraft *flightrules.AircraftFacts) {
	if flight.IsSimulator {
		applySessionCalculations(flight)
		return
	}

	// A zero total is recovered from the block times; a non-zero one is kept.
	if flight.TotalTime == 0 {
		flight.TotalTime = blockMinutes(flight)
	}

	role := determineUserRole(flight, userName, aircraft)
	flight.IsPassenger = role == rolePassenger
	if flight.IsPassenger {
		applyPassengerCalculations(flight)
		return
	}

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

	// 8b. Multi-pilot time: auto-filled when a two-pilot crew flies a
	//     multi-pilot aircraft
	calculateMultiPilotTime(flight, role, aircraft)

	// 9. IFR time: if user did not set it explicitly, derive from
	//    Actual + Simulated instrument (capped at TotalTime).
	flight.IFRTime = flightrules.EffectiveIFRTime(flight)
}

// applySessionCalculations normalises an FSTD session. The device is not
// flown between places, so route, block and pilot-function fields are
// cleared; the session duration in SimulatedFlightTime and the instrument
// work (approaches, holds, simulated instrument time) are kept.
func applySessionCalculations(flight *models.Flight) {
	flight.TotalTime = 0
	flight.IsPIC = false
	flight.IsDual = false
	flight.PICTime = 0
	flight.DualTime = 0
	flight.SICTime = 0
	flight.DualGivenTime = 0
	flight.MultiPilotTime = 0
	flight.PICUSTime = 0
	flight.SPICTime = 0
	flight.ExaminerTime = 0
	flight.ReliefTime = 0
	flight.SoloTime = 0
	flight.CrossCountryTime = 0
	flight.NightTime = 0
	flight.IFRTime = 0
	flight.LandingsDay = 0
	flight.LandingsNight = 0
	flight.AllLandings = 0
	flight.TakeoffsDay = 0
	flight.TakeoffsNight = 0
	flight.Distance = 0
	flight.AircraftReg = ""
	flight.DepartureICAO = nil
	flight.ArrivalICAO = nil
	flight.OffBlockTime = nil
	flight.OnBlockTime = nil
	flight.DepartureTime = nil
	flight.ArrivalTime = nil
	flight.Route = nil
	flight.LaunchMethod = nil

	if flight.SimulatedInstrumentTime > flight.SimulatedFlightTime {
		flight.SimulatedInstrumentTime = flight.SimulatedFlightTime
	}
	// Actual instrument time requires real IMC.
	flight.ActualInstrumentTime = 0
}

// applyPassengerCalculations normalises a flight the user was carried on.
// Route, block times and distance are kept as the record of the trip; every
// flight-time, pilot-function, landing and instrument column is cleared.
func applyPassengerCalculations(flight *models.Flight) {
	flight.TotalTime = 0
	flight.IsPIC = false
	flight.IsDual = false
	flight.PICTime = 0
	flight.DualTime = 0
	flight.SICTime = 0
	flight.DualGivenTime = 0
	flight.MultiPilotTime = 0
	flight.PICUSTime = 0
	flight.SPICTime = 0
	flight.ExaminerTime = 0
	flight.ReliefTime = 0
	flight.SoloTime = 0
	flight.CrossCountryTime = 0
	flight.NightTime = 0
	flight.IFRTime = 0
	flight.LandingsDay = 0
	flight.LandingsNight = 0
	flight.AllLandings = 0
	flight.TakeoffsDay = 0
	flight.TakeoffsNight = 0
	flight.ActualInstrumentTime = 0
	flight.SimulatedInstrumentTime = 0
	flight.SimulatedFlightTime = 0
	flight.Holds = 0
	flight.Approaches = nil
	flight.ApproachesCount = 0

	calculateDistance(flight)
}

// blockMinutes returns the off-block to on-block duration in minutes, or 0
// when either block time is absent or unparseable. Times after midnight are
// treated as the following day.
func blockMinutes(flight *models.Flight) int {
	if flight.OffBlockTime == nil || flight.OnBlockTime == nil {
		return 0
	}
	off, err := parseTimeOfDay(flight.Date, *flight.OffBlockTime)
	if err != nil {
		return 0
	}
	on, err := parseTimeOfDay(flight.Date, *flight.OnBlockTime)
	if err != nil {
		return 0
	}
	if !on.After(off) {
		on = on.Add(24 * time.Hour)
	}
	return int(on.Sub(off).Minutes())
}

// determineUserRole is a thin wrapper over flightrules.DetermineRole.
func determineUserRole(flight *models.Flight, userName string, aircraft *flightrules.AircraftFacts) userPilotRole {
	return flightrules.DetermineRole(flight, userName, aircraft)
}

// declaredFunctionTime returns the pilot-declared function minutes — PICUS,
// SPIC and cruise relief — that carve out of the derived function time for
// the resolved role. Examiner time overlays and is not part of the carve.
func declaredFunctionTime(flight *models.Flight) int {
	return flight.PICUSTime + flight.SPICTime + flight.ReliefTime
}

// derivedFunctionMinutes returns the block time left for the derived function
// column after the declared function times are carved out, floored at zero.
func derivedFunctionMinutes(flight *models.Flight) int {
	m := flight.TotalTime - declaredFunctionTime(flight)
	if m < 0 {
		return 0
	}
	return m
}

// calculatePICDual sets PIC/Dual flags and times based on the resolved user
// role. A user giving instruction is also PIC of the flight. A user flying
// as co-pilot (SIC) on a multi-pilot operation logs neither PIC nor Dual —
// only the designated PIC logs PIC time (AMC1 FCL.050). A passenger logs no
// pilot function time at all. Declared PICUS/SPIC/relief minutes are carved
// out of the derived column so the function times still decompose TotalTime.
func calculatePICDual(flight *models.Flight, role userPilotRole) {
	switch role {
	case roleDualReceiving:
		flight.IsPIC = false
		flight.IsDual = true
		flight.DualTime = derivedFunctionMinutes(flight)
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
		flight.PICTime = derivedFunctionMinutes(flight)
		flight.DualTime = 0
	}
}

// calculateNightTime computes night time from the flight's off-block /
// on-block times and the civil twilight boundaries at the departure airport.
// Per ICAO / EASA, night is the period between the end of evening civil
// twilight and the beginning of morning civil twilight. Block times are used
// exclusively (not takeoff/landing times).
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
// pilot — means the flight is not solo, as does any declared PICUS, SPIC,
// examiner or relief time (each implies another pilot on board).
func calculateSoloTime(flight *models.Flight, userName string) {
	if !flight.IsPIC || flight.IsDual {
		flight.SoloTime = 0
		return
	}
	if flight.PICUSTime > 0 || flight.SPICTime > 0 || flight.ExaminerTime > 0 || flight.ReliefTime > 0 {
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
// SIC — someone else is the designated PIC and the user occupies a co-pilot
// seat the operation provides (AMC1 FCL.050). A co-pilot time the pilot
// declared is left as entered; in every other case the time is zeroed.
// Declared PICUS/SPIC/relief minutes are carved out of the derived value, so
// a full-sector PICUS entry leaves zero co-pilot time.
func calculateSICTime(flight *models.Flight, role userPilotRole) {
	if role != roleSIC {
		flight.SICTime = 0
		return
	}
	if flight.SICTimeOverride {
		return
	}
	flight.SICTime = derivedFunctionMinutes(flight)
}

// calculateMultiPilotTime fills the multi-pilot column (EASA AMC1 FCL.050
// Col 10) when a two-pilot crew flies a multi-pilot aircraft: both the
// designated PIC and the co-pilot log the full flight time there. A value the
// pilot declared is left as entered (e.g. augmented-crew ops where each pilot
// logs a fraction of block time); anything else derivation wrote itself is
// zeroed when the operation is not multi-pilot.
func calculateMultiPilotTime(flight *models.Flight, role userPilotRole, aircraft *flightrules.AircraftFacts) {
	if flight.MultiPilotTimeOverride {
		return
	}
	if flightrules.IsMultiPilotOperation(flight, role, aircraft) {
		flight.MultiPilotTime = flight.TotalTime
		return
	}
	flight.MultiPilotTime = 0
}

// calculateDualGivenTime sets dual given time when the user is acting as
// instructor: a Student is on board OR the user themselves is listed with the
// Instructor role. In all other cases the time is zeroed when crew context
// exists.
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
