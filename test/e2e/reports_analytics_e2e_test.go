//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
	"time"
)

// analyticsResponse mirrors the parts of GET /reports/analytics the tests
// assert on, as a hand-written subset.
type analyticsResponse struct {
	Range struct {
		Months  int     `json:"months"`
		AllTime bool    `json:"allTime"`
		From    *string `json:"from"`
		To      *string `json:"to"`
	} `json:"range"`
	Totals struct {
		TotalFlights          int     `json:"totalFlights"`
		TotalMinutes          int     `json:"totalMinutes"`
		PICMinutes            int     `json:"picMinutes"`
		DualMinutes           int     `json:"dualMinutes"`
		NightMinutes          int     `json:"nightMinutes"`
		IFRMinutes            int     `json:"ifrMinutes"`
		LandingsDay           int     `json:"landingsDay"`
		LandingsNight         int     `json:"landingsNight"`
		Approaches            int     `json:"approaches"`
		Holds                 int     `json:"holds"`
		DistanceNm            float64 `json:"distanceNm"`
		DistinctRegistrations int     `json:"distinctRegistrations"`
		DistinctTypes         int     `json:"distinctTypes"`
		DistinctAirports      int     `json:"distinctAirports"`
		DistinctCountries     int     `json:"distinctCountries"`
		FirstFlightDate       *string `json:"firstFlightDate"`
		LastFlightDate        *string `json:"lastFlightDate"`
	} `json:"totals"`
	Monthly []struct {
		Month             string `json:"month"`
		Flights           int    `json:"flights"`
		TotalMinutes      int    `json:"totalMinutes"`
		CumulativeMinutes int    `json:"cumulativeMinutes"`
	} `json:"monthly"`
	Yearly []struct {
		Year         int `json:"year"`
		Flights      int `json:"flights"`
		TotalMinutes int `json:"totalMinutes"`
	} `json:"yearly"`
	ByAircraftType []analyticsAircraftRow `json:"byAircraftType"`
	ByRegistration []analyticsAircraftRow `json:"byRegistration"`
	ByAirport      []struct {
		Icao       string  `json:"icao"`
		Name       *string `json:"name"`
		Country    *string `json:"country"`
		Departures int     `json:"departures"`
		Arrivals   int     `json:"arrivals"`
		Flights    int     `json:"flights"`
	} `json:"byAirport"`
	ByCountry []struct {
		Country  string `json:"country"`
		Airports int    `json:"airports"`
		Flights  int    `json:"flights"`
	} `json:"byCountry"`
	ByRoute []struct {
		DepartureIcao string  `json:"departureIcao"`
		ArrivalIcao   string  `json:"arrivalIcao"`
		Flights       int     `json:"flights"`
		DistanceNm    float64 `json:"distanceNm"`
	} `json:"byRoute"`
	ByInstructor []struct {
		Name         string `json:"name"`
		Flights      int    `json:"flights"`
		TotalMinutes int    `json:"totalMinutes"`
	} `json:"byInstructor"`
	ByCrew []struct {
		Name         string  `json:"name"`
		Role         *string `json:"role"`
		Flights      int     `json:"flights"`
		TotalMinutes int     `json:"totalMinutes"`
	} `json:"byCrew"`
	ApproachTypes []struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	} `json:"approachTypes"`
	DayOfWeek       []analyticsBucket `json:"dayOfWeek"`
	HourOfDay       []analyticsBucket `json:"hourOfDay"`
	MonthOfYear     []analyticsBucket `json:"monthOfYear"`
	DurationBuckets []analyticsBucket `json:"durationBuckets"`
	Baseline        *analyticsBaseline `json:"baseline"`
	Records         struct {
		LongestFlight *struct {
			Date         string `json:"date"`
			TotalMinutes int    `json:"totalMinutes"`
		} `json:"longestFlight"`
		BusiestDay          *string  `json:"busiestDay"`
		BusiestDayFlights   int      `json:"busiestDayFlights"`
		BusiestMonth        *string  `json:"busiestMonth"`
		ActiveMonths        int      `json:"activeMonths"`
		LongestStreakMonths int      `json:"longestStreakMonths"`
		CurrentStreakMonths int      `json:"currentStreakMonths"`
		DaysSinceLastFlight *int     `json:"daysSinceLastFlight"`
		HomeBase            *string  `json:"homeBase"`
		FarthestAirportNm   *float64 `json:"farthestAirportNm"`
	} `json:"records"`
}

