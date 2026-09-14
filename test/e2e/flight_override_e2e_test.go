//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// Summer midday EDNY→EDDS: derivation yields night 0 and cross-country = block
// time, so an override is visible as a value derivation would never produce.
func overrideFlightBody(extra map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"date": "2025-07-15", "aircraftReg": "D-EOVR", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "10:00", "onBlockTime": "11:30", "landings": 1,
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func TestFlightNightAndCrossCountryOverride(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-override"), "SecurePass123!", "Override")

	var fid string

	t.Run("create without the fields derives them and reports no override", func(t *testing.T) {
		r := c.POST("/flights", overrideFlightBody(nil))
		requireStatus(t, r, http.StatusCreated)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "nightTime", gi(f, "nightTime"), 0)
		assertInt(t, "crossCountryTime", gi(f, "crossCountryTime"), 90)
		assertBool(t, "nightTimeOverride", gb(f, "nightTimeOverride"), false)
		assertBool(t, "crossCountryTimeOverride", gb(f, "crossCountryTimeOverride"), false)
		for _, k := range []string{"takeoffsDayOverride", "takeoffsNightOverride", "landingsDayOverride", "landingsNightOverride", "sicTimeOverride", "multiPilotTimeOverride"} {
			if _, ok := f[k]; !ok {
				t.Errorf("response is missing %s", k)
			}
		}
	})

	t.Run("create with the fields keeps the pilot's values", func(t *testing.T) {
		r := c.POST("/flights", overrideFlightBody(map[string]interface{}{"nightTime": 30, "crossCountryTime": 20}))
		requireStatus(t, r, http.StatusCreated)
		var f map[string]interface{}
		r.JSON(&f)
		fid = f["id"].(string)
		assertInt(t, "nightTime", gi(f, "nightTime"), 30)
		assertInt(t, "crossCountryTime", gi(f, "crossCountryTime"), 20)
		assertBool(t, "nightTimeOverride", gb(f, "nightTimeOverride"), true)
		assertBool(t, "crossCountryTimeOverride", gb(f, "crossCountryTimeOverride"), true)
	})

	t.Run("recalculate keeps the overrides", func(t *testing.T) {
		r := c.POST("/flights/recalculate", map[string]interface{}{})
		requireStatus(t, r, http.StatusOK)
		g := c.GET("/flights/" + fid)
		requireStatus(t, g, http.StatusOK)
		var f map[string]interface{}
		g.JSON(&f)
		assertInt(t, "nightTime after recalculate", gi(f, "nightTime"), 30)
		assertInt(t, "crossCountryTime after recalculate", gi(f, "crossCountryTime"), 20)
		assertBool(t, "nightTimeOverride", gb(f, "nightTimeOverride"), true)
	})

	t.Run("update omitting the fields leaves them alone", func(t *testing.T) {
		r := c.PUT("/flights/"+fid, map[string]interface{}{"remarks": "touched"})
		requireStatus(t, r, http.StatusOK)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "nightTime", gi(f, "nightTime"), 30)
		assertInt(t, "crossCountryTime", gi(f, "crossCountryTime"), 20)
		assertBool(t, "nightTimeOverride", gb(f, "nightTimeOverride"), true)
		assertBool(t, "crossCountryTimeOverride", gb(f, "crossCountryTimeOverride"), true)
	})

	t.Run("update with a number replaces the value", func(t *testing.T) {
		r := c.PUT("/flights/"+fid, map[string]interface{}{"nightTime": 45})
		requireStatus(t, r, http.StatusOK)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "nightTime", gi(f, "nightTime"), 45)
		assertBool(t, "nightTimeOverride", gb(f, "nightTimeOverride"), true)
	})

	t.Run("update with null returns the field to derivation", func(t *testing.T) {
		r := c.PUT("/flights/"+fid, map[string]interface{}{"nightTime": nil, "crossCountryTime": nil})
		requireStatus(t, r, http.StatusOK)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "nightTime", gi(f, "nightTime"), 0)
		assertInt(t, "crossCountryTime", gi(f, "crossCountryTime"), 90)
		assertBool(t, "nightTimeOverride", gb(f, "nightTimeOverride"), false)
		assertBool(t, "crossCountryTimeOverride", gb(f, "crossCountryTimeOverride"), false)
	})

	t.Run("values above block time are rejected", func(t *testing.T) {
		r := c.POST("/flights", overrideFlightBody(map[string]interface{}{"nightTime": 91}))
		assertStatus(t, r, http.StatusBadRequest)
		r = c.POST("/flights", overrideFlightBody(map[string]interface{}{"crossCountryTime": 91}))
		assertStatus(t, r, http.StatusBadRequest)
		r = c.PUT("/flights/"+fid, map[string]interface{}{"crossCountryTime": 91})
		assertStatus(t, r, http.StatusBadRequest)
	})

	t.Run("takeoff override can be cleared with null", func(t *testing.T) {
		r := c.PUT("/flights/"+fid, map[string]interface{}{"takeoffsDay": 4})
		requireStatus(t, r, http.StatusOK)
		var f map[string]interface{}
		r.JSON(&f)
		assertInt(t, "takeoffsDay", gi(f, "takeoffsDay"), 4)
		assertBool(t, "takeoffsDayOverride", gb(f, "takeoffsDayOverride"), true)

		r = c.PUT("/flights/"+fid, map[string]interface{}{"takeoffsDay": nil})
		requireStatus(t, r, http.StatusOK)
		r.JSON(&f)
		assertInt(t, "takeoffsDay", gi(f, "takeoffsDay"), 1)
		assertBool(t, "takeoffsDayOverride", gb(f, "takeoffsDayOverride"), false)
	})
}

