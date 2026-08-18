//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
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

// TestExportEASACSV_DualFlightShowsInstructorAsPIC covers a dual flight whose
// instructor is recorded only in crewMembers: the EASA CSV "PIC Name" column
// carries the instructor's name, not "SELF".
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

	// Every format × layout × page_size combination produces a valid PDF, and
	// a spread export is strictly larger than its single-layout twin.
	for _, format := range []string{"easa", "faa"} {
		for _, pageSize := range []string{"a4", "a5", "letter"} {
			format, pageSize := format, pageSize
			t.Run(fmt.Sprintf("%s spread vs single %s", format, pageSize), func(t *testing.T) {
				spread := c.GET(fmt.Sprintf("/exports/pdf?format=%s&layout=spread&page_size=%s", format, pageSize))
				requireStatus(t, spread, http.StatusOK)
				assertValidPDF(t, spread.Body)

				single := c.GET(fmt.Sprintf("/exports/pdf?format=%s&layout=single&page_size=%s", format, pageSize))
				requireStatus(t, single, http.StatusOK)
				assertValidPDF(t, single.Body)

				if len(spread.Body) <= len(single.Body) {
					t.Errorf("spread PDF (%d bytes) not larger than single-layout PDF (%d bytes)",
						len(spread.Body), len(single.Body))
				}
			})
		}
	}

	t.Run("rows_per_page scales pagination", func(t *testing.T) {
		// Both row counts produce a valid PDF.
		for _, rows := range []int{10, 40} {
			resp := c.GET(fmt.Sprintf("/exports/pdf?format=easa&layout=single&rows_per_page=%d", rows))
			requireStatus(t, resp, http.StatusOK)
			assertValidPDF(t, resp.Body)
		}
	})

	t.Run("layout defaults to spread", func(t *testing.T) {
		def := c.GET("/exports/pdf?format=easa&page_size=a4")
		requireStatus(t, def, http.StatusOK)
		spread := c.GET("/exports/pdf?format=easa&layout=spread&page_size=a4")
		requireStatus(t, spread, http.StatusOK)
		// Same layout yields a near-identical size (only the embedded creation
		// timestamp may differ).
		if diff := len(def.Body) - len(spread.Body); diff < -64 || diff > 64 {
			t.Errorf("default layout differs from explicit spread: %d vs %d bytes",
				len(def.Body), len(spread.Body))
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/exports/pdf?format=faa"), http.StatusUnauthorized)
	})
}

// TestExportPDFCarriesPriorExperience covers the initial-hours snapshot in
// the printed logbook: carried-forward hours open the balance on the sheets.
func TestExportPDFCarriesPriorExperience(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("pdf-baseline"), "SecurePass123!", "PDFBaseline")

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(3), "aircraftReg": "D-EPDF", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		"picName": "Self",
	}), http.StatusCreated)

	formats := []string{"easa", "faa", "summary"}
	exportSize := func(t *testing.T, format string) int {
		t.Helper()
		resp := c.GET("/exports/pdf?format=" + format)
		requireStatus(t, resp, http.StatusOK)
		if len(resp.Body) < 100 || !strings.HasPrefix(string(resp.Body[:5]), "%PDF-") {
			t.Fatalf("%s export is not a valid PDF (%d bytes)", format, len(resp.Body))
		}
		return len(resp.Body)
	}

	loggedOnly := make(map[string]int, len(formats))
	for _, format := range formats {
		loggedOnly[format] = exportSize(t, format)
	}

	// 500 h carried over from a paper logbook, cut off three years ago.
	requireStatus(t, c.PUT("/users/me/baseline", map[string]interface{}{
		"baselineDate":  time.Now().AddDate(-3, 0, 0).Format("2006-01-02"),
		"totalFlights":  400,
		"totalMinutes":  30000,
		"picMinutes":    24000,
		"nightMinutes":  1800,
		"landingsDay":   600,
		"landingsNight": 90,
	}), http.StatusOK)

	// Every format's export grows once the snapshot exists.
	for _, format := range formats {
		if got := exportSize(t, format); got <= loggedOnly[format] {
			t.Errorf("%s export ignored the recorded prior experience: %d bytes with a baseline, %d without",
				format, got, loggedOnly[format])
		}
	}

	// Deleting the snapshot returns every export to logged flights only.
	requireStatus(t, c.DELETE("/users/me/baseline"), http.StatusNoContent)
	for _, format := range formats {
		// Only the embedded creation timestamp may differ.
		if diff := exportSize(t, format) - loggedOnly[format]; diff < -64 || diff > 64 {
			t.Errorf("%s export did not return to its pre-baseline size after the snapshot was deleted", format)
		}
	}
}
