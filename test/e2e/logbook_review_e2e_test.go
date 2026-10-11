//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
	"time"
)

func TestCreatingAFlightWithCrewRecordsNoRevision(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("lb-crew"), "SecurePass123!", "Crew")
	switchStrict(t, c, true)

	body := newFlightBody(today(), "D-ECRW")
	body["crewMembers"] = []map[string]interface{}{{"name": "Pat Instructor", "role": "Instructor"}}
	id := createFlightID(t, c, body)

	if revs := listRevisions(t, c, id); len(revs) != 0 {
		t.Fatalf("revisions after create = %v, want none", revs)
	}

	body["crewMembers"] = []map[string]interface{}{{"name": "Sam Instructor", "role": "Instructor"}}
	requireStatus(t, putFlight(c, id, body), http.StatusOK)
	revs := listRevisions(t, c, id)
	if len(revs) != 1 {
		t.Fatalf("revisions after a crew change = %d, want 1", len(revs))
	}
	if ch := changeFor(revs[0], "crewMembers"); ch == nil {
		t.Errorf("crewMembers change missing: %v", revs[0])
	}
}

func TestRejectedFlightUpdateCreatesNoContacts(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("lb-nocontact"), "SecurePass123!", "NoContact")
	switchStrict(t, c, true)

	id := createFlightID(t, c, newFlightBody(pastDate(5), "D-ENOC"))
	body := newFlightBody(pastDate(5), "D-ENOC")
	body["remarks"] = "late"
	body["crewMembers"] = []map[string]interface{}{{"name": "Never Created", "role": "Instructor"}}

	r := putFlight(c, id, body)
	requireStatus(t, r, http.StatusUnprocessableEntity)
	if code := errorCode(t, r); code != "correction_reason_required" {
		t.Fatalf("code = %q", code)
	}

	cr := c.GET("/contacts/search?q=Never")
	requireStatus(t, cr, http.StatusOK)
	var found []map[string]interface{}
	cr.MustJSON(t, &found)
	if len(found) != 0 {
		t.Fatalf("contacts after a rejected update = %v, want none", found)
	}

	body["correctionReason"] = "late crew entry"
	requireStatus(t, putFlight(c, id, body), http.StatusOK)
	cr = c.GET("/contacts/search?q=Never")
	cr.MustJSON(t, &found)
	if len(found) != 1 {
		t.Fatalf("contacts after an accepted update = %v, want 1", found)
	}
}

func TestContactRenameIsRecordedPerFlight(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("lb-rename"), "SecurePass123!", "Renamer")
	switchStrict(t, c, true)

	crew := []map[string]interface{}{{"name": "Hnas Müller", "role": "Instructor"}}
	fresh := newFlightBody(today(), "D-EFRE")
	fresh["crewMembers"] = crew
	freshID := createFlightID(t, c, fresh)
	late := newFlightBody(pastDate(5), "D-ELAT")
	late["crewMembers"] = crew
	lateID := createFlightID(t, c, late)

	contactID, _ := flightCrew(t, c, freshID)[0]["contactId"].(string)
	if contactID == "" {
		t.Fatal("crew member was not linked to a contact")
	}
	requireStatus(t, c.PUT("/contacts/"+contactID, map[string]interface{}{"name": "Hans Müller"}), http.StatusOK)

	const wantReason = "Crew contact renamed: Hnas Müller → Hans Müller"
	for id, wantCorrection := range map[string]bool{freshID: false, lateID: true} {
		revs := listRevisions(t, c, id)
		if len(revs) != 1 {
			t.Fatalf("revisions of %s = %d, want 1", id, len(revs))
		}
		if revs[0]["source"] != "user_edit" || revs[0]["isCorrection"] != wantCorrection || revs[0]["reason"] != wantReason {
			t.Errorf("revision of %s = %v", id, revs[0])
		}
	}
}