type analyticsAircraftRow struct {
	Label        string  `json:"label"`
	SubLabel     *string `json:"subLabel"`
	Flights      int     `json:"flights"`
	TotalMinutes int     `json:"totalMinutes"`
	Landings     int     `json:"landings"`
}

type analyticsBaseline struct {
	BaselineDate string `json:"baselineDate"`
	TotalFlights int    `json:"totalFlights"`
	TotalMinutes int    `json:"totalMinutes"`
	PICMinutes   int    `json:"picMinutes"`
	LandingsDay  int    `json:"landingsDay"`
}

type analyticsBucket struct {
	Key          int    `json:"key"`
	Label        string `json:"label"`
	Flights      int    `json:"flights"`
	TotalMinutes int    `json:"totalMinutes"`
}

func getAnalytics(t *testing.T, c *E2EClient, query string) analyticsResponse {
	t.Helper()
	resp := c.GET("/reports/analytics" + query)
	requireStatus(t, resp, http.StatusOK)
	var a analyticsResponse
	if err := resp.JSON(&a); err != nil {
		t.Fatalf("Failed to decode analytics response: %v", err)
	}
	return a
}

func TestReportsAnalyticsEmptyLogbook(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("analytics-empty"), "SecurePass123!", "Analytics Empty")

	a := getAnalytics(t, c, "")

	if a.Totals.TotalFlights != 0 || a.Totals.TotalMinutes != 0 {
		t.Errorf("Expected zeroed totals, got %d flights / %d minutes",
			a.Totals.TotalFlights, a.Totals.TotalMinutes)
	}
	if a.Totals.FirstFlightDate != nil || a.Totals.LastFlightDate != nil {
		t.Error("Expected null first/last flight dates on an empty logbook")
	}
	if a.Records.DaysSinceLastFlight != nil {
		t.Error("Expected null daysSinceLastFlight on an empty logbook")
	}

	// Ranked breakdowns are empty; the fixed-axis histograms stay dense.
	if len(a.Monthly) != 0 || len(a.ByAircraftType) != 0 || len(a.ByAirport) != 0 {
		t.Error("Expected empty breakdowns on an empty logbook")
	}
	for _, tc := range []struct {
		name string
		got  []analyticsBucket
		want int
	}{
		{"dayOfWeek", a.DayOfWeek, 7},
		{"hourOfDay", a.HourOfDay, 24},
		{"monthOfYear", a.MonthOfYear, 12},
		{"durationBuckets", a.DurationBuckets, 6},
	} {
		if len(tc.got) != tc.want {
			t.Errorf("%s: expected %d dense buckets, got %d", tc.name, tc.want, len(tc.got))
		}
	}
}

// seedAnalyticsLogbook creates a small but varied logbook: two types, two
// registrations, a dual flight with an instructor, a night flight, an IFR
// flight with structured approaches, and a crew member.
func seedAnalyticsLogbook(t *testing.T, c *E2EClient) {
	t.Helper()

	flights := []map[string]interface{}{
		{
			// 90 min PIC, EDNY -> EDDS.
			"date": pastDate(5), "aircraftReg": "D-EAAA", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30",
			"totalTime": 90, "picTime": 90, "landings": 2,
		},
		{
			// 60 min dual received, instructor on the crew (EASA
			// auto-calculation classifies it as dual).
			"date": pastDate(12), "aircraftReg": "D-EAAA", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "10:00", "onBlockTime": "11:00",
			"totalTime": 60, "dualTime": 60, "landings": 5,
			"instructorName": "M. Keller",
			"crewMembers": []map[string]string{
				{"name": "M. Keller", "role": "Instructor"},
			},
		},
		{
			// 120 min night + IFR with two structured approaches and a hold.
			"date": pastDate(20), "aircraftReg": "D-EBBB", "aircraftType": "PA28",
			"departureIcao": "EDNY", "arrivalIcao": "LSZH",
			"offBlockTime": "20:00", "onBlockTime": "22:00",
			"totalTime": 120, "picTime": 120, "ifrTime": 90,
			"actualInstrumentTime": 45, "holds": 1, "landings": 1,
			"approaches": []map[string]string{
				{"type": "ILS", "airport": "LSZH"},
				{"type": "RNAV/GPS", "airport": "LSZH"},
			},
			"crewMembers": []map[string]string{
				{"name": "J. Moreau", "role": "SIC"},
			},
		},
	}
	for i, f := range flights {
		resp := c.POST("/flights", f)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Failed to create seed flight %d: status %d, body %s",
				i, resp.StatusCode, string(resp.Body))
		}
	}
}

