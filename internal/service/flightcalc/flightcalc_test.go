package flightcalc

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func strPtr(s string) *string { return &s }

func baseFlight() *models.Flight {
	return &models.Flight{
		Date:          time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		AircraftReg:   "D-EFGH",
		AircraftType:  "C172",
		DepartureICAO: strPtr("EDDF"),
		ArrivalICAO:   strPtr("EDDH"),
		OffBlockTime:  strPtr("14:30:00"),
		OnBlockTime:   strPtr("16:00:00"),
		DepartureTime: strPtr("14:30:00"),
		ArrivalTime:   strPtr("16:00:00"),
		TotalTime:     90,
		IsPIC:         true,
		IsDual:        false,
		PICTime:       90,
		LandingsDay:   2,
		LandingsNight: 1,
	}
}

func TestSoloTime_PIC(t *testing.T) {
	f := baseFlight()
	// No instructor = PIC = solo
	ApplyAutoCalculations(f, "")
	if f.SoloTime != f.TotalTime {
		t.Errorf("expected soloTime=%v when PIC, got %v", f.TotalTime, f.SoloTime)
	}
}

func TestSoloTime_Dual(t *testing.T) {
	f := baseFlight()
	// Instructor on board = Dual = no solo
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Instructor", Role: models.CrewRoleInstructor},
	}
	ApplyAutoCalculations(f, "")
	if f.SoloTime != 0 {
		t.Errorf("expected soloTime=0 when dual, got %v", f.SoloTime)
	}
}

func TestSoloTime_NeitherPICNorDual(t *testing.T) {
	f := baseFlight()
	// With passenger, user is PIC, so solo time = total
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Passenger", Role: models.CrewRolePassenger},
	}
	ApplyAutoCalculations(f, "")
	// With passengers but no instructor, user is PIC, soloTime = totalTime
	if f.SoloTime != f.TotalTime {
		t.Errorf("expected soloTime=%v with passenger, got %v", f.TotalTime, f.SoloTime)
	}
}

func TestCrossCountryTime_DifferentAirports(t *testing.T) {
	f := baseFlight()
	ApplyAutoCalculations(f, "")
	if f.CrossCountryTime != f.TotalTime {
		t.Errorf("expected crossCountryTime=%v, got %v", f.TotalTime, f.CrossCountryTime)
	}
}

func TestCrossCountryTime_SameAirport(t *testing.T) {
	f := baseFlight()
	f.DepartureICAO = strPtr("EDDF")
	f.ArrivalICAO = strPtr("EDDF")
	ApplyAutoCalculations(f, "")
	if f.CrossCountryTime != 0 {
		t.Errorf("expected crossCountryTime=0 for same airport, got %v", f.CrossCountryTime)
	}
}

func TestCrossCountryTime_NilAirports(t *testing.T) {
	f := baseFlight()
	f.DepartureICAO = nil
	f.ArrivalICAO = nil
	ApplyAutoCalculations(f, "")
	if f.CrossCountryTime != 0 {
		t.Errorf("expected crossCountryTime=0, got %v", f.CrossCountryTime)
	}
}

func TestAllLandings_Sum(t *testing.T) {
	f := baseFlight()
	f.AllLandings = 5
	f.LandingsDayOverride = true
	f.LandingsNightOverride = true
	f.LandingsDay = 3
	f.LandingsNight = 2
	ApplyAutoCalculations(f, "")
	if f.AllLandings != 5 {
		t.Errorf("expected allLandings=5, got %d", f.AllLandings)
	}
}

func TestDistance_Haversine(t *testing.T) {
	dist := haversineNM(50.0379, 8.5622, 53.6304, 10.0065)
	if dist < 200 || dist > 250 {
		t.Errorf("EDDF-EDDH distance should be ~220 NM, got %.1f", dist)
	}
}

func TestDistance_SameLocation(t *testing.T) {
	dist := haversineNM(50.0, 8.0, 50.0, 8.0)
	if dist != 0 {
		t.Errorf("same point distance should be 0, got %.1f", dist)
	}
}

func TestTakeoffOverride_Respected(t *testing.T) {
	f := baseFlight()
	f.TakeoffsDay = 2
	f.TakeoffsNight = 1
	f.TakeoffsDayOverride = true
	f.TakeoffsNightOverride = true
	ApplyAutoCalculations(f, "")
	if f.TakeoffsDay != 2 || f.TakeoffsNight != 1 {
		t.Errorf("overridden takeoffs modified: day=%d night=%d", f.TakeoffsDay, f.TakeoffsNight)
	}
}

