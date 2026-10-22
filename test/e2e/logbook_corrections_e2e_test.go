//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

const e2eSignatureImage = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYGD4DwABBAEAX+XDSwAAAABJRU5ErkJggg=="

func TestFlightCorrections(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("corr"), "SecurePass123!", "Corrections")
	switchStrict(t, c, true)

	t.Run("an edit after the flight day needs a reason", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(pastDate(5), "D-ECOR"))
		body := newFlightBody(pastDate(5), "D-ECOR")
		body["remarks"] = "late edit"

		r := putFlight(c, id, body)
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "correction_reason_required" {
			t.Errorf("code = %q", code)
		}
		if revs := listRevisions(t, c, id); len(revs) != 0 {
			t.Fatalf("a rejected edit recorded %d revisions", len(revs))
		}
		got := c.GET("/flights/" + id)
		var f map[string]interface{}
		got.MustJSON(t, &f)
		if f["remarks"] != nil {
			t.Errorf("a rejected edit changed the flight: remarks = %v", f["remarks"])
		}

		body["correctionReason"] = "   "
		requireStatus(t, putFlight(c, id, body), http.StatusUnprocessableEntity)

		body["correctionReason"] = "Added the missing remark"
		requireStatus(t, putFlight(c, id, body), http.StatusOK)

		revs := listRevisions(t, c, id)
		if len(revs) != 1 {
			t.Fatalf("revisions = %d, want 1", len(revs))
		}
		if revs[0]["isCorrection"] != true || revs[0]["reason"] != "Added the missing remark" || revs[0]["source"] != "user_edit" {
			t.Errorf("revision = %v", revs[0])
		}
	})

	t.Run("a correction reason that is too long is rejected", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(pastDate(5), "D-ECOR"))
		body := newFlightBody(pastDate(5), "D-ECOR")
		body["remarks"] = "x"
		body["correctionReason"] = repeat("r", 501)
		assertStatus(t, putFlight(c, id, body), http.StatusBadRequest)
	})

	t.Run("a flight inside a certified range is a correction", func(t *testing.T) {
		cc := NewE2EClient(t)
		registerAndLogin(t, cc, uniqueEmail("corr-cert"), "SecurePass123!", "Certified")
		switchStrict(t, cc, true)
		id := createFlightID(t, cc, newFlightBody(today(), "D-ECER"))
		requireStatus(t, cc.POST("/logbook/certifications", map[string]interface{}{"certifiedThrough": today()}), http.StatusCreated)

		body := newFlightBody(today(), "D-ECER")
		body["remarks"] = "after certification"
		r := putFlight(cc, id, body)
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "correction_reason_required" {
			t.Errorf("code = %q", code)
		}
		body["correctionReason"] = "Pilot amendment"
		requireStatus(t, putFlight(cc, id, body), http.StatusOK)
	})

	t.Run("a voided signature makes the edit a correction that still needs its own reason", func(t *testing.T) {
		cs := NewE2EClient(t)
		registerAndLogin(t, cs, uniqueEmail("corr-sig"), "SecurePass123!", "Signed")
		switchStrict(t, cs, true)
		id := createFlightID(t, cs, newFlightBody(today(), "D-ESIG"))

		sr := cs.POST(fmt.Sprintf("/flights/%s/signatures/live", id), map[string]interface{}{
			"signerName": "Jane Instructor", "signatureImage": e2eSignatureImage,
		})
		requireStatus(t, sr, http.StatusCreated)
		var sig map[string]interface{}
		sr.MustJSON(t, &sig)
		sigID := sig["id"].(string)
		if h, _ := sig["fieldsHash"].(string); len(h) != 64 {
			t.Errorf("fieldsHash = %v, want 64 hex characters", sig["fieldsHash"])
		}

		gr := cs.GET(fmt.Sprintf("/flights/%s/signatures/%s", id, sigID))
		requireStatus(t, gr, http.StatusOK)
		var got map[string]interface{}
		gr.MustJSON(t, &got)
		if got["fieldsHash"] != sig["fieldsHash"] || got["fieldsHashValid"] != true {
			t.Errorf("signature = %v", got)
		}

		requireStatus(t, cs.POST(fmt.Sprintf("/flights/%s/signatures/%s/void", id, sigID), map[string]interface{}{
			"reason": "Wrong landing count",
		}), http.StatusOK)

		body := newFlightBody(today(), "D-ESIG")
		body["landings"] = 2
		r := putFlight(cs, id, body)
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "correction_reason_required" {
			t.Errorf("code = %q", code)
		}
		body["correctionReason"] = "Second landing was missing"
		requireStatus(t, putFlight(cs, id, body), http.StatusOK)

		revs := listRevisions(t, cs, id)
		if len(revs) != 1 || revs[0]["isCorrection"] != true || revs[0]["reason"] != "Second landing was missing" {
			t.Fatalf("revisions = %v", revs)
		}

		lr := cs.GET(fmt.Sprintf("/flights/%s/signatures", id))
		requireStatus(t, lr, http.StatusOK)
		var sigs []map[string]interface{}
		lr.MustJSON(t, &sigs)
		if len(sigs) != 1 || sigs[0]["fieldsHashValid"] != false {
			t.Errorf("signatures after the edit = %v, want fieldsHashValid false", sigs)
		}
	})

	t.Run("a locked flight is still 409", func(t *testing.T) {
		cs := NewE2EClient(t)
		registerAndLogin(t, cs, uniqueEmail("corr-lock"), "SecurePass123!", "Locked")
		switchStrict(t, cs, true)
		id := createFlightID(t, cs, newFlightBody(today(), "D-ELCK"))
		requireStatus(t, cs.POST(fmt.Sprintf("/flights/%s/signatures/live", id), map[string]interface{}{
			"signerName": "Jane", "signatureImage": e2eSignatureImage,
		}), http.StatusCreated)
		assertStatus(t, putFlight(cs, id, newFlightBody(today(), "D-ELCK")), http.StatusConflict)
		assertStatus(t, cs.DELETE("/flights/"+id), http.StatusConflict)
	})

	t.Run("raising an override on the flight day is not a correction", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(today(), "D-EOVR"))
		body := newFlightBody(today(), "D-EOVR")
		body["nightTime"] = 15
		requireStatus(t, putFlight(c, id, body), http.StatusOK)
		if revs := listRevisions(t, c, id); len(revs) != 1 || revs[0]["isCorrection"] != false {
			t.Fatalf("same-day override: revisions = %v", revs)
		}
	})
}

