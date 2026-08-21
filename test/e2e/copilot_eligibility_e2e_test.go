//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

// TestCoPilotEligibility covers who may log co-pilot time. The crew list is
// the same throughout; what changes is the aircraft and the declaration, which
// is what decides whether a co-pilot seat exists at all (EASA FCL.010,
// AMC1 FCL.050; 14 CFR 61.51(f), 91.109(b)).
func TestCoPilotEligibility(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("copilot"), "SecurePass123!", "Amelia Earhart")

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-AMPA", "type": "A320",
		"make": "Airbus", "model": "A320", "isMultiPilot": true,
	}), http.StatusCreated)
	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-ESPA", "type": "C172", "make": "Cessna", "model": "172",
	}), http.StatusCreated)

	withCaptain := func(reg, aircraftType, off, on string, extra map[string]interface{}) map[string]interface{} {
		body := map[string]interface{}{
			"date": today(), "aircraftReg": reg, "aircraftType": aircraftType,
			"departureIcao": "EDDF", "arrivalIcao": "EDDM",
			"offBlockTime": off, "onBlockTime": on, "landings": 1,
			"crewMembers": []map[string]interface{}{
				{"name": "Otto Lilienthal", "role": "PIC"},
			},
		}
		for k, v := range extra {
			body[k] = v
		}
		resp := c.POST("/flights", body)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		return f
	}

	t.Run("multi-pilot aircraft logs co-pilot and multi-pilot time", func(t *testing.T) {
		f := withCaptain("D-AMPA", "A320", "06:00", "07:30", nil)
		if f["isPassenger"] == true {
			t.Fatal("isPassenger = true on a multi-pilot aircraft")
		}
		assertInt(t, "totalTime", gi(f, "totalTime"), 90)
		assertInt(t, "sicTime", gi(f, "sicTime"), 90)
		assertInt(t, "picTime", gi(f, "picTime"), 0)
		assertInt(t, "multiPilotTime", gi(f, "multiPilotTime"), 90)
	})

	t.Run("single-pilot aircraft makes the user a passenger", func(t *testing.T) {
		f := withCaptain("D-ESPA", "C172", "08:00", "09:30", nil)
		if f["isPassenger"] != true {
			t.Fatalf("isPassenger = %v, want true", f["isPassenger"])
		}
		for _, field := range []string{"totalTime", "sicTime", "picTime", "multiPilotTime", "soloTime"} {
			if got := gi(f, field); got != 0 {
				t.Errorf("%s = %d, want 0", field, got)
			}
		}
	})

	t.Run("declared co-pilot time is honoured on any aircraft", func(t *testing.T) {
		f := withCaptain("D-ESPA", "C172", "10:00", "11:30", map[string]interface{}{
			"sicTime": 90,
		})
		if f["isPassenger"] == true {
			t.Fatal("isPassenger = true despite a declared co-pilot time")
		}
		assertInt(t, "sicTime", gi(f, "sicTime"), 90)
		assertInt(t, "multiPilotTime", gi(f, "multiPilotTime"), 0)
	})

	t.Run("safety pilot logs co-pilot but not multi-pilot time", func(t *testing.T) {
		resp := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-ESPA", "aircraftType": "C172",
			"departureIcao": "EDDF", "arrivalIcao": "EDDM",
			"offBlockTime": "12:00", "onBlockTime": "13:30", "landings": 1,
			"crewMembers": []map[string]interface{}{
				{"name": "Otto Lilienthal", "role": "PIC"},
				{"name": "Amelia Earhart", "role": "SafetyPilot"},
			},
		})
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)

		if f["isPassenger"] == true {
			t.Fatal("a required safety pilot is a crew member, not a passenger")
		}
		assertInt(t, "sicTime", gi(f, "sicTime"), 90)
		assertInt(t, "multiPilotTime", gi(f, "multiPilotTime"), 0)
	})
}

// Marking an aircraft multi-pilot and re-running the recalculation promotes
// the flights already logged in it, which is the migration path for existing
// data: migration 64 backfills nothing.
func TestCoPilotEligibility_RecalculateAfterMarkingFleet(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("copilot-recalc"), "SecurePass123!", "Amelia Earhart")

	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-ARCA", "type": "B737", "make": "Boeing", "model": "737",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)

	resp = c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-ARCA", "aircraftType": "B737",
		"departureIcao": "EDDF", "arrivalIcao": "EDDM",
		"offBlockTime": "06:00", "onBlockTime": "07:30", "landings": 1,
		"crewMembers": []map[string]interface{}{
			{"name": "Otto Lilienthal", "role": "PIC"},
		},
	})
	requireStatus(t, resp, http.StatusCreated)
	var f map[string]interface{}
	resp.JSON(&f)
	if f["isPassenger"] != true {
		t.Fatalf("isPassenger = %v, want true before the fleet is marked", f["isPassenger"])
	}
	flightID := f["id"].(string)

	requireStatus(t, c.PATCH("/aircraft/"+ac["id"].(string), map[string]interface{}{
		"isMultiPilot": true,
	}), http.StatusOK)

	requireStatus(t, c.POST("/flights/recalculate", map[string]interface{}{}), http.StatusOK)

	resp = c.GET("/flights/" + flightID)
	requireStatus(t, resp, http.StatusOK)
	var after map[string]interface{}
	resp.JSON(&after)

	if after["isPassenger"] == true {
		t.Error("isPassenger = true after the aircraft was marked multi-pilot")
	}
	assertInt(t, "totalTime", gi(after, "totalTime"), 90)
	assertInt(t, "sicTime", gi(after, "sicTime"), 90)
	assertInt(t, "multiPilotTime", gi(after, "multiPilotTime"), 90)
}
