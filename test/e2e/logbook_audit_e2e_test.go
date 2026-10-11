//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func newFlightBody(date, reg string) map[string]interface{} {
	return map[string]interface{}{
		"date": date, "aircraftReg": reg, "aircraftType": "C172",
		"departureIcao": "EDDF", "arrivalIcao": "EDDM",
		"offBlockTime": "08:00", "onBlockTime": "09:30",
		"landings": 1,
	}
}

func createFlightID(t *testing.T, c *E2EClient, body map[string]interface{}) string {
	t.Helper()
	r := c.POST("/flights", body)
	requireStatus(t, r, http.StatusCreated)
	var f map[string]interface{}
	r.MustJSON(t, &f)
	return f["id"].(string)
}

func putFlight(c *E2EClient, id string, body map[string]interface{}) *Response {
	return c.PUT(fmt.Sprintf("/flights/%s", id), body)
}

func errorCode(t *testing.T, r *Response) string {
	t.Helper()
	var e map[string]interface{}
	if err := r.JSON(&e); err != nil {
		t.Fatalf("error body is not JSON: %s", r.Body)
	}
	code, _ := e["code"].(string)
	return code
}

func listRevisions(t *testing.T, c *E2EClient, flightID string) []map[string]interface{} {
	t.Helper()
	r := c.GET(fmt.Sprintf("/flights/%s/revisions", flightID))
	requireStatus(t, r, http.StatusOK)
	var revs []map[string]interface{}
	r.MustJSON(t, &revs)
	return revs
}

func switchStrict(t *testing.T, c *E2EClient, strict bool) map[string]interface{} {
	t.Helper()
	r := c.PUT("/logbook/mode", map[string]interface{}{"strict": strict})
	requireStatus(t, r, http.StatusOK)
	var mode map[string]interface{}
	r.MustJSON(t, &mode)
	return mode
}

func changeFor(revision map[string]interface{}, field string) map[string]interface{} {
	changes, _ := revision["changes"].([]interface{})
	for _, ch := range changes {
		m := ch.(map[string]interface{})
		if m["field"] == field {
			return m
		}
	}
	return nil
}

func TestLogbookMode(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("lbmode"), "SecurePass123!", "LogbookMode")

	t.Run("defaults to standard mode", func(t *testing.T) {
		r := c.GET("/logbook/mode")
		requireStatus(t, r, http.StatusOK)
		var m map[string]interface{}
		r.MustJSON(t, &m)
		if m["strict"] != false {
			t.Errorf("strict = %v", m["strict"])
		}
		for _, key := range []string{"requiredBy", "suggestedBy", "events"} {
			if arr, ok := m[key].([]interface{}); !ok || len(arr) != 0 {
				t.Errorf("%s = %v, want empty array", key, m[key])
			}
		}
	})

	t.Run("switching stores an event only when the state changes", func(t *testing.T) {
		m := switchStrict(t, c, true)
		if m["strict"] != true || m["since"] == nil || m["draftWindow"] != "flight_day" {
			t.Errorf("mode after switching on = %v", m)
		}
		if events := m["events"].([]interface{}); len(events) != 1 {
			t.Fatalf("events = %d, want 1", len(events))
		}
		m = switchStrict(t, c, true)
		if events := m["events"].([]interface{}); len(events) != 1 {
			t.Fatalf("no-op switch stored an event: %d", len(events))
		}
		m = switchStrict(t, c, false)
		events := m["events"].([]interface{})
		if m["strict"] != false || len(events) != 2 {
			t.Fatalf("mode after switching off = %v", m)
		}
		last := events[1].(map[string]interface{})
		if last["trigger"] != "manual" || last["strict"] != false {
			t.Errorf("last event = %v", last)
		}
	})

	t.Run("body is validated", func(t *testing.T) {
		assertStatus(t, c.PUT("/logbook/mode", map[string]interface{}{}), http.StatusBadRequest)
	})

	t.Run("requires authentication", func(t *testing.T) {
		anon := NewE2EClient(t)
		assertStatus(t, anon.GET("/logbook/mode"), http.StatusUnauthorized)
		assertStatus(t, anon.PUT("/logbook/mode", map[string]interface{}{"strict": true}), http.StatusUnauthorized)
		assertStatus(t, anon.GET("/logbook/certifications"), http.StatusUnauthorized)
		assertStatus(t, anon.GET("/flights/voided"), http.StatusUnauthorized)
	})
}

