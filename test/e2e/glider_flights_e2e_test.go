//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// ─── Glider flight facts: launches, batch circuits, outlanding, tow flight ──

// lenaCircuitTemplate is Lena's ASK 21 D-1234 on the winch at one site.
func lenaCircuitTemplate(date string) map[string]interface{} {
	return map[string]interface{}{
		"date": date, "aircraftReg": "D-1234", "aircraftType": "ASK21",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY", "launchMethod": "winch",
	}
}

// circuitLegs returns n legs of 8 minutes, 10 minutes apart from 10:00 UTC.
func circuitLegs(n int) []map[string]interface{} {
	legs := make([]map[string]interface{}, n)
	for i := range legs {
		start := 10*60 + i*10
		legs[i] = map[string]interface{}{
			"departureTime": fmt.Sprintf("%02d:%02d", start/60, start%60),
			"arrivalTime":   fmt.Sprintf("%02d:%02d", (start+8)/60, (start+8)%60),
		}
	}
	return legs
}

// setupLena registers a pilot with an SPL GLIDER rating and the ASK 21 D-1234.
func setupLena(t *testing.T, prefix string) *E2EClient {
	t.Helper()
	c := setupCurrencyUser(t, prefix)
	createAircraftCur(t, c, "D-1234", "ASK21", "GLIDER")
	licID := createLicenseCur(t, c, "EASA", "SPL")
	createRatingCur(t, c, licID, "GLIDER", nil)
	return c
}

// winchLaunches returns the winch launch count of the GLIDER rating.
func winchLaunches(t *testing.T, c *E2EClient) int {
	t.Helper()
	rc := findRatingCur(getCurrencyStatus(t, c), "GLIDER")
	if rc == nil {
		t.Fatal("GLIDER rating currency not found")
	}
	methods, _ := rc["launchMethodCurrency"].([]interface{})
	for _, m := range methods {
		mm := m.(map[string]interface{})
		if mm["method"] == "winch" {
			return gi(mm, "launches")
		}
	}
	return 0
}

func flightCount(t *testing.T, c *E2EClient) int {
	t.Helper()
	resp := c.GET("/flights")
	requireStatus(t, resp, http.StatusOK)
	var page map[string]interface{}
	resp.JSON(&page)
	pg, _ := page["pagination"].(map[string]interface{})
	return gi(pg, "total")
}

func TestGliderFlights_BatchCircuits(t *testing.T) {
	t.Run("L1 six winch circuits in one batch", func(t *testing.T) {
		c := setupLena(t, "l1-batch")
		resp := c.POST("/flights/batch", map[string]interface{}{
			"template": lenaCircuitTemplate(pastDate(5)),
			"legs":     circuitLegs(6),
		})
		requireStatus(t, resp, http.StatusCreated)
		var res struct {
			Flights []map[string]interface{} `json:"flights"`
		}
		if err := resp.JSON(&res); err != nil {
			t.Fatal(err)
		}
		if len(res.Flights) != 6 {
			t.Fatalf("created %d flights, want 6", len(res.Flights))
		}
		total, launches := 0, 0
		for i, f := range res.Flights {
			total += gi(f, "totalTime")
			launches += gi(f, "launches")
			assertStr(t, fmt.Sprintf("leg %d launchMethod", i), f["launchMethod"], "winch")
			assertStr(t, fmt.Sprintf("leg %d arrivalIcao", i), f["arrivalIcao"], "EDNY")
			assertBool(t, fmt.Sprintf("leg %d launchesOverride", i), gb(f, "launchesOverride"), false)
			assertInt(t, fmt.Sprintf("leg %d crossCountryTime", i), gi(f, "crossCountryTime"), 0)
			if f["offBlockTime"] != nil {
				t.Errorf("leg %d offBlockTime = %v, want none", i, f["offBlockTime"])
			}
		}
		assertInt(t, "total minutes", total, 48)
		assertInt(t, "launches", launches, 6)
		assertInt(t, "flights stored", flightCount(t, c), 6)
		assertInt(t, "winch launch currency", winchLaunches(t, c), 6)
	})

	t.Run("batch with an invalid leg creates nothing", func(t *testing.T) {
		c := setupLena(t, "l1-batch-atomic")
		legs := circuitLegs(4)
		legs[2]["launches"] = -1
		resp := c.POST("/flights/batch", map[string]interface{}{
			"template": lenaCircuitTemplate(pastDate(5)),
			"legs":     legs,
		})
		requireStatus(t, resp, http.StatusBadRequest)
		if !strings.Contains(string(resp.Body), "Leg 2: launches cannot be negative") {
			t.Errorf("body = %s, want the leg 2 validation error", resp.Body)
		}
		assertInt(t, "flights stored", flightCount(t, c), 0)
	})

	t.Run("leg missing its landing time names the leg", func(t *testing.T) {
		c := setupLena(t, "l1-batch-shape")
		legs := circuitLegs(3)
		delete(legs[1], "arrivalTime")
		resp := c.POST("/flights/batch", map[string]interface{}{
			"template": lenaCircuitTemplate(pastDate(5)),
			"legs":     legs,
		})
		requireStatus(t, resp, http.StatusBadRequest)
		if !strings.Contains(string(resp.Body), "Leg 1:") {
			t.Errorf("body = %s, want it to name leg 1", resp.Body)
		}
		assertInt(t, "flights stored", flightCount(t, c), 0)
	})

	t.Run("leg count bounds", func(t *testing.T) {
		c := setupLena(t, "l1-batch-bounds")
		for _, n := range []int{0, 51} {
			resp := c.POST("/flights/batch", map[string]interface{}{
				"template": lenaCircuitTemplate(pastDate(5)),
				"legs":     circuitLegs(n),
			})
			requireStatus(t, resp, http.StatusBadRequest)
		}
		resp := c.POST("/flights/batch", map[string]interface{}{
			"template": lenaCircuitTemplate(pastDate(5)),
			"legs":     circuitLegs(50),
		})
		requireStatus(t, resp, http.StatusCreated)
		assertInt(t, "flights stored", flightCount(t, c), 50)
	})

	t.Run("unauthenticated", func(t *testing.T) {
		c := NewE2EClient(t)
		resp := c.POST("/flights/batch", map[string]interface{}{
			"template": lenaCircuitTemplate(pastDate(5)),
			"legs":     circuitLegs(1),
		})
		requireStatus(t, resp, http.StatusUnauthorized)
	})
}

