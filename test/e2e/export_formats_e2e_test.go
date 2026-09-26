//go:build e2e

package e2e_test

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
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

	t.Run("weblogbook uses vsimakhin web-logbook header and value formats", func(t *testing.T) {
		resp := c.GET("/exports/csv?format=weblogbook")
		requireStatus(t, resp, http.StatusOK)
		lines := strings.Split(strings.TrimSpace(string(resp.Body)), "\n")
		wantHeader := "Date,Departure Place,Departure Time,Arrival Place,Arrival Time,Aircraft Model,Aircraft Reg," +
			"Time SE,Time ME,Time MCC,Time Total,Landings Day,Landings Night,Time Night,Time IFR,Time PIC," +
			"Time CoPilot,Time Dual,Time Instructor,SIM Type,SIM Time,PIC Name,Remarks,Tags"
		if lines[0] != wantHeader {
			t.Errorf("header = %q, want %q", lines[0], wantHeader)
		}
		if len(lines) != 4 {
			t.Fatalf("got %d lines, want header + 3 flights", len(lines))
		}
		for _, want := range []string{",EDNY,0800,EDDS,0930,C172,D-ECSV,1:30,", ",Self,Endorsement "} {
			if !strings.Contains(lines[1], want) {
				t.Errorf("row %q does not contain %q", lines[1], want)
			}
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

// pdfStreamText concatenates every inflated content stream of a rendered
// PDF, which is where the cell text of the logbook rows lives.
func pdfStreamText(raw []byte) string {
	var out bytes.Buffer
	const openTag, closeTag = "\nstream\n", "\nendstream"
	for pos := 0; ; {
		i := bytes.Index(raw[pos:], []byte(openTag))
		if i < 0 {
			break
		}
		start := pos + i + len(openTag)
		j := bytes.Index(raw[start:], []byte(closeTag))
		if j < 0 {
			break
		}
		body := raw[start : start+j]
		pos = start + j + len(closeTag)
		r, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			out.Write(body)
			continue
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			continue
		}
		out.Write(data)
	}
	return out.String()
}

// testSignaturePNG returns a small valid PNG standing in for an instructor's
// captured ink.
func testSignaturePNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 240, 80))
	for x := 10; x < 230; x++ {
		for dy := -2; dy <= 2; dy++ {
			img.Set(x, 40+dy+x%9, color.NRGBA{20, 20, 40, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// TestExportPDFRendersInstructorEndorsement covers the printed logbook's
// sign-off surface: a completed instructor signature prints in the remarks
// column of the flight it attests — the ink as an embedded image, the
// signer's name and credential number beside it — and disappears again once
// the signature is voided.
func TestExportPDFRendersInstructorEndorsement(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("pdf-sign"), "SecurePass123!", "PDFSign")

	resp := c.POST("/flights", map[string]interface{}{
		"date": pastDate(2), "aircraftReg": "D-ESGN", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		"isDual": true, "remarks": "Circuits and bumps",
	})
	requireStatus(t, resp, http.StatusCreated)
	var flight struct {
		ID string `json:"id"`
	}
	if err := resp.JSON(&flight); err != nil {
		t.Fatalf("decode flight: %v", err)
	}

	exportPDF := func(t *testing.T, format string) []byte {
		t.Helper()
		r := c.GET("/exports/pdf?format=" + format)
		requireStatus(t, r, http.StatusOK)
		return r.Body
	}

	// Nothing of the instructor appears before the flight is signed.
	if strings.Contains(pdfStreamText(exportPDF(t, "easa")), "Wilbur Wright") {
		t.Fatal("unsigned flight already prints an instructor")
	}

	resp = c.POST("/flights/"+flight.ID+"/signatures/live", map[string]interface{}{
		"signerName":       "Wilbur Wright",
		"credentialNumber": "US.CFI.1903",
		"signatureImage":   testSignaturePNG(),
	})
	requireStatus(t, resp, http.StatusCreated)
	var sig struct {
		ID string `json:"id"`
	}
	if err := resp.JSON(&sig); err != nil {
		t.Fatalf("decode signature: %v", err)
	}

	for _, format := range []string{"easa", "faa"} {
		t.Run(format, func(t *testing.T) {
			body := exportPDF(t, format)
			text := pdfStreamText(body)
			for _, want := range []string{"Wilbur Wright", "US.CFI.1903"} {
				if !strings.Contains(text, want) {
					t.Errorf("%s PDF does not print %q in the remarks column", format, want)
				}
			}
			if !bytes.Contains(body, []byte("/Subtype /Image")) {
				t.Errorf("%s PDF embeds no signature image", format)
			}
		})
	}

	t.Run("voided signature stops printing", func(t *testing.T) {
		requireStatus(t, c.POST("/flights/"+flight.ID+"/signatures/"+sig.ID+"/void", map[string]interface{}{
			"reason": "Signed against the wrong flight",
		}), http.StatusOK)

		body := exportPDF(t, "easa")
		if strings.Contains(pdfStreamText(body), "Wilbur Wright") {
			t.Error("voided signature still prints an instructor")
		}
		if bytes.Contains(body, []byte("/Subtype /Image")) {
			t.Error("voided signature still embeds its ink")
		}
	})
}

// TestExportPDFPrintsCoPilotFlights covers the printed logbook's coverage: a
// flight flown as co-pilot (a third-party PIC in the crew, on an aircraft
// certificated for two pilots) reaches both the logbook sheets and the totals
// summary, like any other logged flight.
func TestExportPDFPrintsCoPilotFlights(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("pdf-role"), "SecurePass123!", "Amelia Earhart")

	// The co-pilot seat only exists on a multi-pilot aircraft, so the fleet
	// entry is what makes the second flight loggable as co-pilot time.
	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-ESIC", "type": "DA42",
		"make": "Diamond", "model": "DA42", "isMultiPilot": true,
	}), http.StatusCreated)

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(2), "aircraftReg": "D-EPIC", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		"picName": "Self",
	}), http.StatusCreated)

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(1), "aircraftReg": "D-ESIC", "aircraftType": "DA42",
		"departureIcao": "EDDS", "arrivalIcao": "EDNY",
		"offBlockTime": "10:00", "onBlockTime": "12:00", "landings": 1,
		"crewMembers": []map[string]interface{}{
			{"name": "Otto Lilienthal", "role": "PIC"},
		},
	}), http.StatusCreated)

	for _, format := range []string{"easa", "faa"} {
		t.Run(format, func(t *testing.T) {
			resp := c.GET("/exports/pdf?format=" + format + "&layout=single")
			requireStatus(t, resp, http.StatusOK)
			text := pdfStreamText(resp.Body)
			for _, reg := range []string{"D-EPIC", "D-ESIC"} {
				if !strings.Contains(text, reg) {
					t.Errorf("%s PDF is missing the %s flight", format, reg)
				}
			}
		})
	}

	t.Run("summary totals every flight", func(t *testing.T) {
		resp := c.GET("/exports/pdf?format=summary")
		requireStatus(t, resp, http.StatusOK)
		text := pdfStreamText(resp.Body)
		if !strings.Contains(text, "(3:30)") {
			t.Errorf("summary should total both flights (3:30):\n%s", text)
		}
	})
}