func TestLogbookModeSwitchedOnByLicence(t *testing.T) {
	licenceBody := func(country interface{}) map[string]interface{} {
		return map[string]interface{}{
			"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "DE.FCL.1",
			"issueDate": "2020-01-01", "issuingAuthority": "LBA", "issuingCountry": country,
		}
	}

	t.Run("a licence without a country never switches strict mode on", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("lic-nocountry"), "SecurePass123!", "NoCountry")
		body := licenceBody(nil)
		delete(body, "issuingCountry")
		requireStatus(t, c.POST("/licenses", body), http.StatusCreated)
		if m := switchModeGet(t, c); m["strict"] != false {
			t.Errorf("strict = %v", m["strict"])
		}
	})

	t.Run("a requiring licence switches strict mode on with a licence event", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("lic-de"), "SecurePass123!", "German")
		r := c.POST("/licenses", licenceBody("de"))
		requireStatus(t, r, http.StatusCreated)
		var lic map[string]interface{}
		r.MustJSON(t, &lic)
		if lic["issuingCountry"] != "DE" {
			t.Errorf("issuingCountry = %v, want DE", lic["issuingCountry"])
		}

		m := switchModeGet(t, c)
		if m["strict"] != true || m["draftWindow"] != "flight_day" || m["overridesDownwardOnly"] != false {
			t.Errorf("mode = %v", m)
		}
		required := m["requiredBy"].([]interface{})
		if len(required) != 1 || required[0].(map[string]interface{})["licenseId"] != lic["id"] {
			t.Errorf("requiredBy = %v", required)
		}
		events := m["events"].([]interface{})
		if len(events) != 1 {
			t.Fatalf("events = %v", events)
		}
		ev := events[0].(map[string]interface{})
		if ev["trigger"] != "licence" || ev["licenseId"] != lic["id"] || ev["strict"] != true {
			t.Errorf("event = %v", ev)
		}

		off := switchStrict(t, c, false)
		if off["strict"] != false {
			t.Error("switching off while a licence requires it must be allowed")
		}
	})

	t.Run("patching the country applies the registry", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("lic-patch"), "SecurePass123!", "Patcher")
		r := c.POST("/licenses", licenceBody("US"))
		requireStatus(t, r, http.StatusCreated)
		var lic map[string]interface{}
		r.MustJSON(t, &lic)
		id := lic["id"].(string)

		m := switchModeGet(t, c)
		if m["strict"] != false || len(m["suggestedBy"].([]interface{})) != 1 {
			t.Fatalf("a suggesting licence must not switch strict mode on: %v", m)
		}

		requireStatus(t, c.PATCH("/licenses/"+id, map[string]interface{}{"issuingCountry": "CH"}), http.StatusOK)
		m = switchModeGet(t, c)
		if m["strict"] != true || m["draftWindow"] != "hours_48" || m["overridesDownwardOnly"] != true {
			t.Errorf("mode after patching to CH = %v", m)
		}

		pr := c.PATCH("/licenses/"+id, map[string]interface{}{"issuingCountry": nil})
		requireStatus(t, pr, http.StatusOK)
		var patched map[string]interface{}
		pr.MustJSON(t, &patched)
		if patched["issuingCountry"] != nil {
			t.Errorf("issuingCountry after null = %v, want cleared", patched["issuingCountry"])
		}
		m = switchModeGet(t, c)
		if m["strict"] != true {
			t.Error("clearing the country must never switch strict mode off")
		}
		if len(m["requiredBy"].([]interface{})) != 0 {
			t.Errorf("requiredBy = %v after clearing the country", m["requiredBy"])
		}
	})

	t.Run("an invalid country is rejected", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("lic-bad"), "SecurePass123!", "Bad")
		assertStatus(t, c.POST("/licenses", licenceBody("DEU")), http.StatusBadRequest)
		assertStatus(t, c.POST("/licenses", licenceBody("1X")), http.StatusBadRequest)
	})
}

func switchModeGet(t *testing.T, c *E2EClient) map[string]interface{} {
	t.Helper()
	r := c.GET("/logbook/mode")
	requireStatus(t, r, http.StatusOK)
	var m map[string]interface{}
	r.MustJSON(t, &m)
	return m
}

