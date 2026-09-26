//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// ─── Soaring statistics: season summary and soaring custom-report metrics ──

type soaringSeasonBody struct {
	Year             int `json:"year"`
	Flights          int `json:"flights"`
	Launches         int `json:"launches"`
	LaunchesByMethod struct {
		Winch       int `json:"winch"`
		Aerotow     int `json:"aerotow"`
		SelfLaunch  int `json:"selfLaunch"`
		Car         int `json:"car"`
		Bungee      int `json:"bungee"`
		Unspecified int `json:"unspecified"`
	} `json:"launchesByMethod"`
	TotalMinutes  int `json:"totalMinutes"`
	LongestFlight *struct {
		FlightID    string  `json:"flightId"`
		Date        string  `json:"date"`
		Minutes     int     `json:"minutes"`
		AircraftReg *string `json:"aircraftReg"`
	} `json:"longestFlight"`
	Outlandings          int `json:"outlandings"`
	AverageFlightMinutes int `json:"averageFlightMinutes"`
	Sites                []struct {
		Place   string `json:"place"`
		Flights int    `json:"flights"`
	} `json:"sites"`
}

type soaringReportRow struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Value       int    `json:"value"`
	Flights     int    `json:"flights"`
	Launches    int    `json:"launches"`
	Outlandings int    `json:"outlandings"`
	TowFlights  int    `json:"towFlights"`
}

type soaringReportResult struct {
	Rows   []soaringReportRow `json:"rows"`
	Totals struct {
		Flights     int `json:"flights"`
		Launches    int `json:"launches"`
		Outlandings int `json:"outlandings"`
		TowFlights  int `json:"towFlights"`
	} `json:"totals"`
}

func getSoaringSeason(t *testing.T, c *E2EClient, query string) soaringSeasonBody {
	t.Helper()
	resp := c.GET("/reports/soaring-season" + query)
	requireStatus(t, resp, http.StatusOK)
	var s soaringSeasonBody
	if err := resp.JSON(&s); err != nil {
		t.Fatalf("decode season: %v", err)
	}
	return s
}

func previewSoaringReport(t *testing.T, c *E2EClient, groupBy, metric string) soaringReportResult {
	t.Helper()
	resp := c.POST("/reports/custom/preview", map[string]interface{}{"definition": map[string]interface{}{
		"filter":  map[string]interface{}{},
		"window":  map[string]interface{}{"kind": "range", "startDate": "2025-01-01", "endDate": "2025-12-31"},
		"groupBy": groupBy,
		"metric":  metric,
	}})
	requireStatus(t, resp, http.StatusOK)
	var res soaringReportResult
	if err := resp.JSON(&res); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return res
}

func soaringRow(rows []soaringReportRow, key string) *soaringReportRow {
	for i := range rows {
		if rows[i].Key == key {
			return &rows[i]
		}
	}
	return nil
}

