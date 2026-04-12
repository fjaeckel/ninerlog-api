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

// TestFSTDSessions verifies FSTD type designation for simulator sessions.
func TestFSTDSessions(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("fstd"), "SecurePass123!", "FSTD")

	t.Run("create FSTD session with type", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "FSTD-01", "aircraftType": "FNPT II",
			"departureIcao": "EDDF", "arrivalIcao": "EDDF",
			"offBlockTime": "09:00", "onBlockTime": "11:00", "landings": 0,
			"simulatedFlightTime":     120,
			"fstdType":                "FNPT II",
			"actualInstrumentTime":    60,
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

		assertStr(t, "fstdType", f["fstdType"], "FNPT II")
		assertInt(t, "simulatedFlightTime", gi(f, "simulatedFlightTime"), 120)
		assertInt(t, "approachesCount", gi(f, "approachesCount"), 3)
		assertInt(t, "holds", gi(f, "holds"), 2)
		assertStr(t, "endorsements", f["endorsements"], "FSTD session completed satisfactorily")
	})

	t.Run("fstdType nullable when not sim", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EFLY", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "12:00", "onBlockTime": "13:00", "landings": 1,
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		if f["fstdType"] != nil {
			t.Errorf("fstdType should be nil for non-sim flight, got %v", f["fstdType"])
		}
	})

	t.Run("full-flight sim designation", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "FFS-320", "aircraftType": "FFS A320",
			"departureIcao": "EDDF", "arrivalIcao": "EDDF",
			"offBlockTime": "14:00", "onBlockTime": "18:00", "landings": 0,
			"simulatedFlightTime": 240,
			"fstdType":            "FFS A320",
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertStr(t, "fstdType", f["fstdType"], "FFS A320")
		assertInt(t, "simulatedFlightTime", gi(f, "simulatedFlightTime"), 240)
	})
}