func TestFlightRevisions(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("revs"), "SecurePass123!", "Revisions")
	switchStrict(t, c, true)

	t.Run("creating a flight records no revision", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(today(), "D-EREV"))
		if revs := listRevisions(t, c, id); len(revs) != 0 {
			t.Fatalf("revisions = %d, want 0", len(revs))
		}
	})

	t.Run("an edit on the flight day is recorded without a correction", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(today(), "D-EREV"))
		body := newFlightBody(today(), "D-EREV")
		body["remarks"] = "first note"
		requireStatus(t, putFlight(c, id, body), http.StatusOK)

		revs := listRevisions(t, c, id)
		if len(revs) != 1 {
			t.Fatalf("revisions = %d, want 1", len(revs))
		}
		rev := revs[0]
		if rev["source"] != "user_edit" || rev["isCorrection"] != false || rev["kind"] != "update" || rev["revisionNo"].(float64) != 1 {
			t.Errorf("revision = %v", rev)
		}
		ch := changeFor(rev, "remarks")
		if ch == nil || ch["oldValue"] != nil || ch["newValue"] != "first note" {
			t.Errorf("remarks change = %v", ch)
		}
		if changeFor(rev, "id") != nil || changeFor(rev, "updatedAt") != nil || changeFor(rev, "userId") != nil {
			t.Error("bookkeeping fields must not appear as changes")
		}
	})

	t.Run("revisions list newest first with the diff against the next state", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(today(), "D-EREV"))
		body := newFlightBody(today(), "D-EREV")
		body["remarks"] = "one"
		requireStatus(t, putFlight(c, id, body), http.StatusOK)
		body["remarks"] = "two"
		body["crewMembers"] = []map[string]interface{}{{"name": "Pat Instructor", "role": "Instructor"}}
		requireStatus(t, putFlight(c, id, body), http.StatusOK)

		revs := listRevisions(t, c, id)
		if len(revs) != 2 {
			t.Fatalf("revisions = %d, want 2 (a flight edit with a crew change is one revision)", len(revs))
		}
		if revs[0]["revisionNo"].(float64) != 2 || revs[1]["revisionNo"].(float64) != 1 {
			t.Fatalf("order = %v, %v", revs[0]["revisionNo"], revs[1]["revisionNo"])
		}
		if ch := changeFor(revs[0], "remarks"); ch == nil || ch["oldValue"] != "one" || ch["newValue"] != "two" {
			t.Errorf("newest remarks change = %v", ch)
		}
		crew := changeFor(revs[0], "crewMembers")
		if crew == nil || len(crew["oldValue"].([]interface{})) != 0 {
			t.Fatalf("crew change = %v", crew)
		}
		added := crew["newValue"].([]interface{})[0].(map[string]interface{})
		if added["name"] != "Pat Instructor" || added["role"] != "Instructor" {
			t.Errorf("crew new value = %v", added)
		}
		if ch := changeFor(revs[1], "remarks"); ch == nil || ch["oldValue"] != nil || ch["newValue"] != "one" {
			t.Errorf("oldest remarks change = %v", ch)
		}
	})

	t.Run("saving without changes records nothing", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(today(), "D-EREV"))
		requireStatus(t, putFlight(c, id, newFlightBody(today(), "D-EREV")), http.StatusOK)
		if revs := listRevisions(t, c, id); len(revs) != 0 {
			t.Fatalf("revisions = %d, want 0", len(revs))
		}
	})

	t.Run("another user's flight is 404", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(today(), "D-EREV"))
		other := NewE2EClient(t)
		registerAndLogin(t, other, uniqueEmail("revs-other"), "SecurePass123!", "Other")
		assertStatus(t, other.GET(fmt.Sprintf("/flights/%s/revisions", id)), http.StatusNotFound)
	})

	t.Run("an unknown flight is 404", func(t *testing.T) {
		assertStatus(t, c.GET("/flights/00000000-0000-0000-0000-000000000000/revisions"), http.StatusNotFound)
	})

	t.Run("a user outside strict mode gets no history", func(t *testing.T) {
		plain := NewE2EClient(t)
		registerAndLogin(t, plain, uniqueEmail("revs-plain"), "SecurePass123!", "Plain")
		id := createFlightID(t, plain, newFlightBody(pastDate(40), "D-EPLN"))
		body := newFlightBody(pastDate(40), "D-EPLN")
		body["remarks"] = "edited long after the fact"
		requireStatus(t, putFlight(plain, id, body), http.StatusOK)
		if revs := listRevisions(t, plain, id); len(revs) != 0 {
			t.Fatalf("revisions = %d, want 0", len(revs))
		}
		requireStatus(t, plain.DELETE("/flights/"+id), http.StatusNoContent)
		assertStatus(t, plain.GET("/flights/"+id+"/revisions"), http.StatusNotFound)
		r := plain.GET("/flights/voided")
		requireStatus(t, r, http.StatusOK)
		var voided []interface{}
		r.MustJSON(t, &voided)
		if len(voided) != 0 {
			t.Errorf("voided = %d, want 0", len(voided))
		}
	})
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

