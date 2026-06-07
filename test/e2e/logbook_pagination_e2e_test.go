//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// TestLogbookFilterPaginationRegression reproduces the production bug reported
// where a PPL holder's SEP logbook only showed ~18 of ~100 eligible flights.
//
// Root cause: the per-license logbook filter (GET /flights?logbookLicenseId=...)
// matched flights to aircraft class IN MEMORY, but only AFTER the database had
// already applied pagination (default page size 20). So at most one page of raw
// flights was ever considered, the reported `total` was wrong, and the pilot
// could not page past the handful of matches that happened to land on page 1.
// "Recalculate all flights" did not help because it only recomputes flight-time
// fields, not the logbook membership.
//
// This test creates more eligible (SEP) flights than fit on a single page and
// deliberately makes the INELIGIBLE (glider) flights the most recent ones, so
// they dilute the first page exactly the way they did for the real user. It then
// asserts that:
//   - the filtered total reflects ALL eligible flights (not just page 1),
//   - every page can be retrieved and contains only eligible flights,
//   - no ineligible (glider) flight ever leaks into the SEP logbook.
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
	// 25 eligible SEP flights and 8 ineligible glider flights. The default page
	// size is 20, so the SEP flights span two pages. The glider flights are given
	// the MOST RECENT dates so that, sorted by date desc, they occupy the top of
	// page 1 — the precise condition that made the pre-fix code undercount.
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

	// Regression: the filtered total must reflect ALL eligible SEP flights, not
	// just the ones that fit on the first raw page. This MUST be queried with a
	// page size SMALLER than the flight count (here the default 20 < 33), because
	// that is the only condition under which the pre-fix in-memory filter saw a
	// truncated page. Pre-fix, page 1's 20 newest rows were 8 gliders + 12 SEP,
	// so the filter returned 12 and reported total=12 — mirroring the real
	// "18 of ~100". With pageSize>=33 every flight fits on one raw page and the
	// bug is completely masked, so do NOT raise the page size here.
	t.Run("filtered total counts all eligible flights", func(t *testing.T) {
		resp := c.GET(fmt.Sprintf("/flights?logbookLicenseId=%s", pplID)) // default page size (20)
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)

		if got := totalOf(r); got != sepCount {
			t.Errorf("filtered total: want %d eligible SEP flights, got %d (regression: filter applied after pagination undercounts)", sepCount, got)
		}

		// Page 1 must contain only eligible SEP flights — no glider may leak in,
		// even though gliders are the most recent flights overall.
		data := r["data"].([]interface{})
		for _, item := range data {
			f := item.(map[string]interface{})
			assertStr(t, "only SEP aircraft in logbook", f["aircraftReg"], sepReg)
		}
	})

	// Regression: pagination must work WITH the filter — every eligible flight is
	// reachable across pages, and no glider flight leaks in. Pre-fix, page 2 was
	// empty because the (wrong) total collapsed the result to a single page.
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
			// Every page must agree on the true eligible total. Pre-fix, page 1
			// reported total=12 (only the SEP rows that survived its single raw
			// page), so a paginating client believed there was just one page.
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
			if page > 10 { // safety valve against an accidental infinite loop
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