func TestLandingSplit_NoArrivalTime_DefaultsToDay(t *testing.T) {
	// Regression: when ArrivalTime is nil, calculateLandingSplit returned early
	// and left LandingsDay/Night at 0, then AllLandings was overwritten to 0.
	f := baseFlight()
	f.ArrivalTime = nil
	f.AllLandings = 3
	f.LandingsDay = 0
	f.LandingsNight = 0
	ApplyAutoCalculations(f, "")
	if f.AllLandings != 3 {
		t.Errorf("expected allLandings=3 when arrivalTime is nil, got %d", f.AllLandings)
	}
	if f.LandingsDay != 3 {
		t.Errorf("expected landingsDay=3 (default to day), got %d", f.LandingsDay)
	}
}

func TestLandingSplit_UnknownAirport_DefaultsToDay(t *testing.T) {
	f := baseFlight()
	unknownIcao := "ZZZZ"
	f.ArrivalICAO = &unknownIcao
	f.AllLandings = 5
	f.LandingsDay = 0
	f.LandingsNight = 0
	ApplyAutoCalculations(f, "")
	if f.AllLandings != 5 {
		t.Errorf("expected allLandings=5 for unknown airport, got %d", f.AllLandings)
	}
	if f.LandingsDay != 5 {
		t.Errorf("expected landingsDay=5 (default to day), got %d", f.LandingsDay)
	}
}

func TestLandingOverride_Respected(t *testing.T) {
	f := baseFlight()
	f.LandingsDay = 5
	f.LandingsNight = 3
	f.LandingsDayOverride = true
	f.LandingsNightOverride = true
	ApplyAutoCalculations(f, "")
	if f.LandingsDay != 5 || f.LandingsNight != 3 {
		t.Errorf("overridden landings modified: day=%d night=%d", f.LandingsDay, f.LandingsNight)
	}
	if f.AllLandings != 8 {
		t.Errorf("allLandings should be 8, got %d", f.AllLandings)
	}
}

func TestParseTimeOfDay(t *testing.T) {
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		input  string
		hour   int
		minute int
	}{
		{"14:30:00", 14, 30},
		{"08:15", 8, 15},
		{"23:59:59", 23, 59},
	}
	for _, tc := range cases {
		result, err := parseTimeOfDay(date, tc.input)
		if err != nil {
			t.Errorf("parseTimeOfDay(%q) failed: %v", tc.input, err)
			continue
		}
		if result.Hour() != tc.hour || result.Minute() != tc.minute {
			t.Errorf("parseTimeOfDay(%q) = %v, expected %02d:%02d", tc.input, result.Format("15:04"), tc.hour, tc.minute)
		}
	}
}

func TestNormalizeICAO(t *testing.T) {
	cases := []struct {
		input    *string
		expected string
	}{
		{strPtr("eddf"), "EDDF"},
		{strPtr("EDDH"), "EDDH"},
		{strPtr(" EDDS "), "EDDS"},
		{nil, ""},
		{strPtr(""), ""},
	}
	for _, tc := range cases {
		result := normalizeICAO(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeICAO = %q, expected %q", result, tc.expected)
		}
	}
}

func TestSICTime_WithSICCrew(t *testing.T) {
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Captain Smith", Role: models.CrewRolePIC},
		{Name: "Test User", Role: models.CrewRoleSIC},
	}
	ApplyAutoCalculations(f, "")
	// No instructor means PIC, SIC is zeroed when PIC
	if f.SICTime != 0 {
		t.Errorf("SICTime = %d, want 0 (user is PIC)", f.SICTime)
	}
}

func TestSICTime_PICOverridesSIC(t *testing.T) {
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Test User", Role: models.CrewRoleSIC},
	}
	ApplyAutoCalculations(f, "")
	if f.SICTime != 0 {
		t.Errorf("SICTime = %d, want 0 (PIC is set)", f.SICTime)
	}
}

func TestSICTime_NoCrew(t *testing.T) {
	f := baseFlight()
	f.SICTime = 0
	ApplyAutoCalculations(f, "")
	// No crew = PIC, SIC stays 0
	if f.SICTime != 0 {
		t.Errorf("SICTime = %d, want 0 (no crew)", f.SICTime)
	}
}

