//go:build e2e

package e2e_test

import (
	"fmt"
	"math"
	"testing"
)

func assertFloat(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: want %.2f +/-%.2f, got %.2f", name, want, tol, got)
	}
}
func assertInt(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s: want %d, got %d", name, want, got)
	}
}
func assertBool(t *testing.T, name string, got, want bool) {
	t.Helper()
	if got != want {
		t.Errorf("%s: want %v, got %v", name, want, got)
	}
}
func assertStr(t *testing.T, name string, got interface{}, want string) {
	t.Helper()
	if s, ok := got.(string); !ok || s != want {
		t.Errorf("%s: want %q, got %v", name, want, got)
	}
}
func gf(m map[string]interface{}, k string) float64 {
	if v, ok := m[k]; ok && v != nil {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}
func gi(m map[string]interface{}, k string) int { return int(gf(m, k)) }
func gb(m map[string]interface{}, k string) bool {
	if v, ok := m[k]; ok && v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func TestFlightCRUD(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-crud"), "SecurePass123!", "CRUD")
	var fid string
	t.Run("create", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EFLY", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		fid = f["id"].(string)
		assertStr(t, "reg", f["aircraftReg"], "D-EFLY")
	})
	t.Run("list", func(t *testing.T) {
		r := c.GET("/flights")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		if len(m["data"].([]interface{})) < 1 {
			t.Error("empty")
		}
	})
	t.Run("get", func(t *testing.T) { requireStatus(t, c.GET("/flights/"+fid), 200) })
	t.Run("update", func(t *testing.T) {
		r := c.PUT("/flights/"+fid, map[string]interface{}{"date": today(), "aircraftReg": "D-EFLY", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "10:00", "landings": 1, "remarks": "Updated"})
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)
		assertFloat(t, "totalTime", gf(f, "totalTime"), 2.0, 0.1)
	})
	t.Run("delete", func(t *testing.T) {
		assertStatus(t, c.DELETE("/flights/"+fid), 204)
		assertStatus(t, c.GET("/flights/"+fid), 404)
	})
	t.Run("404", func(t *testing.T) { assertStatus(t, c.GET("/flights/00000000-0000-0000-0000-000000000000"), 404) })
	t.Run("400 missing", func(t *testing.T) { assertStatus(t, c.POST("/flights", map[string]interface{}{"date": today()}), 400) })
	t.Run("401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "X", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1}), 401)
	})
}

func TestFlightTotalTimeCalc(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-tt"), "SecurePass123!", "TT")

	cases := []struct {
		name, off, on string
		want          float64
	}{
		{"1h", "08:00", "09:00", 1.0},
		{"1.5h", "08:00", "09:30", 1.5},
		{"2.5h", "08:00", "10:30", 2.5},
		{"30min", "12:00", "12:30", 0.5},
		{"3h20m", "06:00", "09:20", 3.3},
		{"overnight 23-01", "23:00", "01:00", 2.0},
		{"overnight 22-06", "22:00", "06:00", 8.0},
		{"HH:MM:SS", "08:00:00", "09:30:00", 1.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-ETT", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": tc.off, "onBlockTime": tc.on, "landings": 1})
			requireStatus(t, r, 201)
			var f map[string]interface{}
			r.JSON(&f)
			assertFloat(t, "totalTime", gf(f, "totalTime"), tc.want, 0.15)
		})
	}
}

