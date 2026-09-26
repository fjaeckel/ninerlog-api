//go:build e2e

package e2e_test

import (
	"encoding/csv"
	"net/http"
	"strings"
	"testing"
	"time"
)

func timeModelFlight(extra map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"date": "2025-07-15", "aircraftReg": "D-KTML", "aircraftType": "SF25",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS", "landings": 1,
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func assertErrorContains(t *testing.T, resp *Response, want string) {
	t.Helper()
	var body map[string]interface{}
	resp.JSON(&body)
	if msg, _ := body["error"].(string); !strings.Contains(msg, want) {
		t.Errorf("error %q does not contain %q", msg, want)
	}
}

func TestTimeModel_TakeoffLandingOnly(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("tm-tol"), "SecurePass123!", "Karl Time")

	var fid string

	t.Run("L3/K3 take-off and landing only", func(t *testing.T) {
		r := c.POST("/flights", timeModelFlight(map[string]interface{}{
			"departureTime": "10:05", "arrivalTime": "10:47",
		}))
		requireStatus(t, r, http.StatusCreated)
		var f map[string]interface{}
		r.JSON(&f)
		fid = f["id"].(string)
		assertInt(t, "totalTime", gi(f, "totalTime"), 42)
		assertInt(t, "picTime", gi(f, "picTime"), 42)
		assertInt(t, "crossCountryTime", gi(f, "crossCountryTime"), 42)
		assertInt(t, "nightTime", gi(f, "nightTime"), 0)
		if f["offBlockTime"] != nil || f["onBlockTime"] != nil {
			t.Errorf("block times = %v/%v, want null", f["offBlockTime"], f["onBlockTime"])
		}
	})

	t.Run("L3/K3 take-off and landing only: GET returns the stored flight", func(t *testing.T) {
		r := c.GET("/flights/" + fid)
		requireStatus(t, r, http.StatusOK)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "totalTime", gi(f, "totalTime"), 42)
		assertStr(t, "departureTime", f["departureTime"], "10:05:00")
		assertStr(t, "arrivalTime", f["arrivalTime"], "10:47:00")
		if f["offBlockTime"] != nil || f["onBlockTime"] != nil {
			t.Errorf("block times = %v/%v, want null", f["offBlockTime"], f["onBlockTime"])
		}
	})

	t.Run("L3/K3 take-off and landing only across midnight", func(t *testing.T) {
		r := c.POST("/flights", timeModelFlight(map[string]interface{}{
			"departureTime": "23:30", "arrivalTime": "00:15",
		}))
		requireStatus(t, r, http.StatusCreated)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "totalTime", gi(f, "totalTime"), 45)
	})

	t.Run("L3/K3 take-off and landing only: night split", func(t *testing.T) {
		r := c.POST("/flights", timeModelFlight(map[string]interface{}{
			"date": "2025-12-15", "departureTime": "18:10", "arrivalTime": "19:20",
		}))
		requireStatus(t, r, http.StatusCreated)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "totalTime", gi(f, "totalTime"), 70)
		assertInt(t, "nightTime", gi(f, "nightTime"), 70)
		assertInt(t, "takeoffsNight", gi(f, "takeoffsNight"), 1)
		assertInt(t, "landingsNight", gi(f, "landingsNight"), 1)
	})

	t.Run("L3/K3 take-off and landing only: updating landing recomputes total", func(t *testing.T) {
		r := c.PUT("/flights/"+fid, map[string]interface{}{"arrivalTime": "11:05"})
		requireStatus(t, r, http.StatusOK)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "totalTime", gi(f, "totalTime"), 60)
		assertInt(t, "picTime", gi(f, "picTime"), 60)
	})

	t.Run("L3/K3 take-off and landing only: recalculation keeps the total", func(t *testing.T) {
		requireStatus(t, c.POST("/flights/recalculate", nil), http.StatusOK)
		r := c.GET("/flights/" + fid)
		requireStatus(t, r, http.StatusOK)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "totalTime", gi(f, "totalTime"), 60)
	})

	t.Run("L3/K3 take-off and landing only: EASA CSV prints take-off and landing", func(t *testing.T) {
		r := c.GET("/exports/csv?format=easa")
		requireStatus(t, r, http.StatusOK)
		records, err := csv.NewReader(strings.NewReader(string(r.Body))).ReadAll()
		if err != nil {
			t.Fatalf("invalid CSV: %v", err)
		}
		col := map[string]int{}
		for i, h := range records[0] {
			col[h] = i
		}
		found := false
		for _, row := range records[1:] {
			if row[col["Dep Time"]] == "10:05" {
				found = true
				if got := row[col["Arr Time"]]; got != "11:05" {
					t.Errorf("Arr Time = %q, want 11:05", got)
				}
			}
		}
		if !found {
			t.Errorf("no EASA CSV row with Dep Time 10:05:\n%s", string(r.Body))
		}
	})
}

