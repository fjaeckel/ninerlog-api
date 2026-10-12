//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestFlightDisplayPICName(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-picname"), "SecurePass123!", "Pic Name Pilot")

	create := func(body map[string]interface{}) map[string]interface{} {
		t.Helper()
		resp := c.POST("/flights", body)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		return f
	}
	flight := func(extra map[string]interface{}) map[string]interface{} {
		body := map[string]interface{}{
			"date": "2026-06-01", "aircraftReg": "D-EPIC", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		}
		for k, v := range extra {
			body[k] = v
		}
		return create(body)
	}

	solo := flight(nil)
	dual := flight(map[string]interface{}{
		"crewMembers": []map[string]interface{}{{"name": "Jacqueline Cochran", "role": "Instructor"}},
	})
	sim := create(map[string]interface{}{
		"date": "2026-06-02", "aircraftType": "PA34", "isSimulator": true,
		"fstdType": "FNPT II", "simulatedFlightTime": 60,
	})

	t.Run("create responses carry it", func(t *testing.T) {
		assertStr(t, "own flight", solo["displayPicName"], "Self")
		assertStr(t, "with instructor", dual["displayPicName"], "Jacqueline Cochran")
		if v, ok := sim["displayPicName"]; ok && v != nil {
			t.Errorf("FSTD session displayPicName = %v, want none", v)
		}
	})

	t.Run("the list carries it", func(t *testing.T) {
		resp := c.GET("/flights")
		requireStatus(t, resp, http.StatusOK)
		var page struct {
			Data []map[string]interface{} `json:"data"`
		}
		resp.JSON(&page)
		got := map[string]interface{}{}
		for _, f := range page.Data {
			got[f["id"].(string)] = f["displayPicName"]
		}
		assertStr(t, "own flight in list", got[solo["id"].(string)], "Self")
		assertStr(t, "dual flight in list", got[dual["id"].(string)], "Jacqueline Cochran")
	})

	t.Run("an update that leaves the crew alone keeps the instructor", func(t *testing.T) {
		resp := c.PUT(fmt.Sprintf("/flights/%s", dual["id"]), map[string]interface{}{"remarks": "circuits"})
		requireStatus(t, resp, http.StatusOK)
		var f map[string]interface{}
		resp.JSON(&f)
		assertStr(t, "after update", f["displayPicName"], "Jacqueline Cochran")
	})

	t.Run("it is not accepted on write", func(t *testing.T) {
		f := flight(map[string]interface{}{"displayPicName": "Someone Else"})
		assertStr(t, "ignored on create", f["displayPicName"], "Self")
	})
}