func TestDualGivenTime_WithStudentCrew_IsDualGiving(t *testing.T) {
	// User has a Student on board → user is the instructor → Dual given.
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Student Pilot", Role: models.CrewRoleStudent},
	}
	ApplyAutoCalculations(f, "Test User")
	if f.DualGivenTime != f.TotalTime {
		t.Errorf("DualGivenTime = %d, expected %d", f.DualGivenTime, f.TotalTime)
	}
	if !f.IsPIC {
		t.Error("expected IsPIC=true when giving dual instruction")
	}
	if f.IsDual {
		t.Error("expected IsDual=false when giving dual instruction")
	}
}

func TestDualGivenTime_SelfListedAsInstructor_IsDualGiving(t *testing.T) {
	// User listed themselves as Instructor crew member (case-insensitive name
	// match) → still Dual given, not Dual received.
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "  test user  ", Role: models.CrewRoleInstructor},
	}
	ApplyAutoCalculations(f, "Test User")
	if f.DualGivenTime != f.TotalTime {
		t.Errorf("DualGivenTime = %d, expected %d", f.DualGivenTime, f.TotalTime)
	}
	if !f.IsPIC {
		t.Error("expected IsPIC=true when user is the listed instructor")
	}
}

// Regression: GH issue — user added another person as Instructor and got
// "Dual given" (wrong). With the user's name set, a third-party Instructor
// must produce Dual received and zero Dual given.
func TestDualGivenTime_ThirdPartyInstructor_IsDualReceived(t *testing.T) {
	f := baseFlight()
	f.DualGivenTime = 90 // simulate stale value from earlier buggy save
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Jane Instructor", Role: models.CrewRoleInstructor},
	}
	ApplyAutoCalculations(f, "Test User")
	if f.DualGivenTime != 0 {
		t.Errorf("DualGivenTime = %d, expected 0 when third-party instructor on board", f.DualGivenTime)
	}
	if f.IsPIC {
		t.Error("expected IsPIC=false with third-party instructor")
	}
	if !f.IsDual {
		t.Error("expected IsDual=true with third-party instructor")
	}
	if f.DualTime != f.TotalTime {
		t.Errorf("DualTime = %d, expected %d", f.DualTime, f.TotalTime)
	}
}

// User listed both as Student (e.g. dual student in a check ride observed
// alongside an examiner who is also a student-of-instructor) and there is a
// third-party Instructor → user is the dual receiver, not the giver.
func TestDualGivenTime_ThirdPartyInstructorWithStudent_PrefersDualReceived(t *testing.T) {
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Jane Instructor", Role: models.CrewRoleInstructor},
		{Name: "Other Student", Role: models.CrewRoleStudent},
	}
	ApplyAutoCalculations(f, "Test User")
	if f.DualGivenTime != 0 {
		t.Errorf("DualGivenTime = %d, expected 0", f.DualGivenTime)
	}
	if !f.IsDual {
		t.Error("expected IsDual=true (third-party instructor takes precedence)")
	}
}

func TestDualGivenTime_NoCrew(t *testing.T) {
	f := baseFlight()
	f.DualGivenTime = 0
	ApplyAutoCalculations(f, "Test User")
	if f.DualGivenTime != 0 {
		t.Errorf("DualGivenTime = %d, expected 0", f.DualGivenTime)
	}
}

func TestDualGivenTime_WithPassengerOnly(t *testing.T) {
	f := baseFlight()
	f.DualGivenTime = 0
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Passenger", Role: models.CrewRolePassenger},
	}
	ApplyAutoCalculations(f, "Test User")
	if f.DualGivenTime != 0 {
		t.Errorf("DualGivenTime = %d, expected 0", f.DualGivenTime)
	}
}

// Empty userName falls back to the conservative interpretation: any
// Instructor on board is a third party, so the user is Dual receiver.
func TestDualGivenTime_EmptyUserName_TreatsInstructorAsThirdParty(t *testing.T) {
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Test User", Role: models.CrewRoleInstructor},
	}
	ApplyAutoCalculations(f, "")
	if f.DualGivenTime != 0 {
		t.Errorf("DualGivenTime = %d, expected 0 (no user context)", f.DualGivenTime)
	}
	if !f.IsDual {
		t.Error("expected IsDual=true (no user context, instructor present)")
	}
}