func TestVoidedFlights(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("void"), "SecurePass123!", "Voided")
	switchStrict(t, c, true)

	body := newFlightBody(pastDate(3), "D-EVOI")
	body["remarks"] = "to be removed"
	id := createFlightID(t, c, body)
	keep := createFlightID(t, c, newFlightBody(today(), "D-EKEP"))

	requireStatus(t, c.DELETE("/flights/"+id+"?reason=Entered+twice"), http.StatusNoContent)
	assertStatus(t, c.GET("/flights/"+id), http.StatusNotFound)

	t.Run("the deleted flight is listed with its reason", func(t *testing.T) {
		r := c.GET("/flights/voided")
		requireStatus(t, r, http.StatusOK)
		var voided []map[string]interface{}
		r.MustJSON(t, &voided)
		if len(voided) != 1 {
			t.Fatalf("voided = %d, want 1", len(voided))
		}
		v := voided[0]
		if v["flightId"] != id || v["reason"] != "Entered twice" || v["aircraftReg"] != "D-EVOI" ||
			v["remarks"] != "to be removed" || v["source"] != "user_edit" || v["date"] != pastDate(3) {
			t.Errorf("void record = %v", v)
		}
		if v["revisionId"] == nil || v["deletedAt"] == nil || v["totalTime"].(float64) != 90 {
			t.Errorf("void record = %v", v)
		}
	})

	t.Run("the deleted flight keeps its history", func(t *testing.T) {
		revs := listRevisions(t, c, id)
		if len(revs) != 1 || revs[0]["kind"] != "delete" || revs[0]["reason"] != "Entered twice" {
			t.Fatalf("revisions = %v", revs)
		}
		if changes := revs[0]["changes"].([]interface{}); len(changes) != 0 {
			t.Errorf("a delete revision carries changes: %v", changes)
		}
	})

	t.Run("another user sees neither", func(t *testing.T) {
		other := NewE2EClient(t)
		registerAndLogin(t, other, uniqueEmail("void-other"), "SecurePass123!", "Other")
		assertStatus(t, other.GET("/flights/"+id+"/revisions"), http.StatusNotFound)
		r := other.GET("/flights/voided")
		requireStatus(t, r, http.StatusOK)
		var voided []interface{}
		r.MustJSON(t, &voided)
		if len(voided) != 0 {
			t.Errorf("voided = %d", len(voided))
		}
	})

	t.Run("a delete does not need a reason and is never a 422", func(t *testing.T) {
		late := createFlightID(t, c, newFlightBody(pastDate(20), "D-ELAT"))
		requireStatus(t, c.DELETE("/flights/"+late), http.StatusNoContent)
	})

	t.Run("an over-long reason is rejected", func(t *testing.T) {
		assertStatus(t, c.DELETE("/flights/"+keep+"?reason="+repeat("r", 501)), http.StatusBadRequest)
	})
}

