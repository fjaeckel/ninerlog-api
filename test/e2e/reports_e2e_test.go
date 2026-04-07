//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

func TestAirportSearch(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("search by ICAO unauthenticated", func(t *testing.T) {
		resp := c.GET("/airports/search?q=EDNY")
		requireStatus(t, resp, http.StatusOK)
		var a []interface{}
		resp.JSON(&a)
		if len(a) < 1 {
			t.Error("Expected results for EDNY")
		}
	})

	t.Run("search partial", func(t *testing.T) {
		requireStatus(t, c.GET("/airports/search?q=ED"), http.StatusOK)
	})

	t.Run("search with limit", func(t *testing.T) {
		resp := c.GET("/airports/search?q=ED&limit=3")
		requireStatus(t, resp, http.StatusOK)
		var a []interface{}
		resp.JSON(&a)
		if len(a) > 3 {
			t.Errorf("Expected <=3, got %d", len(a))
		}
	})

	// Airport GET by ICAO code requires auth
	registerAndLogin(t, c, uniqueEmail("airports"), "SecurePass123!", "Airports")

	t.Run("get specific airport", func(t *testing.T) {
		resp := c.GET("/airports/EDNY")
		requireStatus(t, resp, http.StatusOK)
		var ap map[string]interface{}
		resp.JSON(&ap)
		if ap["icao"] != "EDNY" {
			t.Errorf("Expected EDNY, got %v", ap["icao"])
		}
	})

	t.Run("nonexistent airport", func(t *testing.T) {
		assertStatus(t, c.GET("/airports/ZZZZ"), http.StatusNotFound)
	})
}

func TestReports(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("reports"), "SecurePass123!", "Reports")

	deps := []string{"EDNY", "EDDS", "EDTL"}
	arrs := []string{"EDDS", "EDTL", "EDNY"}
	for i := 0; i < 3; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": pastDate(i * 10), "aircraftReg": "D-ERPT", "aircraftType": "C172",
			"departureIcao": deps[i], "arrivalIcao": arrs[i],
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		}), http.StatusCreated)
	}

	t.Run("routes", func(t *testing.T) { requireStatus(t, c.GET("/reports/routes"), http.StatusOK) })
	t.Run("airport stats", func(t *testing.T) { requireStatus(t, c.GET("/reports/airport-stats"), http.StatusOK) })
	t.Run("trends", func(t *testing.T) { requireStatus(t, c.GET("/reports/trends"), http.StatusOK) })
	t.Run("trends with months", func(t *testing.T) { requireStatus(t, c.GET("/reports/trends?months=6"), http.StatusOK) })
	t.Run("stats by class", func(t *testing.T) { requireStatus(t, c.GET("/reports/stats-by-class"), http.StatusOK) })

	t.Run("no auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/reports/routes"), http.StatusUnauthorized)
	})
}

func TestExports(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("exports"), "SecurePass123!", "Exports")

	for i := 0; i < 3; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": pastDate(i * 5), "aircraftReg": "D-EEXP", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		}), http.StatusCreated)
	}

	t.Run("CSV export", func(t *testing.T) {
		resp := c.GET("/exports/csv")
		requireStatus(t, resp, http.StatusOK)
		if len(resp.Body) == 0 {
			t.Error("Expected non-empty CSV")
		}
	})

	t.Run("JSON export", func(t *testing.T) {
		resp := c.GET("/exports/json")
		requireStatus(t, resp, http.StatusOK)
		if len(resp.Body) == 0 {
			t.Error("Expected non-empty JSON")
		}
	})

	t.Run("PDF export", func(t *testing.T) {
		resp := c.GET("/exports/pdf")
		requireStatus(t, resp, http.StatusOK)
		if len(resp.Body) == 0 {
			t.Error("Expected non-empty PDF")
		}
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/exports/csv"), http.StatusUnauthorized)
	})
}
