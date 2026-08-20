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

	// Deleting a contact is always allowed; the crew row keeps the name it
	// was logged with.
	t.Run("delete contact used as crew", func(t *testing.T) {
		requireStatus(t, c.DELETE(fmt.Sprintf("/contacts/%s", contactID)), 204)
	})

	t.Run("flight crew still has name after contact deletion", func(t *testing.T) {
		r := c.GET(fmt.Sprintf("/flights/%s", fltID))
		requireStatus(t, r, 200)
		var f map[string]interface{}
		r.JSON(&f)

		crew, ok := f["crewMembers"].([]interface{})
		if !ok || len(crew) == 0 {
			t.Fatalf("crew members disappeared with the contact: %v", f["crewMembers"])
		}
		member := crew[0].(map[string]interface{})
		assertStr(t, "crew name preserved", member["name"], "Instructor Linked")
		if member["contactId"] != nil {
			t.Errorf("contactId = %v, want null after the contact was deleted", member["contactId"])
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