func TestGliderFlights_Launches(t *testing.T) {
	t.Run("L1 series entry of six launches counts six", func(t *testing.T) {
		c := setupLena(t, "l1-series")
		body := lenaCircuitTemplate(pastDate(5))
		body["departureTime"], body["arrivalTime"] = "10:00", "11:00"
		body["landings"] = 6
		body["launches"] = 6
		resp := c.POST("/flights", body)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		assertInt(t, "launches", gi(f, "launches"), 6)
		assertBool(t, "launchesOverride", gb(f, "launchesOverride"), true)
		assertInt(t, "winch launch currency", winchLaunches(t, c), 6)

		upd := c.PUT("/flights/"+f["id"].(string), map[string]interface{}{"launches": nil, "landings": 1})
		requireStatus(t, upd, http.StatusOK)
		upd.JSON(&f)
		assertInt(t, "launches after null", gi(f, "launches"), 1)
		assertBool(t, "launchesOverride after null", gb(f, "launchesOverride"), false)
		assertInt(t, "winch launch currency after null", winchLaunches(t, c), 1)
	})

	t.Run("SFCL.160(a) counts 15 launches from three series entries", func(t *testing.T) {
		c := setupLena(t, "l1-series-15")
		for i := 0; i < 3; i++ {
			body := lenaCircuitTemplate(pastDate(10 + i))
			body["departureTime"], body["arrivalTime"] = "10:00", "11:40"
			body["landings"] = 1
			body["launches"] = 5
			createFlightCur(t, c, body)
		}
		rc := findRatingCur(getCurrencyStatus(t, c), "GLIDER")
		if rc == nil {
			t.Fatal("GLIDER rating currency not found")
		}
		req := getReq(rc, "requirement.launches")
		if req == nil {
			t.Fatal("launches requirement not found")
		}
		assertInt(t, "launches requirement", gi(req, "current"), 15)
		assertBool(t, "launches met", gb(req, "met"), true)
		assertInt(t, "winch launch currency", winchLaunches(t, c), 15)
	})

	t.Run("take-offs derive launches", func(t *testing.T) {
		c := setupLena(t, "launch-derive")
		body := lenaCircuitTemplate(pastDate(5))
		body["departureTime"], body["arrivalTime"] = "10:00", "10:30"
		body["landings"] = 3
		resp := c.POST("/flights", body)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		assertInt(t, "launches", gi(f, "launches"), 3)
		assertBool(t, "launchesOverride", gb(f, "launchesOverride"), false)
	})

	t.Run("negative launches rejected", func(t *testing.T) {
		c := setupLena(t, "launch-negative")
		body := lenaCircuitTemplate(pastDate(5))
		body["departureTime"], body["arrivalTime"] = "10:00", "10:30"
		body["landings"] = 1
		body["launches"] = -1
		assertStatus(t, c.POST("/flights", body), http.StatusBadRequest)
	})
}