func TestRestoreRejectsUnreadableRevisions(t *testing.T) {
	src := NewE2EClient(t)
	registerAndLogin(t, src, uniqueEmail("lb-unreadable"), "SecurePass123!", "Source")
	switchStrict(t, src, true)
	createFlightID(t, src, newFlightBody(today(), "D-EUNR"))

	export := src.GET("/users/me/data")
	requireStatus(t, export, http.StatusOK)
	var payload map[string]interface{}
	export.MustJSON(t, &payload)

	revision := func(snapshot interface{}) []map[string]interface{} {
		return []map[string]interface{}{{
			"flightId": "11111111-1111-1111-1111-111111111111", "revisionNo": 1, "kind": "update",
			"snapshot": snapshot, "crewSnapshot": []interface{}{}, "source": "user_edit", "changedAt": "2026-01-01T00:00:00Z",
		}}
	}
	for name, snapshot := range map[string]interface{}{
		"null snapshot":    nil,
		"array snapshot":   []interface{}{},
		"no date":          map[string]interface{}{"id": "11111111-1111-1111-1111-111111111111"},
		"no id":            map[string]interface{}{"date": "2026-01-01"},
		"bad total":        map[string]interface{}{"id": "11111111-1111-1111-1111-111111111111", "date": "2026-01-01", "total_time": "x"},
		"datetime in date": map[string]interface{}{"id": "11111111-1111-1111-1111-111111111111", "date": "2026-01-01T00:00:00Z"},
	} {
		t.Run(name, func(t *testing.T) {
			bad := map[string]interface{}{}
			for k, v := range payload {
				bad[k] = v
			}
			bad["flightRevisions"] = revision(snapshot)
			dst := NewE2EClient(t)
			registerAndLogin(t, dst, uniqueEmail("lb-unreadable-dst"), "SecurePass123!", "Dest")
			assertStatus(t, dst.POST("/users/me/data", bad), http.StatusBadRequest)

			fr := dst.GET("/flights")
			requireStatus(t, fr, http.StatusOK)
			var page struct {
				Data []map[string]interface{} `json:"data"`
			}
			fr.MustJSON(t, &page)
			if len(page.Data) != 0 {
				t.Errorf("a rejected restore left %d flights", len(page.Data))
			}
		})
	}
}

func TestRestoreClampsFlightCreatedAt(t *testing.T) {
	src := NewE2EClient(t)
	registerAndLogin(t, src, uniqueEmail("lb-created"), "SecurePass123!", "Source")
	createFlightID(t, src, newFlightBody(today(), "D-ECR1"))
	createFlightID(t, src, newFlightBody(today(), "D-ECR2"))

	export := src.GET("/users/me/data")
	requireStatus(t, export, http.StatusOK)
	var payload map[string]interface{}
	export.MustJSON(t, &payload)

	past := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Second)
	flights := payload["flights"].([]interface{})
	flights[0].(map[string]interface{})["createdAt"] = "2999-01-01T00:00:00Z"
	flights[1].(map[string]interface{})["createdAt"] = past.Format(time.RFC3339)
	wantPast := map[string]bool{}
	wantPast[flights[1].(map[string]interface{})["aircraftReg"].(string)] = true

	dst := NewE2EClient(t)
	registerAndLogin(t, dst, uniqueEmail("lb-created-dst"), "SecurePass123!", "Dest")
	requireStatus(t, dst.POST("/users/me/data", payload), http.StatusOK)

	fr := dst.GET("/flights?pageSize=50")
	requireStatus(t, fr, http.StatusOK)
	var page struct {
		Data []map[string]interface{} `json:"data"`
	}
	fr.MustJSON(t, &page)
	if len(page.Data) != 2 {
		t.Fatalf("restored flights = %d", len(page.Data))
	}
	for _, f := range page.Data {
		created, err := time.Parse(time.RFC3339Nano, f["createdAt"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if wantPast[f["aircraftReg"].(string)] {
			if !created.Equal(past) {
				t.Errorf("%v createdAt = %v, want the backup value %v", f["aircraftReg"], created, past)
			}
			continue
		}
		if created.After(time.Now().Add(time.Minute)) {
			t.Errorf("%v createdAt = %v, want clamped to the restore time", f["aircraftReg"], created)
		}
	}
}
