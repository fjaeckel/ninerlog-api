//go:build e2e

package e2e_test

import (
	"encoding/json"
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
		{"weblogbook", "WEB_LOGBOOK_CSV", 0},
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
	for _, layout := range []string{"standard", "easa", "faa", "weblogbook"} {
		t.Run(layout, func(t *testing.T) {
			c := NewE2EClient(t)
			registerAndLogin(t, c, uniqueEmail("roundtrip-dedup"), "SecurePass123!", "Dedup")

			for i, date := range []string{pastDate(20), pastDate(19)} {
				r := c.POST("/flights", map[string]interface{}{
					"date": date, "aircraftReg": "D-EDUPE", "aircraftType": "C152",
					"departureIcao": "EDDF", "arrivalIcao": "EDDM",
					"offBlockTime": fmt.Sprintf("%02d:00", 8+i), "onBlockTime": fmt.Sprintf("%02d:30", 9+i),
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

// TestExportImportRoundTrip_PilotProfile asserts that the pilot profile's mode,
// intents and acknowledgements survive GET /exports/json → POST /imports/json
// into a fresh account, and that derived evidence is not exported.
func TestExportImportRoundTrip_PilotProfile(t *testing.T) {
	source := setupCurrencyUser(t, "pp-export")
	licID := createLicenceNumbered(t, source, "LBA", "SPL", "DE.SFCL.RT")
	createRatingCur(t, source, licID, "GLIDER", nil)
	patchPilotProfile(t, source, map[string]interface{}{
		"mode":        "everything",
		"intents":     map[string]string{"IFR": "goal", "MULTI_CREW": "off"},
		"acknowledge": []string{"SAILPLANE"},
	})
	sourceProfile := getPilotProfile(t, source)

	backupResp := source.GET("/exports/json")
	requireStatus(t, backupResp, http.StatusOK)
	var backup map[string]interface{}
	if err := json.Unmarshal(backupResp.Body, &backup); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	section, ok := backup["pilotProfile"].(map[string]interface{})
	if !ok {
		t.Fatalf("backup is missing the pilotProfile section: %v", backup["pilotProfile"])
	}
	if section["mode"] != "everything" {
		t.Errorf("pilotProfile.mode = %v", section["mode"])
	}
	disciplines, _ := section["disciplines"].(map[string]interface{})
	if len(disciplines) != 3 {
		t.Errorf("pilotProfile.disciplines = %v, want IFR, MULTI_CREW and SAILPLANE", disciplines)
	}
	if _, leaked := section["evidence"]; leaked {
		t.Error("derived evidence was exported")
	}

	dest := NewE2EClient(t)
	registerAndLogin(t, dest, uniqueEmail("pp-import"), "SecurePass123!", "PP Import")
	restore := dest.Do("POST", "/imports/json", backup)
	requireStatus(t, restore, http.StatusOK)
	var summary struct {
		PilotProfileImported bool `json:"pilotProfileImported"`
	}
	if err := restore.JSON(&summary); err != nil || !summary.PilotProfileImported {
		t.Fatalf("pilotProfileImported = %v (%v) body=%s", summary.PilotProfileImported, err, string(restore.Body))
	}

	got := getPilotProfile(t, dest)
	if got.Mode != "everything" {
		t.Errorf("restored mode = %s", got.Mode)
	}
	for _, d := range ppAllDisciplines {
		if got.state(d).Intent != sourceProfile.state(d).Intent {
			t.Errorf("%s intent = %s, want %s", d, got.state(d).Intent, sourceProfile.state(d).Intent)
		}
	}
	if a, b := got.state("SAILPLANE").AcknowledgedAt, sourceProfile.state("SAILPLANE").AcknowledgedAt; a == nil || b == nil || *a != *b {
		t.Errorf("SAILPLANE acknowledgedAt = %v, want %v", a, b)
	}
	if got.state("SAILPLANE").Status != "active" || len(got.PendingAcknowledgement) != 0 {
		t.Errorf("restored SAILPLANE = %+v pending %v", got.state("SAILPLANE"), got.PendingAcknowledgement)
	}

	bad := map[string]interface{}{
		"format":       "NinerLog JSON Backup",
		"pilotProfile": map[string]interface{}{"mode": "sometimes"},
	}
	assertStatus(t, dest.Do("POST", "/imports/json", bad), http.StatusBadRequest)
}

// Aircraft reminders survive a JSON backup moved to a fresh account, attach to
// the restored aircraft, and are not duplicated by restoring the same backup
// again.
func TestExportImportRoundTrip_AircraftRemindersJSON(t *testing.T) {
	source := NewE2EClient(t)
	registerAndLogin(t, source, uniqueEmail("rem-rt-src"), "SecurePass123!", "Mehmet")
	c42 := createReminderAircraft(t, source, "D-MXYZ")
	createReminder(t, source, c42, map[string]interface{}{
		"kind": "ANNUAL_INSPECTION", "dueDate": futureDate(40), "intervalMonths": 12,
		"lastDoneOn": pastDate(325), "notes": "DULV Prüfer",
	})
	createReminder(t, source, c42, map[string]interface{}{
		"kind": "CUSTOM", "label": "Prop overhaul", "dueDate": futureDate(700),
	})

	exportResp := source.GET("/exports/json")
	requireStatus(t, exportResp, http.StatusOK)
	var backup map[string]interface{}
	if err := exportResp.JSON(&backup); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	if rems, _ := backup["aircraftReminders"].([]interface{}); len(rems) != 2 {
		t.Fatalf("backup carries %d aircraft reminders, want 2", len(rems))
	}

	dest := NewE2EClient(t)
	registerAndLogin(t, dest, uniqueEmail("rem-rt-dst"), "SecurePass123!", "Mehmet")

	type summary struct {
		AircraftImported          int `json:"aircraftImported"`
		AircraftRemindersImported int `json:"aircraftRemindersImported"`
		AircraftRemindersSkipped  int `json:"aircraftRemindersSkipped"`
	}
	restore := func() summary {
		resp := dest.Do("POST", "/imports/json", backup)
		requireStatus(t, resp, http.StatusOK)
		var s summary
		if err := resp.JSON(&s); err != nil {
			t.Fatalf("invalid summary: %v", err)
		}
		return s
	}

	first := restore()
	assertInt(t, "aircraftImported", first.AircraftImported, 1)
	assertInt(t, "aircraftRemindersImported", first.AircraftRemindersImported, 2)
	assertInt(t, "aircraftRemindersSkipped", first.AircraftRemindersSkipped, 0)

	list := listReminders(t, dest, "/aircraft-reminders")
	if len(list) != 2 {
		t.Fatalf("destination holds %d reminders, want 2", len(list))
	}
	annual := list[0]
	if annual.Kind != "ANNUAL_INSPECTION" || annual.AircraftRegistration != "D-MXYZ" ||
		annual.AircraftID == c42 || annual.DueDate != futureDate(40) ||
		annual.IntervalMonths == nil || *annual.IntervalMonths != 12 ||
		annual.LastDoneOn == nil || *annual.LastDoneOn != pastDate(325) ||
		annual.Notes == nil || *annual.Notes != "DULV Prüfer" {
		t.Errorf("annual inspection not restored faithfully: %+v", annual)
	}
	if list[1].Label == nil || *list[1].Label != "Prop overhaul" {
		t.Errorf("custom reminder label lost: %+v", list[1])
	}

	second := restore()
	assertInt(t, "aircraftRemindersImported on re-restore", second.AircraftRemindersImported, 0)
	assertInt(t, "aircraftRemindersSkipped on re-restore", second.AircraftRemindersSkipped, 2)
	if n := len(listReminders(t, dest, "/aircraft-reminders")); n != 2 {
		t.Errorf("re-restore duplicated reminders: %d, want 2", n)
	}
}

// gliderFactsFlights logs Lena's series entry and an outlanding with its release height.
func gliderFactsFlights(t *testing.T, c *E2EClient) {
	t.Helper()
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(20), "aircraftReg": "D-1234", "aircraftType": "ASK21",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"departureTime": "10:00", "arrivalTime": "11:00",
		"landings": 6, "launchMethod": "winch", "launches": 6,
	})
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(10), "aircraftReg": "D-1234", "aircraftType": "ASK21",
		"departureIcao": "EDNY", "arrivalIcao": "Feld bei Riedlingen",
		"departureTime": "11:00", "arrivalTime": "14:30",
		"landings": 1, "launchMethod": "aerotow", "isOutlanding": true, "releaseHeightM": 600,
	})
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(5), "aircraftReg": "D-EXYZ", "aircraftType": "DR40",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"offBlockTime": "10:00", "onBlockTime": "10:15",
		"landings": 1, "isTowFlight": true,
	})
}

// flightsByDate returns the account's flights keyed by date.
func flightsByDate(t *testing.T, c *E2EClient) map[string]map[string]interface{} {
	t.Helper()
	resp := c.GET("/flights?pageSize=100")
	requireStatus(t, resp, http.StatusOK)
	var page map[string]interface{}
	resp.JSON(&page)
	out := map[string]map[string]interface{}{}
	data, _ := page["data"].([]interface{})
	for _, d := range data {
		f := d.(map[string]interface{})
		out[f["date"].(string)] = f
	}
	return out
}

// assertGliderFacts checks the flights gliderFactsFlights logged.
func assertGliderFacts(t *testing.T, flights map[string]map[string]interface{}) {
	t.Helper()
	if len(flights) != 3 {
		t.Fatalf("got %d flights, want 3", len(flights))
	}
	series := flights[pastDate(20)]
	assertInt(t, "series launches", gi(series, "launches"), 6)
	assertBool(t, "series launchesOverride", gb(series, "launchesOverride"), true)
	out := flights[pastDate(10)]
	assertBool(t, "isOutlanding", gb(out, "isOutlanding"), true)
	assertInt(t, "outlanding crossCountryTime", gi(out, "crossCountryTime"), 0)
	assertInt(t, "releaseHeightM", gi(out, "releaseHeightM"), 600)
	assertInt(t, "outlanding launches", gi(out, "launches"), 1)
	tow := flights[pastDate(5)]
	assertBool(t, "isTowFlight", gb(tow, "isTowFlight"), true)
	if tow["releaseHeightM"] != nil {
		t.Errorf("tow releaseHeightM = %v, want none", tow["releaseHeightM"])
	}
}

func TestExportImportRoundTrip_GliderFlightFacts(t *testing.T) {
	t.Run("JSON backup", func(t *testing.T) {
		source := setupLena(t, "facts-json-src")
		gliderFactsFlights(t, source)
		backupResp := source.GET("/exports/json")
		requireStatus(t, backupResp, http.StatusOK)
		var backup map[string]interface{}
		if err := json.Unmarshal(backupResp.Body, &backup); err != nil {
			t.Fatal(err)
		}

		dest := NewE2EClient(t)
		registerAndLogin(t, dest, uniqueEmail("facts-json-dst"), "SecurePass123!", "Facts JSON")
		requireStatus(t, dest.Do("POST", "/imports/json", backup), http.StatusOK)
		assertGliderFacts(t, flightsByDate(t, dest))
	})

	t.Run("backup without glider facts derives launches", func(t *testing.T) {
		dest := NewE2EClient(t)
		registerAndLogin(t, dest, uniqueEmail("facts-json-old"), "SecurePass123!", "Facts Old")
		backup := map[string]interface{}{
			"format": "NinerLog JSON Backup",
			"flights": []map[string]interface{}{{
				"date": pastDate(3) + "T00:00:00Z", "aircraftReg": "D-1234", "aircraftType": "ASK21",
				"departureIcao": "EDNY", "arrivalIcao": "EDNY",
				"departureTime": "10:00:00", "arrivalTime": "10:08:00",
				"totalTime": 8, "isPic": true, "picTime": 8,
				"landingsDay": 1, "allLandings": 1, "takeoffsDay": 1, "launchMethod": "winch",
			}},
		}
		requireStatus(t, dest.Do("POST", "/imports/json", backup), http.StatusOK)
		f := flightsByDate(t, dest)[pastDate(3)]
		assertInt(t, "launches", gi(f, "launches"), 1)
		assertBool(t, "launchesOverride", gb(f, "launchesOverride"), false)
		assertBool(t, "isOutlanding", gb(f, "isOutlanding"), false)
	})

	t.Run("standard CSV", func(t *testing.T) {
		source := setupLena(t, "facts-csv-src")
		gliderFactsFlights(t, source)
		exportResp := source.GET("/exports/csv?format=standard")
		requireStatus(t, exportResp, http.StatusOK)

		dest := NewE2EClient(t)
		registerAndLogin(t, dest, uniqueEmail("facts-csv-dst"), "SecurePass123!", "Facts CSV")
		upload := uploadCSV(t, dest, "ninerlog_standard.csv", string(exportResp.Body))
		requireStatus(t, upload, http.StatusOK)
		var up map[string]interface{}
		upload.JSON(&up)
		token, _ := up["uploadToken"].(string)
		prev := dest.POST("/imports/preview", map[string]interface{}{
			"uploadToken": token, "mappings": up["suggestedMappings"], "skipDuplicates": false,
		})
		requireStatus(t, prev, http.StatusOK)
		requireStatus(t, dest.POST("/imports/confirm", map[string]interface{}{"uploadToken": token}), http.StatusCreated)
		assertGliderFacts(t, flightsByDate(t, dest))
	})
}