func TestTimeModel_BlockTimes(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("tm-blk"), "SecurePass123!", "Mark Block")

	t.Run("A2 block times unchanged", func(t *testing.T) {
		r := c.POST("/flights", timeModelFlight(map[string]interface{}{
			"aircraftType": "C172", "offBlockTime": "08:00", "onBlockTime": "09:30",
		}))
		requireStatus(t, r, http.StatusCreated)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "totalTime", gi(f, "totalTime"), 90)
		if f["departureTime"] != nil || f["arrivalTime"] != nil {
			t.Errorf("take-off/landing = %v/%v, want null", f["departureTime"], f["arrivalTime"])
		}
	})

	t.Run("A2 block times unchanged: block span wins over take-off/landing", func(t *testing.T) {
		r := c.POST("/flights", timeModelFlight(map[string]interface{}{
			"aircraftType": "C172", "offBlockTime": "08:00", "onBlockTime": "09:30",
			"departureTime": "08:10", "arrivalTime": "09:20",
		}))
		requireStatus(t, r, http.StatusCreated)
		var f map[string]interface{}
		r.JSON(&f)
		fid := f["id"].(string)
		assertInt(t, "totalTime", gi(f, "totalTime"), 90)

		r = c.PUT("/flights/"+fid, map[string]interface{}{"arrivalTime": "09:00"})
		requireStatus(t, r, http.StatusOK)
		r.JSON(&f)
		assertInt(t, "totalTime after landing change", gi(f, "totalTime"), 90)
	})

	t.Run("A2 block times unchanged: lone take-off beside block pair is accepted", func(t *testing.T) {
		r := c.POST("/flights", timeModelFlight(map[string]interface{}{
			"aircraftType": "C172", "offBlockTime": "08:00", "onBlockTime": "09:00", "departureTime": "08:10",
		}))
		requireStatus(t, r, http.StatusCreated)
	})

	t.Run("A2 block times unchanged: bad block time format", func(t *testing.T) {
		r := c.POST("/flights", timeModelFlight(map[string]interface{}{
			"offBlockTime": "nope", "onBlockTime": "09:00",
		}))
		assertStatus(t, r, http.StatusBadRequest)
		assertErrorContains(t, r, "Invalid block times format")
	})
}

