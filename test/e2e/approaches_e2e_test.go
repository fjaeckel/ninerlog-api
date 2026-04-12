//go:build e2e

package e2e_test

import (
	"testing"
)

// TestInstrumentApproaches verifies structured approach entries:
// - CRUD with type, airport, runway
// - approachesCount auto-derived from array length
// - Valid approach types accepted
func TestInstrumentApproaches(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("approaches"), "SecurePass123!", "Approaches")

	t.Run("create with structured approaches", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EAPR", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "10:00", "landings": 1,
			"approaches": []map[string]interface{}{
				{"type": "ILS", "airport": "EDDS", "runway": "25"},
				{"type": "VOR", "airport": "EDDF"},
			},
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)

		approaches := f["approaches"].([]interface{})
		if len(approaches) != 2 {
			t.Fatalf("want 2 approaches, got %d", len(approaches))
		}
		a0 := approaches[0].(map[string]interface{})
		assertStr(t, "type", a0["type"], "ILS")
		assertStr(t, "airport", a0["airport"], "EDDS")
		assertStr(t, "runway", a0["runway"], "25")

		a1 := approaches[1].(map[string]interface{})
		assertStr(t, "type", a1["type"], "VOR")
		assertStr(t, "airport", a1["airport"], "EDDF")

		assertInt(t, "approachesCount", gi(f, "approachesCount"), 2)
	})

	t.Run("count derived from array length", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-ECNT", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "10:00", "onBlockTime": "11:00", "landings": 1,
			"approaches": []map[string]interface{}{
				{"type": "ILS", "airport": "EDDS", "runway": "25"},
				{"type": "VOR", "airport": "EDDS"},
				{"type": "RNAV/GPS"},
			},
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "approachesCount", gi(f, "approachesCount"), 3)
	})

	t.Run("all valid approach types accepted", func(t *testing.T) {
		validTypes := []string{"ILS", "LOC", "VOR", "RNAV/GPS", "NDB", "Visual", "Circling", "Other"}
		for _, at := range validTypes {
			r := c.POST("/flights", map[string]interface{}{
				"date": today(), "aircraftReg": "D-EVAL", "aircraftType": "C172",
				"departureIcao": "EDNY", "arrivalIcao": "EDDS",
				"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
				"approaches": []map[string]interface{}{{"type": at, "airport": "EDDS"}},
			})
			if r.StatusCode != 201 {
				t.Errorf("approach type %s rejected: %d %s", at, r.StatusCode, string(r.Body))
			}
		}
	})

	t.Run("update approaches", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EUPD", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "12:00", "onBlockTime": "13:00", "landings": 1,
			"approaches": []map[string]interface{}{{"type": "ILS", "airport": "EDDS"}},
		})
		requireStatus(t, r, 201)
		var created map[string]interface{}
		r.JSON(&created)
		fid := created["id"].(string)

		r = c.PUT("/flights/"+fid, map[string]interface{}{
			"approaches": []map[string]interface{}{
				{"type": "RNAV/GPS", "airport": "EDNY", "runway": "06"},
				{"type": "LOC", "airport": "EDDS"},
				{"type": "NDB"},
			},
		})
		requireStatus(t, r, 200)
		var updated map[string]interface{}
		r.JSON(&updated)
		assertInt(t, "approachesCount after update", gi(updated, "approachesCount"), 3)

		approaches := updated["approaches"].([]interface{})
		a0 := approaches[0].(map[string]interface{})
		assertStr(t, "updated type", a0["type"], "RNAV/GPS")
	})

	t.Run("persists on get", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EPER", "aircraftType": "PA28",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "14:00", "onBlockTime": "15:00", "landings": 1,
			"approaches": []map[string]interface{}{
				{"type": "ILS", "airport": "EDDF", "runway": "07L"},
			},
		})
		requireStatus(t, r, 201)
		var created map[string]interface{}
		r.JSON(&created)

		r = c.GET("/flights/" + created["id"].(string))
		requireStatus(t, r, 200)
		var fetched map[string]interface{}
		r.JSON(&fetched)

		approaches := fetched["approaches"].([]interface{})
		if len(approaches) != 1 {
			t.Fatalf("want 1 approach on GET, got %d", len(approaches))
		}
		a0 := approaches[0].(map[string]interface{})
		assertStr(t, "type on GET", a0["type"], "ILS")
		assertStr(t, "airport on GET", a0["airport"], "EDDF")
		assertStr(t, "runway on GET", a0["runway"], "07L")
	})

	t.Run("flight without approaches", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-ENOA", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "16:00", "onBlockTime": "17:00", "landings": 1,
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "approachesCount zero", gi(f, "approachesCount"), 0)
	})
}