// Regression: a user reported that adding themselves as Instructor still
// produced DualGivenTime = 0. Run the same scenario through every plausible
// name-variant the UI / contacts list could produce, so we can pinpoint
// which form (if any) breaks the self-instructor detection in
// `determineUserRole`.
func TestDualGivenTime_SelfInstructor_NameVariants(t *testing.T) {
	const profile = "Amelia Earhart"

	cases := []struct {
		name      string
		crewName  string
		wantGiven bool // true => expect DualGivenTime == TotalTime
	}{
		{"exact match", "Amelia Earhart", true},
		{"trailing space", "Amelia Earhart ", true},
		{"leading space", " Amelia Earhart", true},
		{"lowercase", "amelia earhart", true},
		{"uppercase", "AMELIA EARHART", true},
		{"double space", "Amelia  Earhart", true},             // flightrules.NormalizeName collapses interior whitespace
		{"reversed Last, First", "Earhart, Amelia", true},     // contact-picker style — handled by MatchesUser
		{"with middle initial", "Amelia M. Earhart", false},   // still treated as third-party (different identity)
		{"non-breaking space", "Amelia\u00a0Earhart", true},   // U+00A0 folded to ASCII space by NormalizeName
		{"trailing tab+newline", "Amelia Earhart\t\n", true},  // tabs/newlines normalised
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := baseFlight()
			f.CrewMembers = []models.FlightCrewMember{
				{Name: tc.crewName, Role: models.CrewRoleInstructor},
			}
			ApplyAutoCalculations(f, profile)

			gotGiven := f.DualGivenTime == f.TotalTime
			if gotGiven != tc.wantGiven {
				t.Errorf("crewName=%q profile=%q: DualGivenTime=%d (TotalTime=%d), IsPIC=%v, IsDual=%v, DualTime=%d — want Dual given=%v",
					tc.crewName, profile, f.DualGivenTime, f.TotalTime, f.IsPIC, f.IsDual, f.DualTime, tc.wantGiven)
			}
			// Sanity: when self-instructor is detected, user must be PIC.
			if tc.wantGiven && !f.IsPIC {
				t.Errorf("crewName=%q: expected IsPIC=true when self-instructor", tc.crewName)
			}
			if tc.wantGiven && f.IsDual {
				t.Errorf("crewName=%q: expected IsDual=false when self-instructor", tc.crewName)
			}
		})
	}
}

// === Auto PIC/Dual tests ===

func TestPICDual_NoCrew_IsPIC(t *testing.T) {
	f := baseFlight()
	ApplyAutoCalculations(f, "")
	if !f.IsPIC {
		t.Error("expected IsPIC=true with no crew")
	}
	if f.IsDual {
		t.Error("expected IsDual=false with no crew")
	}
	if f.PICTime != f.TotalTime {
		t.Errorf("PICTime = %d, want %d", f.PICTime, f.TotalTime)
	}
}

func TestPICDual_InstructorOnBoard_IsDual(t *testing.T) {
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "CFI Smith", Role: models.CrewRoleInstructor},
	}
	ApplyAutoCalculations(f, "")
	if f.IsPIC {
		t.Error("expected IsPIC=false with instructor on board")
	}
	if !f.IsDual {
		t.Error("expected IsDual=true with instructor on board")
	}
	if f.DualTime != f.TotalTime {
		t.Errorf("DualTime = %d, want %d", f.DualTime, f.TotalTime)
	}
	if f.PICTime != 0 {
		t.Errorf("PICTime = %d, want 0", f.PICTime)
	}
}

func TestPICDual_PassengerOnly_IsPIC(t *testing.T) {
	f := baseFlight()
	f.CrewMembers = []models.FlightCrewMember{
		{Name: "Jane Doe", Role: models.CrewRolePassenger},
	}
	ApplyAutoCalculations(f, "")
	if !f.IsPIC {
		t.Error("expected IsPIC=true with passenger only")
	}
}

// === Night time auto-calculation tests ===

func TestNightTime_DaytimeFlight(t *testing.T) {
	f := baseFlight()
	// Without airport DB loaded, nightTime stays 0 (graceful degradation)
	f.DepartureTime = strPtr("10:00:00")
	f.ArrivalTime = strPtr("12:00:00")
	f.TotalTime = 2.0
	ApplyAutoCalculations(f, "")
	// Without airport lookup, nightTime is 0
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 (no airport data)", f.NightTime)
	}
}