// The same crew list in a single-pilot aircraft is not a co-pilot flight: a
// GA pilot sitting beside the pilot-in-command of a C172 is not a crew member
// and logs nothing (EASA FCL.010; 14 CFR 61.51(f)). The row survives as the
// record of the trip and drops out of every total.
func TestExportPDFExcludesPassengerFlights(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("pdf-pax"), "SecurePass123!", "Amelia Earhart")

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EPAX", "type": "C172", "make": "Cessna", "model": "172",
	}), http.StatusCreated)

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(2), "aircraftReg": "D-EPIC", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		"picName": "Self",
	}), http.StatusCreated)

	resp := c.POST("/flights", map[string]interface{}{
		"date": pastDate(1), "aircraftReg": "D-EPAX", "aircraftType": "C172",
		"departureIcao": "EDDS", "arrivalIcao": "EDNY",
		"offBlockTime": "10:00", "onBlockTime": "12:00", "landings": 1,
		"crewMembers": []map[string]interface{}{
			{"name": "Otto Lilienthal", "role": "PIC"},
		},
	})
	requireStatus(t, resp, http.StatusCreated)

	var pax map[string]interface{}
	resp.JSON(&pax)
	if pax["isPassenger"] != true {
		t.Fatalf("isPassenger = %v, want true — a C172 has no co-pilot seat", pax["isPassenger"])
	}
	for _, field := range []string{"totalTime", "picTime", "sicTime", "multiPilotTime"} {
		if got := gi(pax, field); got != 0 {
			t.Errorf("%s = %d, want 0 on a passenger flight", field, got)
		}
	}
	if pax["departureIcao"] != "EDDS" {
		t.Errorf("departureIcao = %v, want EDDS — the trip record must survive", pax["departureIcao"])
	}

	t.Run("summary totals only the flown flight", func(t *testing.T) {
		resp := c.GET("/exports/pdf?format=summary")
		requireStatus(t, resp, http.StatusOK)
		text := pdfStreamText(resp.Body)
		if !strings.Contains(text, "(1:30)") {
			t.Errorf("summary should total only the flown flight (1:30):\n%s", text)
		}
		if strings.Contains(text, "(3:30)") {
			t.Errorf("passenger time must not reach the totals:\n%s", text)
		}
	})
}