func TestTimeModel_ImportTakeoffLandingOnly(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("tm-imp"), "SecurePass123!", "Lena Import")

	content := "Datum,Kennzeichen,Muster,Von,Nach,Start,Landung,Landungen\n" +
		"2025-05-01,D-1234,ASK21,EDNY,EDNY,10:05,10:47,1\n"

	var uploadToken string
	t.Run("upload", func(t *testing.T) {
		r := uploadCSV(t, c, "vereinsflieger.csv", content)
		requireStatus(t, r, http.StatusOK)
		var res map[string]interface{}
		r.JSON(&res)
		uploadToken, _ = res["uploadToken"].(string)
		if uploadToken == "" {
			t.Fatalf("no uploadToken: %s", string(r.Body))
		}
	})

	t.Run("L3/K3 take-off and landing only: preview and confirm", func(t *testing.T) {
		pr := c.POST("/imports/preview", map[string]interface{}{
			"uploadToken": uploadToken,
			"mappings": []map[string]interface{}{
				{"sourceColumn": "Datum", "targetField": "date"},
				{"sourceColumn": "Kennzeichen", "targetField": "aircraftReg"},
				{"sourceColumn": "Muster", "targetField": "aircraftType"},
				{"sourceColumn": "Von", "targetField": "departureIcao"},
				{"sourceColumn": "Nach", "targetField": "arrivalIcao"},
				{"sourceColumn": "Start", "targetField": "departureTime"},
				{"sourceColumn": "Landung", "targetField": "arrivalTime"},
				{"sourceColumn": "Landungen", "targetField": "landings"},
			},
		})
		requireStatus(t, pr, http.StatusOK)

		r := c.POST("/imports/confirm", map[string]interface{}{"uploadToken": uploadToken})
		if r.StatusCode != http.StatusOK && r.StatusCode != http.StatusCreated {
			t.Fatalf("confirm status %d: %s", r.StatusCode, string(r.Body))
		}
		var res map[string]interface{}
		r.JSON(&res)
		if got, _ := res["importedCount"].(float64); got != 1 {
			t.Fatalf("importedCount = %v, want 1: %s", res["importedCount"], string(r.Body))
		}
	})

	t.Run("L3/K3 take-off and landing only: imported flight", func(t *testing.T) {
		r := c.GET("/flights")
		requireStatus(t, r, http.StatusOK)
		var list map[string]interface{}
		r.JSON(&list)
		data, _ := list["data"].([]interface{})
		if len(data) != 1 {
			t.Fatalf("got %d flights, want 1", len(data))
		}
		f := data[0].(map[string]interface{})
		assertInt(t, "totalTime", gi(f, "totalTime"), 42)
		assertStr(t, "departureTime", f["departureTime"], "10:05:00")
		if f["offBlockTime"] != nil {
			t.Errorf("offBlockTime = %v, want null", f["offBlockTime"])
		}
	})
}

func TestTimeModel_FlightSessionTakeoffLandingOnly(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("tm-ses"), "SecurePass123!", "Lena Session")

	takeoff := time.Now().UTC().Add(-50 * time.Minute).Truncate(time.Minute)
	landing := takeoff.Add(38 * time.Minute)

	t.Run("L3/K3 take-off and landing only: takeoff opens a session", func(t *testing.T) {
		r := c.POST("/flight-sessions/current/events", map[string]interface{}{
			"type": "takeoff", "occurredAt": takeoff.Format(time.RFC3339),
			"aircraftReg": "D-1234", "icao": "EDNY",
		})
		requireStatus(t, r, http.StatusCreated)
	})

	t.Run("L3/K3 take-off and landing only: onblock is rejected", func(t *testing.T) {
		r := c.POST("/flight-sessions/current/events", map[string]interface{}{"type": "onblock"})
		assertStatus(t, r, http.StatusBadRequest)
	})

	t.Run("L3/K3 take-off and landing only: landing completes the flight", func(t *testing.T) {
		r := c.POST("/flight-sessions/current/events", map[string]interface{}{
			"type": "landing", "occurredAt": landing.Format(time.RFC3339), "icao": "EDNY",
		})
		requireStatus(t, r, http.StatusOK)
		var s map[string]interface{}
		r.JSON(&s)
		assertStr(t, "status", s["status"], "completed")
		fid, _ := s["flightId"].(string)
		if fid == "" {
			t.Fatalf("no flightId: %s", string(r.Body))
		}

		fr := c.GET("/flights/" + fid)
		requireStatus(t, fr, http.StatusOK)
		var f map[string]interface{}
		fr.JSON(&f)
		assertInt(t, "totalTime", gi(f, "totalTime"), 38)
		if f["offBlockTime"] != nil || f["onBlockTime"] != nil {
			t.Errorf("block times = %v/%v, want null", f["offBlockTime"], f["onBlockTime"])
		}
	})
}
