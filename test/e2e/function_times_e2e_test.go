//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestFunctionTimes covers the declared pilot function times — PICUS, SPIC,
// examiner and cruise relief. They are never auto-derived; a declared value
// carves out of the derived PIC/SIC/dual time so the function columns still
// decompose block time.
func TestFunctionTimes(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("functime"), "SecurePass123!", "Amelia Earhart")

	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-AFTA", "type": "A320",
		"make": "Airbus", "model": "A320", "isMultiPilot": true,
	}), http.StatusCreated)

	create := func(t *testing.T, extra map[string]interface{}) map[string]interface{} {
		body := map[string]interface{}{
			"date": today(), "aircraftReg": "D-AFTA", "aircraftType": "A320",
			"departureIcao": "EDDF", "arrivalIcao": "EDDM",
			"offBlockTime": "06:00", "onBlockTime": "07:30", "landings": 1,
		}
		for k, v := range extra {
			body[k] = v
		}
		resp := c.POST("/flights", body)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		return f
	}

	t.Run("full-sector PICUS with the captain on board", func(t *testing.T) {
		f := create(t, map[string]interface{}{
			"picusTime": 90,
			"crewMembers": []map[string]interface{}{
				{"name": "Otto Lilienthal", "role": "PIC"},
			},
		})
		assertInt(t, "picusTime", gi(f, "picusTime"), 90)
		assertInt(t, "picTime", gi(f, "picTime"), 0)
		assertInt(t, "sicTime", gi(f, "sicTime"), 0)
		assertInt(t, "multiPilotTime", gi(f, "multiPilotTime"), 90)
		if f["isPic"] == true || f["isPassenger"] == true {
			t.Errorf("isPic=%v isPassenger=%v, want false/false", f["isPic"], f["isPassenger"])
		}
	})

	t.Run("partial cruise relief leaves the remainder as co-pilot time", func(t *testing.T) {
		f := create(t, map[string]interface{}{
			"reliefTime": 30,
			"crewMembers": []map[string]interface{}{
				{"name": "Otto Lilienthal", "role": "PIC"},
			},
		})
		assertInt(t, "reliefTime", gi(f, "reliefTime"), 30)
		assertInt(t, "sicTime", gi(f, "sicTime"), 60)
	})

	t.Run("SPIC with an instructor on board carves out of dual", func(t *testing.T) {
		f := create(t, map[string]interface{}{
			"spicTime": 90,
			"crewMembers": []map[string]interface{}{
				{"name": "FI Jones", "role": "Instructor"},
			},
		})
		assertInt(t, "spicTime", gi(f, "spicTime"), 90)
		assertInt(t, "dualTime", gi(f, "dualTime"), 0)
		if f["isDual"] != true {
			t.Errorf("isDual = %v, want true", f["isDual"])
		}
	})

	t.Run("examiner time overlays the function time", func(t *testing.T) {
		f := create(t, map[string]interface{}{
			"examinerTime": 90,
		})
		assertInt(t, "examinerTime", gi(f, "examinerTime"), 90)
		assertInt(t, "picTime", gi(f, "picTime"), 90)
	})

	t.Run("function times exceeding block time are rejected", func(t *testing.T) {
		resp := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-AFTA", "aircraftType": "A320",
			"departureIcao": "EDDF", "arrivalIcao": "EDDM",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
			"picusTime": 90, "sicTime": 90,
			"crewMembers": []map[string]interface{}{
				{"name": "Otto Lilienthal", "role": "PIC"},
			},
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("update declares PICUS on an existing flight and recalculation keeps it", func(t *testing.T) {
		f := create(t, map[string]interface{}{
			"crewMembers": []map[string]interface{}{
				{"name": "Otto Lilienthal", "role": "PIC"},
			},
		})
		assertInt(t, "sicTime", gi(f, "sicTime"), 90)
		id := f["id"].(string)

		resp := c.PUT("/flights/"+id, map[string]interface{}{"picusTime": 90})
		requireStatus(t, resp, http.StatusOK)
		var updated map[string]interface{}
		resp.JSON(&updated)
		assertInt(t, "picusTime", gi(updated, "picusTime"), 90)
		assertInt(t, "sicTime", gi(updated, "sicTime"), 0)

		requireStatus(t, c.POST("/flights/recalculate", map[string]interface{}{}), http.StatusOK)
		resp = c.GET("/flights/" + id)
		requireStatus(t, resp, http.StatusOK)
		var after map[string]interface{}
		resp.JSON(&after)
		assertInt(t, "picusTime", gi(after, "picusTime"), 90)
		assertInt(t, "sicTime", gi(after, "sicTime"), 0)
	})

	t.Run("analytics totals report the function times", func(t *testing.T) {
		resp := c.GET("/reports/analytics?months=0")
		requireStatus(t, resp, http.StatusOK)
		var out map[string]interface{}
		resp.JSON(&out)
		totals, ok := out["totals"].(map[string]interface{})
		if !ok {
			t.Fatal("analytics response has no totals object")
		}
		if gi(totals, "picusMinutes") < 90 {
			t.Errorf("picusMinutes = %d, want >= 90", gi(totals, "picusMinutes"))
		}
		if gi(totals, "spicMinutes") < 90 {
			t.Errorf("spicMinutes = %d, want >= 90", gi(totals, "spicMinutes"))
		}
		if gi(totals, "examinerMinutes") < 90 {
			t.Errorf("examinerMinutes = %d, want >= 90", gi(totals, "examinerMinutes"))
		}
		if gi(totals, "reliefMinutes") < 30 {
			t.Errorf("reliefMinutes = %d, want >= 30", gi(totals, "reliefMinutes"))
		}
	})

	t.Run("standard CSV export carries the function time columns", func(t *testing.T) {
		resp := c.GET("/exports/csv")
		requireStatus(t, resp, http.StatusOK)
		header := strings.SplitN(string(resp.Body), "\n", 2)[0]
		for _, col := range []string{"PICUS", "SPIC", "ExaminerTime", "ReliefTime"} {
			if !strings.Contains(header, col) {
				t.Errorf("CSV header missing %q: %s", col, header)
			}
		}
	})
}

// TestFunctionTimesBaseline covers the carried-forward function time columns.
func TestFunctionTimesBaseline(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("functime-base"), "SecurePass123!", "Amelia Earhart")

	resp := c.PUT("/users/me/baseline", map[string]interface{}{
		"baselineDate": pastDate(30),
		"totalMinutes": 6000, "totalFlights": 40,
		"picusMinutes": 1200, "spicMinutes": 300,
		"examinerMinutes": 60, "reliefMinutes": 240,
	})
	requireStatus(t, resp, http.StatusOK)
	var b map[string]interface{}
	resp.JSON(&b)
	assertInt(t, "picusMinutes", gi(b, "picusMinutes"), 1200)
	assertInt(t, "spicMinutes", gi(b, "spicMinutes"), 300)
	assertInt(t, "examinerMinutes", gi(b, "examinerMinutes"), 60)
	assertInt(t, "reliefMinutes", gi(b, "reliefMinutes"), 240)

	resp = c.GET("/users/me/baseline")
	requireStatus(t, resp, http.StatusOK)
	resp.JSON(&b)
	assertInt(t, "picusMinutes", gi(b, "picusMinutes"), 1200)

	// The baseline flows into the analytics totals.
	resp = c.GET("/reports/analytics?months=0")
	requireStatus(t, resp, http.StatusOK)
	var out map[string]interface{}
	resp.JSON(&out)
	totals := out["totals"].(map[string]interface{})
	assertInt(t, "picusMinutes", gi(totals, "picusMinutes"), 1200)
	assertInt(t, "reliefMinutes", gi(totals, "reliefMinutes"), 240)
}
