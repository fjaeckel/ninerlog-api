//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestExportCSVFormats verifies CSV export with EASA, FAA, and standard column formats.
func TestExportCSVFormats(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("csv-fmt"), "SecurePass123!", "CSVFmt")

	// Seed flights
	for i := 0; i < 3; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": pastDate(i * 5), "aircraftReg": "D-ECSV", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
			"picName":      "Self",
			"endorsements": fmt.Sprintf("Endorsement %d", i),
			"approaches":   []map[string]interface{}{{"type": "ILS", "airport": "EDDS", "runway": "25"}},
		}), http.StatusCreated)
	}

	t.Run("standard includes PICName column", func(t *testing.T) {
		resp := c.GET("/exports/csv")
		requireStatus(t, resp, http.StatusOK)
		if !strings.Contains(string(resp.Body), "PICName") {
			t.Error("missing PICName column in standard CSV")
		}
	})

	t.Run("standard includes Endorsements column", func(t *testing.T) {
		resp := c.GET("/exports/csv")
		requireStatus(t, resp, http.StatusOK)
		if !strings.Contains(string(resp.Body), "Endorsements") {
			t.Error("missing Endorsements column in standard CSV")
		}
	})

	t.Run("easa has SP-SE and PIC Name columns", func(t *testing.T) {
		resp := c.GET("/exports/csv?format=easa")
		requireStatus(t, resp, http.StatusOK)
		body := string(resp.Body)
		if !strings.Contains(body, "SP-SE") {
			t.Error("missing SP-SE in EASA CSV")
		}
		if !strings.Contains(body, "PIC Name") {
			t.Error("missing PIC Name in EASA CSV")
		}
		if !strings.Contains(body, "Multi-Pilot") {
			t.Error("missing Multi-Pilot in EASA CSV")
		}
		if !strings.Contains(body, "FSTD Type") {
			t.Error("missing FSTD Type in EASA CSV")
		}
	})

	t.Run("faa has Solo and Remarks/Endorsements columns", func(t *testing.T) {
		resp := c.GET("/exports/csv?format=faa")
		requireStatus(t, resp, http.StatusOK)
		body := string(resp.Body)
		if !strings.Contains(body, "Solo") {
			t.Error("missing Solo in FAA CSV")
		}
		if !strings.Contains(body, "Remarks/Endorsements") {
			t.Error("missing Remarks/Endorsements in FAA CSV")
		}
	})

	t.Run("format=standard same as no param", func(t *testing.T) {
		r1 := c.GET("/exports/csv")
		requireStatus(t, r1, http.StatusOK)
		r2 := c.GET("/exports/csv?format=standard")
		requireStatus(t, r2, http.StatusOK)
		// Both should have the same header row
		h1 := strings.SplitN(string(r1.Body), "\n", 2)[0]
		h2 := strings.SplitN(string(r2.Body), "\n", 2)[0]
		if h1 != h2 {
			t.Errorf("standard headers differ:\n  default: %s\n  explicit: %s", h1, h2)
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/exports/csv?format=easa"), http.StatusUnauthorized)
	})
}

// TestExportPDFFormats verifies PDF export with EASA, FAA, and summary formats.
func TestExportPDFFormats(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("pdf-fmt"), "SecurePass123!", "PDFFmt")

	// Seed flights
	for i := 0; i < 3; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": pastDate(i * 5), "aircraftReg": "D-EPDF", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
			"picName": "Self",
		}), http.StatusCreated)
	}

	assertValidPDF := func(t *testing.T, body []byte) {
		t.Helper()
		if len(body) < 100 {
			t.Error("PDF too small")
		}
		if !strings.HasPrefix(string(body[:5]), "%PDF-") {
			t.Error("not a valid PDF")
		}
	}

	t.Run("default is EASA", func(t *testing.T) {
		resp := c.GET("/exports/pdf")
		requireStatus(t, resp, http.StatusOK)
		assertValidPDF(t, resp.Body)
	})

	t.Run("easa format", func(t *testing.T) {
		resp := c.GET("/exports/pdf?format=easa")
		requireStatus(t, resp, http.StatusOK)
		assertValidPDF(t, resp.Body)
	})

	t.Run("faa format", func(t *testing.T) {
		resp := c.GET("/exports/pdf?format=faa")
		requireStatus(t, resp, http.StatusOK)
		assertValidPDF(t, resp.Body)
	})

	t.Run("summary format", func(t *testing.T) {
		resp := c.GET("/exports/pdf?format=summary")
		requireStatus(t, resp, http.StatusOK)
		assertValidPDF(t, resp.Body)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/exports/pdf?format=faa"), http.StatusUnauthorized)
	})
}
