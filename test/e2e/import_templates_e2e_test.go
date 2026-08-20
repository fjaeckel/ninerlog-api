//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// todayGerman renders today the way a German-language club export writes it.
func todayGerman() string { return time.Now().Format("02.01.2006") }

// The import screen asks for the catalogue before a file is chosen, so it can
// tell a pilot whether their current logbook is covered and how to export from
// it. It must be authenticated, complete, and self-describing.
func TestImportTemplates_Catalogue(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("requires authentication", func(t *testing.T) {
		anon := NewE2EClient(t)
		resp := anon.GET("/imports/templates")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("unauthenticated GET /imports/templates = %d, want 401", resp.StatusCode)
		}
	})

	registerAndLogin(t, c, uniqueEmail("import-templates"), "SecurePass123!", "Templates")

	resp := c.GET("/imports/templates")
	requireStatus(t, resp, http.StatusOK)

	var result struct {
		Templates []struct {
			ID           string   `json:"id"`
			Name         string   `json:"name"`
			Description  string   `json:"description"`
			Confidence   string   `json:"confidence"`
			Regions      []string `json:"regions"`
			ExportSteps  []string `json:"exportSteps"`
			AutoDetected bool     `json:"autoDetected"`
		} `json:"templates"`
	}
	resp.JSON(&result)

	if len(result.Templates) < 10 {
		t.Fatalf("catalogue returned %d templates, want at least 10", len(result.Templates))
	}

	byID := make(map[string]bool, len(result.Templates))
	for _, tpl := range result.Templates {
		byID[tpl.ID] = true
		if tpl.Name == "" || tpl.Description == "" {
			t.Errorf("template %s: missing name or description", tpl.ID)
		}
		if len(tpl.ExportSteps) == 0 {
			t.Errorf("template %s: no export instructions — the pilot cannot get the file out", tpl.ID)
		}
		if len(tpl.Regions) == 0 {
			t.Errorf("template %s: no regions", tpl.ID)
		}
		if tpl.Confidence != "exact" && tpl.Confidence != "best-effort" {
			t.Errorf("template %s: confidence = %q", tpl.ID, tpl.Confidence)
		}
	}

	// The logbooks pilots actually migrate from must all be listed.
	for _, want := range []string{
		"FOREFLIGHT_CSV", "LOGTEN_CSV", "MYFLIGHTBOOK_CSV", "CAPZLOG_CSV",
		"FLYLOG_CSV", "WADER_CSV", "VEREINSFLIEGER_CSV", "VEREINSFLIEGER_EXTENDED_CSV",
		"SKYDEMON_CSV", "EASA_CSV", "FAA_CSV", "NINERLOG_CSV", "CSV",
	} {
		if !byID[want] {
			t.Errorf("catalogue is missing %s", want)
		}
	}
}

