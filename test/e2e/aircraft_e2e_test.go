//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestAircraftCRUD(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("aircraft"), "SecurePass123!", "AC User")
	var acID string

	t.Run("create aircraft", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EABC", "type": "C172", "make": "Cessna", "model": "172S",
			"aircraftClass": "SEP_LAND",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		acID = ac["id"].(string)
	})

	t.Run("create with all fields", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EMEP", "type": "PA44", "make": "Piper", "model": "Seminole",
			"aircraftClass": "MEP_LAND", "isComplex": true, "isHighPerformance": true,
			"isTailwheel": false, "notes": "Club MEP",
		})
		requireStatus(t, resp, http.StatusCreated)
	})

	t.Run("duplicate registration returns 409", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EABC", "type": "PA28", "make": "Piper", "model": "Cherokee",
		})
		assertStatus(t, resp, http.StatusConflict)
	})

	t.Run("list with pagination", func(t *testing.T) {
		resp := c.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		data := r["data"].([]interface{})
		if len(data) < 2 {
			t.Errorf("Expected >=2, got %d", len(data))
		}
	})

	t.Run("get by id", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/aircraft/%s", acID)), http.StatusOK)
	})

	t.Run("update", func(t *testing.T) {
		resp := c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{"notes": "Updated"})
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("delete", func(t *testing.T) {
		assertStatus(t, c.DELETE(fmt.Sprintf("/aircraft/%s", acID)), http.StatusNoContent)
		assertStatus(t, c.GET(fmt.Sprintf("/aircraft/%s", acID)), http.StatusNotFound)
	})

	t.Run("nonexistent returns 404", func(t *testing.T) {
		assertStatus(t, c.GET("/aircraft/00000000-0000-0000-0000-000000000000"), http.StatusNotFound)
	})

	t.Run("missing fields returns 400", func(t *testing.T) {
		assertStatus(t, c.POST("/aircraft", map[string]interface{}{"registration": "X"}), http.StatusBadRequest)
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.POST("/aircraft", map[string]interface{}{
			"registration": "X", "type": "C172", "make": "Cessna", "model": "172",
		}), http.StatusUnauthorized)
	})
}

func TestAircraftLoggingDefaults(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ac-def"), "SecurePass123!", "Def User")

	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EDEF", "type": "C172", "make": "Cessna", "model": "172",
		"defaultDepartureIcao": "lszh", "defaultArrivalIcao": "LSZH",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)
	acID := ac["id"].(string)

	t.Run("defaults are uppercased on create", func(t *testing.T) {
		assertStr(t, "defaultDepartureIcao", ac["defaultDepartureIcao"], "LSZH")
		assertStr(t, "defaultArrivalIcao", ac["defaultArrivalIcao"], "LSZH")
	})

	t.Run("defaults can be updated", func(t *testing.T) {
		resp := c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{
			"defaultDepartureIcao": "eddm",
		})
		requireStatus(t, resp, http.StatusOK)
		var updated map[string]interface{}
		resp.JSON(&updated)
		assertStr(t, "defaultDepartureIcao", updated["defaultDepartureIcao"], "EDDM")
		assertStr(t, "defaultArrivalIcao", updated["defaultArrivalIcao"], "LSZH")
	})

	t.Run("too long default is rejected", func(t *testing.T) {
		assertStatus(t, c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{
			"defaultDepartureIcao": "TOOLONG",
		}), http.StatusBadRequest)
	})
}

