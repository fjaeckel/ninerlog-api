//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// NinerLog must always be able to re-import its own CSV export.
//
// This is the one interchange path where we own both ends, so it is the one
// that must never regress: a pilot moving to another installation, restoring an
// archived export, or splitting one logbook across accounts depends on it, and
// unlike a third-party format there is nobody else to blame when it breaks.
//
// The guarantee is asserted over the full export surface — every column layout
// crossed with every date-format and decimal-separator preference.
//
// Only the standard layout honours those preferences today; the EASA and FAA
// layouts hardcode their regulatory date and duration conventions. Their
// repeated combinations are kept anyway so that wiring a preference into them
// later lands on coverage that already exists.
//
// internal/api/handlers/export_import_roundtrip_test.go asserts the same
// invariant at unit level without Docker. This test exists because the unit
// test cannot see what the round trip does through real HTTP against a real
// database: that the flights genuinely land, are attributed to the right
// aircraft, and are recognised as duplicates on a second pass.

type roundTripCase struct {
	layout    string // export format query value
	dateFmt   string // user preference
	decimal   string // user preference
	wantFmt   string // ImportFormat the upload must report
	tolerance int    // total-time tolerance in minutes
}

// The FAA layout has no time-of-day columns: its total is a decimal-hours cell
// rounded to one decimal (0.1h = 6 min), so the worst case is half a step. The
// standard and EASA layouts write block times, from which the importer derives
// an exact total.
func roundTripCases() []roundTripCase {
	var cases []roundTripCase
	for _, layout := range []struct {
		name      string
		wantFmt   string
		tolerance int
	}{
		{"standard", "NINERLOG_CSV", 0},
		{"easa", "EASA_CSV", 0},
		{"faa", "FAA_CSV", 3},
	} {
		for _, dateFmt := range []string{"DD.MM.YYYY", "MM/DD/YYYY", "YYYY-MM-DD"} {
			for _, decimal := range []string{"dot", "comma"} {
				cases = append(cases, roundTripCase{
					layout:    layout.name,
					dateFmt:   dateFmt,
					decimal:   decimal,
					wantFmt:   layout.wantFmt,
					tolerance: layout.tolerance,
				})
			}
		}
	}
	return cases
}

func TestExportImportRoundTrip_EveryLayoutAndPreference(t *testing.T) {
	for _, tc := range roundTripCases() {
		name := fmt.Sprintf("%s/%s/%s", tc.layout, tc.dateFmt, tc.decimal)
		t.Run(name, func(t *testing.T) {
			// A fresh account per combination: the import must land in an empty
			// logbook, or duplicate detection would mask a failure to import.
			c := NewE2EClient(t)
			registerAndLogin(t, c, uniqueEmail("roundtrip"), "SecurePass123!", "RoundTrip")

			pr := c.PATCH("/users/me", map[string]interface{}{
				"dateFormat":       tc.dateFmt,
				"decimalSeparator": tc.decimal,
			})
			requireStatus(t, pr, http.StatusOK)

			created := c.POST("/flights", map[string]interface{}{
				"date":          pastDate(30),
				"aircraftReg":   "D-ERTP",
				"aircraftType":  "C172",
				"departureIcao": "EDDF",
				"arrivalIcao":   "EDDM",
				"offBlockTime":  "08:15",
				"onBlockTime":   "09:45",
				"landings":      3,
			})
			requireStatus(t, created, http.StatusCreated)
			var original map[string]interface{}
			created.JSON(&original)
			originalTotal, _ := original["totalTime"].(float64)

			exportResp := c.GET("/exports/csv?format=" + tc.layout)
			requireStatus(t, exportResp, http.StatusOK)
			exported := string(exportResp.Body)

			// Re-import into a second, empty account so the flights have to be
			// created rather than skipped as duplicates of the originals.
			c2 := NewE2EClient(t)
			registerAndLogin(t, c2, uniqueEmail("roundtrip-target"), "SecurePass123!", "RoundTripTarget")

			upload := uploadCSV(t, c2, "ninerlog_"+tc.layout+".csv", exported)
			requireStatus(t, upload, http.StatusOK)
			var up map[string]interface{}
			upload.JSON(&up)

			if up["format"] != tc.wantFmt {
				t.Errorf("our own %s export was detected as %v, want %s",
					tc.layout, up["format"], tc.wantFmt)
			}
			if up["detectedTemplate"] == nil {
				t.Errorf("our own %s export produced no detectedTemplate", tc.layout)
			}
			token, _ := up["uploadToken"].(string)
			suggested, _ := up["suggestedMappings"].([]interface{})
			if len(suggested) == 0 {
				t.Fatalf("no suggested mappings for our own %s export", tc.layout)
			}

			// Preview must report the row as importable, with no field failing
			// to parse under this date/decimal preference.
			prev := c2.POST("/imports/preview", map[string]interface{}{
				"uploadToken":    token,
				"mappings":       suggested,
				"skipDuplicates": false,
			})
			requireStatus(t, prev, http.StatusOK)
			var preview map[string]interface{}
			prev.JSON(&preview)

			flights, _ := preview["flights"].([]interface{})
			if len(flights) != 1 {
				t.Fatalf("previewed %d rows, want 1: %s", len(flights), string(prev.Body))
			}
			row := flights[0].(map[string]interface{})
			if row["status"] == "error" {
				t.Fatalf("our own %s export previews as an error row: %v", tc.layout, row["errors"])
			}
			mapped := row["flight"].(map[string]interface{})
			if mapped["departureIcao"] != "EDDF" || mapped["arrivalIcao"] != "EDDM" {
				t.Errorf("airports = %v → %v, want EDDF → EDDM",
					mapped["departureIcao"], mapped["arrivalIcao"])
			}
			if mapped["aircraftReg"] != "D-ERTP" {
				t.Errorf("aircraftReg = %v, want D-ERTP", mapped["aircraftReg"])
			}

			// Confirm, then read the flight back and compare against the source.
			conf := c2.POST("/imports/confirm", map[string]interface{}{"uploadToken": token})
			requireStatus(t, conf, http.StatusCreated)
			var result map[string]interface{}
			conf.JSON(&result)
			if imported, _ := result["importedCount"].(float64); imported != 1 {
				t.Fatalf("importedCount = %v, want 1: %s", result["importedCount"], string(conf.Body))
			}
			if result["format"] != tc.wantFmt {
				t.Errorf("recorded import format = %v, want %s", result["format"], tc.wantFmt)
			}

			list := c2.GET("/flights")
			requireStatus(t, list, http.StatusOK)
			var page map[string]interface{}
			list.JSON(&page)
			data, _ := page["data"].([]interface{})
			if len(data) != 1 {
				t.Fatalf("target account holds %d flights after import, want 1", len(data))
			}
			imported := data[0].(map[string]interface{})

			if imported["aircraftReg"] != "D-ERTP" {
				t.Errorf("imported aircraftReg = %v, want D-ERTP", imported["aircraftReg"])
			}
			if imported["aircraftType"] != "C172" {
				t.Errorf("imported aircraftType = %v, want C172", imported["aircraftType"])
			}
			importedTotal, _ := imported["totalTime"].(float64)
			if diff := importedTotal - originalTotal; diff > float64(tc.tolerance) || diff < -float64(tc.tolerance) {
				t.Errorf("imported totalTime = %v, want %v ±%d", importedTotal, originalTotal, tc.tolerance)
			}
			if landings, _ := imported["allLandings"].(float64); landings != 3 {
				t.Errorf("imported allLandings = %v, want 3", landings)
			}

			// The fleet must be backfilled from the import, not left empty.
			fleetResp := c2.GET("/aircraft")
			requireStatus(t, fleetResp, http.StatusOK)
			var fleetPage map[string]interface{}
			fleetResp.JSON(&fleetPage)
			fleet, _ := fleetPage["data"].([]interface{})
			if len(fleet) != 1 {
				t.Errorf("target fleet has %d aircraft after import, want 1: %s",
					len(fleet), string(fleetResp.Body))
			}
		})
	}
}