func TestGliderFlights_FlightFacts(t *testing.T) {
	t.Run("P1 outlanding round trip derives no cross-country time", func(t *testing.T) {
		c := setupLena(t, "p1-outlanding")
		body := map[string]interface{}{
			"date": pastDate(5), "aircraftReg": "D-1234", "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "Feld bei Riedlingen",
			"departureTime": "11:00", "arrivalTime": "14:30", "landings": 1,
			"launchMethod": "aerotow", "isOutlanding": true, "releaseHeightM": 600,
		}
		resp := c.POST("/flights", body)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		assertBool(t, "isOutlanding", gb(f, "isOutlanding"), true)
		assertInt(t, "crossCountryTime", gi(f, "crossCountryTime"), 0)
		assertInt(t, "releaseHeightM", gi(f, "releaseHeightM"), 600)

		get := c.GET("/flights/" + f["id"].(string))
		requireStatus(t, get, http.StatusOK)
		get.JSON(&f)
		assertBool(t, "isOutlanding after GET", gb(f, "isOutlanding"), true)
		assertInt(t, "releaseHeightM after GET", gi(f, "releaseHeightM"), 600)

		upd := c.PUT("/flights/"+f["id"].(string), map[string]interface{}{"isOutlanding": false})
		requireStatus(t, upd, http.StatusOK)
		upd.JSON(&f)
		assertBool(t, "isOutlanding cleared", gb(f, "isOutlanding"), false)
		assertInt(t, "crossCountryTime re-derived", gi(f, "crossCountryTime"), 210)

		search := c.GET("/flights?q=outlanding:true")
		requireStatus(t, search, http.StatusOK)
		var page map[string]interface{}
		search.JSON(&page)
		if data, _ := page["data"].([]interface{}); len(data) != 0 {
			t.Errorf("outlanding:true matched %d flights after clearing, want 0", len(data))
		}
	})

	t.Run("tow flight round trip", func(t *testing.T) {
		c := setupCurrencyUser(t, "tow-flight")
		createAircraftCur(t, c, "D-EXYZ", "DR40", "SEP_LAND")
		resp := c.POST("/flights", map[string]interface{}{
			"date": pastDate(5), "aircraftReg": "D-EXYZ", "aircraftType": "DR40",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "10:00", "onBlockTime": "10:15", "landings": 1,
			"isTowFlight": true,
		})
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		assertBool(t, "isTowFlight", gb(f, "isTowFlight"), true)
		assertBool(t, "isOutlanding", gb(f, "isOutlanding"), false)
		if f["launchMethod"] != nil {
			t.Errorf("launchMethod = %v, want none on the tug", f["launchMethod"])
		}

		search := c.GET("/flights?q=towflight:true")
		requireStatus(t, search, http.StatusOK)
		var page map[string]interface{}
		search.JSON(&page)
		if data, _ := page["data"].([]interface{}); len(data) != 1 {
			t.Errorf("towflight:true matched %d flights, want 1", len(data))
		}

		upd := c.PUT("/flights/"+f["id"].(string), map[string]interface{}{"isTowFlight": false})
		requireStatus(t, upd, http.StatusOK)
		upd.JSON(&f)
		assertBool(t, "isTowFlight cleared", gb(f, "isTowFlight"), false)
	})

	t.Run("release height bounds", func(t *testing.T) {
		c := setupLena(t, "release-height")
		for _, tc := range []struct {
			height int
			want   int
		}{
			{0, http.StatusCreated},
			{20000, http.StatusCreated},
			{-1, http.StatusBadRequest},
			{20001, http.StatusBadRequest},
		} {
			body := lenaCircuitTemplate(pastDate(5))
			body["departureTime"], body["arrivalTime"] = "10:00", "10:08"
			body["landings"] = 1
			body["releaseHeightM"] = tc.height
			assertStatus(t, c.POST("/flights", body), tc.want)
		}

		body := lenaCircuitTemplate(pastDate(5))
		body["departureTime"], body["arrivalTime"] = "10:00", "10:08"
		body["landings"] = 1
		body["releaseHeightM"] = 400
		resp := c.POST("/flights", body)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		id := f["id"].(string)
		assertStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"releaseHeightM": 20001}), http.StatusBadRequest)
		upd := c.PUT("/flights/"+id, map[string]interface{}{"releaseHeightM": nil})
		requireStatus(t, upd, http.StatusOK)
		var cleared map[string]interface{}
		upd.JSON(&cleared)
		if cleared["releaseHeightM"] != nil {
			t.Errorf("releaseHeightM = %v after null, want none", f["releaseHeightM"])
		}
	})
}

func TestGliderFlights_SPLRecencyCountsSupervisedSolo(t *testing.T) {
	c := setupLena(t, "j3-spic")
	for i := 0; i < 2; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(40 + i), "aircraftReg": "D-1234", "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"departureTime": "10:00", "arrivalTime": "10:30",
			"landings": 1, "launchMethod": "winch",
			"crewMembers": []map[string]interface{}{{"name": "FI Weber", "role": "Instructor"}},
		})
	}
	for i := 0; i < 13; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(10 + i), "aircraftReg": "D-1234", "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"departureTime": "11:00", "arrivalTime": "11:20",
			"landings": 1, "launchMethod": "winch", "spicTime": 20,
		})
	}

	rc := findRatingCur(getCurrencyStatus(t, c), "GLIDER")
	if rc == nil {
		t.Fatal("GLIDER rating currency not found")
	}
	progress, _ := rc["progress"].(map[string]interface{})
	assertInt(t, "progress.spicMinutes", gi(progress, "spicMinutes"), 260)
	assertInt(t, "progress.picMinutes", gi(progress, "picMinutes"), 0)
	req := getReq(rc, "requirement.flight_time")
	if req == nil {
		t.Fatal("flight time requirement not found")
	}
	assertInt(t, "flight time counts dual + supervised solo", gi(req, "current"), 320)
	assertBool(t, "flight time met", gb(req, "met"), true)
	assertStr(t, "status", rc["status"], "current")
}