func TestLogbookCertifications(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cert"), "SecurePass123!", "Certifier")

	post := func(date string) *Response {
		return c.POST("/logbook/certifications", map[string]interface{}{"certifiedThrough": date})
	}

	t.Run("needs strict mode", func(t *testing.T) {
		r := post(today())
		requireStatus(t, r, http.StatusConflict)
		if code := errorCode(t, r); code != "strict_mode_required" {
			t.Errorf("code = %q", code)
		}
	})

	switchStrict(t, c, true)

	t.Run("a date beyond today at UTC+14 is rejected", func(t *testing.T) {
		r := post(futureDate(2))
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "certification_in_future" {
			t.Errorf("code = %q", code)
		}
	})

	t.Run("certifications are stored and listed newest first", func(t *testing.T) {
		r := post(pastDate(10))
		requireStatus(t, r, http.StatusCreated)
		var first map[string]interface{}
		r.MustJSON(t, &first)
		if first["certifiedThrough"] != pastDate(10) || first["id"] == nil || first["certifiedAt"] == nil {
			t.Errorf("certification = %v", first)
		}
		requireStatus(t, post(pastDate(2)), http.StatusCreated)

		lr := c.GET("/logbook/certifications")
		requireStatus(t, lr, http.StatusOK)
		var list []map[string]interface{}
		lr.MustJSON(t, &list)
		if len(list) != 2 || list[0]["certifiedThrough"] != pastDate(2) || list[1]["certifiedThrough"] != pastDate(10) {
			t.Errorf("list = %v", list)
		}
		if m := switchModeGet(t, c); m["latestCertifiedThrough"] != pastDate(2) {
			t.Errorf("latestCertifiedThrough = %v", m["latestCertifiedThrough"])
		}
	})

	t.Run("a date before the latest is rejected", func(t *testing.T) {
		r := post(pastDate(5))
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "certification_before_latest" {
			t.Errorf("code = %q", code)
		}
	})

	t.Run("a malformed date is a bad request", func(t *testing.T) {
		assertStatus(t, post("not-a-date"), http.StatusBadRequest)
	})

	t.Run("certifications stay private", func(t *testing.T) {
		other := NewE2EClient(t)
		registerAndLogin(t, other, uniqueEmail("cert-other"), "SecurePass123!", "Other")
		r := other.GET("/logbook/certifications")
		requireStatus(t, r, http.StatusOK)
		var list []interface{}
		r.MustJSON(t, &list)
		if len(list) != 0 {
			t.Errorf("list = %v", list)
		}
	})
}

func TestAircraftRenameIsRecordedAsSystemChange(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("rename"), "SecurePass123!", "Renamer")
	switchStrict(t, c, true)

	ar := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-ERNM", "type": "C172", "make": "Cessna", "model": "172S", "aircraftClass": "SEP_LAND",
	})
	requireStatus(t, ar, http.StatusCreated)
	var ac map[string]interface{}
	ar.MustJSON(t, &ac)
	id := createFlightID(t, c, newFlightBody(pastDate(2), "D-ERNM"))

	requireStatus(t, c.PATCH("/aircraft/"+ac["id"].(string), map[string]interface{}{"registration": "D-ENEW", "renameFlights": true}), http.StatusOK)

	revs := listRevisions(t, c, id)
	if len(revs) != 1 || revs[0]["source"] != "aircraft_rename" || revs[0]["isCorrection"] != false {
		t.Fatalf("revisions = %v", revs)
	}
	if ch := changeFor(revs[0], "aircraftReg"); ch == nil || ch["oldValue"] != "D-ERNM" || ch["newValue"] != "D-ENEW" {
		t.Errorf("aircraftReg change = %v", ch)
	}
}