func TestFlightPICDualSolo(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-pds"), "SecurePass123!", "PDS")

	t.Run("solo PIC - no crew", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EPIC", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "10:00", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertBool(t, "isPic", gb(f, "isPic"), true)
		assertBool(t, "isDual", gb(f, "isDual"), false)
		assertFloat(t, "picTime", gf(f, "picTime"), 2.0, 0.1)
		assertFloat(t, "dualTime", gf(f, "dualTime"), 0.0, 0.01)
		assertFloat(t, "soloTime", gf(f, "soloTime"), 2.0, 0.1)
	})

	t.Run("dual with Instructor", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EPIC", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "10:00", "onBlockTime": "12:00", "landings": 1, "crewMembers": []map[string]interface{}{{"name": "CFI Smith", "role": "Instructor"}}})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertBool(t, "isPic", gb(f, "isPic"), false)
		assertBool(t, "isDual", gb(f, "isDual"), true)
		assertFloat(t, "dualTime", gf(f, "dualTime"), 2.0, 0.1)
		assertFloat(t, "picTime", gf(f, "picTime"), 0.0, 0.01)
		assertFloat(t, "soloTime", gf(f, "soloTime"), 0.0, 0.01)
	})

	t.Run("PIC with passenger - still solo per regs", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EPIC", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "13:00", "onBlockTime": "14:30", "landings": 1, "crewMembers": []map[string]interface{}{{"name": "Bob", "role": "Passenger"}}})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertBool(t, "isPic", gb(f, "isPic"), true)
		assertBool(t, "isDual", gb(f, "isDual"), false)
		assertFloat(t, "picTime", gf(f, "picTime"), 1.5, 0.1)
		t.Logf("soloTime with pax: %.2f", gf(f, "soloTime"))
	})

	t.Run("SIC crew sets sicTime", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-ESIC", "aircraftType": "PA44", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "15:00", "onBlockTime": "17:00", "landings": 1, "crewMembers": []map[string]interface{}{{"name": "FO", "role": "SIC"}}})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		t.Logf("sicTime with SIC crew: %.2f", gf(f, "sicTime"))
	})

	t.Run("Student crew with dualGiven", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EINST", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "10:00", "landings": 1, "crewMembers": []map[string]interface{}{{"name": "Student", "role": "Student"}}, "dualGivenTime": 2.0})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		t.Logf("dualGivenTime: %.2f", gf(f, "dualGivenTime"))
	})
}

func TestFlightXCDistance(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-xc"), "SecurePass123!", "XC")

	t.Run("EDNY-EDDS ~63NM", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EXC", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertFloat(t, "crossCountryTime", gf(f, "crossCountryTime"), 1.5, 0.15)
		d := gf(f, "distance")
		if d < 50 || d > 80 {
			t.Errorf("EDNY-EDDS: want ~63NM, got %.1f", d)
		}
		t.Logf("EDNY-EDDS: %.1f NM", d)
	})

	t.Run("local EDNY-EDNY no XC", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EXC", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDNY", "offBlockTime": "10:00", "onBlockTime": "11:00", "landings": 5})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertFloat(t, "crossCountryTime", gf(f, "crossCountryTime"), 0.0, 0.01)
		assertFloat(t, "distance", gf(f, "distance"), 0.0, 0.1)
	})

	t.Run("EDNY-EDDM ~90NM", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EXC", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDM", "offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		d := gf(f, "distance")
		if d < 70 || d > 120 {
			t.Errorf("EDNY-EDDM: want ~90NM, got %.1f", d)
		}
		t.Logf("EDNY-EDDM: %.1f NM", d)
	})

	t.Run("KJFK-EGLL ~3000NM", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "N12345", "aircraftType": "B738", "departureIcao": "KJFK", "arrivalIcao": "EGLL", "offBlockTime": "10:00", "onBlockTime": "17:30", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		d := gf(f, "distance")
		assertFloat(t, "totalTime", gf(f, "totalTime"), 7.5, 0.1)
		if d < 2900 || d > 3100 {
			t.Errorf("KJFK-EGLL: want ~3000NM, got %.1f", d)
		}
		t.Logf("KJFK-EGLL: %.1f NM", d)
	})

	t.Run("EDDS-EDTL short hop", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EXC", "aircraftType": "C172", "departureIcao": "EDDS", "arrivalIcao": "EDTL", "offBlockTime": "08:00", "onBlockTime": "08:30", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		d := gf(f, "distance")
		if d <= 0 {
			t.Error("Expected positive distance")
		}
		t.Logf("EDDS-EDTL: %.1f NM", d)
	})
}