// TestExportCSVSearchWithTotals verifies the CSV export honours the GET /flights
// filters across page boundaries and appends a totals row on request.
func TestExportCSVSearchWithTotals(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("csv-search"), "SecurePass123!", "CSVSearch")

	for i := 0; i < 25; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": pastDate(i), "aircraftReg": "D-EMAT", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		}), http.StatusCreated)
	}
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": pastDate(1), "aircraftReg": "D-EOTH", "aircraftType": "PA28",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "10:00", "landings": 1,
	}), http.StatusCreated)

	t.Run("q filter exports every match with totals", func(t *testing.T) {
		resp := c.GET("/exports/csv?q=reg:D-EMAT&totals=true")
		requireStatus(t, resp, http.StatusOK)
		lines := strings.Split(strings.TrimSpace(string(resp.Body)), "\n")
		if len(lines) != 27 {
			t.Fatalf("got %d lines, want header + 25 flights + totals", len(lines))
		}
		if strings.Contains(string(resp.Body), "D-EOTH") {
			t.Error("non-matching flight exported")
		}
		last := lines[len(lines)-1]
		if !strings.HasPrefix(last, "Total (25 flights)") || !strings.Contains(last, "25,0h") {
			t.Errorf("unexpected totals row: %s", last)
		}
	})

	t.Run("aircraftReg filter and EASA format", func(t *testing.T) {
		resp := c.GET("/exports/csv?format=easa&aircraftReg=D-EOTH&totals=true")
		requireStatus(t, resp, http.StatusOK)
		lines := strings.Split(strings.TrimSpace(string(resp.Body)), "\n")
		if len(lines) != 3 {
			t.Fatalf("got %d lines, want header + 1 flight + totals", len(lines))
		}
		if !strings.HasPrefix(lines[2], "Total (1 flights)") || !strings.Contains(lines[2], "2:00") {
			t.Errorf("unexpected totals row: %s", lines[2])
		}
	})

	t.Run("no totals row by default", func(t *testing.T) {
		resp := c.GET("/exports/csv")
		requireStatus(t, resp, http.StatusOK)
		if strings.Contains(string(resp.Body), "Total (") {
			t.Error("totals row present without totals=true")
		}
	})

	t.Run("invalid query is rejected", func(t *testing.T) {
		requireStatus(t, c.GET("/exports/csv?q=%28reg%3AD-EMAT"), http.StatusBadRequest)
	})
}

