//go:build e2e

package e2e_test

import (
	"fmt"
	"testing"
)

func TestFlightUpdateEdgeCases(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-upd"), "SecurePass123!", "FlightUpd")

	// Create base flight
	r := c.POST("/flights", map[string]interface{}{
		"date": "2025-06-15", "aircraftReg": "D-EUPD", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30",
		"departureTime": "08:10", "arrivalTime": "09:20",
		"landings": 2, "remarks": "Original remarks",
		"ifrTime": 30, "holds": 1, "approachesCount": 2,
		"instructorName": "Original CFI",
		"crewMembers": []map[string]interface{}{
			{"name": "Pax One", "role": "Passenger"},
		},
	})
	requireStatus(t, r, 201)
	var orig map[string]interface{}
	r.JSON(&orig)
	fid := orig["id"].(string)

	t.Run("PUT updates all fields and recalculates", func(t *testing.T) {
		r := c.PUT(fmt.Sprintf("/flights/%s", fid), map[string]interface{}{
			"date": "2025-06-16", "aircraftReg": "D-EUPD", "aircraftType": "PA28",
			"departureIcao": "EDDS", "arrivalIcao": "EDDM",
			"offBlockTime": "10:00", "onBlockTime": "12:00",
			"departureTime": "10:10", "arrivalTime": "11:50",
			"landings": 3, "remarks": "Updated remarks",
			"ifrTime": 60, "holds": 2, "approachesCount": 4,
			"instructorName": "New CFI", "instructorComments": "Updated comments",
			"crewMembers": []map[string]interface{}{
				{"name": "Instructor New", "role": "Instructor"},
			},
		})
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)

		// Verify recalculated fields
		assertFloat(t, "totalTime", gf(f, "totalTime"), 120, 6)
		assertBool(t, "isDual (instructor crew)", gb(f, "isDual"), true)
		assertFloat(t, "dualTime", gf(f, "dualTime"), 120, 6)
		assertFloat(t, "picTime", gf(f, "picTime"), 0, 1)

		// Distance should change (EDDS-EDDM != EDNY-EDDS)
		dist := gf(f, "distance")
		if dist <= 0 {
			t.Error("Expected positive distance for EDDS-EDDM")
		}

		// Verify user-set fields persisted
		assertStr(t, "remarks", f["remarks"], "Updated remarks")
		assertFloat(t, "ifrTime", gf(f, "ifrTime"), 60, 1)
		assertInt(t, "holds", gi(f, "holds"), 2)
		assertInt(t, "approachesCount", gi(f, "approachesCount"), 4)
		assertStr(t, "aircraftType", f["aircraftType"], "PA28")
		assertInt(t, "landings", gi(f, "allLandings"), 3)
	})

	t.Run("PUT with crew change from dual to solo recalculates", func(t *testing.T) {
		// Remove instructor → should switch from dual to PIC
		r := c.PUT(fmt.Sprintf("/flights/%s", fid), map[string]interface{}{
			"date": "2025-06-16", "aircraftReg": "D-EUPD", "aircraftType": "PA28",
			"departureIcao": "EDDS", "arrivalIcao": "EDDM",
			"offBlockTime": "10:00", "onBlockTime": "12:00",
			"landings": 1,
		})
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)

		assertBool(t, "isPic (no instructor)", gb(f, "isPic"), true)
		assertBool(t, "isDual (no instructor)", gb(f, "isDual"), false)
		assertFloat(t, "picTime", gf(f, "picTime"), 120, 6)
		assertFloat(t, "dualTime", gf(f, "dualTime"), 0, 1)
	})

	t.Run("GET after PUT returns updated fields", func(t *testing.T) {
		r := c.GET(fmt.Sprintf("/flights/%s", fid))
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)

		assertFloat(t, "totalTime on GET", gf(f, "totalTime"), 120, 6)
		assertBool(t, "isPic on GET", gb(f, "isPic"), true)
	})

	t.Run("PUT changes departure/arrival recalculates XC and distance", func(t *testing.T) {
		// Local flight
		r := c.PUT(fmt.Sprintf("/flights/%s", fid), map[string]interface{}{
			"date": "2025-06-16", "aircraftReg": "D-EUPD", "aircraftType": "PA28",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "10:00", "onBlockTime": "11:00", "landings": 5,
		})
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)

		assertFloat(t, "crossCountryTime (local)", gf(f, "crossCountryTime"), 0, 1)
		assertFloat(t, "distance (local)", gf(f, "distance"), 0.0, 0.1)

		// Back to XC
		r = c.PUT(fmt.Sprintf("/flights/%s", fid), map[string]interface{}{
			"date": "2025-06-16", "aircraftReg": "D-EUPD", "aircraftType": "PA28",
			"departureIcao": "EDNY", "arrivalIcao": "KJFK",
			"offBlockTime": "06:00", "onBlockTime": "14:00", "landings": 1,
		})
		requireStatus(t, r, 200)
		r.JSON(&f)

		assertFloat(t, "crossCountryTime (XC)", gf(f, "crossCountryTime"), 480, 6)
		d := gf(f, "distance")
		if d < 3000 {
			t.Errorf("Expected >3000NM for EDNY-KJFK, got %.1f", d)
		}
	})
}