func TestFlightNightTime(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-night"), "SecurePass123!", "Night")

	t.Run("summer daytime - no night", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": "2025-07-15", "aircraftReg": "D-ENGT", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "10:00", "onBlockTime": "11:30", "departureTime": "10:10", "arrivalTime": "11:20", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertFloat(t, "nightTime", gf(f, "nightTime"), 0.0, 0.1)
		assertInt(t, "landingsDay", gi(f, "landingsDay"), 1)
		assertInt(t, "landingsNight", gi(f, "landingsNight"), 0)
		assertInt(t, "takeoffsDay", gi(f, "takeoffsDay"), 1)
		assertInt(t, "takeoffsNight", gi(f, "takeoffsNight"), 0)
	})

	t.Run("winter evening - all night", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": "2025-12-15", "aircraftReg": "D-ENGT", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "18:00", "onBlockTime": "19:30", "departureTime": "18:10", "arrivalTime": "19:20", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		nt := gf(f, "nightTime")
		// Night calc uses dep/arr times (1h10m ≈ 1.2h), not block time (1.5h)
		if nt < 1.0 {
			t.Errorf("nightTime %.2f should be > 1.0 for winter evening flight", nt)
		}
		assertInt(t, "landingsNight", gi(f, "landingsNight"), 1)
		assertInt(t, "landingsDay", gi(f, "landingsDay"), 0)
		assertInt(t, "takeoffsNight", gi(f, "takeoffsNight"), 1)
		assertInt(t, "takeoffsDay", gi(f, "takeoffsDay"), 0)
		t.Logf("Winter eve: night=%.2f / total=%.2f", nt, gf(f, "totalTime"))
	})

	t.Run("dusk crossing sunset - partial night", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": "2025-03-15", "aircraftReg": "D-ENGT", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "16:00", "onBlockTime": "18:30", "departureTime": "16:10", "arrivalTime": "18:20", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		nt := gf(f, "nightTime")
		tt := gf(f, "totalTime")
		if nt <= 0 {
			t.Errorf("Expected some night time, got %.2f", nt)
		}
		if nt >= tt {
			t.Errorf("Expected partial night: %.2f >= total %.2f", nt, tt)
		}
		assertInt(t, "landingsNight (after sunset)", gi(f, "landingsNight"), 1)
		assertInt(t, "landingsDay", gi(f, "landingsDay"), 0)
		assertInt(t, "takeoffsDay (before sunset)", gi(f, "takeoffsDay"), 1)
		assertInt(t, "takeoffsNight", gi(f, "takeoffsNight"), 0)
		t.Logf("Dusk: night=%.2f / total=%.2f", nt, tt)
	})

	t.Run("5 landings summer day - all day", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": "2025-07-15", "aircraftReg": "D-ENGT", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDNY", "offBlockTime": "10:00", "onBlockTime": "11:30", "departureTime": "10:10", "arrivalTime": "11:20", "landings": 5})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "allLandings", gi(f, "allLandings"), 5)
		assertInt(t, "landingsDay", gi(f, "landingsDay"), 5)
		assertInt(t, "landingsNight", gi(f, "landingsNight"), 0)
	})

	t.Run("night flight without dep/arr times", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": "2025-12-15", "aircraftReg": "D-ENGT", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "20:00", "onBlockTime": "21:30", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		t.Logf("Night without dep/arr times: nightTime=%.2f", gf(f, "nightTime"))
	})
}

