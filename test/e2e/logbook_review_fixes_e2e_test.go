//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// signingToken requests a signature for the flight and returns the raw token
// of its signing link.
func signingToken(t *testing.T, c *E2EClient, flightID string) string {
	t.Helper()
	r := c.POST(fmt.Sprintf("/flights/%s/signatures", flightID), map[string]interface{}{})
	requireStatus(t, r, http.StatusCreated)
	var created struct {
		SignURL string `json:"signUrl"`
	}
	r.MustJSON(t, &created)
	u, err := url.Parse(created.SignURL)
	if err != nil || u.Query().Get("token") == "" {
		t.Fatalf("signUrl = %q (%v)", created.SignURL, err)
	}
	return u.Query().Get("token")
}

func TestPublicSigningRefusesAFlightChangedSinceItWasShown(t *testing.T) {
	owner := NewE2EClient(t)
	registerAndLogin(t, owner, uniqueEmail("sign-hash"), "SecurePass123!", "Signer")
	public := NewE2EClient(t)

	complete := func(token string, hash interface{}) *Response {
		body := map[string]interface{}{"signerName": "Jane Instructor", "signatureImage": e2eSignatureImage}
		if hash != nil {
			body["fieldsHash"] = hash
		}
		return public.POST("/sign/"+token, body)
	}
	shownHash := func(token string) string {
		r := public.GET("/sign/" + token)
		requireStatus(t, r, http.StatusOK)
		var info struct {
			FieldsHash string `json:"fieldsHash"`
		}
		r.MustJSON(t, &info)
		if info.FieldsHash == "" {
			t.Fatal("the signing page carries no fieldsHash")
		}
		return info.FieldsHash
	}

	t.Run("an edit between the page and the signature is a 409 and nothing is recorded", func(t *testing.T) {
		id := createFlightID(t, owner, newFlightBody(today(), "D-EHSH"))
		token := signingToken(t, owner, id)
		shown := shownHash(token)

		body := newFlightBody(today(), "D-EHSH")
		body["remarks"] = "edited after the page was opened"
		requireStatus(t, putFlight(owner, id, body), http.StatusOK)

		r := complete(token, shown)
		requireStatus(t, r, http.StatusConflict)
		if code := errorCode(t, r); code != "flight_changed" {
			t.Fatalf("code = %q, want flight_changed", code)
		}
		fr := owner.GET("/flights/" + id)
		requireStatus(t, fr, http.StatusOK)
		var flight map[string]interface{}
		fr.MustJSON(t, &flight)
		if flight["signatureId"] != nil {
			t.Errorf("flight locked by a refused signature: %v", flight["signatureId"])
		}

		refreshed := shownHash(token)
		if refreshed == shown {
			t.Fatal("the hash did not change with the edit")
		}
		requireStatus(t, complete(token, refreshed), http.StatusOK)
	})

	t.Run("a matching hash completes", func(t *testing.T) {
		id := createFlightID(t, owner, newFlightBody(today(), "D-EHSM"))
		token := signingToken(t, owner, id)
		requireStatus(t, complete(token, shownHash(token)), http.StatusOK)
	})

	t.Run("an absent hash completes whatever changed", func(t *testing.T) {
		id := createFlightID(t, owner, newFlightBody(today(), "D-EHSA"))
		token := signingToken(t, owner, id)
		shownHash(token)
		body := newFlightBody(today(), "D-EHSA")
		body["remarks"] = "edited"
		requireStatus(t, putFlight(owner, id, body), http.StatusOK)
		requireStatus(t, complete(token, nil), http.StatusOK)
	})
}

func TestBulkDeleteFlagsLateAndSignedVoidRecords(t *testing.T) {
	voidedByReg := func(t *testing.T, c *E2EClient) map[string]bool {
		t.Helper()
		r := c.GET("/flights/voided")
		requireStatus(t, r, http.StatusOK)
		var voided []map[string]interface{}
		r.MustJSON(t, &voided)
		out := map[string]bool{}
		for _, v := range voided {
			out[v["aircraftReg"].(string)] = v["isCorrection"].(bool)
		}
		return out
	}

	setup := func(t *testing.T) *E2EClient {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("bulk-del"), "SecurePass123!", "Bulk")
		switchStrict(t, c, true)
		createFlightID(t, c, newFlightBody(today(), "D-EFRS"))
		createFlightID(t, c, newFlightBody(pastDate(5), "D-ELAT"))
		signedID := createFlightID(t, c, newFlightBody(today(), "D-ESGN"))
		requireStatus(t, c.POST(fmt.Sprintf("/flights/%s/signatures/live", signedID), map[string]interface{}{
			"signerName": "Jane", "signatureImage": e2eSignatureImage,
		}), http.StatusCreated)
		return c
	}
	want := map[string]bool{"D-EFRS": false, "D-ELAT": true, "D-ESGN": true}

	for _, path := range []string{"/users/me/data?scope=flights", "/flights/delete-all", "/users/me/data"} {
		t.Run("DELETE "+path, func(t *testing.T) {
			c := setup(t)
			requireStatus(t, c.DELETE(path), http.StatusOK)

			got := voidedByReg(t, c)
			if len(got) != len(want) {
				t.Fatalf("voided = %v, want %v", got, want)
			}
			for reg, correction := range want {
				if g, ok := got[reg]; !ok || g != correction {
					t.Errorf("%s: isCorrection = %v (present %v), want %v", reg, g, ok, correction)
				}
			}
		})
	}
}

func TestLateStrictSaveThatChangesNothingNeedsNoReason(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("late-noop"), "SecurePass123!", "NoOp")
	switchStrict(t, c, true)

	body := newFlightBody(pastDate(5), "D-ENOP")
	body["offBlockTime"], body["onBlockTime"] = "14:15", "15:15"
	body["crewMembers"] = []map[string]interface{}{
		{"name": "Pat Instructor", "role": "Instructor"},
		{"name": "Sam Student", "role": "Student"},
	}
	id := createFlightID(t, c, body)

	respelled := newFlightBody(pastDate(5), "D-ENOP")
	respelled["offBlockTime"], respelled["onBlockTime"] = "14:15:00", "15:15:00"
	respelled["crewMembers"] = []map[string]interface{}{
		{"name": "Sam Student", "role": "Student"},
		{"name": "Pat Instructor", "role": "Instructor"},
	}
	requireStatus(t, putFlight(c, id, respelled), http.StatusOK)
	if revs := listRevisions(t, c, id); len(revs) != 0 {
		t.Fatalf("revisions after a save that changed nothing = %v, want none", revs)
	}

	respelled["remarks"] = "now it changes"
	r := putFlight(c, id, respelled)
	requireStatus(t, r, http.StatusUnprocessableEntity)
	if code := errorCode(t, r); code != "correction_reason_required" {
		t.Fatalf("code = %q", code)
	}
	fr := c.GET("/flights/" + id)
	requireStatus(t, fr, http.StatusOK)
	var flight map[string]interface{}
	fr.MustJSON(t, &flight)
	if flight["remarks"] != nil {
		t.Errorf("remarks = %v after a refused update", flight["remarks"])
	}
	if revs := listRevisions(t, c, id); len(revs) != 0 {
		t.Fatalf("revisions after a refused update = %v, want none", revs)
	}
}