// Upload must report which template matched, so the mapping screen can say what
// it recognised instead of presenting an unexplained set of pre-filled columns.
func TestImportTemplates_DetectedOnUpload(t *testing.T) {
	cases := []struct {
		name       string
		filename   string
		csv        string
		wantFormat string
	}{
		{
			name:     "MyFlightbook",
			filename: "myflightbook.csv",
			csv: "Date,Tail Number,Model,Category/Class,Route,Comments,Approaches,Hold,Landings,FS Day Landings,FS Night Landings,X-Country,Night,Simulated Instrument,IMC,Ground Simulator,Dual Received,CFI,SIC,PIC,Total Flight Time,Hobbs Start,Hobbs End,Engine Start,Engine End,Flight Start,Flight End,Flight ID\n" +
				fmt.Sprintf("%s,N12345,C172,airplane_single_engine_land,KSFO KOAK,Bay tour,1,0,3,1,0,1.5,0,0,0.2,0,0,0,0,1.5,1.5,,,,,,,1\n", today()),
			wantFormat: "MYFLIGHTBOOK_CSV",
		},
		{
			name:     "LogTen Pro",
			filename: "logten.csv",
			csv: "flight_flightDate,flight_selectedAircraftID,flight_from,flight_to,flight_totalTime,flight_dayLandings,flight_nightLandings,flight_dualReceived,flight_selectedCrewPIC,flight_remarks\n" +
				fmt.Sprintf("%s,N778LT,KSFO,KLAX,1.4,1,0,0.0,Alex Rivera,Cross country\n", today()),
			wantFormat: "LOGTEN_CSV",
		},
		{
			// The real standard export: semicolon-delimited, every cell quoted,
			// airborne times only, durations as bare whole minutes, and places
			// written "Name ICAO".
			name:     "Vereinsflieger (standard)",
			filename: "vereinsflieger.csv",
			csv: "\"Datum\";\"Lfz.\";\"Pilot\";\"Begleiter/FI\";\"Start\";\"Landung\";\"Flugzeit\";\"Startort\";\"Landeort\";\"Landungen\";\"S.-Art\";\"Flugart\";\"Abr.\";\"Verein\";\"Bemerkung\"\n" +
				fmt.Sprintf("\"%s\";\"D-EABC\";\"Rivera, Alex\";\"\";\"09:12\";\"10:47\";\"95\";\"Uetersen EDHE\";\"Stade EDHS\";\"1\";\"E\";\"N\";\"K\";\"Aero-Club Musterstadt e.V.\";\"\"\n", todayGerman()),
			wantFormat: "VEREINSFLIEGER_CSV",
		},
		{
			// The extended export is the same list plus the block columns, and
			// must not be reported as the standard one: the two differ by three
			// columns out of sixteen and share every other alias.
			name:     "Vereinsflieger (extended)",
			filename: "vereinsflieger-extended.csv",
			csv: "\"Datum\";\"Lfz.\";\"Pilot\";\"Begleiter/FI\";\"Start\";\"Landung\";\"Flugzeit\";\"Startort\";\"Landeort\";\"Landungen\";\"Off-Block\";\"On-Block\";\"Blockzeit in Minuten\";\"Flugart\";\"Bemerkung\";\"Abr.\"\n" +
				fmt.Sprintf("\"%s\";\"D-EABC\";\"Rivera, Alex\";\"\";\"09:12\";\"10:47\";\"95\";\"Uetersen EDHE\";\"Stade EDHS\";\"1\";\"09:04\";\"10:56\";\"112\";\"N\";\"\";\"K\"\n", todayGerman()),
			wantFormat: "VEREINSFLIEGER_EXTENDED_CSV",
		},
		{
			name:     "capzlog.aero",
			filename: "capzlog.csv",
			csv: "Date,Departure Place,Departure Time,Arrival Place,Arrival Time,Aircraft Model,Aircraft Registration,Single Pilot SE,Single Pilot ME,Multi Pilot,Total Time of Flight,Name(s) PIC,Landings Day,Landings Night,Night,IFR,PIC,Co-Pilot,Dual,Instructor,FSTD Date,FSTD Type,FSTD Total Time,Remarks and Endorsements\n" +
				fmt.Sprintf("%s,LSZH,09:00,LSGG,10:00,C172,HB-ABC,1.0,,,1.0,Anna Berger,1,0,0,0,1.0,,,,,,,Training\n", today()),
			wantFormat: "CAPZLOG_CSV",
		},
		{
			name:     "unrecognised file still maps by name",
			filename: "mysheet.csv",
			csv: "Date,Registration,Type,From,To,Off Block,On Block,Total Time,Remarks\n" +
				fmt.Sprintf("%s,D-EABC,C172,EDDF,EDDM,10:15,11:45,1:30,Spreadsheet\n", today()),
			wantFormat: "", // any — asserted below as "mappings are still suggested"
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// One account per case. A pending upload is only released by a
			// confirm or the session TTL, and maxSessionsPerUser is 3, so
			// detecting more than three formats under one account rejects the
			// fourth upload with 429 rather than reporting its format.
			c := NewE2EClient(t)
			registerAndLogin(t, c, uniqueEmail("import-detect"), "SecurePass123!", "Detect")

			resp := uploadCSV(t, c, tc.filename, tc.csv)
			requireStatus(t, resp, http.StatusOK)

			var result struct {
				Format            string `json:"format"`
				SuggestedMappings []struct {
					SourceColumn string `json:"sourceColumn"`
					TargetField  string `json:"targetField"`
				} `json:"suggestedMappings"`
				DetectedTemplate *struct {
					ID          string   `json:"id"`
					Name        string   `json:"name"`
					ExportSteps []string `json:"exportSteps"`
				} `json:"detectedTemplate"`
			}
			resp.JSON(&result)

			if len(result.SuggestedMappings) == 0 {
				t.Fatalf("no suggested mappings for %s", tc.name)
			}

			// Whatever was detected, the fields a flight cannot be created
			// without have to be mapped, or the import is dead on arrival.
			mapped := make(map[string]bool)
			for _, m := range result.SuggestedMappings {
				mapped[m.TargetField] = true
			}
			for _, field := range []string{"date", "aircraftReg", "totalTime"} {
				if !mapped[field] {
					t.Errorf("%s: %s was not mapped", tc.name, field)
				}
			}

			if tc.wantFormat == "" {
				return
			}
			if result.Format != tc.wantFormat {
				t.Errorf("format = %q, want %q", result.Format, tc.wantFormat)
			}
			if result.DetectedTemplate == nil {
				t.Fatalf("%s: no detectedTemplate on the upload response", tc.name)
			}
			if result.DetectedTemplate.ID != tc.wantFormat {
				t.Errorf("detectedTemplate.id = %q, want %q", result.DetectedTemplate.ID, tc.wantFormat)
			}
			if result.DetectedTemplate.Name == "" || len(result.DetectedTemplate.ExportSteps) == 0 {
				t.Errorf("%s: detectedTemplate is not renderable (name=%q, steps=%d)",
					tc.name, result.DetectedTemplate.Name, len(result.DetectedTemplate.ExportSteps))
			}
		})
	}
}

