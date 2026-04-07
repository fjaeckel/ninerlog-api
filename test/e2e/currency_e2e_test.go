//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestCurrencyStatus(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("currency"), "SecurePass123!", "Currency")

	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "CURR-001",
		"issueDate": "2023-01-01", "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	lid := lic["id"].(string)

	resp = c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
		"classType": "SEP_LAND", "issueDate": "2023-01-15", "expiryDate": futureDate(365),
	})
	requireStatus(t, resp, http.StatusCreated)

	t.Run("currency with no flights", func(t *testing.T) {
		resp := c.GET("/currency")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		if r["ratings"] == nil {
			t.Error("Expected ratings")
		}
	})

	for i := 0; i < 3; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": pastDate(i * 10), "aircraftReg": "D-ECUR", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "10:00", "onBlockTime": "11:00", "landings": 1,
		}), http.StatusCreated)
	}

	t.Run("currency with flights", func(t *testing.T) {
		resp := c.GET("/currency")
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("license currency", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/licenses/%s/currency", lid)), http.StatusOK)
	})
}

func TestLicenseStatistics(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("stats"), "SecurePass123!", "Stats")

	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "STATS-001",
		"issueDate": "2023-01-01", "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	lid := lic["id"].(string)

	for i := 0; i < 5; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": pastDate(i * 7), "aircraftReg": "D-ESTA", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 2,
		}), http.StatusCreated)
	}

	t.Run("get statistics", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/licenses/%s/statistics", lid)), http.StatusOK)
	})

	t.Run("statistics with date range", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/licenses/%s/statistics?startDate=%s&endDate=%s",
			lid, pastDate(21), pastDate(7))), http.StatusOK)
	})

	t.Run("nonexistent license returns 404", func(t *testing.T) {
		assertStatus(t, c.GET("/licenses/00000000-0000-0000-0000-000000000000/statistics"), http.StatusNotFound)
	})
}
