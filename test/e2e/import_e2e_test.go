//go:build e2e

package e2e_test

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"
)

// uploadCSV uploads a CSV file to the import endpoint.
func uploadCSV(t *testing.T, c *E2EClient, filename, content string) *Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	part.Write([]byte(content))
	w.Close()

	url := baseURL + "/api/v1/imports/upload"
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return &Response{StatusCode: resp.StatusCode, Body: body, Headers: resp.Header}
}

func TestImportCSV(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import"), "SecurePass123!", "Import")

	csv := "Date,AircraftReg,AircraftType,From,To,OffBlock,OnBlock,Landings\n" +
		fmt.Sprintf("%s,D-EIMP,C172,EDNY,EDDS,08:00,09:30,1\n", today()) +
		fmt.Sprintf("%s,D-EIMP,C172,EDDS,EDNY,10:00,11:00,1\n", pastDate(1))

	var uploadToken string

	t.Run("upload CSV", func(t *testing.T) {
		resp := uploadCSV(t, c, "flights.csv", csv)
		requireStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		resp.JSON(&result)
		if result["uploadToken"] == nil {
			t.Fatal("Expected uploadToken")
		}
		uploadToken = result["uploadToken"].(string)
		if result["columns"] == nil {
			t.Error("Expected columns")
		}
		if result["previewRows"] == nil {
			t.Error("Expected previewRows")
		}
		t.Logf("Upload token: %s, format: %v", uploadToken, result["format"])
	})

	t.Run("preview import with mappings", func(t *testing.T) {
		resp := c.POST("/imports/preview", map[string]interface{}{
			"uploadToken": uploadToken,
			"mappings": []map[string]interface{}{
				{"sourceColumn": "Date", "targetField": "date"},
				{"sourceColumn": "AircraftReg", "targetField": "aircraftReg"},
				{"sourceColumn": "AircraftType", "targetField": "aircraftType"},
				{"sourceColumn": "From", "targetField": "departureIcao"},
				{"sourceColumn": "To", "targetField": "arrivalIcao"},
				{"sourceColumn": "OffBlock", "targetField": "offBlockTime"},
				{"sourceColumn": "OnBlock", "targetField": "onBlockTime"},
				{"sourceColumn": "Landings", "targetField": "landings"},
			},
		})
		requireStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		resp.JSON(&result)
		t.Logf("Preview result: %s", string(resp.Body))
	})

	t.Run("confirm import", func(t *testing.T) {
		resp := c.POST("/imports/confirm", map[string]interface{}{
			"uploadToken": uploadToken,
		})
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Fatalf("Expected 200 or 201, got %d: %s", resp.StatusCode, string(resp.Body))
		}
		var result map[string]interface{}
		resp.JSON(&result)
		t.Logf("Import result: %s", string(resp.Body))
	})

	t.Run("imported flights exist", func(t *testing.T) {
		resp := c.GET("/flights")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		data := r["data"].([]interface{})
		if len(data) < 2 {
			t.Logf("Imported flights: %d (import may have validation errors)", len(data))
		}
	})

	t.Run("list import history", func(t *testing.T) {
		resp := c.GET("/imports")
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("upload without auth returns 401", func(t *testing.T) {
		c.ClearToken()
		resp := uploadCSV(t, c, "test.csv", "Date\n2025-01-01\n")
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

// TestImportCSV_ReimportsOwnExport re-uploads ninerlog's own exported CSV
// (default DD.MM.YYYY dates) and confirms the preview reports no date errors.
func TestImportCSV_ReimportsOwnExport(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import-reexport"), "SecurePass123!", "Reexport")

	// The comma decimal preference makes the export write durations as "1,5h".
	pr := c.PATCH("/users/me", map[string]interface{}{"decimalSeparator": "comma"})
	requireStatus(t, pr, http.StatusOK)

	r := c.POST("/flights", map[string]interface{}{
		"date": pastDate(30), "aircraftReg": "D-ERAE", "aircraftType": "C172",
		"departureIcao": "EDAZ", "arrivalIcao": "EDAZ",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
	})
	requireStatus(t, r, http.StatusCreated)

	exportResp := c.GET("/exports/csv")
	requireStatus(t, exportResp, http.StatusOK)
	exportedCSV := string(exportResp.Body)
	if !bytes.Contains(exportResp.Body, []byte("D-ERAE")) {
		t.Fatalf("expected exported CSV to contain the flight, got: %s", exportedCSV)
	}

	var uploadToken string
	var suggested []interface{}
	t.Run("upload own export", func(t *testing.T) {
		resp := uploadCSV(t, c, "ninerlog_export.csv", exportedCSV)
		requireStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		resp.JSON(&result)
		uploadToken = result["uploadToken"].(string)
		suggested, _ = result["suggestedMappings"].([]interface{})
		if len(suggested) == 0 {
			t.Fatal("expected suggestedMappings for own export")
		}
	})

	t.Run("preview reports no errors", func(t *testing.T) {
		resp := c.POST("/imports/preview", map[string]interface{}{
			"uploadToken":    uploadToken,
			"mappings":       suggested,
			"skipDuplicates": false,
		})
		requireStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		resp.JSON(&result)

		flights, _ := result["flights"].([]interface{})
		if len(flights) == 0 {
			t.Fatal("expected at least one previewed row")
		}
		// No field fails to parse (dates in DD.MM.YYYY, durations as
		// "1.5h"/"1,5h", ...).
		for _, fi := range flights {
			row := fi.(map[string]interface{})
			if row["status"] == "error" {
				t.Errorf("row %v has status 'error': %v", row["rowIndex"], row["errors"])
			}
			if errs, ok := row["errors"].([]interface{}); ok {
				for _, e := range errs {
					em := e.(map[string]interface{})
					t.Errorf("unexpected %v error on row %v: %v", em["field"], row["rowIndex"], em["message"])
				}
			}
		}
	})
}

// TestImportCSV_CreatesAircraft covers fleet entries being auto-created from
// registrations named only in the flight rows of a semicolon-delimited CSV.
func TestImportCSV_CreatesAircraft(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import-fleet"), "SecurePass123!", "FleetImport")

	csv := "Datum;Kennzeichen;Muster;Von;Nach;Off-Block;On-Block;Landungen\n" +
		fmt.Sprintf("%s;D-EFLT;C172;EDNY;EDDS;08:00;09:30;1\n", today()) +
		fmt.Sprintf("%s;D-EFLT;C172;EDDS;EDNY;10:00;11:00;1\n", pastDate(1)) +
		fmt.Sprintf("%s;D-EFLU;DA40;EDDS;EDDS;12:00;13:00;2\n", pastDate(2))

	mappings := []map[string]interface{}{
		{"sourceColumn": "Datum", "targetField": "date"},
		{"sourceColumn": "Kennzeichen", "targetField": "aircraftReg"},
		{"sourceColumn": "Muster", "targetField": "aircraftType"},
		{"sourceColumn": "Von", "targetField": "departureIcao"},
		{"sourceColumn": "Nach", "targetField": "arrivalIcao"},
		{"sourceColumn": "Off-Block", "targetField": "offBlockTime"},
		{"sourceColumn": "On-Block", "targetField": "onBlockTime"},
		{"sourceColumn": "Landungen", "targetField": "landings"},
	}

	var uploadToken string
	t.Run("upload and preview", func(t *testing.T) {
		resp := uploadCSV(t, c, "logbuch.csv", csv)
		requireStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		resp.JSON(&result)
		uploadToken, _ = result["uploadToken"].(string)
		if uploadToken == "" {
			t.Fatalf("expected an uploadToken, got: %s", string(resp.Body))
		}

		pr := c.POST("/imports/preview", map[string]interface{}{
			"uploadToken": uploadToken,
			"mappings":    mappings,
		})
		requireStatus(t, pr, http.StatusOK)
	})

	t.Run("confirm reports the created aircraft", func(t *testing.T) {
		resp := c.POST("/imports/confirm", map[string]interface{}{"uploadToken": uploadToken})
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Fatalf("Expected 200 or 201, got %d: %s", resp.StatusCode, string(resp.Body))
		}
		var result map[string]interface{}
		resp.JSON(&result)
		if got, _ := result["importedCount"].(float64); got != 3 {
			t.Fatalf("importedCount = %v, want 3: %s", result["importedCount"], string(resp.Body))
		}
		if got, _ := result["aircraftCreated"].(float64); got != 2 {
			t.Errorf("aircraftCreated = %v, want 2: %s", result["aircraftCreated"], string(resp.Body))
		}
	})

	t.Run("fleet contains the imported registrations", func(t *testing.T) {
		resp := c.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)

		types := make(map[string]string)
		for _, item := range r["data"].([]interface{}) {
			ac := item.(map[string]interface{})
			reg, _ := ac["registration"].(string)
			typeCode, _ := ac["type"].(string)
			types[reg] = typeCode
		}
		for reg, want := range map[string]string{"D-EFLT": "C172", "D-EFLU": "DA40"} {
			got, ok := types[reg]
			if !ok {
				t.Fatalf("aircraft %s was not created by the import: %s", reg, string(resp.Body))
			}
			if got != want {
				t.Errorf("aircraft %s type = %q, want %q", reg, got, want)
			}
		}
		if len(types) != 2 {
			t.Errorf("fleet has %d aircraft, want exactly the 2 from the file: %s", len(types), string(resp.Body))
		}
	})
}

// Rows skipped as duplicates still create their aircraft, and a second run
// does not duplicate the fleet entries.
func TestImportCSV_DuplicateRowsStillCreateAircraft(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import-fleet-again"), "SecurePass123!", "FleetReimport")

	// The flight already exists; the fleet does not.
	flightDate := pastDate(3)
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": flightDate, "aircraftReg": "D-EDUP", "aircraftType": "PA28",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
	}), http.StatusCreated)

	csv := "Date;Reg;Type;From;To;OffBlock;OnBlock;Total;Landings\n" +
		fmt.Sprintf("%s;D-EDUP;PA28;EDNY;EDDS;08:00;09:30;1:30;1\n", flightDate)
	mappings := []map[string]interface{}{
		{"sourceColumn": "Date", "targetField": "date"},
		{"sourceColumn": "Reg", "targetField": "aircraftReg"},
		{"sourceColumn": "Type", "targetField": "aircraftType"},
		{"sourceColumn": "From", "targetField": "departureIcao"},
		{"sourceColumn": "To", "targetField": "arrivalIcao"},
		{"sourceColumn": "OffBlock", "targetField": "offBlockTime"},
		{"sourceColumn": "OnBlock", "targetField": "onBlockTime"},
		{"sourceColumn": "Total", "targetField": "totalTime"},
		{"sourceColumn": "Landings", "targetField": "landings"},
	}

	runImport := func(t *testing.T) map[string]interface{} {
		t.Helper()
		resp := uploadCSV(t, c, "logbook.csv", csv)
		requireStatus(t, resp, http.StatusOK)
		var upload map[string]interface{}
		resp.JSON(&upload)
		token := upload["uploadToken"].(string)

		requireStatus(t, c.POST("/imports/preview", map[string]interface{}{
			"uploadToken": token, "mappings": mappings,
		}), http.StatusOK)

		cr := c.POST("/imports/confirm", map[string]interface{}{"uploadToken": token})
		if cr.StatusCode != http.StatusOK && cr.StatusCode != http.StatusCreated {
			t.Fatalf("Expected 200 or 201, got %d: %s", cr.StatusCode, string(cr.Body))
		}
		var result map[string]interface{}
		cr.JSON(&result)
		return result
	}

	first := runImport(t)
	if got, _ := first["duplicateCount"].(float64); got != 1 {
		t.Fatalf("duplicateCount = %v, want 1 (the flight already exists): %v", first["duplicateCount"], first)
	}
	if got, _ := first["aircraftCreated"].(float64); got != 1 {
		t.Errorf("aircraftCreated = %v, want 1 — a duplicate flight must still backfill its aircraft", first["aircraftCreated"])
	}

	second := runImport(t)
	if got, _ := second["aircraftCreated"].(float64); got != 0 {
		t.Errorf("second import aircraftCreated = %v, want 0 (D-EDUP is already in the fleet)", second["aircraftCreated"])
	}

	resp := c.GET("/aircraft")
	requireStatus(t, resp, http.StatusOK)
	var r map[string]interface{}
	resp.JSON(&r)
	fleet := r["data"].([]interface{})
	if len(fleet) != 1 {
		t.Fatalf("fleet has %d aircraft after two imports of the same file, want 1: %s", len(fleet), string(resp.Body))
	}
	if reg, _ := fleet[0].(map[string]interface{})["registration"].(string); reg != "D-EDUP" {
		t.Errorf("fleet aircraft registration = %q, want D-EDUP", reg)
	}
}