func TestReportsAnalyticsTotalsAndBreakdowns(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("analytics"), "SecurePass123!", "Analytics")
	seedAnalyticsLogbook(t, c)

	a := getAnalytics(t, c, "?months=0")

	if !a.Range.AllTime {
		t.Error("Expected allTime range for months=0")
	}
	if a.Range.From != nil {
		t.Errorf("Expected null range.from for all time, got %v", *a.Range.From)
	}

	// ── Totals ──
	if a.Totals.TotalFlights != 3 {
		t.Errorf("Expected 3 flights, got %d", a.Totals.TotalFlights)
	}
	if want := 90 + 60 + 120; a.Totals.TotalMinutes != want {
		t.Errorf("Expected %d total minutes, got %d", want, a.Totals.TotalMinutes)
	}
	if want := 90 + 120; a.Totals.PICMinutes != want {
		t.Errorf("Expected %d PIC minutes, got %d", want, a.Totals.PICMinutes)
	}
	if a.Totals.DualMinutes != 60 {
		t.Errorf("Expected 60 dual minutes, got %d", a.Totals.DualMinutes)
	}
	if a.Totals.IFRMinutes != 90 {
		t.Errorf("Expected 90 IFR minutes, got %d", a.Totals.IFRMinutes)
	}
	if a.Totals.Holds != 1 {
		t.Errorf("Expected 1 hold, got %d", a.Totals.Holds)
	}
	if a.Totals.DistinctRegistrations != 2 {
		t.Errorf("Expected 2 registrations, got %d", a.Totals.DistinctRegistrations)
	}
	if a.Totals.DistinctTypes != 2 {
		t.Errorf("Expected 2 types, got %d", a.Totals.DistinctTypes)
	}
	// EDNY, EDDS, LSZH.
	if a.Totals.DistinctAirports != 3 {
		t.Errorf("Expected 3 distinct airports, got %d", a.Totals.DistinctAirports)
	}
	// Germany and Switzerland.
	if a.Totals.DistinctCountries != 2 {
		t.Errorf("Expected 2 distinct countries, got %d", a.Totals.DistinctCountries)
	}
	if a.Totals.DistanceNm <= 0 {
		t.Errorf("Expected a positive total distance, got %f", a.Totals.DistanceNm)
	}

	// ── Series consistency ──
	var monthlyFlights, monthlyMinutes int
	for _, m := range a.Monthly {
		monthlyFlights += m.Flights
		monthlyMinutes += m.TotalMinutes
	}
	if monthlyFlights != a.Totals.TotalFlights || monthlyMinutes != a.Totals.TotalMinutes {
		t.Errorf("Monthly series (%d flights / %d min) does not sum to totals (%d / %d)",
			monthlyFlights, monthlyMinutes, a.Totals.TotalFlights, a.Totals.TotalMinutes)
	}
	var yearlyMinutes int
	for _, y := range a.Yearly {
		yearlyMinutes += y.TotalMinutes
	}
	if yearlyMinutes != a.Totals.TotalMinutes {
		t.Errorf("Yearly series (%d min) does not sum to totals (%d)",
			yearlyMinutes, a.Totals.TotalMinutes)
	}

	// Cumulative hours must be non-decreasing and, over all time with no
	// initial-hours baseline, end exactly at the total.
	prev := -1
	for _, m := range a.Monthly {
		if m.CumulativeMinutes < prev {
			t.Errorf("Cumulative minutes decreased at %s: %d after %d", m.Month, m.CumulativeMinutes, prev)
		}
		prev = m.CumulativeMinutes
	}
	if len(a.Monthly) > 0 {
		if last := a.Monthly[len(a.Monthly)-1].CumulativeMinutes; last != a.Totals.TotalMinutes {
			t.Errorf("Cumulative curve ends at %d, expected %d", last, a.Totals.TotalMinutes)
		}
	}

	// ── Aircraft ──
	if len(a.ByAircraftType) != 2 {
		t.Fatalf("Expected 2 aircraft types, got %d", len(a.ByAircraftType))
	}
	// Sorted by time descending: C172 has 150 min, PA28 has 120.
	if a.ByAircraftType[0].Label != "C172" || a.ByAircraftType[0].TotalMinutes != 150 {
		t.Errorf("Expected C172 with 150 min first, got %s with %d",
			a.ByAircraftType[0].Label, a.ByAircraftType[0].TotalMinutes)
	}
	if len(a.ByRegistration) != 2 || a.ByRegistration[0].Label != "D-EAAA" {
		t.Errorf("Expected D-EAAA to lead byRegistration, got %+v", a.ByRegistration)
	}

	// ── Places ──
	// EDNY is the departure of all three flights and the arrival of one, but
	// that local flight must not be double-counted.
	var edny *struct {
		Icao       string  `json:"icao"`
		Name       *string `json:"name"`
		Country    *string `json:"country"`
		Departures int     `json:"departures"`
		Arrivals   int     `json:"arrivals"`
		Flights    int     `json:"flights"`
	}
	for i := range a.ByAirport {
		if a.ByAirport[i].Icao == "EDNY" {
			edny = &a.ByAirport[i]
		}
	}
	if edny == nil {
		t.Fatalf("Expected EDNY in byAirport, got %+v", a.ByAirport)
	}
	if edny.Departures != 3 || edny.Arrivals != 1 {
		t.Errorf("EDNY: expected 3 departures / 1 arrival, got %d / %d", edny.Departures, edny.Arrivals)
	}
	if edny.Flights != 3 {
		t.Errorf("EDNY: expected 3 distinct flights (local flight counted once), got %d", edny.Flights)
	}
	if edny.Name == nil || *edny.Name == "" {
		t.Error("Expected EDNY to resolve to an airport name")
	}
	if edny.Country == nil || *edny.Country != "DE" {
		t.Errorf("Expected EDNY country DE, got %v", edny.Country)
	}
	if a.Records.HomeBase == nil || *a.Records.HomeBase != "EDNY" {
		t.Errorf("Expected home base EDNY, got %v", a.Records.HomeBase)
	}
	if a.Records.FarthestAirportNm == nil || *a.Records.FarthestAirportNm <= 0 {
		t.Error("Expected a positive distance to the farthest airport")
	}

	// ── People ──
	if len(a.ByInstructor) != 1 || a.ByInstructor[0].Name != "M. Keller" || a.ByInstructor[0].TotalMinutes != 60 {
		t.Errorf("Expected 60 min of dual with M. Keller, got %+v", a.ByInstructor)
	}
	crewRoles := map[string]string{}
	for _, cm := range a.ByCrew {
		role := ""
		if cm.Role != nil {
			role = *cm.Role
		}
		crewRoles[cm.Name] = role
	}
	if len(a.ByCrew) != 2 {
		t.Errorf("Expected 2 crew members, got %d: %+v", len(a.ByCrew), a.ByCrew)
	}
	if crewRoles["J. Moreau"] != "SIC" {
		t.Errorf("Expected J. Moreau logged as SIC, got %q", crewRoles["J. Moreau"])
	}
	if crewRoles["M. Keller"] != "Instructor" {
		t.Errorf("Expected M. Keller logged as Instructor, got %q", crewRoles["M. Keller"])
	}

	// ── Instrument ──
	var approachTotal int
	seen := map[string]int{}
	for _, ap := range a.ApproachTypes {
		approachTotal += ap.Count
		seen[ap.Type] = ap.Count
	}
	if approachTotal != a.Totals.Approaches {
		t.Errorf("Approach breakdown sums to %d but totals report %d", approachTotal, a.Totals.Approaches)
	}
	if seen["ILS"] != 1 || seen["RNAV/GPS"] != 1 {
		t.Errorf("Expected one ILS and one RNAV/GPS approach, got %+v", seen)
	}

	// ── Patterns ──
	for _, tc := range []struct {
		name string
		got  []analyticsBucket
	}{
		{"dayOfWeek", a.DayOfWeek},
		{"monthOfYear", a.MonthOfYear},
		{"durationBuckets", a.DurationBuckets},
	} {
		var sum int
		for _, b := range tc.got {
			sum += b.Flights
		}
		if sum != a.Totals.TotalFlights {
			t.Errorf("%s buckets sum to %d flights, expected %d", tc.name, sum, a.Totals.TotalFlights)
		}
	}
	// All three seed flights record an off-block time; the hour histogram
	// covers every flight.
	var hourSum int
	for _, b := range a.HourOfDay {
		hourSum += b.Flights
	}
	if hourSum != a.Totals.TotalFlights {
		t.Errorf("hourOfDay buckets sum to %d flights, expected %d", hourSum, a.Totals.TotalFlights)
	}

	// ── Records ──
	if a.Records.LongestFlight == nil || a.Records.LongestFlight.TotalMinutes != 120 {
		t.Errorf("Expected the 120 min flight to be the longest, got %+v", a.Records.LongestFlight)
	}
	if a.Records.ActiveMonths != len(a.Monthly) {
		t.Errorf("activeMonths (%d) should match the number of monthly points (%d)",
			a.Records.ActiveMonths, len(a.Monthly))
	}
	if a.Records.DaysSinceLastFlight == nil || *a.Records.DaysSinceLastFlight != 5 {
		t.Errorf("Expected 5 days since the last flight, got %v", a.Records.DaysSinceLastFlight)
	}
	if a.Records.BusiestDay == nil || a.Records.BusiestDayFlights != 1 {
		t.Errorf("Expected a busiest day with 1 flight, got %v / %d",
			a.Records.BusiestDay, a.Records.BusiestDayFlights)
	}
}