// Re-importing an export into the account it came from must be a no-op, not a
// second copy of the logbook. This is the mistake a pilot is most likely to
// make with their own export, so duplicate detection has to hold across the
// export/import boundary for every layout.
func TestExportImportRoundTrip_ReimportIntoSameAccountIsDeduplicated(t *testing.T) {
	for _, layout := range []string{"standard", "easa", "faa"} {
		t.Run(layout, func(t *testing.T) {
			c := NewE2EClient(t)
			registerAndLogin(t, c, uniqueEmail("roundtrip-dedup"), "SecurePass123!", "Dedup")

			for i, date := range []string{pastDate(20), pastDate(19)} {
				r := c.POST("/flights", map[string]interface{}{
					"date": date, "aircraftReg": "D-EDUPE", "aircraftType": "C152",
					"departureIcao": "EDDF", "arrivalIcao": "EDDM",
					"offBlockTime": fmt.Sprintf("0%d:00", 8+i), "onBlockTime": fmt.Sprintf("0%d:30", 9+i),
					"landings": 1,
				})
				requireStatus(t, r, http.StatusCreated)
			}

			exportResp := c.GET("/exports/csv?format=" + layout)
			requireStatus(t, exportResp, http.StatusOK)

			upload := uploadCSV(t, c, "own_export.csv", string(exportResp.Body))
			requireStatus(t, upload, http.StatusOK)
			var up map[string]interface{}
			upload.JSON(&up)
			token := up["uploadToken"].(string)
			suggested, _ := up["suggestedMappings"].([]interface{})

			prev := c.POST("/imports/preview", map[string]interface{}{
				"uploadToken":    token,
				"mappings":       suggested,
				"skipDuplicates": true,
			})
			requireStatus(t, prev, http.StatusOK)
			var preview map[string]interface{}
			prev.JSON(&preview)

			if dupes, _ := preview["duplicateCount"].(float64); dupes != 2 {
				t.Errorf("re-importing our own %s export flags %v of 2 rows as duplicates: %s",
					layout, preview["duplicateCount"], string(prev.Body))
			}

			conf := c.POST("/imports/confirm", map[string]interface{}{"uploadToken": token})
			requireStatus(t, conf, http.StatusCreated)

			list := c.GET("/flights")
			requireStatus(t, list, http.StatusOK)
			var page map[string]interface{}
			list.JSON(&page)
			data, _ := page["data"].([]interface{})
			if len(data) != 2 {
				t.Errorf("logbook holds %d flights after re-importing its own %s export, want 2",
					len(data), layout)
			}
		})
	}
}