func TestLogbookHistorySurvivesExportAndImport(t *testing.T) {
	src := NewE2EClient(t)
	registerAndLogin(t, src, uniqueEmail("lb-export"), "SecurePass123!", "Exporter")
	switchStrict(t, src, true)

	kept := createFlightID(t, src, newFlightBody(today(), "D-EEXP"))
	body := newFlightBody(today(), "D-EEXP")
	body["remarks"] = "edited"
	requireStatus(t, putFlight(src, kept, body), http.StatusOK)

	gone := createFlightID(t, src, newFlightBody(pastDate(6), "D-EGON"))
	requireStatus(t, src.DELETE("/flights/"+gone+"?reason=Duplicate"), http.StatusNoContent)
	requireStatus(t, src.POST("/logbook/certifications", map[string]interface{}{"certifiedThrough": pastDate(1)}), http.StatusCreated)

	export := src.GET("/users/me/data")
	requireStatus(t, export, http.StatusOK)
	var payload map[string]interface{}
	export.MustJSON(t, &payload)
	mode, _ := payload["logbookMode"].(map[string]interface{})
	if mode == nil || mode["strict"] != true || len(mode["events"].([]interface{})) != 1 {
		t.Fatalf("exported logbookMode = %v", payload["logbookMode"])
	}
	if revs, _ := payload["flightRevisions"].([]interface{}); len(revs) != 2 {
		t.Fatalf("exported flightRevisions = %d, want 2", len(revs))
	}
	if certs, _ := payload["logbookCertifications"].([]interface{}); len(certs) != 1 {
		t.Fatalf("exported logbookCertifications = %v", payload["logbookCertifications"])
	}

	dst := NewE2EClient(t)
	registerAndLogin(t, dst, uniqueEmail("lb-import"), "SecurePass123!", "Importer")
	ir := dst.POST("/users/me/data", payload)
	requireStatus(t, ir, http.StatusOK)
	var result map[string]interface{}
	ir.MustJSON(t, &result)
	if result["flightRevisionsImported"].(float64) != 2 || result["logbookCertificationsImported"].(float64) != 1 {
		t.Fatalf("import result = %v", result)
	}

	t.Run("strict mode follows the backup with an import event", func(t *testing.T) {
		m := switchModeGet(t, dst)
		if m["strict"] != true {
			t.Fatalf("mode = %v", m)
		}
		events := m["events"].([]interface{})
		if len(events) != 2 || events[0].(map[string]interface{})["trigger"] != "manual" || events[1].(map[string]interface{})["trigger"] != "import" {
			t.Errorf("events = %v", events)
		}
		if m["latestCertifiedThrough"] != pastDate(1) {
			t.Errorf("latestCertifiedThrough = %v", m["latestCertifiedThrough"])
		}
	})

	t.Run("revisions follow the restored flight id", func(t *testing.T) {
		fr := dst.GET("/flights?pageSize=50")
		requireStatus(t, fr, http.StatusOK)
		var page struct {
			Data []map[string]interface{} `json:"data"`
		}
		fr.MustJSON(t, &page)
		if len(page.Data) != 1 {
			t.Fatalf("restored flights = %d", len(page.Data))
		}
		newID := page.Data[0]["id"].(string)
		if newID == kept {
			t.Fatal("flight ids must be regenerated")
		}
		revs := listRevisions(t, dst, newID)
		if len(revs) != 1 || revs[0]["source"] != "user_edit" || revs[0]["isCorrection"] != false {
			t.Fatalf("restored revisions = %v", revs)
		}
		if ch := changeFor(revs[0], "remarks"); ch == nil || ch["newValue"] != "edited" {
			t.Errorf("remarks change = %v", ch)
		}
	})

	t.Run("a deleted flight is restored as a void record under one fresh id", func(t *testing.T) {
		r := dst.GET("/flights/voided")
		requireStatus(t, r, http.StatusOK)
		var voided []map[string]interface{}
		r.MustJSON(t, &voided)
		if len(voided) != 1 || voided[0]["reason"] != "Duplicate" || voided[0]["aircraftReg"] != "D-EGON" {
			t.Fatalf("voided = %v", voided)
		}
		if voided[0]["flightId"] == gone {
			t.Error("void record kept the original flight id")
		}
		revs := listRevisions(t, dst, voided[0]["flightId"].(string))
		if len(revs) != 1 || revs[0]["kind"] != "delete" {
			t.Errorf("revisions of the voided flight = %v", revs)
		}
	})

	t.Run("a later edit continues the restored numbering", func(t *testing.T) {
		fr := dst.GET("/flights?pageSize=50")
		var page struct {
			Data []map[string]interface{} `json:"data"`
		}
		fr.MustJSON(t, &page)
		newID := page.Data[0]["id"].(string)
		edit := newFlightBody(today(), "D-EEXP")
		edit["remarks"] = "after restore"
		requireStatus(t, putFlight(dst, newID, edit), http.StatusOK)
		revs := listRevisions(t, dst, newID)
		if len(revs) != 2 || revs[0]["revisionNo"].(float64) != 2 {
			t.Fatalf("revisions = %v", revs)
		}
	})

	t.Run("a backup with unusable logbook rows is rejected", func(t *testing.T) {
		bad := map[string]interface{}{}
		for k, v := range payload {
			bad[k] = v
		}
		bad["flightRevisions"] = []map[string]interface{}{
			{"flightId": gone, "revisionNo": 1, "kind": "rewrite", "snapshot": map[string]interface{}{}, "crewSnapshot": []interface{}{}, "source": "user_edit", "changedAt": "2026-01-01T00:00:00Z"},
		}
		fresh := NewE2EClient(t)
		registerAndLogin(t, fresh, uniqueEmail("lb-bad"), "SecurePass123!", "Bad")
		assertStatus(t, fresh.POST("/users/me/data", bad), http.StatusBadRequest)
	})
}

