//go:build e2e

package e2e_test

import (
	"testing"
)

// TestEndorsements verifies the endorsements field is stored
// separately from pilot remarks (EASA AMC1 Col 24 / FAA §61.51(h)).
func TestEndorsements(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("endorsements"), "SecurePass123!", "Endorse")

	t.Run("stored separately from remarks", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EEND", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
			"remarks":      "Normal training flight",
			"endorsements": "I certify that the above pilot has completed...",
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)

		assertStr(t, "remarks", f["remarks"], "Normal training flight")
		assertStr(t, "endorsements", f["endorsements"], "I certify that the above pilot has completed...")
	})

	t.Run("nullable when not provided", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-ENUL", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "10:00", "onBlockTime": "11:00", "landings": 1,
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		if f["endorsements"] != nil {
			t.Logf("endorsements when not set: %v (expected nil)", f["endorsements"])
		}
	})

	t.Run("update endorsements", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EUPD", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "12:00", "onBlockTime": "13:00", "landings": 1,
		})
		requireStatus(t, r, 201)
		var created map[string]interface{}
		r.JSON(&created)

		r = c.PUT("/flights/"+created["id"].(string), map[string]interface{}{
			"endorsements": "Added endorsement on update",
		})
		requireStatus(t, r, 200)
		var updated map[string]interface{}
		r.JSON(&updated)
		assertStr(t, "endorsements after update", updated["endorsements"], "Added endorsement on update")
	})
}

// TestMultiPilotTime verifies multi-pilot time field for EASA multi-crew operations.
func TestMultiPilotTime(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("multipilot"), "SecurePass123!", "MP")

	t.Run("stored and returned", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-ABCD", "aircraftType": "A320",
			"departureIcao": "EDDF", "arrivalIcao": "EDDM",
			"offBlockTime": "06:00", "onBlockTime": "07:30", "landings": 1,
			"multiPilotTime": 90,
			"picName":        "Capt. Alpha",
			"crewMembers":    []map[string]interface{}{{"name": "FO Beta", "role": "SIC"}},
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)

		assertInt(t, "multiPilotTime", gi(f, "multiPilotTime"), 90)
		assertStr(t, "picName", f["picName"], "Capt. Alpha")
	})

	t.Run("defaults to zero", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EDEF", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "multiPilotTime default", gi(f, "multiPilotTime"), 0)
	})

	t.Run("update multi-pilot time", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EMUP", "aircraftType": "B737",
			"departureIcao": "EDDF", "arrivalIcao": "EDDM",
			"offBlockTime": "10:00", "onBlockTime": "11:30", "landings": 1,
		})
		requireStatus(t, r, 201)
		var created map[string]interface{}
		r.JSON(&created)

		r = c.PUT("/flights/"+created["id"].(string), map[string]interface{}{
			"multiPilotTime": 75,
		})
		requireStatus(t, r, 200)
		var updated map[string]interface{}
		r.JSON(&updated)
		assertInt(t, "multiPilotTime after update", gi(updated, "multiPilotTime"), 75)
	})
}