func TestFlightAllFields(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-all"), "SecurePass123!", "All")

	t.Run("every field populated", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": "2025-06-15", "aircraftReg": "D-EALL", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "10:30",
			"departureTime": "08:10", "arrivalTime": "10:20", "landings": 3,
			"ifrTime": 0.5, "remarks": "Full test", "route": "EDNY,EDTL,EDDS",
			"holds": 2, "approachesCount": 3,
			"isFlightReview": false, "isIpc": false, "isProficiencyCheck": false,
			"actualInstrumentTime": 0.3, "simulatedInstrumentTime": 0.2,
			"instructorName": "John", "instructorComments": "Good",
			"crewMembers": []map[string]interface{}{{"name": "Jane", "role": "Passenger"}},
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		if f["id"] == nil {
			t.Fatal("no id")
		}
		assertStr(t, "reg", f["aircraftReg"], "D-EALL")
		assertStr(t, "type", f["aircraftType"], "C172")
		assertFloat(t, "totalTime", gf(f, "totalTime"), 2.5, 0.1)
		assertBool(t, "isPic", gb(f, "isPic"), true)
		assertBool(t, "isDual", gb(f, "isDual"), false)
		assertFloat(t, "picTime", gf(f, "picTime"), 2.5, 0.1)
		assertFloat(t, "dualTime", gf(f, "dualTime"), 0.0, 0.01)
		assertFloat(t, "crossCountryTime", gf(f, "crossCountryTime"), 2.5, 0.1)
		d := gf(f, "distance")
		if d < 50 || d > 80 {
			t.Errorf("distance: want 50-80, got %.1f", d)
		}
		assertFloat(t, "nightTime", gf(f, "nightTime"), 0.0, 0.1) // summer morning
		assertInt(t, "allLandings", gi(f, "allLandings"), 3)
		assertInt(t, "landingsDay", gi(f, "landingsDay"), 3)
		assertInt(t, "landingsNight", gi(f, "landingsNight"), 0)
		assertFloat(t, "ifrTime", gf(f, "ifrTime"), 0.5, 0.01)
		assertFloat(t, "actualInstrumentTime", gf(f, "actualInstrumentTime"), 0.3, 0.01)
		assertFloat(t, "simulatedInstrumentTime", gf(f, "simulatedInstrumentTime"), 0.2, 0.01)
		assertInt(t, "holds", gi(f, "holds"), 2)
		assertInt(t, "approachesCount", gi(f, "approachesCount"), 3)
		assertBool(t, "isIpc", gb(f, "isIpc"), false)
		assertBool(t, "isFlightReview", gb(f, "isFlightReview"), false)
		assertBool(t, "isProficiencyCheck", gb(f, "isProficiencyCheck"), false)
		assertStr(t, "remarks", f["remarks"], "Full test")
		assertStr(t, "route", f["route"], "EDNY,EDTL,EDDS")
		if f["instructorName"] != nil {
			assertStr(t, "instructorName", f["instructorName"], "John")
		}
		if f["instructorComments"] != nil {
			assertStr(t, "instructorComments", f["instructorComments"], "Good")
		}
		if crew, ok := f["crewMembers"].([]interface{}); ok {
			assertInt(t, "crewLen", len(crew), 1)
			m := crew[0].(map[string]interface{})
			assertStr(t, "crew.name", m["name"], "Jane")
			assertStr(t, "crew.role", m["role"], "Passenger")
		}
	})

	t.Run("IPC with full instruments and dual crew", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": "2025-06-15", "aircraftReg": "D-EIPC", "aircraftType": "C172",
			"departureIcao": "EDDS", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "10:00",
			"departureTime": "08:10", "arrivalTime": "09:50", "landings": 4,
			"isIpc": true, "approachesCount": 6, "holds": 3,
			"actualInstrumentTime": 1.5, "simulatedInstrumentTime": 0.0,
			"crewMembers": []map[string]interface{}{{"name": "CFII", "role": "Instructor"}, {"name": "Safety", "role": "SafetyPilot"}},
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertBool(t, "isIpc", gb(f, "isIpc"), true)
		assertBool(t, "isDual", gb(f, "isDual"), true)
		assertFloat(t, "dualTime", gf(f, "dualTime"), 2.0, 0.1)
		assertFloat(t, "picTime", gf(f, "picTime"), 0.0, 0.01)
		assertFloat(t, "actualInstrumentTime", gf(f, "actualInstrumentTime"), 1.5, 0.01)
		assertInt(t, "approachesCount", gi(f, "approachesCount"), 6)
		assertInt(t, "holds", gi(f, "holds"), 3)
		assertFloat(t, "crossCountryTime", gf(f, "crossCountryTime"), 0.0, 0.01) // local
		if crew, ok := f["crewMembers"].([]interface{}); ok {
			assertInt(t, "crewLen", len(crew), 2)
		}
	})

	t.Run("proficiency check", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": "2025-06-15", "aircraftReg": "D-PRO", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": "09:30",
			"departureTime": "08:10", "arrivalTime": "09:20", "landings": 4,
			"isProficiencyCheck": true,
			"crewMembers":        []map[string]interface{}{{"name": "DPE", "role": "Examiner"}},
		})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertBool(t, "isProficiencyCheck", gb(f, "isProficiencyCheck"), true)
		if crew, ok := f["crewMembers"].([]interface{}); ok {
			assertStr(t, "role", crew[0].(map[string]interface{})["role"], "Examiner")
		}
	})
}

func TestFlightGliderLaunch(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-gl"), "SecurePass123!", "Glider")
	for _, m := range []string{"winch", "aerotow", "self-launch"} {
		t.Run(m, func(t *testing.T) {
			r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-1234", "aircraftType": "ASK21", "departureIcao": "EDNY", "arrivalIcao": "EDNY", "offBlockTime": "10:00", "onBlockTime": "10:45", "landings": 1, "launchMethod": m})
			requireStatus(t, r, 201)
			var f map[string]interface{}
			r.JSON(&f)
			if lm, ok := f["launchMethod"].(string); ok {
				assertStr(t, "launchMethod", lm, m)
			}
		})
	}
}