func TestReportsAnalyticsTimeframeScoping(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("analytics-scope"), "SecurePass123!", "Analytics Scope")

	// One recent flight and one from roughly three years ago.
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(3), "aircraftReg": "D-ENEW", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00",
		"totalTime": 60, "picTime": 60, "landings": 1,
	}), http.StatusCreated)
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date":        time.Now().AddDate(-3, 0, 0).Format("2006-01-02"),
		"aircraftReg": "D-EOLD", "aircraftType": "PA28",
		"departureIcao": "EDNY", "arrivalIcao": "EDMA",
		"offBlockTime": "08:00", "onBlockTime": "10:00",
		"totalTime": 120, "picTime": 120, "landings": 1,
	}), http.StatusCreated)

	all := getAnalytics(t, c, "?months=0")
	if all.Totals.TotalFlights != 2 || all.Totals.TotalMinutes != 180 {
		t.Errorf("All time: expected 2 flights / 180 min, got %d / %d",
			all.Totals.TotalFlights, all.Totals.TotalMinutes)
	}

	recent := getAnalytics(t, c, "?months=6")
	if recent.Range.AllTime {
		t.Error("Expected allTime=false for months=6")
	}
	if recent.Range.From == nil {
		t.Fatal("Expected a range.from for a bounded timeframe")
	}
	if recent.Totals.TotalFlights != 1 || recent.Totals.TotalMinutes != 60 {
		t.Errorf("6 months: expected 1 flight / 60 min, got %d / %d",
			recent.Totals.TotalFlights, recent.Totals.TotalMinutes)
	}
	if recent.Totals.DistinctTypes != 1 {
		t.Errorf("6 months: expected 1 aircraft type in range, got %d", recent.Totals.DistinctTypes)
	}

	// The cumulative curve carries the out-of-range hours forward.
	if len(recent.Monthly) == 0 {
		t.Fatal("Expected at least one month in the 6-month window")
	}
	if last := recent.Monthly[len(recent.Monthly)-1].CumulativeMinutes; last != 180 {
		t.Errorf("Expected the cumulative curve to reach 180 career minutes, got %d", last)
	}

	// Days since last flight ignores the timeframe.
	if recent.Records.DaysSinceLastFlight == nil || *recent.Records.DaysSinceLastFlight != 3 {
		t.Errorf("Expected 3 days since last flight regardless of timeframe, got %v",
			recent.Records.DaysSinceLastFlight)
	}
}

