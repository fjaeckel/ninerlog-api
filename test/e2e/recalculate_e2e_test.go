//go:build e2e

package e2e_test

import (
	"fmt"
	"testing"
)

func TestRecalculatePreservesDualFlights(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("recalc-dual"), "SecurePass123!", "RecalcDual")

	// Create a PIC flight (no instructor)
	r := c.POST("/flights", map[string]interface{}{
		"date": "2025-06-10", "aircraftReg": "D-EPIC", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00",
		"departureTime": "08:10", "arrivalTime": "08:50",
		"landings": 1,
	})
	requireStatus(t, r, 201)
	var picFlight map[string]interface{}
	r.JSON(&picFlight)
	picID := picFlight["id"].(string)

	// Verify PIC flight is correctly set up
	assertBool(t, "picFlight isPic before recalc", gb(picFlight, "isPic"), true)
	assertBool(t, "picFlight isDual before recalc", gb(picFlight, "isDual"), false)
	assertFloat(t, "picFlight picTime before recalc", gf(picFlight, "picTime"), 60, 6)
	assertFloat(t, "picFlight dualTime before recalc", gf(picFlight, "dualTime"), 0, 1)

	// Create a dual flight (with instructor crew member)
	r = c.POST("/flights", map[string]interface{}{
		"date": "2025-06-11", "aircraftReg": "D-EDUA", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "10:00", "onBlockTime": "11:30",
		"departureTime": "10:10", "arrivalTime": "11:20",
		"landings": 1,
		"crewMembers": []map[string]interface{}{
			{"name": "CFI Mueller", "role": "Instructor"},
		},
	})
	requireStatus(t, r, 201)
	var dualFlight map[string]interface{}
	r.JSON(&dualFlight)
	dualID := dualFlight["id"].(string)

	// Verify dual flight is correctly set up
	assertBool(t, "dualFlight isPic before recalc", gb(dualFlight, "isPic"), false)
	assertBool(t, "dualFlight isDual before recalc", gb(dualFlight, "isDual"), true)
	assertFloat(t, "dualFlight dualTime before recalc", gf(dualFlight, "dualTime"), 90, 6)
	assertFloat(t, "dualFlight picTime before recalc", gf(dualFlight, "picTime"), 0, 1)

	// Recalculate all flights
	r = c.POST("/flights/recalculate", nil)
	requireStatus(t, r, 200)
	var recalcResult map[string]interface{}
	r.JSON(&recalcResult)
	assertInt(t, "recalc total", gi(recalcResult, "total"), 2)
	assertInt(t, "recalc errors", gi(recalcResult, "errors"), 0)

	// GET the PIC flight — should still be PIC
	t.Run("PIC flight stays PIC after recalculate", func(t *testing.T) {
		r := c.GET(fmt.Sprintf("/flights/%s", picID))
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)

		assertBool(t, "isPic", gb(f, "isPic"), true)
		assertBool(t, "isDual", gb(f, "isDual"), false)
		assertFloat(t, "picTime", gf(f, "picTime"), 60, 6)
		assertFloat(t, "dualTime", gf(f, "dualTime"), 0, 1)
	})

	// GET the dual flight — must still be dual
	t.Run("Dual flight stays dual after recalculate", func(t *testing.T) {
		r := c.GET(fmt.Sprintf("/flights/%s", dualID))
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)

		assertBool(t, "isPic", gb(f, "isPic"), false)
		assertBool(t, "isDual", gb(f, "isDual"), true)
		assertFloat(t, "dualTime", gf(f, "dualTime"), 90, 6)
		assertFloat(t, "picTime", gf(f, "picTime"), 0, 1)

		// Crew members should still be present
		crew, ok := f["crewMembers"].([]interface{})
		if !ok || len(crew) == 0 {
			t.Error("Expected crew members to still be present after recalculate")
		} else {
			member := crew[0].(map[string]interface{})
			assertStr(t, "crew role", member["role"], "Instructor")
			assertStr(t, "crew name", member["name"], "CFI Mueller")
		}
	})
}