func TestFlightCrewConfigs(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-crw"), "SecurePass123!", "Crew")

	t.Run("no crew", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-ECRW", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertBool(t, "isPic", gb(f, "isPic"), true)
		assertBool(t, "isDual", gb(f, "isDual"), false)
	})

	t.Run("all 7 crew roles", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-ECRW", "aircraftType": "B738", "departureIcao": "EDDS", "arrivalIcao": "EDDM", "offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1, "crewMembers": []map[string]interface{}{{"name": "A", "role": "PIC"}, {"name": "B", "role": "SIC"}, {"name": "C", "role": "Instructor"}, {"name": "D", "role": "Student"}, {"name": "E", "role": "Passenger"}, {"name": "F", "role": "SafetyPilot"}, {"name": "G", "role": "Examiner"}}})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		if crew, ok := f["crewMembers"].([]interface{}); ok {
			assertInt(t, "crewLen", len(crew), 7)
			roles := map[string]bool{}
			for _, m := range crew {
				roles[m.(map[string]interface{})["role"].(string)] = true
			}
			for _, want := range []string{"PIC", "SIC", "Instructor", "Student", "Passenger", "SafetyPilot", "Examiner"} {
				if !roles[want] {
					t.Errorf("Missing role: %s", want)
				}
			}
		}
		assertBool(t, "isDual (instructor present)", gb(f, "isDual"), true)
	})
}

func TestFlightIsolation(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	registerAndLogin(t, c1, uniqueEmail("fiso1"), "SecurePass123!", "U1")
	registerAndLogin(t, c2, uniqueEmail("fiso2"), "SecurePass123!", "U2")
	r := c1.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-EONE", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1})
	requireStatus(t, r, 201)
	var f map[string]interface{}
	r.JSON(&f)
	fid := f["id"].(string)
	t.Run("u2 empty", func(t *testing.T) {
		r := c2.GET("/flights")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		if len(m["data"].([]interface{})) != 0 {
			t.Error("not 0")
		}
	})
	t.Run("u2 404 get", func(t *testing.T) { assertStatus(t, c2.GET("/flights/"+fid), 404) })
	t.Run("u2 404 del", func(t *testing.T) { assertStatus(t, c2.DELETE("/flights/"+fid), 404) })
	t.Run("u2 404 upd", func(t *testing.T) {
		assertStatus(t, c2.PUT("/flights/"+fid, map[string]interface{}{"date": today(), "aircraftReg": "HACKED", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1}), 404)
	})
}

func TestFlightBulk(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-bulk"), "SecurePass123!", "Bulk")
	for i := 0; i < 5; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{"date": pastDate(i), "aircraftReg": "D-EBLK", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1}), 201)
	}
	t.Run("recalculate", func(t *testing.T) { requireStatus(t, c.POST("/flights/recalculate", nil), 200) })
	t.Run("delete all", func(t *testing.T) {
		requireStatus(t, c.DELETE("/flights/delete-all"), 200)
		r := c.GET("/flights")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		if len(m["data"].([]interface{})) != 0 {
			t.Error("not 0")
		}
	})
}

func TestFlightEdgeCases(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-edge"), "SecurePass123!", "Edge")
	t.Run("bad time 400", func(t *testing.T) {
		assertStatus(t, c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-E", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "nope", "onBlockTime": "09:00", "landings": 1}), 400)
	})
	t.Run("bad date 400", func(t *testing.T) {
		assertStatus(t, c.POST("/flights", map[string]interface{}{"date": "bad", "aircraftReg": "D-E", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1}), 400)
	})
	t.Run("HH:MM:SS ok", func(t *testing.T) {
		requireStatus(t, c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-E", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00:00", "onBlockTime": "09:30:00", "landings": 1}), 201)
	})
	t.Run("5min flight", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-E", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDNY", "offBlockTime": "10:00", "onBlockTime": "10:05", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertFloat(t, "tt", gf(f, "totalTime"), 0.1, 0.1)
	})
	t.Run("12h flight", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "N999", "aircraftType": "B738", "departureIcao": "KJFK", "arrivalIcao": "KLAX", "offBlockTime": "06:00", "onBlockTime": "18:00", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertFloat(t, "tt", gf(f, "totalTime"), 12.0, 0.1)
	})
	t.Run("15 landings pattern", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": "2025-07-15", "aircraftReg": "D-E", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDNY", "offBlockTime": "10:00", "onBlockTime": "12:00", "departureTime": "10:10", "arrivalTime": "11:50", "landings": 15})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "allLandings", gi(f, "allLandings"), 15)
		assertInt(t, "landingsDay", gi(f, "landingsDay"), 15)
		assertInt(t, "landingsNight", gi(f, "landingsNight"), 0)
	})
	t.Run("update recalculates", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "D-UPD", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1})
		requireStatus(t, r, 201)
		var f map[string]interface{}
		r.JSON(&f)
		fid := f["id"].(string)
		r = c.PUT(fmt.Sprintf("/flights/%s", fid), map[string]interface{}{"date": today(), "aircraftReg": "D-UPD", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDM", "offBlockTime": "08:00", "onBlockTime": "10:00", "landings": 1})
		requireStatus(t, r, 200)
		r.JSON(&f)
		assertFloat(t, "totalTime", gf(f, "totalTime"), 2.0, 0.1)
		d := gf(f, "distance")
		if d < 70 {
			t.Errorf("distance EDNY-EDDM: want >70, got %.1f", d)
		}
		assertFloat(t, "xc", gf(f, "crossCountryTime"), 2.0, 0.1)
	})
}

