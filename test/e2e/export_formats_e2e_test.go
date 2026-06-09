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

// TestExportEASACSV_DualFlightShowsInstructorAsPIC is a regression test for
// the bug where Dual flights with the instructor recorded only in
// `crewMembers` (not in the legacy `instructorName` column) exported as
// "SELF" instead of the instructor's name in the EASA CSV "PIC Name" column.
//
// Reproduces the exact data shape the modern frontend writes:
//   - no legacy instructorName field
//   - Instructor lives in crewMembers
//   - User is NOT the instructor
//
// The exported EASA CSV row MUST contain the instructor's name in the
// PIC Name column and MUST NOT contain "SELF" for that row.
func TestExportEASACSV_DualFlightShowsInstructorAsPIC(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("easa-pic"), "SecurePass123!", "Amelia Earhart")

	// Dual flight: instructor only in crewMembers, no legacy instructorName.
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-EDUA", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		"crewMembers": []map[string]interface{}{
			{"name": "CFI Mueller", "role": "Instructor"},
		},
	}), http.StatusCreated)

	resp := c.GET("/exports/csv?format=easa")
	requireStatus(t, resp, http.StatusOK)
	body := string(resp.Body)

	// Find the data row (skip header) and check the PIC Name column.
	lines := strings.Split(body, "\n")
	if len(lines) < 2 {
		t.Fatalf("EASA CSV has no data rows: %q", body)
	}
	header := lines[0]
	dataRow := ""
	for _, ln := range lines[1:] {
		if strings.Contains(ln, "D-EDUA") {
			dataRow = ln
			break
		}
	}
	if dataRow == "" {
		t.Fatalf("could not find D-EDUA data row in EASA CSV.\nheader: %s\nbody:\n%s", header, body)
	}

	if !strings.Contains(dataRow, "CFI Mueller") {
		t.Errorf("EASA CSV PIC Name column should contain 'CFI Mueller' (the crew Instructor) for a Dual flight where the legacy instructorName is empty.\nheader: %s\nrow:    %s", header, dataRow)
	}
	if strings.Contains(dataRow, "SELF") {
		t.Errorf("EASA CSV PIC Name column shows 'SELF' for a Dual flight with crew Instructor — regression of the export-PIC-fallback bug.\nrow: %s", dataRow)
	}
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

	t.Run("glider format", func(t *testing.T) {
		resp := c.GET("/exports/pdf?format=glider")
		requireStatus(t, resp, http.StatusOK)
		assertValidPDF(t, resp.Body)
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/exports/pdf?format=faa"), http.StatusUnauthorized)
	})
}