func TestSwissOverridesOnlyDownward(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ch"), "SecurePass123!", "Swiss")
	requireStatus(t, c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "CH.1",
		"issueDate": "2020-01-01", "issuingAuthority": "FOCA", "issuingCountry": "CH",
	}), http.StatusCreated)
	if m := switchModeGet(t, c); m["strict"] != true || m["overridesDownwardOnly"] != true {
		t.Fatalf("mode = %v", m)
	}

	t.Run("create rejects an override above the calculated value", func(t *testing.T) {
		body := newFlightBody(today(), "D-ECHA")
		body["nightTime"] = 45
		r := c.POST("/flights", body)
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "override_exceeds_calculated" {
			t.Errorf("code = %q", code)
		}
		var e struct {
			Details []struct {
				Field string `json:"field"`
			} `json:"details"`
		}
		_ = json.Unmarshal(r.Body, &e)
		if len(e.Details) != 1 || e.Details[0].Field != "nightTime" {
			t.Errorf("details = %s", r.Body)
		}
	})

	t.Run("create accepts a reducing override", func(t *testing.T) {
		body := newFlightBody(today(), "D-ECHA")
		body["nightTime"] = 0
		requireStatus(t, c.POST("/flights", body), http.StatusCreated)
	})

	t.Run("update rejects a raised override and names every field", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(today(), "D-ECHB"))
		body := newFlightBody(today(), "D-ECHB")
		body["nightTime"] = 30
		body["takeoffsDay"] = 5
		r := putFlight(c, id, body)
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "override_exceeds_calculated" {
			t.Errorf("code = %q", code)
		}
		var e struct {
			Details []struct {
				Field string `json:"field"`
			} `json:"details"`
		}
		_ = json.Unmarshal(r.Body, &e)
		fields := map[string]bool{}
		for _, d := range e.Details {
			fields[d.Field] = true
		}
		if !fields["nightTime"] || !fields["takeoffsDay"] {
			t.Errorf("details = %s", r.Body)
		}
	})

	t.Run("a non-Swiss strict user may raise overrides", func(t *testing.T) {
		de := NewE2EClient(t)
		registerAndLogin(t, de, uniqueEmail("de-override"), "SecurePass123!", "German")
		switchStrict(t, de, true)
		body := newFlightBody(today(), "D-EDEO")
		body["nightTime"] = 45
		requireStatus(t, de.POST("/flights", body), http.StatusCreated)
	})
}