// A MyFlightbook file has no departure/arrival columns at all — the sector is
// in the route string. The whole import must survive that, end to end, and the
// recorded import must name the source format.
func TestImportTemplates_MyFlightbookEndToEnd(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import-mfb"), "SecurePass123!", "MFB")

	csv := "Date,Tail Number,Model,Route,Comments,Approaches,Hold,Landings,FS Day Landings,FS Night Landings,Night,Simulated Instrument,IMC,Dual Received,CFI,PIC,Total Flight Time\n" +
		fmt.Sprintf("%s,N54321,PA28,KSFO KSJC KOAK,Bay tour,1,0,3,1,0,0,0,0.2,0,0,1.5,1.5\n", today())

	resp := uploadCSV(t, c, "myflightbook.csv", csv)
	requireStatus(t, resp, http.StatusOK)
	var upload map[string]interface{}
	resp.JSON(&upload)
	token := upload["uploadToken"].(string)
	suggested, _ := upload["suggestedMappings"].([]interface{})

	t.Run("preview resolves both airports from the route", func(t *testing.T) {
		resp := c.POST("/imports/preview", map[string]interface{}{
			"uploadToken":    token,
			"mappings":       suggested,
			"skipDuplicates": false,
		})
		requireStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		resp.JSON(&result)

		flights, _ := result["flights"].([]interface{})
		if len(flights) != 1 {
			t.Fatalf("previewed %d rows, want 1: %s", len(flights), string(resp.Body))
		}
		row := flights[0].(map[string]interface{})
		if row["status"] == "error" {
			t.Fatalf("row errored: %v", row["errors"])
		}
		flight := row["flight"].(map[string]interface{})
		if flight["departureIcao"] != "KSFO" {
			t.Errorf("departureIcao = %v, want KSFO", flight["departureIcao"])
		}
		if flight["arrivalIcao"] != "KOAK" {
			t.Errorf("arrivalIcao = %v, want KOAK", flight["arrivalIcao"])
		}
	})

	t.Run("confirm records the source format", func(t *testing.T) {
		resp := c.POST("/imports/confirm", map[string]interface{}{"uploadToken": token})
		requireStatus(t, resp, http.StatusCreated)
		var result map[string]interface{}
		resp.JSON(&result)

		if result["format"] != "MYFLIGHTBOOK_CSV" {
			t.Errorf("import format = %v, want MYFLIGHTBOOK_CSV", result["format"])
		}
		if imported, _ := result["importedCount"].(float64); imported != 1 {
			t.Errorf("importedCount = %v, want 1: %s", result["importedCount"], string(resp.Body))
		}

		// The history row must carry the format too — it is what the admin
		// dashboard groups by.
		hist := c.GET("/imports")
		requireStatus(t, hist, http.StatusOK)
		var page map[string]interface{}
		hist.JSON(&page)
		rows, _ := page["data"].([]interface{})
		if len(rows) == 0 {
			t.Fatal("no import history rows")
		}
		if rows[0].(map[string]interface{})["format"] != "MYFLIGHTBOOK_CSV" {
			t.Errorf("history format = %v, want MYFLIGHTBOOK_CSV",
				rows[0].(map[string]interface{})["format"])
		}
	})
}