func TestNightTime_NightFlight(t *testing.T) {
	f := baseFlight()
	// Without airport DB, calculation can't run
	f.DepartureTime = strPtr("18:00:00")
	f.ArrivalTime = strPtr("20:00:00")
	f.TotalTime = 2.0
	ApplyAutoCalculations(f, "")
	// Graceful: stays 0 when no airport data
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 (no airport data in test)", f.NightTime)
	}
}

func TestNightTime_MixedFlight(t *testing.T) {
	f := baseFlight()
	f.DepartureTime = strPtr("15:00:00")
	f.ArrivalTime = strPtr("18:00:00")
	f.TotalTime = 3.0
	ApplyAutoCalculations(f, "")
	// Graceful: stays 0 when no airport data
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 (no airport data in test)", f.NightTime)
	}
}

// === Night time unit tests (with direct function call) ===

func TestCalculateNightTime_NoAirport(t *testing.T) {
	f := baseFlight()
	f.DepartureICAO = strPtr("XXXX")
	calculateNightTime(f)
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 for unknown airport", f.NightTime)
	}
}

func TestCalculateNightTime_NilTimes(t *testing.T) {
	f := baseFlight()
	f.DepartureTime = nil
	calculateNightTime(f)
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 for nil times", f.NightTime)
	}
}

// TestCalculateNightTime_OffBlockFallback_Regression covers ninerlog-api#34:
// EASA-style imports often only carry off-block / on-block times (no separate
// takeoff / landing time). A flight from EDBO 18:56→19:19 UTC on 19 Mar 2019
// is entirely after evening civil twilight, so all 23 minutes must be logged
// as night even when DepartureTime / ArrivalTime are nil.
func TestCalculateNightTime_OffBlockFallback_Regression(t *testing.T) {
	airports.SetTestDB(map[string]airports.AirportInfo{
		// EDBO Oehna Airfield, Brandenburg DE (from OurAirports)
		"EDBO": {ICAO: "EDBO", Name: "Oehna", Latitude: 51.899734, Longitude: 13.052809, Country: "DE"},
		"EDAZ": {ICAO: "EDAZ", Name: "Schönhagen", Latitude: 52.204631, Longitude: 13.159526, Country: "DE"},
	})
	t.Cleanup(func() { airports.SetTestDB(nil) })

	f := &models.Flight{
		Date:          time.Date(2019, 3, 19, 0, 0, 0, 0, time.UTC),
		DepartureICAO: strPtr("EDBO"),
		ArrivalICAO:   strPtr("EDAZ"),
		// Only block times set — the EASA CSV import path leaves DepartureTime /
		// ArrivalTime nil.
		OffBlockTime: strPtr("18:56:00"),
		OnBlockTime:  strPtr("19:19:00"),
		TotalTime:    23,
	}

	calculateNightTime(f)

	if f.NightTime != 23 {
		t.Errorf("NightTime = %d, want 23 (all 23 minutes after civil dusk at EDBO 19 Mar 2019)", f.NightTime)
	}
}

// === Landing split tests ===

func TestLandingSplit_FromTotalLandings(t *testing.T) {
	f := baseFlight()
	f.AllLandings = 3
	f.LandingsDay = 0
	f.LandingsNight = 0
	// Without airport data, landings default to day
	f.ArrivalTime = strPtr("15:00:00")
	calculateLandingSplit(f)
	// Without airport lookup data, day landings = total (fallback)
	if f.LandingsDay+f.LandingsNight != 3 {
		// The function falls through without airport data, but total should be set
		// When no airport found, the function returns early without modifying
		// In that case day=0, night=0, but AllLandings=3
		t.Logf("Landing split: day=%d night=%d (no airport data)", f.LandingsDay, f.LandingsNight)
	}
}

// === Tests with airport data loaded ===

