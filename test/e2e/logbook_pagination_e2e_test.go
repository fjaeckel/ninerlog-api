//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestLogbookFilterPaginationRegression covers the per-license logbook filter
// (GET /flights?logbookLicenseId=...) under pagination: the filtered total
// counts all eligible flights, every page contains only eligible flights, and
// ineligible flights never leak in.
func TestLogbookFilterPaginationRegression(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("logbook-pagination"), "SecurePass123!", "Logbook Pagination")

	// --- License with a SEP_LAND class rating (the PPL logbook under test) ---
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "PPL-PAG",
		"issueDate": "2020-01-01", "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	pplID := lic["id"].(string)

	resp = c.POST(fmt.Sprintf("/licenses/%s/ratings", pplID), map[string]interface{}{
		"classType": "SEP_LAND", "issueDate": "2020-01-01", "expiryDate": futureDate(365),
	})
	requireStatus(t, resp, http.StatusCreated)

	// --- Aircraft: one SEP_LAND (eligible) and one glider (ineligible) ---
	const sepReg = "D-ESEP"
	const gliderReg = "D-GLID"
	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": sepReg, "type": "DA20", "make": "Diamond", "model": "Katana",
		"aircraftClass": "SEP_LAND",
	}), http.StatusCreated)
	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": gliderReg, "type": "ASK21", "make": "Schleicher", "model": "ASK 21",
		"aircraftClass": "GLIDER",
	}), http.StatusCreated)

	// --- Flights ---
	// 25 eligible SEP flights spanning two default-size pages, and 8
	// ineligible glider flights on the most recent dates, occupying the top of
	// page 1 when sorted by date desc.
	const sepCount = 25
	const gliderCount = 8

	createFlight := func(reg, acType, date string) {
		t.Helper()
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": date, "aircraftReg": reg, "aircraftType": acType,
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		}), http.StatusCreated)
	}

	// SEP flights on older dates (days 9..33 ago).
	for i := 0; i < sepCount; i++ {
		createFlight(sepReg, "DA20", pastDate(9+i))
	}
	// Glider flights on the most recent dates (days 0..7 ago).
	for i := 0; i < gliderCount; i++ {
		createFlight(gliderReg, "ASK21", pastDate(i))
	}

	totalOf := func(r map[string]interface{}) int {
		pg, ok := r["pagination"].(map[string]interface{})
		if !ok {
			t.Fatalf("response missing pagination object: %v", r)
		}
		return int(gf(pg, "total"))
	}

	// Sanity: without any filter all flights are visible.
	t.Run("unfiltered total counts every flight", func(t *testing.T) {
		resp := c.GET("/flights?pageSize=100")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		assertInt(t, "unfiltered total", totalOf(r), sepCount+gliderCount)
	})

	// Queried at the default page size (20), smaller than the 33 flights.
	t.Run("filtered total counts all eligible flights", func(t *testing.T) {
		resp := c.GET(fmt.Sprintf("/flights?logbookLicenseId=%s", pplID)) // default page size (20)
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)

		if got := totalOf(r); got != sepCount {
			t.Errorf("filtered total: want %d eligible SEP flights, got %d (regression: filter applied after pagination undercounts)", sepCount, got)
		}

		// Page 1 contains only eligible SEP flights.
		data := r["data"].([]interface{})
		for _, item := range data {
			f := item.(map[string]interface{})
			assertStr(t, "only SEP aircraft in logbook", f["aircraftReg"], sepReg)
		}
	})

	t.Run("filtered flights are reachable across pages", func(t *testing.T) {
		const pageSize = 20
		seen := map[string]bool{}
		gliderLeaks := 0

		for page := 1; ; page++ {
			resp := c.GET(fmt.Sprintf("/flights?logbookLicenseId=%s&pageSize=%d&page=%d", pplID, pageSize, page))
			requireStatus(t, resp, http.StatusOK)
			var r map[string]interface{}
			resp.JSON(&r)
			data := r["data"].([]interface{})
			if len(data) == 0 {
				break
			}
			// Every page reports the same eligible total.
			if got := totalOf(r); got != sepCount {
				t.Errorf("page %d reported total %d, want %d (regression: paginated total is wrong)", page, got, sepCount)
			}
			for _, item := range data {
				f := item.(map[string]interface{})
				reg, _ := f["aircraftReg"].(string)
				if reg == gliderReg {
					gliderLeaks++
				}
				if id, ok := f["id"].(string); ok {
					seen[id] = true
				}
			}
			if page > 10 { // page-count bound
				t.Fatal("too many pages; pagination likely broken")
			}
		}

		if gliderLeaks != 0 {
			t.Errorf("glider flights leaked into SEP logbook: %d", gliderLeaks)
		}
		if len(seen) != sepCount {
			t.Errorf("paginated through %d eligible flights, want %d (regression: matches beyond page 1 were unreachable)", len(seen), sepCount)
		}
	})
}