// seedLenaSeason logs Lena's 2025 season on the ASK 21 D-1234 and LS4 D-5678,
// a tow in the club DR400 and one 2024 circuit.
func seedLenaSeason(t *testing.T, c *E2EClient) {
	t.Helper()
	createAircraftCur(t, c, "D-5678", "LS4", "GLIDER")
	createAircraftCur(t, c, "D-EXYZ", "DR40", "SEP_LAND")

	requireStatus(t, c.POST("/flights/batch", map[string]interface{}{
		"template": lenaCircuitTemplate("2025-05-10"),
		"legs":     circuitLegs(6),
	}), http.StatusCreated)

	for _, f := range []map[string]interface{}{
		{"date": "2025-06-14", "aircraftReg": "D-5678", "aircraftType": "LS4",
			"departureIcao": "EDNY", "arrivalIcao": "Feld bei Riedlingen",
			"departureTime": "11:00", "arrivalTime": "14:30", "landings": 1,
			"launchMethod": "aerotow", "isOutlanding": true},
		{"date": "2025-07-05", "aircraftReg": "D-1234", "aircraftType": "ASK21",
			"departureIcao": "EDTM", "arrivalIcao": "EDTM",
			"departureTime": "10:00", "arrivalTime": "10:40", "landings": 1, "launchMethod": "winch"},
		{"date": "2025-08-01", "aircraftReg": "D-5678", "aircraftType": "LS4",
			"departureIcao": "EDTM", "arrivalIcao": "EDTM",
			"departureTime": "12:00", "arrivalTime": "13:00", "landings": 1},
		{"date": "2025-06-14", "aircraftReg": "D-EXYZ", "aircraftType": "DR40",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "10:00", "onBlockTime": "10:15", "landings": 1, "isTowFlight": true},
		{"date": "2024-07-01", "aircraftReg": "D-1234", "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"departureTime": "10:00", "arrivalTime": "10:08", "landings": 1, "launchMethod": "winch"},
	} {
		createFlightCur(t, c, f)
	}
}

func TestSoaringStats_LenaSeason(t *testing.T) {
	c := setupLena(t, "soaring-lena")
	seedLenaSeason(t, c)

	t.Run("L Lena season: launches by method, longest flight, outlandings, top sites", func(t *testing.T) {
		s := getSoaringSeason(t, c, "?year=2025")
		assertInt(t, "year", s.Year, 2025)
		assertInt(t, "flights", s.Flights, 9)
		assertInt(t, "launches", s.Launches, 9)
		assertInt(t, "winch", s.LaunchesByMethod.Winch, 7)
		assertInt(t, "aerotow", s.LaunchesByMethod.Aerotow, 1)
		assertInt(t, "unspecified", s.LaunchesByMethod.Unspecified, 1)
		assertInt(t, "selfLaunch", s.LaunchesByMethod.SelfLaunch, 0)
		assertInt(t, "totalMinutes", s.TotalMinutes, 48+210+40+60)
		assertInt(t, "averageFlightMinutes", s.AverageFlightMinutes, 40)
		assertInt(t, "outlandings", s.Outlandings, 1)
		if s.LongestFlight == nil {
			t.Fatal("longestFlight missing")
		}
		assertInt(t, "longest minutes", s.LongestFlight.Minutes, 210)
		assertStr(t, "longest date", s.LongestFlight.Date, "2025-06-14")
		if s.LongestFlight.AircraftReg == nil || *s.LongestFlight.AircraftReg != "D-5678" {
			t.Errorf("longest aircraftReg = %v, want D-5678", s.LongestFlight.AircraftReg)
		}
		if len(s.Sites) != 2 || s.Sites[0].Place != "EDNY" || s.Sites[0].Flights != 7 ||
			s.Sites[1].Place != "EDTM" || s.Sites[1].Flights != 2 {
			t.Errorf("sites = %+v, want EDNY 7, EDTM 2", s.Sites)
		}
	})

	t.Run("an earlier season holds only its own flights", func(t *testing.T) {
		s := getSoaringSeason(t, c, "?year=2024")
		assertInt(t, "flights", s.Flights, 1)
		assertInt(t, "winch", s.LaunchesByMethod.Winch, 1)
	})

	t.Run("L custom report with launches metric grouped by launchMethod", func(t *testing.T) {
		res := previewSoaringReport(t, c, "launchMethod", "launches")
		if len(res.Rows) != 3 {
			t.Fatalf("rows = %+v, want winch, (none), aerotow", res.Rows)
		}
		assertStr(t, "first key", res.Rows[0].Key, "winch")
		assertStr(t, "first label", res.Rows[0].Label, "Winch")
		assertInt(t, "winch value", res.Rows[0].Value, 7)
		if r := soaringRow(res.Rows, "aerotow"); r == nil || r.Value != 1 || r.Outlandings != 1 {
			t.Errorf("aerotow row = %+v", r)
		}
		none := soaringRow(res.Rows, "")
		if none == nil || none.Value != 1 || none.Flights != 2 || none.TowFlights != 1 {
			t.Errorf("(none) row = %+v, want the LS4 flight's launch and the tow flight", none)
		}
		assertInt(t, "totals launches", res.Totals.Launches, 9)
		assertInt(t, "totals outlandings", res.Totals.Outlandings, 1)
		assertInt(t, "totals towFlights", res.Totals.TowFlights, 1)
	})

	t.Run("aircraftClass grouping reads the fleet class", func(t *testing.T) {
		res := previewSoaringReport(t, c, "aircraftClass", "flights")
		if r := soaringRow(res.Rows, "GLIDER"); r == nil || r.Flights != 9 || r.Launches != 9 {
			t.Errorf("GLIDER row = %+v", r)
		}
		if r := soaringRow(res.Rows, "SEP_LAND"); r == nil || r.Flights != 1 || r.Launches != 0 || r.TowFlights != 1 {
			t.Errorf("SEP_LAND row = %+v, want the tow with no launches", r)
		}
	})

	t.Run("invalid metric and groupBy are 400", func(t *testing.T) {
		for _, def := range []map[string]interface{}{
			{"filter": map[string]interface{}{}, "window": map[string]interface{}{"kind": "all"}, "groupBy": "launchMethod", "metric": "distanceKm"},
			{"filter": map[string]interface{}{}, "window": map[string]interface{}{"kind": "all"}, "groupBy": "launch_method", "metric": "launches"},
		} {
			assertStatus(t, c.POST("/reports/custom/preview", map[string]interface{}{"definition": def}), http.StatusBadRequest)
		}
	})
}

func TestSoaringStats_SabineULKind(t *testing.T) {
	c := setupCurrencyUser(t, "soaring-sabine")
	createULAircraftCur(t, c, "D-MTRK", "WEIGHT_SHIFT")
	createULAircraftCur(t, c, "PPG-SABINE", "POWERED_PARAGLIDER")
	for i, reg := range []string{"D-MTRK", "D-MTRK", "D-MTRK", "PPG-SABINE", "PPG-SABINE"} {
		createFlightCur(t, c, map[string]interface{}{
			"date": fmt.Sprintf("2025-06-%02d", i+1), "aircraftReg": reg, "aircraftType": "UL",
			"departureIcao": "Hof Sonnenfeld", "arrivalIcao": "Hof Sonnenfeld",
			"offBlockTime": "08:00", "onBlockTime": "08:30", "landings": 1,
		})
	}

	t.Run("S ulKind grouping keeps trike and paraglider apart", func(t *testing.T) {
		res := previewSoaringReport(t, c, "ulKind", "flights")
		if len(res.Rows) != 2 {
			t.Fatalf("rows = %+v, want WEIGHT_SHIFT and POWERED_PARAGLIDER", res.Rows)
		}
		if r := soaringRow(res.Rows, "WEIGHT_SHIFT"); r == nil || r.Flights != 3 || r.Label != "Weight-shift" {
			t.Errorf("WEIGHT_SHIFT row = %+v", r)
		}
		if r := soaringRow(res.Rows, "POWERED_PARAGLIDER"); r == nil || r.Flights != 2 || r.Label != "Powered paraglider" {
			t.Errorf("POWERED_PARAGLIDER row = %+v", r)
		}
		assertInt(t, "UL flights carry no launches", res.Totals.Launches, 0)
	})

	t.Run("S powered ultralights are not soaring flights", func(t *testing.T) {
		s := getSoaringSeason(t, c, "?year=2025")
		assertInt(t, "flights", s.Flights, 0)
		assertInt(t, "launches", s.Launches, 0)
	})
}

func TestSoaringStats_MarkGuard(t *testing.T) {
	c := setupCurrencyUser(t, "soaring-mark")
	createAircraftCur(t, c, "D-AIPX", "A320", "MEP_LAND")
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(3), "aircraftReg": "D-AIPX", "aircraftType": "A320",
		"departureIcao": "EDDF", "arrivalIcao": "EDDM",
		"offBlockTime": "08:00", "onBlockTime": "09:10", "landings": 1,
	})

	t.Run("A1 Mark season is empty: zeros, not an error", func(t *testing.T) {
		s := getSoaringSeason(t, c, "")
		assertInt(t, "year", s.Year, time.Now().UTC().Year())
		assertInt(t, "flights", s.Flights, 0)
		assertInt(t, "launches", s.Launches, 0)
		assertInt(t, "totalMinutes", s.TotalMinutes, 0)
		assertInt(t, "averageFlightMinutes", s.AverageFlightMinutes, 0)
		assertInt(t, "outlandings", s.Outlandings, 0)
		if s.LongestFlight != nil {
			t.Errorf("longestFlight = %+v, want none", s.LongestFlight)
		}
		if s.Sites == nil || len(s.Sites) != 0 {
			t.Errorf("sites = %#v, want []", s.Sites)
		}
	})

	t.Run("A1 airline flights carry no launches in custom reports", func(t *testing.T) {
		resp := c.POST("/reports/custom/preview", map[string]interface{}{"definition": map[string]interface{}{
			"filter": map[string]interface{}{}, "window": map[string]interface{}{"kind": "all"},
			"groupBy": "aircraftClass", "metric": "launches",
		}})
		requireStatus(t, resp, http.StatusOK)
		var res soaringReportResult
		resp.JSON(&res)
		assertInt(t, "flights", res.Totals.Flights, 1)
		assertInt(t, "launches", res.Totals.Launches, 0)
	})

	t.Run("400 on bad year", func(t *testing.T) {
		next := time.Now().UTC().Year() + 1
		for _, q := range []string{"?year=1899", fmt.Sprintf("?year=%d", next+1), "?year=abc"} {
			assertStatus(t, c.GET("/reports/soaring-season"+q), http.StatusBadRequest)
		}
		getSoaringSeason(t, c, fmt.Sprintf("?year=%d", next))
		getSoaringSeason(t, c, "?year=1900")
	})

	t.Run("unauthenticated is 401", func(t *testing.T) {
		anon := NewE2EClient(t)
		assertStatus(t, anon.GET("/reports/soaring-season"), http.StatusUnauthorized)
	})
}