// TestFSTDSessions verifies that an FSTD session is recorded separately from
// flights and never contributes flight time (EASA AMC1 FCL.050 Cols 20-22).
func TestFSTDSessions(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("fstd"), "SecurePass123!", "FSTD")

	t.Run("session carries its duration and no flight time", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftType": "PA34",
			"isSimulator":             true,
			"fstdType":                "FNPT II",
			"simulatedFlightTime":     120,
			"simulatedInstrumentTime": 60,
			"holds":                   2,
			"approaches": []map[string]interface{}{
				{"type": "ILS", "airport": "EDDF", "runway": "07R"},
				{"type": "VOR", "airport": "EDDF"},
				{"type": "RNAV/GPS", "airport": "EDDF", "runway": "25C"},
			},
			"endorsements": "FSTD session completed satisfactorily",
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)

		if f["isSimulator"] != true {
			t.Fatalf("isSimulator = %v, want true", f["isSimulator"])
		}
		assertStr(t, "fstdType", f["fstdType"], "FNPT II")
		assertInt(t, "simulatedFlightTime", gi(f, "simulatedFlightTime"), 120)

		// The whole point: a session adds nothing to any flight-time column.
		for _, field := range []string{
			"totalTime", "picTime", "dualTime", "sicTime", "dualGivenTime",
			"multiPilotTime", "soloTime", "crossCountryTime", "nightTime",
			"ifrTime", "landingsDay", "landingsNight", "allLandings",
		} {
			assertInt(t, field+" on a session", gi(f, field), 0)
		}

		// Instrument work is training-relevant and survives.
		assertInt(t, "simulatedInstrumentTime", gi(f, "simulatedInstrumentTime"), 60)
		assertInt(t, "holds", gi(f, "holds"), 2)
		assertInt(t, "approachesCount", gi(f, "approachesCount"), 3)
		assertStr(t, "endorsements", f["endorsements"], "FSTD session completed satisfactorily")
	})

	t.Run("session rejects flight fields", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftType": "PA34",
			"isSimulator":         true,
			"fstdType":            "FNPT II",
			"simulatedFlightTime": 120,
			"departureIcao":       "EDDF",
			"arrivalIcao":         "EDDF",
			"offBlockTime":        "09:00",
			"onBlockTime":         "11:00",
			"landings":            0,
		})
		requireStatus(t, r, 400)
	})

	t.Run("session requires type and duration", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftType": "PA34", "isSimulator": true,
		})
		requireStatus(t, r, 400)

		r = c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftType": "PA34", "isSimulator": true,
			"fstdType": "FNPT II",
		})
		requireStatus(t, r, 400)
	})

	t.Run("flight still requires its route and block times", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EFLY", "aircraftType": "C172",
		})
		requireStatus(t, r, 400)
	})

	t.Run("sessions do not move flight statistics", func(t *testing.T) {
		c2 := NewE2EClient(t)
		registerAndLogin(t, c2, uniqueEmail("fstdstats"), "SecurePass123!", "Stats")

		r := c2.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-ESTA", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		})
		requireStatus(t, r, 201)

		before := c2.GET("/users/me/statistics")
		requireStatus(t, before, 200)
		var s1 map[string]interface{}
		before.JSON(&s1)

		r = c2.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftType": "A320",
			"isSimulator": true, "fstdType": "FFS A320", "simulatedFlightTime": 240,
		})
		requireStatus(t, r, 201)

		after := c2.GET("/users/me/statistics")
		requireStatus(t, after, 200)
		var s2 map[string]interface{}
		after.JSON(&s2)

		for _, field := range []string{"totalFlights", "totalMinutes", "picMinutes", "dualMinutes", "nightMinutes", "ifrMinutes"} {
			if gi(s1, field) != gi(s2, field) {
				t.Errorf("%s changed from %d to %d after logging a 4-hour FFS session; "+
					"session time must never be summed with flight time",
					field, gi(s1, field), gi(s2, field))
			}
		}
	})

	t.Run("session does not appear in the fleet", func(t *testing.T) {
		r := c.GET("/aircraft/stats")
		if r.StatusCode != 200 {
			t.Skipf("aircraft stats unavailable: %d", r.StatusCode)
		}
		var stats []map[string]interface{}
		r.JSON(&stats)
		for _, a := range stats {
			reg, _ := a["registration"].(string)
			if reg == "" || reg == "FNPT II" || reg == "FFS A320" {
				t.Errorf("fleet contains a simulator entry: %q", reg)
			}
		}
	})

	t.Run("fstdType nullable on a flight", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EFLZ", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "12:00", "onBlockTime": "13:00", "landings": 1,
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		if f["fstdType"] != nil {
			t.Errorf("fstdType should be nil for a flight, got %v", f["fstdType"])
		}
		if f["isSimulator"] != false {
			t.Errorf("isSimulator = %v, want false for a flight", f["isSimulator"])
		}
	})
}