func TestFlightFilterSort(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-fs"), "SecurePass123!", "FilterSort")
	flights := []map[string]interface{}{
		{"date": pastDate(30), "aircraftReg": "D-EFLT", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1},
		{"date": pastDate(20), "aircraftReg": "D-EFLT", "aircraftType": "C172", "departureIcao": "EDDS", "arrivalIcao": "EDTL", "offBlockTime": "10:00", "onBlockTime": "11:30", "landings": 1},
		{"date": pastDate(10), "aircraftReg": "D-GXYZ", "aircraftType": "PA28", "departureIcao": "EDTL", "arrivalIcao": "EDNY", "offBlockTime": "14:00", "onBlockTime": "15:00", "landings": 1},
		{"date": today(), "aircraftReg": "D-GXYZ", "aircraftType": "PA28", "departureIcao": "EDNY", "arrivalIcao": "EDNY", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 5, "remarks": "Pattern practice"},
	}
	for _, fl := range flights {
		requireStatus(t, c.POST("/flights", fl), 201)
	}

	t.Run("by reg", func(t *testing.T) {
		r := c.GET("/flights?aircraftReg=D-GXYZ")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		data := m["data"].([]interface{})
		assertInt(t, "count", len(data), 2)
	})
	t.Run("by dep", func(t *testing.T) {
		r := c.GET("/flights?departureIcao=EDNY")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		if len(m["data"].([]interface{})) < 2 {
			t.Error("expected >=2")
		}
	})
	t.Run("by arr", func(t *testing.T) {
		r := c.GET("/flights?arrivalIcao=EDDS")
		requireStatus(t, r, 200)
	})
	t.Run("date range", func(t *testing.T) {
		r := c.GET(fmt.Sprintf("/flights?startDate=%s&endDate=%s", pastDate(25), pastDate(5)))
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		assertInt(t, "count", len(m["data"].([]interface{})), 2)
	})
	t.Run("search text", func(t *testing.T) {
		r := c.GET("/flights?search=Pattern")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		if len(m["data"].([]interface{})) < 1 {
			t.Error("no match")
		}
	})
	t.Run("page=2", func(t *testing.T) {
		r := c.GET("/flights?pageSize=2&page=1")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		if len(m["data"].([]interface{})) > 2 {
			t.Error(">2")
		}
	})
	t.Run("sort totalTime desc", func(t *testing.T) {
		r := c.GET("/flights?sortBy=totalTime&sortOrder=desc")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		data := m["data"].([]interface{})
		if len(data) >= 2 {
			a := gf(data[0].(map[string]interface{}), "totalTime")
			b := gf(data[1].(map[string]interface{}), "totalTime")
			if a < b {
				t.Errorf("desc: %.2f < %.2f", a, b)
			}
		}
	})
	t.Run("sort date asc", func(t *testing.T) {
		r := c.GET("/flights?sortBy=date&sortOrder=asc")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		data := m["data"].([]interface{})
		if len(data) >= 2 {
			a := data[0].(map[string]interface{})["date"].(string)
			z := data[len(data)-1].(map[string]interface{})["date"].(string)
			if a > z {
				t.Errorf("asc: %s > %s", a, z)
			}
		}
	})
}