func TestReplaceRestoreKeepsLogbookHistory(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("lb-replace"), "SecurePass123!", "Replacer")
	switchStrict(t, c, true)

	kept := createFlightID(t, c, newFlightBody(today(), "D-EREP"))
	edit := newFlightBody(today(), "D-EREP")
	edit["remarks"] = "edited before the backup"
	requireStatus(t, putFlight(c, kept, edit), http.StatusOK)
	requireStatus(t, c.POST("/logbook/certifications", map[string]interface{}{"certifiedThrough": pastDate(1)}), http.StatusCreated)

	export := c.GET("/users/me/data")
	requireStatus(t, export, http.StatusOK)
	var payload map[string]interface{}
	export.MustJSON(t, &payload)

	later := createFlightID(t, c, newFlightBody(pastDate(3), "D-ELAT"))

	rr := c.POST("/users/me/data?mode=replace", payload)
	requireStatus(t, rr, http.StatusOK)
	var result map[string]interface{}
	rr.MustJSON(t, &result)
	if result["flightsImported"].(float64) != 1 || result["flightRevisionsImported"].(float64) != 1 {
		t.Fatalf("restore result = %v", result)
	}

	t.Run("every wiped flight gets a void record naming the restore", func(t *testing.T) {
		r := c.GET("/flights/voided")
		requireStatus(t, r, http.StatusOK)
		var voided []map[string]interface{}
		r.MustJSON(t, &voided)
		byID := map[string]map[string]interface{}{}
		for _, v := range voided {
			byID[v["flightId"].(string)] = v
		}
		for _, id := range []string{kept, later} {
			v := byID[id]
			if v == nil {
				t.Fatalf("no void record for %s in %v", id, voided)
			}
			if v["source"] != "import" || v["reason"] != "Replaced by backup restore" {
				t.Errorf("void record = %v", v)
			}
		}
	})

	t.Run("the wiped flight keeps its earlier revisions", func(t *testing.T) {
		revs := listRevisions(t, c, kept)
		if len(revs) != 2 || revs[0]["kind"] != "delete" || revs[1]["kind"] != "update" {
			t.Fatalf("revisions of the wiped flight = %v", revs)
		}
	})

	t.Run("the restored flight carries the backup's revision", func(t *testing.T) {
		fr := c.GET("/flights?pageSize=50")
		requireStatus(t, fr, http.StatusOK)
		var page struct {
			Data []map[string]interface{} `json:"data"`
		}
		fr.MustJSON(t, &page)
		if len(page.Data) != 1 || page.Data[0]["id"] == kept {
			t.Fatalf("flights after replace = %v", page.Data)
		}
		revs := listRevisions(t, c, page.Data[0]["id"].(string))
		if len(revs) != 1 || revs[0]["source"] != "user_edit" {
			t.Fatalf("restored revisions = %v", revs)
		}
	})

	t.Run("mode switches and certifications are kept, not duplicated", func(t *testing.T) {
		m := switchModeGet(t, c)
		if m["strict"] != true || len(m["events"].([]interface{})) != 1 {
			t.Errorf("mode = %v", m)
		}
		r := c.GET("/logbook/certifications")
		requireStatus(t, r, http.StatusOK)
		var certs []interface{}
		r.MustJSON(t, &certs)
		if len(certs) != 1 {
			t.Errorf("certifications = %v", certs)
		}
	})
}

func TestLogbookAdminStats(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("lb-stats"), "SecurePass123!", "Stats")
	switchStrict(t, c, true)
	id := createFlightID(t, c, newFlightBody(today(), "D-ESTA"))
	body := newFlightBody(today(), "D-ESTA")
	body["remarks"] = "x"
	requireStatus(t, putFlight(c, id, body), http.StatusOK)
	requireStatus(t, c.DELETE("/flights/"+id), http.StatusNoContent)
	requireStatus(t, c.POST("/logbook/certifications", map[string]interface{}{"certifiedThrough": pastDate(1)}), http.StatusCreated)

	ac := getAdminClient(t)
	r := ac.GET("/admin/stats")
	requireStatus(t, r, http.StatusOK)
	var s map[string]interface{}
	r.MustJSON(t, &s)
	for _, key := range []string{"strictLogbookUsers", "flightRevisions", "voidedFlights", "logbookCertifications"} {
		v, ok := s[key].(float64)
		if !ok || v < 1 {
			t.Errorf("%s = %v, want at least 1", key, s[key])
		}
	}
	if _, ok := s["flightCorrections"].(float64); !ok {
		t.Errorf("flightCorrections = %v, want a number", s["flightCorrections"])
	}
}