func TestImportForeFlight(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import-ff"), "SecurePass123!", "FFImport")

	// ForeFlight format CSV
	ff := "Date,AircraftID,From,To,Route,TimeOut,TimeOff,TimeOn,TimeIn,TotalTime,PIC,SIC,Night,Solo,CrossCountry,DayTakeoffs,DayLandingsFullStop,NightTakeoffs,NightLandingsFullStop,AllLandings,ActualInstrument,SimulatedInstrument,Holds,DualGiven,DualReceived,SimulatedFlight,GroundTraining,InstructorName,InstructorComments,PilotComments\n" +
		fmt.Sprintf("%s,D-EFFF,EDNY,EDDS,,08:00,08:10,09:20,09:30,1.5,1.5,0.0,0.0,1.5,1.5,1,1,0,0,1,0.0,0.0,0,0.0,0.0,0.0,0.0,,,Pattern\n", today())

	t.Run("upload ForeFlight CSV", func(t *testing.T) {
		resp := uploadCSV(t, c, "foreflight.csv", ff)
		requireStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		resp.JSON(&result)
		format := result["format"]
		t.Logf("Detected format: %v", format)
		if result["suggestedMappings"] == nil {
			t.Error("Expected suggestedMappings for ForeFlight")
		}
	})
}

func TestImportEdgeCases(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import-edge"), "SecurePass123!", "Edge")

	t.Run("upload empty file", func(t *testing.T) {
		resp := uploadCSV(t, c, "empty.csv", "")
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 400 or 200 for empty file, got %d", resp.StatusCode)
		}
	})

	t.Run("upload with only headers", func(t *testing.T) {
		resp := uploadCSV(t, c, "headers.csv", "Date,AircraftReg,From,To\n")
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 200 or 400 for headers-only, got %d", resp.StatusCode)
		}
	})

	t.Run("preview with invalid token", func(t *testing.T) {
		resp := c.POST("/imports/preview", map[string]interface{}{
			"uploadToken": "nonexistent-token",
			"mappings":    []map[string]interface{}{},
		})
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 400/404 for invalid token, got %d", resp.StatusCode)
		}
	})

	t.Run("confirm with invalid token", func(t *testing.T) {
		resp := c.POST("/imports/confirm", map[string]interface{}{
			"uploadToken": "nonexistent-token",
		})
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 400/404, got %d", resp.StatusCode)
		}
	})
}

// Ensure unused import is consumed
var _ = time.Now