func TestAircraftStats(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ac-stats"), "SecurePass123!", "Stats User")

	// Two flights on D-ESTA (90 + 60 min, 3 + 1 landings), one on D-ESTB
	for _, f := range []map[string]interface{}{
		{"date": today(), "aircraftReg": "D-ESTA", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 3},
		{"date": today(), "aircraftReg": "D-ESTA", "aircraftType": "C172", "departureIcao": "EDDS", "arrivalIcao": "EDNY", "offBlockTime": "10:00", "onBlockTime": "11:00", "landings": 1},
		{"date": today(), "aircraftReg": "D-ESTB", "aircraftType": "PA28", "departureIcao": "EDNY", "arrivalIcao": "EDNY", "offBlockTime": "12:00", "onBlockTime": "12:30", "landings": 1},
	} {
		requireStatus(t, c.POST("/flights", f), http.StatusCreated)
	}

	resp := c.GET("/aircraft/stats")
	requireStatus(t, resp, http.StatusOK)
	var r map[string]interface{}
	resp.JSON(&r)
	data := r["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("Expected stats for 2 registrations, got %d", len(data))
	}

	stats := map[string]map[string]interface{}{}
	for _, item := range data {
		s := item.(map[string]interface{})
		stats[s["registration"].(string)] = s
	}

	t.Run("aggregates per registration", func(t *testing.T) {
		a := stats["D-ESTA"]
		if a == nil {
			t.Fatal("Missing stats for D-ESTA")
		}
		assertInt(t, "totalFlights", gi(a, "totalFlights"), 2)
		assertInt(t, "totalMinutes", gi(a, "totalMinutes"), 150)
		assertStr(t, "lastFlightDate", a["lastFlightDate"], today())
		b := stats["D-ESTB"]
		if b == nil {
			t.Fatal("Missing stats for D-ESTB")
		}
		assertInt(t, "totalFlights", gi(b, "totalFlights"), 1)
		assertInt(t, "totalMinutes", gi(b, "totalMinutes"), 30)
	})

	t.Run("aggregates per type with 90-day recency", func(t *testing.T) {
		byType := map[string]map[string]interface{}{}
		for _, item := range r["byType"].([]interface{}) {
			s := item.(map[string]interface{})
			byType[s["aircraftType"].(string)] = s
		}
		c172 := byType["C172"]
		if c172 == nil {
			t.Fatal("Missing type stats for C172")
		}
		assertInt(t, "totalFlights", gi(c172, "totalFlights"), 2)
		assertInt(t, "totalMinutes", gi(c172, "totalMinutes"), 150)
		assertInt(t, "landingsLast90Days", gi(c172, "landingsLast90Days"), 4)
		// 4 landings today → recency lapses 90 days from today
		if c172["recencyLapsesOn"] == nil {
			t.Error("Expected recencyLapsesOn for C172")
		}
		pa28 := byType["PA28"]
		if pa28 == nil {
			t.Fatal("Missing type stats for PA28")
		}
		assertInt(t, "landingsLast90Days", gi(pa28, "landingsLast90Days"), 1)
		if pa28["recencyLapsesOn"] != nil {
			t.Errorf("Expected no recencyLapsesOn for PA28 (only 1 landing), got %v", pa28["recencyLapsesOn"])
		}
		// Per-registration recency mirrors the same rule
		assertInt(t, "landingsLast90Days", gi(stats["D-ESTA"], "landingsLast90Days"), 4)
		if stats["D-ESTA"]["recencyLapsesOn"] == nil {
			t.Error("Expected recencyLapsesOn for D-ESTA")
		}
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		c2 := NewE2EClient(t)
		assertStatus(t, c2.GET("/aircraft/stats"), http.StatusUnauthorized)
	})
}

func TestAircraftRenameFlights(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ac-ren"), "SecurePass123!", "Rename User")

	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EOLD", "type": "C172", "make": "Cessna", "model": "172",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)
	acID := ac["id"].(string)

	for i := 0; i < 2; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EOLD", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": fmt.Sprintf("%02d:00", 8+i), "onBlockTime": fmt.Sprintf("%02d:00", 9+i),
			"landings": 1,
		}), http.StatusCreated)
	}

	statsByReg := func() map[string]map[string]interface{} {
		resp := c.GET("/aircraft/stats")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		stats := map[string]map[string]interface{}{}
		for _, item := range r["data"].([]interface{}) {
			s := item.(map[string]interface{})
			stats[s["registration"].(string)] = s
		}
		return stats
	}

	t.Run("rename with flag repoints flights", func(t *testing.T) {
		requireStatus(t, c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{
			"registration": "D-ENEW", "renameFlights": true,
		}), http.StatusOK)

		stats := statsByReg()
		if stats["D-EOLD"] != nil {
			t.Error("Expected no flights left under D-EOLD")
		}
		if stats["D-ENEW"] == nil {
			t.Fatal("Expected flights under D-ENEW")
		}
		assertInt(t, "totalFlights", gi(stats["D-ENEW"], "totalFlights"), 2)
	})

	t.Run("rename without flag leaves flights untouched", func(t *testing.T) {
		requireStatus(t, c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{
			"registration": "D-EFIN",
		}), http.StatusOK)

		stats := statsByReg()
		if stats["D-ENEW"] == nil {
			t.Error("Expected flights to still be logged under D-ENEW")
		}
		if stats["D-EFIN"] != nil {
			t.Error("Expected no flights under D-EFIN")
		}
	})
}

func TestAircraftIsolation(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	registerAndLogin(t, c1, uniqueEmail("ac-iso1"), "SecurePass123!", "U1")
	registerAndLogin(t, c2, uniqueEmail("ac-iso2"), "SecurePass123!", "U2")

	resp := c1.POST("/aircraft", map[string]interface{}{
		"registration": "D-EISO", "type": "C172", "make": "Cessna", "model": "172",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)
	aid := ac["id"].(string)

	t.Run("user2 cannot see", func(t *testing.T) {
		resp := c2.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		data := r["data"].([]interface{})
		if len(data) != 0 {
			t.Errorf("Expected 0, got %d", len(data))
		}
	})

	t.Run("user2 cannot access by id", func(t *testing.T) {
		assertStatus(t, c2.GET(fmt.Sprintf("/aircraft/%s", aid)), http.StatusNotFound)
	})

	t.Run("different users same registration ok", func(t *testing.T) {
		requireStatus(t, c2.POST("/aircraft", map[string]interface{}{
			"registration": "D-EISO", "type": "PA28", "make": "Piper", "model": "Cherokee",
		}), http.StatusCreated)
	})
}
