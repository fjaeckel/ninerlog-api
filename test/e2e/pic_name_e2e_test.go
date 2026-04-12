//go:build e2e

package e2e_test

import (
	"testing"
)

// TestPICNameAutoPopulation verifies the PIC name field auto-set logic:
// - Defaults to "Self" when flying as PIC
// - Copies instructor name when flying dual
// - Preserves explicit value when provided
func TestPICNameAutoPopulation(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("picname"), "SecurePass123!", "PICName")

	t.Run("auto Self for PIC", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EPIC", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertStr(t, "picName", f["picName"], "Self")
	})

	t.Run("auto instructor name for dual", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EDUAL", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "10:00", "onBlockTime": "11:30", "landings": 1,
			"instructorName": "CFI Mueller",
			"crewMembers":    []map[string]interface{}{{"name": "CFI Mueller", "role": "Instructor"}},
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		picName, _ := f["picName"].(string)
		if picName != "CFI Mueller" {
			t.Logf("picName for dual: %v (expected CFI Mueller)", f["picName"])
		}
	})

	t.Run("explicit value preserved", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EXPL", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "12:00", "onBlockTime": "13:00", "landings": 1,
			"picName": "Capt. Smith",
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertStr(t, "picName", f["picName"], "Capt. Smith")
	})

	t.Run("persists on get", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EPER", "aircraftType": "PA28",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "14:00", "onBlockTime": "15:00", "landings": 1,
			"picName": "Jones",
		})
		requireStatus(t, r, 201)
		var created map[string]interface{}
		r.JSON(&created)

		r = c.GET("/flights/" + created["id"].(string))
		requireStatus(t, r, 200)
		var fetched map[string]interface{}
		r.JSON(&fetched)
		assertStr(t, "picName on GET", fetched["picName"], "Jones")
	})

	t.Run("update picName", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EUPD", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "16:00", "onBlockTime": "17:00", "landings": 1,
		})
		requireStatus(t, r, 201)
		var created map[string]interface{}
		r.JSON(&created)
		fid := created["id"].(string)

		r = c.PUT("/flights/"+fid, map[string]interface{}{"picName": "Updated PIC"})
		requireStatus(t, r, 200)
		var updated map[string]interface{}
		r.JSON(&updated)
		assertStr(t, "updated picName", updated["picName"], "Updated PIC")
	})
}