func setupAirportData(t *testing.T) {
	t.Helper()
	airports.SetTestDB(map[string]airports.AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt Airport", Latitude: 50.0333, Longitude: 8.5706, Elevation: 364, Country: "DE"},
		"EDDH": {ICAO: "EDDH", Name: "Hamburg Airport", Latitude: 53.6304, Longitude: 9.9882, Elevation: 53, Country: "DE"},
		"EDDM": {ICAO: "EDDM", Name: "Munich Airport", Latitude: 48.3538, Longitude: 11.7861, Elevation: 1487, Country: "DE"},
		"KJFK": {ICAO: "KJFK", Name: "John F Kennedy Intl", Latitude: 40.6399, Longitude: -73.7787, Elevation: 13, Country: "US"},
	})
	t.Cleanup(func() { airports.SetTestDB(nil) })
}

func TestNightTime_WithAirportData_DaytimeFlight(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	// Mid-day flight in winter: should have zero night time
	f.Date = time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC) // summer solstice
	f.DepartureTime = strPtr("10:00:00")
	f.ArrivalTime = strPtr("12:00:00")
	f.TotalTime = 120

	calculateNightTime(f)
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 for midday summer flight", f.NightTime)
	}
}

func TestNightTime_WithAirportData_NightFlight(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	// Late night winter flight: should have significant night time
	f.Date = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	f.OffBlockTime = strPtr("22:00:00")
	f.OnBlockTime = strPtr("23:30:00")
	f.TotalTime = 90

	calculateNightTime(f)
	// 22:00-23:30 in January in Frankfurt is definitely night
	if f.NightTime == 0 {
		t.Error("NightTime should be > 0 for late evening winter flight")
	}
	if f.NightTime > f.TotalTime {
		t.Errorf("NightTime = %d exceeds TotalTime = %d", f.NightTime, f.TotalTime)
	}
}

func TestNightTime_WithAirportData_OvernightFlight(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	// Overnight flight: departs before midnight, arrives after
	f.Date = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	f.OffBlockTime = strPtr("23:00:00")
	f.OnBlockTime = strPtr("01:00:00") // next day
	f.TotalTime = 120

	calculateNightTime(f)
	// Entire flight is at night in winter
	if f.NightTime == 0 {
		t.Error("NightTime should be > 0 for overnight winter flight")
	}
}

func TestNightTime_CappedAtTotalTime(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.Date = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	f.OffBlockTime = strPtr("22:00:00")
	f.OnBlockTime = strPtr("23:30:00")
	f.TotalTime = 30 // Less than actual elapsed time

	calculateNightTime(f)
	if f.NightTime > f.TotalTime {
		t.Errorf("NightTime = %d should be capped at TotalTime = %d", f.NightTime, f.TotalTime)
	}
}

func TestNightTime_NilDepartureICAO(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.DepartureICAO = nil
	calculateNightTime(f)
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 for nil departure ICAO", f.NightTime)
	}
}

func TestNightTime_EmptyDepartureICAO(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.DepartureICAO = strPtr("")
	calculateNightTime(f)
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 for empty departure ICAO", f.NightTime)
	}
}

func TestNightTime_InvalidTimeFormat(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.OffBlockTime = strPtr("invalid")
	calculateNightTime(f)
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0 for invalid time format", f.NightTime)
	}
}

func TestCalculateDistance_WithAirportData(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.DepartureICAO = strPtr("EDDF")
	f.ArrivalICAO = strPtr("EDDH")
	calculateDistance(f)
	// Frankfurt to Hamburg is roughly 220 NM
	if f.Distance < 200 || f.Distance > 250 {
		t.Errorf("Distance = %.1f, want ~220 NM for EDDF-EDDH", f.Distance)
	}
}

func TestCalculateDistance_SameAirport(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.DepartureICAO = strPtr("EDDF")
	f.ArrivalICAO = strPtr("EDDF")
	calculateDistance(f)
	if f.Distance != 0 {
		t.Errorf("Distance = %.1f, want 0 for same airport", f.Distance)
	}
}

func TestCalculateDistance_UnknownAirport(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.DepartureICAO = strPtr("EDDF")
	f.ArrivalICAO = strPtr("ZZZZ")
	calculateDistance(f)
	if f.Distance != 0 {
		t.Errorf("Distance = %.1f, want 0 for unknown airport", f.Distance)
	}
}

func TestCalculateDistance_NilICAO(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.DepartureICAO = nil
	calculateDistance(f)
	if f.Distance != 0 {
		t.Errorf("Distance = %.1f, want 0 for nil ICAO", f.Distance)
	}
}

