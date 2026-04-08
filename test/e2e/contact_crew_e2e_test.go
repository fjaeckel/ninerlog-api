//go:build e2e

package e2e_test

import (
	"fmt"
	"testing"
)

func TestContactDeletionWithCrewReferences(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-crew"), "SecurePass123!", "ContactCrew")

	// Create contact
	r := c.POST("/contacts", map[string]interface{}{"name": "Instructor Linked"})
	requireStatus(t, r, 201)
	var cnt map[string]interface{}
	r.JSON(&cnt)
	contactID := cnt["id"].(string)

	// Create flight referencing this contact as crew
	r = c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-ECNT", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		"crewMembers": []map[string]interface{}{
			{"name": "Instructor Linked", "role": "Instructor", "contactId": contactID},
		},
	})
	requireStatus(t, r, 201)
	var flt map[string]interface{}
	r.JSON(&flt)
	fltID := flt["id"].(string)

	t.Run("delete contact used as crew", func(t *testing.T) {
		r := c.DELETE(fmt.Sprintf("/contacts/%s", contactID))
		// Should succeed (FK is SET NULL) or fail (409)
		if r.StatusCode == 204 || r.StatusCode == 200 {
			t.Log("Contact deleted despite being crew — FK SET NULL")
		} else if r.StatusCode == 409 {
			t.Log("Contact protected — 409 Conflict returned")
		} else {
			t.Errorf("Unexpected status %d: %s", r.StatusCode, string(r.Body))
		}
	})

	t.Run("flight crew still has name after contact deletion", func(t *testing.T) {
		r := c.GET(fmt.Sprintf("/flights/%s", fltID))
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)

		if crew, ok := f["crewMembers"].([]interface{}); ok && len(crew) > 0 {
			member := crew[0].(map[string]interface{})
			assertStr(t, "crew name preserved", member["name"], "Instructor Linked")
			// contactId should be null now if FK was SET NULL
			t.Logf("contactId after deletion: %v", member["contactId"])
		} else {
			t.Log("No crew members in response after contact deletion")
		}
	})
}

func TestContactSpecialCharSearch(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cnt-special"), "SecurePass123!", "SpecialCnt")

	// Create contacts with special characters
	names := []string{
		"O'Brien",
		"Müller-Schmidt",
		"Dr. Hans (Captain)",
		"José García",
	}
	for _, n := range names {
		requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": n}), 201)
	}

	t.Run("search apostrophe", func(t *testing.T) {
		r := c.GET("/contacts/search?q=O'Brien")
		requireStatus(t, r, 200)
		var cts []interface{}
		r.JSON(&cts)
		if len(cts) < 1 {
			t.Error("Expected to find O'Brien")
		}
	})

	t.Run("search umlaut", func(t *testing.T) {
		r := c.GET("/contacts/search?q=Müller")
		requireStatus(t, r, 200)
		var cts []interface{}
		r.JSON(&cts)
		if len(cts) < 1 {
			t.Error("Expected to find Müller-Schmidt")
		}
	})

	t.Run("search accent", func(t *testing.T) {
		r := c.GET("/contacts/search?q=García")
		requireStatus(t, r, 200)
		var cts []interface{}
		r.JSON(&cts)
		if len(cts) < 1 {
			t.Error("Expected to find García")
		}
	})
}