// TestReportsAnalyticsBaseline covers the initial-hours snapshot: it counts
// into the Reports totals and agrees with GET /users/me/statistics.
func TestReportsAnalyticsBaseline(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("analytics-baseline"), "SecurePass123!", "Analytics Baseline")

	// 500h carried over from a paper logbook, cut off three years ago.
	baselineDate := time.Now().AddDate(-3, 0, 0).Format("2006-01-02")
	requireStatus(t, c.PUT("/users/me/baseline", map[string]interface{}{
		"baselineDate": baselineDate,
		"totalFlights": 400,
		"totalMinutes": 30000,
		"picMinutes":   24000,
		"landingsDay":  600,
	}), http.StatusOK)

	// One logged flight on top: 60 min PIC, 1 day landing.
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(3), "aircraftReg": "D-ENEW", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00",
		"totalTime": 60, "picTime": 60, "landings": 1,
	}), http.StatusCreated)

	all := getAnalytics(t, c, "?months=0")

	if all.Baseline == nil {
		t.Fatal("All time: expected the baseline contribution to be reported")
	}
	if all.Baseline.TotalMinutes != 30000 || all.Baseline.BaselineDate != baselineDate {
		t.Errorf("All time: unexpected baseline block %+v", *all.Baseline)
	}
	if want := 30000 + 60; all.Totals.TotalMinutes != want {
		t.Errorf("All time: expected %d total minutes including the baseline, got %d",
			want, all.Totals.TotalMinutes)
	}
	if want := 400 + 1; all.Totals.TotalFlights != want {
		t.Errorf("All time: expected %d flights including the baseline, got %d",
			want, all.Totals.TotalFlights)
	}
	if want := 24000 + 60; all.Totals.PICMinutes != want {
		t.Errorf("All time: expected %d PIC minutes including the baseline, got %d",
			want, all.Totals.PICMinutes)
	}
	if want := 600 + 1; all.Totals.LandingsDay != want {
		t.Errorf("All time: expected %d day landings including the baseline, got %d",
			want, all.Totals.LandingsDay)
	}

	// Totals agree with GET /users/me/statistics.
	statsResp := c.GET("/users/me/statistics")
	requireStatus(t, statsResp, http.StatusOK)
	var stats struct {
		TotalFlights int `json:"totalFlights"`
		TotalMinutes int `json:"totalMinutes"`
		PICMinutes   int `json:"picMinutes"`
	}
	if err := statsResp.JSON(&stats); err != nil {
		t.Fatalf("Failed to decode statistics response: %v", err)
	}
	if stats.TotalMinutes != all.Totals.TotalMinutes || stats.TotalFlights != all.Totals.TotalFlights ||
		stats.PICMinutes != all.Totals.PICMinutes {
		t.Errorf("Reports totals (%d flights / %d min / %d PIC) disagree with dashboard statistics (%d / %d / %d)",
			all.Totals.TotalFlights, all.Totals.TotalMinutes, all.Totals.PICMinutes,
			stats.TotalFlights, stats.TotalMinutes, stats.PICMinutes)
	}

	// The cumulative curve starts from the carried-forward hours and, over all
	// time, ends exactly at the totals.
	if len(all.Monthly) == 0 {
		t.Fatal("Expected at least one month in the all-time series")
	}
	if last := all.Monthly[len(all.Monthly)-1].CumulativeMinutes; last != all.Totals.TotalMinutes {
		t.Errorf("Cumulative curve ends at %d, expected %d", last, all.Totals.TotalMinutes)
	}

	// A window that starts after the baseline cutoff reports logged time only.
	recent := getAnalytics(t, c, "?months=6")
	if recent.Baseline != nil {
		t.Errorf("6 months: expected no baseline contribution, got %+v", *recent.Baseline)
	}
	if recent.Totals.TotalMinutes != 60 || recent.Totals.TotalFlights != 1 {
		t.Errorf("6 months: expected 1 flight / 60 min without the baseline, got %d / %d",
			recent.Totals.TotalFlights, recent.Totals.TotalMinutes)
	}
	// The baseline is still carried into the cumulative curve.
	if len(recent.Monthly) == 0 {
		t.Fatal("Expected at least one month in the 6-month window")
	}
	if last := recent.Monthly[len(recent.Monthly)-1].CumulativeMinutes; last != 30060 {
		t.Errorf("Expected the cumulative curve to reach 30060 career minutes, got %d", last)
	}
}

func TestReportsAnalyticsValidationAndAuth(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("analytics-validation"), "SecurePass123!", "Analytics Validation")

	for _, q := range []string{"?months=-1", "?months=601", "?limit=0", "?limit=201", "?months=notanumber"} {
		t.Run("rejects "+q, func(t *testing.T) {
			assertStatus(t, c.GET("/reports/analytics"+q), http.StatusBadRequest)
		})
	}

	t.Run("limit caps ranked breakdowns", func(t *testing.T) {
		seedAnalyticsLogbook(t, c)
		a := getAnalytics(t, c, "?months=0&limit=1")
		if len(a.ByAircraftType) != 1 {
			t.Errorf("Expected limit=1 to return a single aircraft type, got %d", len(a.ByAircraftType))
		}
		if len(a.ByAirport) != 1 {
			t.Errorf("Expected limit=1 to return a single airport, got %d", len(a.ByAirport))
		}
		// Dense histograms are not ranked breakdowns and ignore the limit.
		if len(a.DayOfWeek) != 7 {
			t.Errorf("Expected dayOfWeek to stay dense under limit=1, got %d", len(a.DayOfWeek))
		}
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/reports/analytics"), http.StatusUnauthorized)
	})
}