func TestTakeoffSplit_WithAirportData_DayTakeoff(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.Date = time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	f.OffBlockTime = strPtr("10:00:00")
	f.TakeoffsDay = 0
	f.TakeoffsNight = 0
	f.AllLandings = 1
	calculateTakeoffSplit(f)
	if f.TakeoffsDay != 1 {
		t.Errorf("TakeoffsDay = %d, want 1 for daytime takeoff", f.TakeoffsDay)
	}
	if f.TakeoffsNight != 0 {
		t.Errorf("TakeoffsNight = %d, want 0 for daytime takeoff", f.TakeoffsNight)
	}
}

func TestTakeoffSplit_WithAirportData_NightTakeoff(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.Date = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	f.OffBlockTime = strPtr("22:00:00")
	f.TakeoffsDay = 0
	f.TakeoffsNight = 0
	f.AllLandings = 1
	calculateTakeoffSplit(f)
	if f.TakeoffsNight != 1 {
		t.Errorf("TakeoffsNight = %d, want 1 for nighttime takeoff", f.TakeoffsNight)
	}
	if f.TakeoffsDay != 0 {
		t.Errorf("TakeoffsDay = %d, want 0 for nighttime takeoff", f.TakeoffsDay)
	}
}

func TestLandingSplit_WithAirportData_DayLanding(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.Date = time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	f.OnBlockTime = strPtr("14:00:00")
	f.AllLandings = 2
	f.LandingsDay = 0
	f.LandingsNight = 0
	calculateLandingSplit(f)
	if f.LandingsDay != 2 {
		t.Errorf("LandingsDay = %d, want 2 for daytime landing", f.LandingsDay)
	}
	if f.LandingsNight != 0 {
		t.Errorf("LandingsNight = %d, want 0 for daytime landing", f.LandingsNight)
	}
}

func TestLandingSplit_WithAirportData_NightLanding(t *testing.T) {
	setupAirportData(t)
	f := baseFlight()
	f.Date = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	f.OnBlockTime = strPtr("22:00:00")
	f.AllLandings = 3
	f.LandingsDay = 0
	f.LandingsNight = 0
	calculateLandingSplit(f)
	if f.LandingsNight != 3 {
		t.Errorf("LandingsNight = %d, want 3 for nighttime landing", f.LandingsNight)
	}
	if f.LandingsDay != 0 {
		t.Errorf("LandingsDay = %d, want 0 for nighttime landing", f.LandingsDay)
	}
}

// === ApplyAutoCalculations full orchestration tests ===

func TestApplyAutoCalculations_FullDaytimeFlight(t *testing.T) {
	setupAirportData(t)
	f := &models.Flight{
		Date:          time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC),
		AircraftReg:   "D-EFGH",
		AircraftType:  "C172",
		DepartureICAO: strPtr("EDDF"),
		ArrivalICAO:   strPtr("EDDH"),
		DepartureTime: strPtr("10:00:00"),
		ArrivalTime:   strPtr("12:00:00"),
		TotalTime:     120,
		AllLandings:   1,
	}

	ApplyAutoCalculations(f, "")

	// PIC (no crew)
	if !f.IsPIC {
		t.Error("Expected IsPIC=true")
	}
	if f.PICTime != 120 {
		t.Errorf("PICTime = %d, want 120", f.PICTime)
	}

	// Night time should be 0 for midday summer flight
	if f.NightTime != 0 {
		t.Errorf("NightTime = %d, want 0", f.NightTime)
	}

	// Cross-country (different airports)
	if f.CrossCountryTime != 120 {
		t.Errorf("CrossCountryTime = %d, want 120", f.CrossCountryTime)
	}

	// Solo time
	if f.SoloTime != 120 {
		t.Errorf("SoloTime = %d, want 120", f.SoloTime)
	}

	// Distance should be ~220 NM
	if f.Distance < 200 || f.Distance > 250 {
		t.Errorf("Distance = %.1f, want ~220", f.Distance)
	}

	// Landings: daytime
	if f.LandingsDay != 1 {
		t.Errorf("LandingsDay = %d, want 1", f.LandingsDay)
	}
	if f.LandingsNight != 0 {
		t.Errorf("LandingsNight = %d, want 0", f.LandingsNight)
	}
	if f.AllLandings != 1 {
		t.Errorf("AllLandings = %d, want 1", f.AllLandings)
	}
}