func TestCorrectionsAreClassifiedOnDelete(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("corr-del"), "SecurePass123!", "DeleteCorrection")
	switchStrict(t, c, true)

	late := createFlightID(t, c, newFlightBody(pastDate(5), "D-EDLT"))
	same := createFlightID(t, c, newFlightBody(today(), "D-EDST"))
	requireStatus(t, c.DELETE("/flights/"+late), http.StatusNoContent)
	requireStatus(t, c.DELETE("/flights/"+same), http.StatusNoContent)

	r := c.GET("/flights/voided")
	requireStatus(t, r, http.StatusOK)
	var voided []map[string]interface{}
	r.MustJSON(t, &voided)
	if len(voided) != 2 {
		t.Fatalf("voided = %d, want 2", len(voided))
	}
	for _, v := range voided {
		want := v["flightId"] == late
		if v["isCorrection"] != want {
			t.Errorf("flight %v: isCorrection = %v, want %v", v["aircraftReg"], v["isCorrection"], want)
		}
	}
}

func TestCorrectionsSurviveExportAndImport(t *testing.T) {
	src := NewE2EClient(t)
	registerAndLogin(t, src, uniqueEmail("corr-export"), "SecurePass123!", "Exporter")
	switchStrict(t, src, true)

	id := createFlightID(t, src, newFlightBody(pastDate(4), "D-ECEX"))
	body := newFlightBody(pastDate(4), "D-ECEX")
	body["remarks"] = "corrected"
	body["correctionReason"] = "Fixed remark"
	requireStatus(t, putFlight(src, id, body), http.StatusOK)

	export := src.GET("/users/me/data")
	requireStatus(t, export, http.StatusOK)
	var payload map[string]interface{}
	export.MustJSON(t, &payload)

	dst := NewE2EClient(t)
	registerAndLogin(t, dst, uniqueEmail("corr-import"), "SecurePass123!", "Importer")
	requireStatus(t, dst.POST("/users/me/data", payload), http.StatusOK)

	fr := dst.GET("/flights?pageSize=50")
	requireStatus(t, fr, http.StatusOK)
	var page struct {
		Data []map[string]interface{} `json:"data"`
	}
	fr.MustJSON(t, &page)
	if len(page.Data) != 1 {
		t.Fatalf("restored flights = %d", len(page.Data))
	}
	revs := listRevisions(t, dst, page.Data[0]["id"].(string))
	if len(revs) != 1 || revs[0]["reason"] != "Fixed remark" || revs[0]["isCorrection"] != true {
		t.Fatalf("restored revisions = %v", revs)
	}

	t.Run("a restored flight past its draft window still needs a reason", func(t *testing.T) {
		edit := newFlightBody(pastDate(4), "D-ECEX")
		edit["remarks"] = "again"
		r := putFlight(dst, page.Data[0]["id"].(string), edit)
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "correction_reason_required" {
			t.Errorf("code = %q", code)
		}
	})
}

func TestCorrectionCountsInAdminStats(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("corr-stats"), "SecurePass123!", "Stats")
	switchStrict(t, c, true)
	id := createFlightID(t, c, newFlightBody(pastDate(3), "D-ECST"))
	body := newFlightBody(pastDate(3), "D-ECST")
	body["remarks"] = "x"
	body["correctionReason"] = "stats"
	requireStatus(t, putFlight(c, id, body), http.StatusOK)

	ac := getAdminClient(t)
	r := ac.GET("/admin/stats")
	requireStatus(t, r, http.StatusOK)
	var s map[string]interface{}
	r.MustJSON(t, &s)
	if v, ok := s["flightCorrections"].(float64); !ok || v < 1 {
		t.Errorf("flightCorrections = %v, want at least 1", s["flightCorrections"])
	}
}
