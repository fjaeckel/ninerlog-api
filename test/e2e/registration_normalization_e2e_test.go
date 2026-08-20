//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// Registrations are stored in the canonical notation of their state of
// registry (pkg/registration) on every write path.

func TestAircraftRegistrationNormalization(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ac-norm"), "SecurePass123!", "Norm User")

	t.Run("hyphenated state gets its hyphen", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "deabc", "type": "C172", "make": "Cessna", "model": "172S",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		assertStr(t, "registration", ac["registration"], "D-EABC")
	})

	t.Run("canonical spelling of an existing aircraft returns 409", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EABC", "type": "PA28", "make": "Piper", "model": "Cherokee",
		})
		assertStatus(t, resp, http.StatusConflict)
	})

	t.Run("unhyphenated state loses its hyphen", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "n-12345", "type": "C182", "make": "Cessna", "model": "182T",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		assertStr(t, "registration", ac["registration"], "N12345")
	})

	t.Run("unrecognised mark is left alone", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "SIM", "type": "FNPT2", "make": "Elite", "model": "S812",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		assertStr(t, "registration", ac["registration"], "SIM")
	})
}

func TestFlightRegistrationNormalization(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("fl-norm"), "SecurePass123!", "Flight Norm")

	resp := c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "deabc", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
	})
	requireStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	resp.JSON(&created)
	assertStr(t, "aircraftReg", created["aircraftReg"], "D-EABC")
	flightID := created["id"].(string)

	t.Run("stored normalised, not just echoed", func(t *testing.T) {
		r := c.GET(fmt.Sprintf("/flights/%s", flightID))
		requireStatus(t, r, http.StatusOK)
		var f map[string]interface{}
		r.JSON(&f)
		assertStr(t, "aircraftReg", f["aircraftReg"], "D-EABC")
	})
}

func TestImportCSV_NormalizesRegistrations(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import-norm"), "SecurePass123!", "ImportNorm")

	csv := "Datum;Kennzeichen;Muster;Von;Nach;Off-Block;On-Block;Landungen\n" +
		fmt.Sprintf("%s;D-EABC;C172;EDNY;EDDS;08:00;09:30;1\n", today()) +
		fmt.Sprintf("%s;DEABC;C172;EDDS;EDNY;10:00;11:00;1\n", pastDate(1))

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

	resp := uploadCSV(t, c, "logbuch.csv", csv)
	requireStatus(t, resp, http.StatusOK)
	var upload map[string]interface{}
	resp.JSON(&upload)
	uploadToken, _ := upload["uploadToken"].(string)
	if uploadToken == "" {
		t.Fatalf("expected an uploadToken, got: %s", string(resp.Body))
	}

	requireStatus(t, c.POST("/imports/preview", map[string]interface{}{
		"uploadToken": uploadToken, "mappings": mappings,
	}), http.StatusOK)

	cr := c.POST("/imports/confirm", map[string]interface{}{"uploadToken": uploadToken})
	if cr.StatusCode != http.StatusOK && cr.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201, got %d: %s", cr.StatusCode, string(cr.Body))
	}
	var result map[string]interface{}
	cr.JSON(&result)
	if got, _ := result["importedCount"].(float64); got != 2 {
		t.Fatalf("importedCount = %v, want 2: %s", result["importedCount"], string(cr.Body))
	}

	t.Run("both spellings create one fleet entry", func(t *testing.T) {
		if got, _ := result["aircraftCreated"].(float64); got != 1 {
			t.Errorf("aircraftCreated = %v, want 1 — D-EABC and DEABC are one aircraft: %s",
				result["aircraftCreated"], string(cr.Body))
		}

		resp := c.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		fleet := r["data"].([]interface{})
		if len(fleet) != 1 {
			t.Fatalf("fleet has %d aircraft, want 1: %s", len(fleet), string(resp.Body))
		}
		assertStr(t, "registration", fleet[0].(map[string]interface{})["registration"], "D-EABC")
	})

	t.Run("both flights carry the canonical registration", func(t *testing.T) {
		resp := c.GET("/flights")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		flights := r["data"].([]interface{})
		if len(flights) != 2 {
			t.Fatalf("got %d flights, want 2: %s", len(flights), string(resp.Body))
		}
		for _, item := range flights {
			f := item.(map[string]interface{})
			assertStr(t, fmt.Sprintf("aircraftReg of flight on %v", f["date"]), f["aircraftReg"], "D-EABC")
		}
	})
}

func TestRecalculateReportsFleetNormalization(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("recalc-norm"), "SecurePass123!", "RecalcNorm")

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "deabc", "type": "C172", "make": "Cessna", "model": "172S",
	}), http.StatusCreated)
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D EABC", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
	}), http.StatusCreated)

	r := c.POST("/flights/recalculate", nil)
	requireStatus(t, r, http.StatusOK)
	var result map[string]interface{}
	r.JSON(&result)

	for _, key := range []string{"aircraftNormalized", "aircraftConflicts"} {
		v, ok := result[key]
		if !ok {
			t.Fatalf("recalculate response is missing %q: %s", key, string(r.Body))
		}
		if _, isNumber := v.(float64); !isNumber {
			t.Errorf("%s = %v, want a number: %s", key, v, string(r.Body))
		}
	}
	assertInt(t, "aircraftNormalized", gi(result, "aircraftNormalized"), 0)
	assertInt(t, "aircraftConflicts", gi(result, "aircraftConflicts"), 0)
	assertInt(t, "total", gi(result, "total"), 1)
	assertInt(t, "errors", gi(result, "errors"), 0)

	t.Run("the flight stays on its canonical registration", func(t *testing.T) {
		resp := c.GET("/flights")
		requireStatus(t, resp, http.StatusOK)
		var body map[string]interface{}
		resp.JSON(&body)
		flights := body["data"].([]interface{})
		if len(flights) != 1 {
			t.Fatalf("got %d flights, want 1: %s", len(flights), string(resp.Body))
		}
		assertStr(t, "aircraftReg", flights[0].(map[string]interface{})["aircraftReg"], "D-EABC")
	})
}
