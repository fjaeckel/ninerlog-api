//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func createLicence(t *testing.T, c *E2EClient, authority, licenceType, country string) {
	t.Helper()
	requireStatus(t, c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": authority, "licenseType": licenceType, "licenseNumber": "P-1",
		"issueDate": "2020-01-01", "issuingAuthority": authority, "issuingCountry": country,
	}), http.StatusCreated)
}

// L6: a flight entered after its flight day is a correction under the German rule.
func TestPersonaL6_LateEntryUnderGermanRuleIsCorrection(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("l6"), "SecurePass123!", "Lena")
	createLicence(t, c, "EASA", "SPL", "DE")
	if m := switchModeGet(t, c); m["strict"] != true || m["draftWindow"] != "flight_day" {
		t.Fatalf("mode = %v", m)
	}

	id := createFlightID(t, c, newFlightBody(pastDate(1), "D-1234"))
	edit := newFlightBody(pastDate(1), "D-1234")
	edit["remarks"] = "added the next day"

	r := putFlight(c, id, edit)
	requireStatus(t, r, http.StatusUnprocessableEntity)
	if code := errorCode(t, r); code != "correction_reason_required" {
		t.Fatalf("code = %q", code)
	}

	edit["correctionReason"] = "Entered after the flight day"
	requireStatus(t, putFlight(c, id, edit), http.StatusOK)
	revs := listRevisions(t, c, id)
	if len(revs) != 1 || revs[0]["isCorrection"] != true || revs[0]["reason"] != "Entered after the flight day" {
		t.Fatalf("revisions = %v", revs)
	}
}

// S4: a DULV licence under a German issuing country only suggests strict mode.
func TestPersonaS4_DULVLicenceOnlySuggestsStrictMode(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("s4"), "SecurePass123!", "Sabine")
	createLicence(t, c, "DULV", "Sportpilotenlizenz", "DE")

	m := switchModeGet(t, c)
	if m["strict"] != false || len(m["requiredBy"].([]interface{})) != 0 {
		t.Fatalf("a DULV licence must not require strict mode: %v", m)
	}
	suggested := m["suggestedBy"].([]interface{})
	if len(suggested) != 1 || suggested[0].(map[string]interface{})["basis"] != "§120 LuftPersV" {
		t.Errorf("suggestedBy = %v", suggested)
	}
}

// S5: Austrian hang-glider and paraglider licences require strict mode, a Part-FCL gyroplane licence only suggests it.
func TestPersonaS5_AustrianNationalLicenceClassification(t *testing.T) {
	tests := []struct {
		licenceType string
		wantStrict  bool
	}{
		{"HG", true},
		{"PG", true},
		{"Gleitschirm", true},
		{"GPL", false},
		{"PPL", false},
	}
	for _, tt := range tests {
		t.Run(tt.licenceType, func(t *testing.T) {
			c := NewE2EClient(t)
			registerAndLogin(t, c, uniqueEmail("s5"), "SecurePass123!", "Sabine")
			createLicence(t, c, "Austro Control", tt.licenceType, "AT")

			m := switchModeGet(t, c)
			if m["strict"] != tt.wantStrict {
				t.Fatalf("strict = %v, want %v: %v", m["strict"], tt.wantStrict, m)
			}
			required, suggested := len(m["requiredBy"].([]interface{})), len(m["suggestedBy"].([]interface{}))
			if tt.wantStrict && required != 1 || !tt.wantStrict && suggested != 1 {
				t.Errorf("requiredBy = %d, suggestedBy = %d", required, suggested)
			}
		})
	}
}

// M5: under a Swiss licence an override above the calculated value is rejected on a strip without coordinates.
func TestPersonaM5_SwissOverrideOnStripWithoutCoordinates(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("m5"), "SecurePass123!", "Mehmet")
	createLicence(t, c, "EASA", "PPL", "CH")

	strip := newFlightBody(today(), "D-MXYZ")
	strip["departureIcao"] = "UL-Platz Musterstadt"
	strip["arrivalIcao"] = "UL-Platz Musterstadt"
	strip["nightTime"] = 20

	r := c.POST("/flights", strip)
	requireStatus(t, r, http.StatusUnprocessableEntity)
	if code := errorCode(t, r); code != "override_exceeds_calculated" {
		t.Fatalf("code = %q", code)
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

	strip["nightTime"] = 0
	requireStatus(t, c.POST("/flights", strip), http.StatusCreated)
}

func TestSwissFreeEditWindowFollowsLanding(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ch-window"), "SecurePass123!", "Swiss")
	createLicence(t, c, "EASA", "PPL", "CH")

	t.Run("a flight that landed yesterday is still free to edit", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(pastDate(1), "D-ECW1"))
		edit := newFlightBody(pastDate(1), "D-ECW1")
		edit["remarks"] = "within 48 hours of landing"
		requireStatus(t, putFlight(c, id, edit), http.StatusOK)
		if revs := listRevisions(t, c, id); len(revs) != 1 || revs[0]["isCorrection"] != false {
			t.Fatalf("revisions = %v", revs)
		}
	})

	t.Run("a flight that landed four days ago needs a reason however recently it was entered", func(t *testing.T) {
		id := createFlightID(t, c, newFlightBody(pastDate(4), "D-ECW2"))
		edit := newFlightBody(pastDate(4), "D-ECW2")
		edit["remarks"] = "late"
		r := putFlight(c, id, edit)
		requireStatus(t, r, http.StatusUnprocessableEntity)
		if code := errorCode(t, r); code != "correction_reason_required" {
			t.Errorf("code = %q", code)
		}
	})
}
