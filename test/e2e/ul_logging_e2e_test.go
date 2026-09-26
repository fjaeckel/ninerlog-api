//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// ─── WP-31: UL logging fit — paraglider names, night and MTOM warnings ───────

func warningCodesOf(v interface{}) []string {
	var out []string
	arr, _ := v.([]interface{})
	for _, w := range arr {
		if m, ok := w.(map[string]interface{}); ok {
			out = append(out, fmt.Sprint(m["code"]))
		}
	}
	return out
}

func requireWarnings(t *testing.T, name string, body map[string]interface{}, want ...string) {
	t.Helper()
	got := warningCodesOf(body["warnings"])
	if len(want) == 0 {
		if _, present := body["warnings"]; present {
			t.Errorf("%s: warnings = %v, want the field absent", name, body["warnings"])
		}
		return
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s: warning codes = %v, want %v", name, got, want)
	}
}

func ulNightFlight(reg string, extra map[string]interface{}) map[string]interface{} {
	f := map[string]interface{}{
		"date": pastDate(3), "aircraftReg": reg, "aircraftType": "C42",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"offBlockTime": "12:00", "onBlockTime": "13:00", "landings": 1,
	}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

func TestULLogging_PoweredParagliderName(t *testing.T) {
	t.Run("S2 powered paraglider without a registration", func(t *testing.T) {
		c := setupCurrencyUser(t, "s2-ppg")
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "PPG-Viper", "type": "PPG", "make": "Ozone", "model": "Viper",
			"aircraftClass": "ULTRALIGHT", "ulKind": "POWERED_PARAGLIDER",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		assertStr(t, "registration", ac["registration"], "PPG-VIPER")
		requireWarnings(t, "aircraft", ac)

		resp = c.POST("/flights", map[string]interface{}{
			"date": pastDate(2), "aircraftReg": "ppg-viper", "aircraftType": "PPG",
			"departureIcao": "Farm strip", "arrivalIcao": "Farm strip",
			"offBlockTime": "10:00", "onBlockTime": "10:40", "landings": 1,
		})
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		assertStr(t, "flight aircraftReg", f["aircraftReg"], "PPG-VIPER")
		requireWarnings(t, "flight", f)
	})

	t.Run("S2 paraglider name shaped like a registration is kept", func(t *testing.T) {
		c := setupCurrencyUser(t, "s2-ppg-apco")
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "Apco", "type": "PPG", "make": "Apco", "model": "Hybrid",
			"aircraftClass": "ULTRALIGHT", "ulKind": "POWERED_PARAGLIDER",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		assertStr(t, "registration", ac["registration"], "APCO")

		resp = c.POST("/flights", map[string]interface{}{
			"date": pastDate(2), "aircraftReg": "Apco", "aircraftType": "PPG",
			"departureIcao": "Farm strip", "arrivalIcao": "Farm strip",
			"offBlockTime": "10:00", "onBlockTime": "10:30", "landings": 1,
		})
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		assertStr(t, "flight aircraftReg", f["aircraftReg"], "APCO")
		id := f["id"].(string)

		resp = c.PUT("/flights/"+id, map[string]interface{}{"remarks": "thermal soaring"})
		requireStatus(t, resp, http.StatusOK)
		resp.JSON(&f)
		assertStr(t, "updated aircraftReg", f["aircraftReg"], "APCO")
	})

	t.Run("A2 a registration shaped the same on a SEP is still canonicalised", func(t *testing.T) {
		c := setupCurrencyUser(t, "s2-ppg-guard")
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "deabc", "type": "C172", "make": "Cessna", "model": "172",
			"aircraftClass": "SEP_LAND",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		assertStr(t, "registration", ac["registration"], "D-EABC")
		requireWarnings(t, "aircraft", ac)
	})
}

func TestULLogging_NightWarning(t *testing.T) {
	c := setupCurrencyUser(t, "m4-night")
	createULAircraftCur(t, c, "D-MXYZ", "THREE_AXIS")
	createAircraftCur(t, c, "D-EFGH", "C172", "SEP_LAND")

	t.Run("M4 night on a UL warns but saves", func(t *testing.T) {
		resp := c.POST("/flights", ulNightFlight("D-MXYZ", map[string]interface{}{"nightTime": 20}))
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		requireWarnings(t, "flight", f, "ul_night_flight")
		w := f["warnings"].([]interface{})[0].(map[string]interface{})
		assertStr(t, "severity", w["severity"], "warning")
		params := w["params"].(map[string]interface{})
		assertInt(t, "params.nightTime", gi(params, "nightTime"), 20)
		assertStr(t, "params.ulKind", params["ulKind"], "THREE_AXIS")

		get := c.GET("/flights/" + f["id"].(string))
		requireStatus(t, get, http.StatusOK)
		var stored map[string]interface{}
		get.JSON(&stored)
		assertInt(t, "stored nightTime", gi(stored, "nightTime"), 20)
		requireWarnings(t, "GET", stored)
	})

	t.Run("M4 night derived from block times on a UL warns", func(t *testing.T) {
		resp := c.POST("/flights", ulNightFlight("D-MXYZ", map[string]interface{}{"offBlockTime": "22:00", "onBlockTime": "23:00"}))
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		if gi(f, "landingsNight") != 1 {
			t.Fatalf("landingsNight = %v, want a derived night landing", f["landingsNight"])
		}
		requireWarnings(t, "flight", f, "ul_night_flight")
	})

	t.Run("M4 update adding night time warns", func(t *testing.T) {
		resp := c.POST("/flights", ulNightFlight("D-MXYZ", nil))
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		requireWarnings(t, "day flight", f)

		resp = c.PUT("/flights/"+f["id"].(string), map[string]interface{}{"nightTime": 15})
		requireStatus(t, resp, http.StatusOK)
		resp.JSON(&f)
		requireWarnings(t, "updated flight", f, "ul_night_flight")
	})

	t.Run("M4 each batch leg carries its warning", func(t *testing.T) {
		resp := c.POST("/flights/batch", map[string]interface{}{
			"template": map[string]interface{}{
				"date": pastDate(4), "aircraftReg": "D-MXYZ", "aircraftType": "C42",
				"departureIcao": "EDNY", "arrivalIcao": "EDNY", "nightTime": 10,
				"offBlockTime": "12:00", "onBlockTime": "12:30",
			},
			"legs": []map[string]interface{}{{}, {}},
		})
		requireStatus(t, resp, http.StatusCreated)
		var res struct {
			Flights []map[string]interface{} `json:"flights"`
		}
		resp.JSON(&res)
		if len(res.Flights) != 2 {
			t.Fatalf("created %d flights, want 2", len(res.Flights))
		}
		for i, f := range res.Flights {
			requireWarnings(t, fmt.Sprintf("leg %d", i), f, "ul_night_flight")
		}
	})

	t.Run("A2 night on a SEP has no warning", func(t *testing.T) {
		resp := c.POST("/flights", ulNightFlight("D-EFGH", map[string]interface{}{"offBlockTime": "22:00", "onBlockTime": "23:00"}))
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		if gi(f, "nightTime") == 0 {
			t.Fatalf("nightTime = 0, want a night flight")
		}
		requireWarnings(t, "SEP flight", f)
	})
}

func TestULLogging_MTOMWarnings(t *testing.T) {
	c := setupCurrencyUser(t, "ul-mtom")
	create := func(reg, class, kind string, mtom int) map[string]interface{} {
		t.Helper()
		body := map[string]interface{}{
			"registration": reg, "type": "UL", "make": "Test", "model": "Test",
			"aircraftClass": class, "maxTakeoffMassKg": mtom,
		}
		if kind != "" {
			body["ulKind"] = kind
		}
		resp := c.POST("/aircraft", body)
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		return ac
	}

	t.Run("UL above 600 kg warns", func(t *testing.T) {
		ac := create("D-MHVY", "ULTRALIGHT", "THREE_AXIS", 650)
		requireWarnings(t, "aircraft", ac, "ul_mtom_exceeds_600")
		p := ac["warnings"].([]interface{})[0].(map[string]interface{})["params"].(map[string]interface{})
		assertInt(t, "params.mtomKg", gi(p, "mtomKg"), 650)
		assertInt(t, "params.limitKg", gi(p, "limitKg"), 600)
	})

	t.Run("UL at 600 kg is silent and a PATCH above warns", func(t *testing.T) {
		ac := create("D-MOKK", "ULTRALIGHT", "THREE_AXIS", 600)
		requireWarnings(t, "aircraft", ac)
		resp := c.PATCH("/aircraft/"+ac["id"].(string), map[string]interface{}{"maxTakeoffMassKg": 601})
		requireStatus(t, resp, http.StatusOK)
		var got map[string]interface{}
		resp.JSON(&got)
		requireWarnings(t, "patched aircraft", got, "ul_mtom_exceeds_600")
	})

	t.Run("S2 paraglider at 115 kg is the 120 kg class", func(t *testing.T) {
		ac := create("PPG Nucleon", "ULTRALIGHT", "POWERED_PARAGLIDER", 115)
		requireWarnings(t, "aircraft", ac, "ul_120kg_class")
		w := ac["warnings"].([]interface{})[0].(map[string]interface{})
		assertStr(t, "severity", w["severity"], "info")
	})

	t.Run("A2 SEP above 600 kg has no warning", func(t *testing.T) {
		ac := create("D-EHVY", "SEP_LAND", "", 1111)
		requireWarnings(t, "aircraft", ac)
	})
}