// TestExportPDFLicenceLogbook covers the licence-filtered PDF and the web
// logbook list: classes compared case-insensitively, ultralights filtered by
// the rating's kind, and flights credited from other classes included and
// marked, never for a towed launch.
func TestExportPDFLicenceLogbook(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("pdf-logbook"), "SecurePass123!", "Mehmet Sabine")

	addAircraft := func(reg, class, kind string) {
		t.Helper()
		body := map[string]interface{}{
			"registration": reg, "type": "T", "make": "M", "model": "M", "aircraftClass": class,
		}
		if kind != "" {
			body["ulKind"] = kind
		}
		requireStatus(t, c.POST("/aircraft", body), http.StatusCreated)
	}
	addFlight := func(reg, acType, remarks, launch string, daysAgo int) {
		t.Helper()
		body := map[string]interface{}{
			"date": pastDate(daysAgo), "aircraftReg": reg, "aircraftType": acType,
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		}
		if remarks != "" {
			body["remarks"] = remarks
		}
		if launch != "" {
			body["launchMethod"] = launch
		}
		requireStatus(t, c.POST("/flights", body), http.StatusCreated)
	}
	addLicence := func(authority, typ string, ratings ...map[string]interface{}) string {
		t.Helper()
		resp := c.POST("/licenses", map[string]interface{}{
			"regulatoryAuthority": authority, "licenseType": typ,
			"licenseNumber": fmt.Sprintf("%s-%s-%d", authority, typ, time.Now().UnixNano()),
			"issueDate":     "2020-01-01", "issuingAuthority": authority,
		})
		requireStatus(t, resp, http.StatusCreated)
		var lic map[string]interface{}
		resp.JSON(&lic)
		id := lic["id"].(string)
		for _, r := range ratings {
			r["issueDate"] = "2020-01-01"
			requireStatus(t, c.POST(fmt.Sprintf("/licenses/%s/ratings", id), r), http.StatusCreated)
		}
		return id
	}
	logbookRegs := func(t *testing.T, licID string) map[string]int {
		t.Helper()
		resp := c.GET(fmt.Sprintf("/flights?logbookLicenseId=%s&pageSize=100", licID))
		requireStatus(t, resp, http.StatusOK)
		var r struct {
			Data []struct {
				AircraftReg string `json:"aircraftReg"`
			} `json:"data"`
		}
		if err := resp.JSON(&r); err != nil {
			t.Fatalf("decode flights: %v", err)
		}
		regs := map[string]int{}
		for _, f := range r.Data {
			regs[f.AircraftReg]++
		}
		return regs
	}
	exportText := func(t *testing.T, licID, format string) string {
		t.Helper()
		resp := c.GET(fmt.Sprintf("/exports/pdf?format=%s&layout=single&logbookLicenseId=%s", format, licID))
		requireStatus(t, resp, http.StatusOK)
		return pdfStreamText(resp.Body)
	}

	addAircraft("D-MXYZ", "ULTRALIGHT", "THREE_AXIS")
	addAircraft("D-MTRK", "ULTRALIGHT", "WEIGHT_SHIFT")
	addAircraft("D-EABC", "sep_land", "")
	addAircraft("D-KTMG", "TMG", "")
	addAircraft("D-1234", "GLIDER", "")

	addFlight("D-MXYZ", "C42", "Platzrunden", "", 1)
	addFlight("D-MTRK", "TRIKE", "Trike local", "", 2)
	addFlight("D-EABC", "C172", "Rundflug", "", 3)
	addFlight("D-KTMG", "SF25", "Motorsegler", "self-launch", 4)
	addFlight("D-KTMG", "SF25", "Towed TMG", "aerotow", 5)
	addFlight("D-1234", "ASK21", "Winde", "winch", 6)

	threeAxis := addLicence("DULV", "UL", map[string]interface{}{"classType": "ULTRALIGHT", "ulKind": "THREE_AXIS"})
	trike := addLicence("DULV", "UL", map[string]interface{}{"classType": "ULTRALIGHT", "ulKind": "WEIGHT_SHIFT"})
	lapl := addLicence("EASA", "LAPL(A)", map[string]interface{}{"classType": "SEP_LAND"})

	t.Run("M2 three-axis UL licence prints SEP and TMG flights as credited", func(t *testing.T) {
		for _, format := range []string{"easa", "faa"} {
			text := exportText(t, threeAxis, format)
			for _, want := range []string{"D-MXYZ", "[Credited] Rundflug", "[Credited] Motorsegler"} {
				if !strings.Contains(text, want) {
					t.Errorf("%s PDF lacks %q", format, want)
				}
			}
			for _, absent := range []string{"D-MTRK", "D-1234", "Towed TMG", "[Credited] Platzrunden"} {
				if strings.Contains(text, absent) {
					t.Errorf("%s PDF contains %q", format, absent)
				}
			}
		}
	})

	t.Run("S trike licence excludes three-axis flights", func(t *testing.T) {
		text := exportText(t, trike, "easa")
		if !strings.Contains(text, "D-MTRK") {
			t.Error("PDF lacks the trike flight")
		}
		for _, absent := range []string{"D-MXYZ", "D-EABC", "[Credited]"} {
			if strings.Contains(text, absent) {
				t.Errorf("PDF contains %q", absent)
			}
		}
	})

	t.Run("N2 towed glider launch never credited to LAPL(A)", func(t *testing.T) {
		text := exportText(t, lapl, "easa")
		for _, want := range []string{"D-EABC", "[Credited] Platzrunden", "[Credited] Motorsegler"} {
			if !strings.Contains(text, want) {
				t.Errorf("PDF lacks %q", want)
			}
		}
		for _, absent := range []string{"D-1234", "Towed TMG", "D-MTRK"} {
			if strings.Contains(text, absent) {
				t.Errorf("PDF contains %q", absent)
			}
		}
	})

	t.Run("web logbook list uses the same scope", func(t *testing.T) {
		cases := []struct {
			name  string
			licID string
			want  map[string]int
		}{
			{"M2 three-axis", threeAxis, map[string]int{"D-MXYZ": 1, "D-EABC": 1, "D-KTMG": 1}},
			{"S trike", trike, map[string]int{"D-MTRK": 1}},
			{"N2 LAPL(A)", lapl, map[string]int{"D-EABC": 1, "D-MXYZ": 1, "D-KTMG": 1}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := logbookRegs(t, tc.licID)
				if fmt.Sprint(got) != fmt.Sprint(tc.want) {
					t.Errorf("logbook registrations = %v, want %v", got, tc.want)
				}
			})
		}
	})
}