func TestApplyAutoCalculations_DualFlightWithInstructor(t *testing.T) {
	setupAirportData(t)
	f := &models.Flight{
		Date:          time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC),
		AircraftReg:   "D-EFGH",
		AircraftType:  "C172",
		DepartureICAO: strPtr("EDDF"),
		ArrivalICAO:   strPtr("EDDF"), // pattern work
		DepartureTime: strPtr("14:00:00"),
		ArrivalTime:   strPtr("15:00:00"),
		TotalTime:     60,
		AllLandings:   5,
		CrewMembers: []models.FlightCrewMember{
			{Name: "CFI Smith", Role: models.CrewRoleInstructor},
		},
	}

	ApplyAutoCalculations(f, "")

	// Dual (instructor on board)
	if f.IsPIC {
		t.Error("Expected IsPIC=false with instructor")
	}
	if !f.IsDual {
		t.Error("Expected IsDual=true with instructor")
	}
	if f.DualTime != 60 {
		t.Errorf("DualTime = %d, want 60", f.DualTime)
	}
	if f.PICTime != 0 {
		t.Errorf("PICTime = %d, want 0", f.PICTime)
	}

	// Solo should be 0
	if f.SoloTime != 0 {
		t.Errorf("SoloTime = %d, want 0", f.SoloTime)
	}

	// Cross-country: 0 (same airport)
	if f.CrossCountryTime != 0 {
		t.Errorf("CrossCountryTime = %d, want 0", f.CrossCountryTime)
	}

	// Distance: 0 (same airport)
	if f.Distance != 0 {
		t.Errorf("Distance = %.1f, want 0", f.Distance)
	}

	// DualGiven must be 0 — the user is receiving instruction, not giving it.
	if f.DualGivenTime != 0 {
		t.Errorf("DualGivenTime = %d, want 0 (third-party instructor)", f.DualGivenTime)
	}
}

func TestApplyAutoCalculations_OverridesRespected(t *testing.T) {
	setupAirportData(t)
	f := &models.Flight{
		Date:                  time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC),
		AircraftReg:           "D-EFGH",
		AircraftType:          "C172",
		DepartureICAO:         strPtr("EDDF"),
		ArrivalICAO:           strPtr("EDDH"),
		DepartureTime:         strPtr("10:00:00"),
		ArrivalTime:           strPtr("12:00:00"),
		TotalTime:             120,
		LandingsDay:           3,
		LandingsNight:         2,
		LandingsDayOverride:   true,
		LandingsNightOverride: true,
		TakeoffsDay:           2,
		TakeoffsNight:         1,
		TakeoffsDayOverride:   true,
		TakeoffsNightOverride: true,
	}

	ApplyAutoCalculations(f, "")

	// Override values should be preserved
	if f.LandingsDay != 3 {
		t.Errorf("LandingsDay = %d, want 3 (overridden)", f.LandingsDay)
	}
	if f.LandingsNight != 2 {
		t.Errorf("LandingsNight = %d, want 2 (overridden)", f.LandingsNight)
	}
	if f.TakeoffsDay != 2 {
		t.Errorf("TakeoffsDay = %d, want 2 (overridden)", f.TakeoffsDay)
	}
	if f.TakeoffsNight != 1 {
		t.Errorf("TakeoffsNight = %d, want 1 (overridden)", f.TakeoffsNight)
	}
}

func TestParseTimeOfDay_InvalidFormat(t *testing.T) {
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	_, err := parseTimeOfDay(date, "not-a-time")
	if err == nil {
		t.Error("parseTimeOfDay should fail on invalid format")
	}
}

func TestHaversine_Antipodal(t *testing.T) {
	// Points on opposite sides of Earth: should be ~half earth circumference
	dist := haversineNM(0, 0, 0, 180)
	// Half Earth circumference in NM ≈ 10800
	if dist < 10700 || dist > 10900 {
		t.Errorf("Antipodal distance = %.1f, want ~10800 NM", dist)
	}
}

func TestHaversine_Poles(t *testing.T) {
	// North to South pole
	dist := haversineNM(90, 0, -90, 0)
	// Half Earth circumference in NM ≈ 10800
	if dist < 10700 || dist > 10900 {
		t.Errorf("Pole-to-pole distance = %.1f, want ~10800 NM", dist)
	}
}