// Override flags and values survive a JSON export and restore.
func TestFlightOverrideSurvivesJSONRoundTrip(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-override-json"), "SecurePass123!", "OverrideJSON")

	r := c.POST("/flights", overrideFlightBody(map[string]interface{}{"nightTime": 30, "crossCountryTime": 20}))
	requireStatus(t, r, http.StatusCreated)

	exp := c.GET("/exports/json")
	requireStatus(t, exp, http.StatusOK)
	var payload map[string]interface{}
	exp.JSON(&payload)
	flights, _ := payload["flights"].([]interface{})
	if len(flights) != 1 {
		t.Fatalf("exported %d flights, want 1", len(flights))
	}
	exported := flights[0].(map[string]interface{})
	assertBool(t, "exported nightTimeOverride", gb(exported, "nightTimeOverride"), true)
	assertBool(t, "exported crossCountryTimeOverride", gb(exported, "crossCountryTimeOverride"), true)

	c2 := NewE2EClient(t)
	registerAndLogin(t, c2, uniqueEmail("flt-override-json-target"), "SecurePass123!", "OverrideJSONTarget")
	imp := c2.POST("/imports/json", payload)
	requireStatus(t, imp, http.StatusOK)

	list := c2.GET("/flights")
	requireStatus(t, list, http.StatusOK)
	var lr map[string]interface{}
	list.JSON(&lr)
	data, _ := lr["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("restored %d flights, want 1", len(data))
	}
	restored := data[0].(map[string]interface{})
	assertInt(t, "restored nightTime", gi(restored, "nightTime"), 30)
	assertInt(t, "restored crossCountryTime", gi(restored, "crossCountryTime"), 20)
	assertBool(t, "restored nightTimeOverride", gb(restored, "nightTimeOverride"), true)
	assertBool(t, "restored crossCountryTimeOverride", gb(restored, "crossCountryTimeOverride"), true)
}

// A CSV night or cross-country column is stored as an override.
func TestImportCSV_NightAndCrossCountryKept(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("import-night"), "SecurePass123!", "ImportNight")

	csv := "Date,AircraftReg,AircraftType,From,To,OffBlock,OnBlock,Landings,Night,XC\n" +
		fmt.Sprintf("%s,D-ENXC,C172,EDNY,EDDS,10:00,11:30,1,0:30,0:20\n", "2025-07-15")

	up := uploadCSV(t, c, "night.csv", csv)
	requireStatus(t, up, http.StatusOK)
	var u map[string]interface{}
	up.JSON(&u)
	token, _ := u["uploadToken"].(string)

	prev := c.POST("/imports/preview", map[string]interface{}{
		"uploadToken": token,
		"mappings": []map[string]interface{}{
			{"sourceColumn": "Date", "targetField": "date"},
			{"sourceColumn": "AircraftReg", "targetField": "aircraftReg"},
			{"sourceColumn": "AircraftType", "targetField": "aircraftType"},
			{"sourceColumn": "From", "targetField": "departureIcao"},
			{"sourceColumn": "To", "targetField": "arrivalIcao"},
			{"sourceColumn": "OffBlock", "targetField": "offBlockTime"},
			{"sourceColumn": "OnBlock", "targetField": "onBlockTime"},
			{"sourceColumn": "Landings", "targetField": "landingsTotal"},
			{"sourceColumn": "Night", "targetField": "nightTime"},
			{"sourceColumn": "XC", "targetField": "crossCountryTime"},
		},
	})
	requireStatus(t, prev, http.StatusOK)

	conf := c.POST("/imports/confirm", map[string]interface{}{"uploadToken": token})
	if conf.StatusCode != http.StatusOK && conf.StatusCode != http.StatusCreated {
		t.Fatalf("confirm: got %d: %s", conf.StatusCode, string(conf.Body))
	}

	list := c.GET("/flights")
	requireStatus(t, list, http.StatusOK)
	var lr map[string]interface{}
	list.JSON(&lr)
	data, _ := lr["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("imported %d flights, want 1: %s", len(data), string(conf.Body))
	}
	f := data[0].(map[string]interface{})
	assertInt(t, "imported nightTime", gi(f, "nightTime"), 30)
	assertInt(t, "imported crossCountryTime", gi(f, "crossCountryTime"), 20)
	assertBool(t, "imported nightTimeOverride", gb(f, "nightTimeOverride"), true)
	assertBool(t, "imported crossCountryTimeOverride", gb(f, "crossCountryTimeOverride"), true)
}
