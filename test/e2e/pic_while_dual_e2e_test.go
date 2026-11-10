//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestPICWhileDual covers a flight logged as both PIC and dual received
// (14 CFR §61.51(e)(1)(i)): declared with isPic, kept across updates and
// recalculation, counted once against block time, and restored from a backup.
func TestPICWhileDual(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("picdual"), "SecurePass123!", "Tom Pilot")

	create := func(t *testing.T, extra map[string]interface{}) map[string]interface{} {
		body := map[string]interface{}{
			"date": today(), "aircraftReg": "N734TB", "aircraftType": "C182",
			"departureIcao": "KFDK", "arrivalIcao": "KFDK",
			"offBlockTime": "14:00", "onBlockTime": "15:36", "landings": 3,
		}
		for k, v := range extra {
			body[k] = v
		}
		resp := c.POST("/flights", body)
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.MustJSON(t, &f)
		return f
	}
	get := func(t *testing.T, id string) map[string]interface{} {
		resp := c.GET("/flights/" + id)
		requireStatus(t, resp, http.StatusOK)
		var f map[string]interface{}
		resp.MustJSON(t, &f)
		return f
	}
	assertBoth := func(t *testing.T, f map[string]interface{}, isPic bool, pic int) {
		t.Helper()
		assertBool(t, "isPic", gb(f, "isPic"), isPic)
		assertBool(t, "isDual", gb(f, "isDual"), true)
		assertInt(t, "totalTime", gi(f, "totalTime"), 96)
		assertInt(t, "picTime", gi(f, "picTime"), pic)
		assertInt(t, "dualTime", gi(f, "dualTime"), 96)
		assertInt(t, "soloTime", gi(f, "soloTime"), 0)
	}
	instructor := []map[string]interface{}{{"name": "Jane Instructor", "role": "Instructor"}}

	t.Run("declared with an instructor on board", func(t *testing.T) {
		f := create(t, map[string]interface{}{"isPic": true, "crewMembers": instructor, "isFlightReview": true})
		assertBoth(t, f, true, 96)
		id := f["id"].(string)

		requireStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"remarks": "Flight review"}), http.StatusOK)
		assertBoth(t, get(t, id), true, 96)

		requireStatus(t, c.POST("/flights/recalculate", map[string]interface{}{}), http.StatusOK)
		assertBoth(t, get(t, id), true, 96)

		requireStatus(t, c.PUT("/flights/"+id, map[string]interface{}{"isPic": false}), http.StatusOK)
		assertBoth(t, get(t, id), false, 0)
	})

	t.Run("without isPic an instructor makes the flight dual only", func(t *testing.T) {
		f := create(t, map[string]interface{}{"crewMembers": instructor})
		assertBoth(t, f, false, 0)
	})

	t.Run("an update without crewMembers keeps a dual flight dual", func(t *testing.T) {
		f := create(t, map[string]interface{}{"crewMembers": instructor})
		id := f["id"].(string)
		resp := c.PUT("/flights/"+id, map[string]interface{}{"remarks": "Steep turns"})
		requireStatus(t, resp, http.StatusOK)
		var updated map[string]interface{}
		resp.MustJSON(t, &updated)
		assertBoth(t, updated, false, 0)
		assertBoth(t, get(t, id), false, 0)
	})

	t.Run("without an instructor isPic is plain PIC", func(t *testing.T) {
		f := create(t, map[string]interface{}{"isPic": true})
		assertBool(t, "isPic", gb(f, "isPic"), true)
		assertBool(t, "isDual", gb(f, "isDual"), false)
		assertInt(t, "picTime", gi(f, "picTime"), 96)
		assertInt(t, "dualTime", gi(f, "dualTime"), 0)
	})
}

// TestPICWhileDualRestore restores a backup whose flight carries PIC and dual
// received over the same 96 minutes, as an FAA logbook migrated into the
// backup format does.
func TestPICWhileDualRestore(t *testing.T) {
	source := NewE2EClient(t)
	registerAndLogin(t, source, uniqueEmail("picdualsrc"), "SecurePass123!", "Tom Pilot")
	requireStatus(t, source.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "N734TB", "aircraftType": "C182",
		"departureIcao": "KFDK", "arrivalIcao": "KHGR",
		"offBlockTime": "14:00", "onBlockTime": "15:36", "landings": 1,
		"crewMembers": []map[string]interface{}{{"name": "Jane Instructor", "role": "Instructor"}},
	}), http.StatusCreated)

	backupResp := source.GET("/users/me/data")
	requireStatus(t, backupResp, http.StatusOK)
	var backup map[string]interface{}
	if err := json.Unmarshal(backupResp.Body, &backup); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	flights := backup["flights"].([]interface{})
	if len(flights) != 1 {
		t.Fatalf("backup flights = %d, want 1", len(flights))
	}
	f := flights[0].(map[string]interface{})
	f["isPic"] = true
	f["isDual"] = true
	f["picTime"] = 96
	f["dualTime"] = 96

	dest := NewE2EClient(t)
	registerAndLogin(t, dest, uniqueEmail("picdualdst"), "SecurePass123!", "Tom Pilot")
	restoreResp := dest.Do("POST", "/users/me/data", backup)
	requireStatus(t, restoreResp, http.StatusOK)
	var summary struct {
		FlightsImported int               `json:"flightsImported"`
		Warnings        []json.RawMessage `json:"warnings"`
	}
	if err := restoreResp.JSON(&summary); err != nil {
		t.Fatalf("invalid summary: %v", err)
	}
	assertInt(t, "flightsImported", summary.FlightsImported, 1)
	assertInt(t, "warnings", len(summary.Warnings), 0)

	list := dest.GET("/flights")
	requireStatus(t, list, http.StatusOK)
	var page map[string]interface{}
	list.MustJSON(t, &page)
	data := page["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("flights = %d, want 1", len(data))
	}
	got := data[0].(map[string]interface{})
	assertBool(t, "isPic", gb(got, "isPic"), true)
	assertBool(t, "isDual", gb(got, "isDual"), true)
	assertInt(t, "picTime", gi(got, "picTime"), 96)
	assertInt(t, "dualTime", gi(got, "dualTime"), 96)
}
