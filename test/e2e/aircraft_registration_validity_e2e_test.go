//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestAircraftRegistrationReuse covers one registration carried by different
// airframes at different times (issue #302).
func TestAircraftRegistrationReuse(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("regreuse"), "SecurePass123!", "Reuse Pilot")

	var firstID, secondID string

	t.Run("same registration with disjoint ranges is accepted", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-ARRU", "type": "A320", "make": "Airbus", "model": "A320",
			"isMultiPilot": true, "validTo": "2019-12-31",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.MustJSON(t, &ac)
		firstID = ac["id"].(string)
		if ac["validTo"] != "2019-12-31" || ac["validFrom"] != nil {
			t.Errorf("validity = %v..%v, want open..2019-12-31", ac["validFrom"], ac["validTo"])
		}

		resp = c.POST("/aircraft", map[string]interface{}{
			"registration": "D-ARRU", "type": "C172", "make": "Cessna", "model": "172",
			"validFrom": "2020-01-01",
		})
		requireStatus(t, resp, http.StatusCreated)
		resp.MustJSON(t, &ac)
		secondID = ac["id"].(string)
	})

	t.Run("overlapping range returns 409", func(t *testing.T) {
		assertStatus(t, c.POST("/aircraft", map[string]interface{}{
			"registration": "D-ARRU", "type": "PA28", "make": "Piper", "model": "Cherokee",
			"validFrom": "2019-06-01", "validTo": "2019-06-30",
		}), http.StatusConflict)
	})

	t.Run("open-ended duplicate returns 409", func(t *testing.T) {
		assertStatus(t, c.POST("/aircraft", map[string]interface{}{
			"registration": "D-ARRU", "type": "PA28", "make": "Piper", "model": "Cherokee",
		}), http.StatusConflict)
	})

	t.Run("validFrom after validTo returns 400", func(t *testing.T) {
		assertStatus(t, c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EINV", "type": "C152", "make": "Cessna", "model": "152",
			"validFrom": "2020-01-02", "validTo": "2020-01-01",
		}), http.StatusBadRequest)
	})

	t.Run("patching into an overlap returns 409", func(t *testing.T) {
		assertStatus(t, c.PATCH(fmt.Sprintf("/aircraft/%s", secondID), map[string]interface{}{
			"validFrom": "2019-12-31",
		}), http.StatusConflict)
	})

	t.Run("patching validTo to null reopens the range", func(t *testing.T) {
		resp := c.PATCH(fmt.Sprintf("/aircraft/%s", secondID), map[string]interface{}{
			"validTo": nil,
		})
		requireStatus(t, resp, http.StatusOK)
		var ac map[string]interface{}
		resp.MustJSON(t, &ac)
		if ac["validFrom"] != "2020-01-01" || ac["validTo"] != nil {
			t.Errorf("validity = %v..%v, want 2020-01-01..open", ac["validFrom"], ac["validTo"])
		}
	})

	logWithCaptain := func(date string) map[string]interface{} {
		resp := c.POST("/flights", map[string]interface{}{
			"date": date, "aircraftReg": "D-ARRU", "aircraftType": "A320",
			"departureIcao": "EDDF", "arrivalIcao": "EDDM",
			"offBlockTime": "06:00", "onBlockTime": "07:30", "landings": 1,
			"crewMembers": []map[string]interface{}{{"name": "Otto Lilienthal", "role": "PIC"}},
		})
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.MustJSON(t, &f)
		return f
	}

	t.Run("flight facts follow the airframe of the flight date", func(t *testing.T) {
		before := logWithCaptain("2019-12-31")
		assertInt(t, "sicTime before the change", gi(before, "sicTime"), 90)
		assertInt(t, "multiPilotTime before the change", gi(before, "multiPilotTime"), 90)

		after := logWithCaptain("2020-01-01")
		if after["isPassenger"] != true {
			t.Errorf("isPassenger after the change = %v, want true on the single-pilot airframe", after["isPassenger"])
		}
		assertInt(t, "sicTime after the change", gi(after, "sicTime"), 0)
	})

	t.Run("both entries are listed", func(t *testing.T) {
		resp := c.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.MustJSON(t, &r)
		seen := map[string]bool{}
		for _, item := range r["data"].([]interface{}) {
			seen[item.(map[string]interface{})["id"].(string)] = true
		}
		if !seen[firstID] || !seen[secondID] {
			t.Errorf("list is missing an N-number entry: %v", seen)
		}
	})
}
