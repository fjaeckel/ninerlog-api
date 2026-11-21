//go:build e2e

package e2e_test

import "testing"

func TestHobbsFlightLogging(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("hobbs"), "SecurePass123!", "Hobbs Pilot")
	read := func(id string) map[string]interface{} {
		t.Helper()
		r := c.GET("/flights/" + id)
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.MustJSON(t, &f)
		return f
	}
	block := map[string]interface{}{"date": today(), "aircraftReg": "N12345", "aircraftType": "C172", "departureIcao": "EDDF", "arrivalIcao": "EDDM", "offBlockTime": "08:00", "onBlockTime": "10:00", "landings": 1}
	r := c.POST("/flights", block)
	requireStatus(t, r, 201)
	var old map[string]interface{}
	r.MustJSON(t, &old)
	oldID := old["id"].(string)
	requireStatus(t, c.PUT("/flights/"+oldID, map[string]interface{}{"totalTime": 83}), 200)
	requireStatus(t, c.PATCH("/users/me", map[string]interface{}{"defaultFlightTimeMethod": "hobbs"}), 200)
	r = c.GET("/users/me")
	requireStatus(t, r, 200)
	var profile map[string]interface{}
	r.MustJSON(t, &profile)
	assertStr(t, "preference", profile["defaultFlightTimeMethod"], "hobbs")
	assertInt(t, "H1 historical total", gi(read(oldID), "totalTime"), 83)
	hobbs := map[string]interface{}{"date": today(), "aircraftReg": "N12345", "aircraftType": "C172", "departureIcao": "EDDF", "arrivalIcao": "EDDM", "hobbsStartTenths": 15234, "hobbsEndTenths": 15247, "landings": 1}
	r = c.POST("/flights", hobbs)
	requireStatus(t, r, 201)
	var f map[string]interface{}
	r.MustJSON(t, &f)
	id := f["id"].(string)
	assertStr(t, "saved method", f["flightTimeMethod"], "hobbs")
	assertInt(t, "total", gi(f, "totalTime"), 78)
	assertInt(t, "PIC", gi(f, "picTime"), 78)
	requireStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"offBlockTime": "08:00", "onBlockTime": "11:00", "totalTime": 999}), 200)
	assertInt(t, "timestamps cannot override Hobbs", gi(read(id), "totalTime"), 78)
	requireStatus(t, c.PATCH("/users/me", map[string]interface{}{"defaultFlightTimeMethod": "block"}), 200)
	assertStr(t, "saved method unaffected", read(id)["flightTimeMethod"], "hobbs")
	requireStatus(t, c.POST("/flights/recalculate", map[string]interface{}{}), 200)
	assertInt(t, "H1 manual total survives recalc", gi(read(oldID), "totalTime"), 83)
	assertInt(t, "Hobbs total survives recalc", gi(read(id), "totalTime"), 78)
	hobbs["flightTimeMethod"] = "hobbs"
	hobbs["hobbsEndTenths"] = 15233
	requireStatus(t, c.POST("/flights", hobbs), 400)
	hobbs["hobbsEndTenths"] = 1523.4
	requireStatus(t, c.POST("/flights", hobbs), 400)
	requireStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"flightTimeMethod": "block"}), 200)
	f = read(id)
	assertInt(t, "per-flight block override", gi(f, "totalTime"), 180)
	assertStr(t, "block method", f["flightTimeMethod"], "block")
}

func TestManualFlightIndependentCredits(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("manualcredit"), "SecurePass123!", "Manual Pilot")
	requireStatus(t, c.PATCH("/users/me", map[string]interface{}{"defaultFlightTimeMethod": "manual"}), 200)
	r := c.POST("/flights", map[string]interface{}{"date": today(), "aircraftReg": "N483MC", "aircraftType": "C172", "departureIcao": "KORD", "arrivalIcao": "KORD", "landings": 1, "manualTotalTime": 90, "picTime": 66, "dualTime": 90, "simulatedInstrumentTime": 30})
	requireStatus(t, r, 201)
	var f map[string]interface{}
	r.MustJSON(t, &f)
	id := f["id"].(string)
	assertStr(t, "method", f["flightTimeMethod"], "manual")
	assertInt(t, "total", gi(f, "totalTime"), 90)
	assertInt(t, "PIC", gi(f, "picTime"), 66)
	assertInt(t, "dual", gi(f, "dualTime"), 90)
	assertInt(t, "instrument", gi(f, "simulatedInstrumentTime"), 30)
	assertInt(t, "solo", gi(f, "soloTime"), 0)
	requireStatus(t, c.POST("/flights/recalculate", map[string]interface{}{}), 200)
	r = c.GET("/flights/" + id)
	requireStatus(t, r, 200)
	r.MustJSON(t, &f)
	assertInt(t, "saved PIC after recalc", gi(f, "picTime"), 66)
	assertInt(t, "saved dual after recalc", gi(f, "dualTime"), 90)
	requireStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"offBlockTime": "08:00", "onBlockTime": "12:00"}), 200)
	r = c.GET("/flights/" + id)
	requireStatus(t, r, 200)
	r.MustJSON(t, &f)
	assertInt(t, "manual overrides clocks", gi(f, "totalTime"), 90)
	requireStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"picTime": 91}), 400)
	requireStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"picTime": 0, "dualTime": nil}), 200)
	r = c.GET("/flights/" + id)
	requireStatus(t, r, 200)
	r.MustJSON(t, &f)
	assertInt(t, "explicit zero PIC", gi(f, "picTime"), 0)
	assertInt(t, "automatic dual", gi(f, "dualTime"), 0)
}
