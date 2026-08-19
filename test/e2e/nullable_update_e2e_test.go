//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestNullableFieldsClearOnNull covers JSON Merge Patch (RFC 7386) semantics
// on update requests: an explicit `null` clears a nullable field, while
// omitting the field leaves it unchanged.
func TestNullableFieldsClearOnNull(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("nullable-upd"), "SecurePass123!", "NullableUpd")

	t.Run("flight", func(t *testing.T) {
		resp := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-ENUL", "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": "08:45", "landings": 1,
			"remarks": "Original remarks", "instructorName": "Original CFI",
			"launchMethod": "winch",
		})
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		fid := f["id"].(string)
		if f["remarks"] != "Original remarks" || f["launchMethod"] != "winch" {
			t.Fatalf("expected seeded remarks/launchMethod, got %v / %v", f["remarks"], f["launchMethod"])
		}

		// Omitting remarks/launchMethod on an unrelated update must leave them unchanged.
		resp = c.PUT(fmt.Sprintf("/flights/%s", fid), map[string]interface{}{
			"date": today(), "aircraftReg": "D-ENUL", "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": "08:45", "landings": 1,
		})
		requireStatus(t, resp, http.StatusOK)
		// Decode into a fresh map: a cleared field is omitted from the
		// response JSON, not sent as null.
		f = map[string]interface{}{}
		resp.JSON(&f)
		if f["remarks"] != "Original remarks" {
			t.Errorf("omitted remarks should be unchanged, got %v", f["remarks"])
		}
		if f["launchMethod"] != "winch" {
			t.Errorf("omitted launchMethod should be unchanged, got %v", f["launchMethod"])
		}

		// Explicit null clears the fields.
		resp = c.PUT(fmt.Sprintf("/flights/%s", fid), map[string]interface{}{
			"date": today(), "aircraftReg": "D-ENUL", "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": "08:45", "landings": 1,
			"remarks": nil, "launchMethod": nil, "instructorName": nil,
		})
		requireStatus(t, resp, http.StatusOK)
		f = map[string]interface{}{}
		resp.JSON(&f)
		if f["remarks"] != nil {
			t.Errorf("expected remarks cleared to null, got %v", f["remarks"])
		}
		if f["launchMethod"] != nil {
			t.Errorf("expected launchMethod cleared to null, got %v", f["launchMethod"])
		}
		if f["instructorName"] != nil {
			t.Errorf("expected instructorName cleared to null, got %v", f["instructorName"])
		}

		// GET confirms the clear persisted, not just the PUT response.
		resp = c.GET(fmt.Sprintf("/flights/%s", fid))
		requireStatus(t, resp, http.StatusOK)
		f = map[string]interface{}{}
		resp.JSON(&f)
		if f["remarks"] != nil || f["launchMethod"] != nil {
			t.Errorf("expected cleared fields to persist, got remarks=%v launchMethod=%v", f["remarks"], f["launchMethod"])
		}
	})

	t.Run("aircraft", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EACL", "type": "C172", "make": "Cessna", "model": "172S",
			"aircraftClass": "SEP_LAND", "notes": "Club aircraft",
		})
		requireStatus(t, resp, http.StatusCreated)
		var a map[string]interface{}
		resp.JSON(&a)
		acID := a["id"].(string)

		// Omitting notes/aircraftClass on an unrelated update leaves them unchanged.
		resp = c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{"isActive": true})
		requireStatus(t, resp, http.StatusOK)
		// Decode into a fresh map: a cleared field is omitted from the
		// response JSON, not sent as null.
		a = map[string]interface{}{}
		resp.JSON(&a)
		if a["notes"] != "Club aircraft" || a["aircraftClass"] != "SEP_LAND" {
			t.Errorf("omitted fields should be unchanged, got notes=%v aircraftClass=%v", a["notes"], a["aircraftClass"])
		}

		resp = c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{
			"notes": nil, "aircraftClass": nil,
		})
		requireStatus(t, resp, http.StatusOK)
		a = map[string]interface{}{}
		resp.JSON(&a)
		if a["notes"] != nil {
			t.Errorf("expected notes cleared to null, got %v", a["notes"])
		}
		if a["aircraftClass"] != nil {
			t.Errorf("expected aircraftClass cleared to null, got %v", a["aircraftClass"])
		}
	})

	t.Run("credential", func(t *testing.T) {
		resp := c.POST("/credentials", map[string]interface{}{
			"credentialType": "EASA_CLASS2_MEDICAL", "credentialNumber": "MED-NUL",
			"issueDate": today(), "expiryDate": futureDate(365),
			"issuingAuthority": "AME Smith", "notes": "Annual",
		})
		requireStatus(t, resp, http.StatusCreated)
		var cr map[string]interface{}
		resp.JSON(&cr)
		credID := cr["id"].(string)

		// Omitting expiryDate/notes leaves them unchanged.
		resp = c.PATCH(fmt.Sprintf("/credentials/%s", credID), map[string]interface{}{"issuingAuthority": "AME Smith"})
		requireStatus(t, resp, http.StatusOK)
		// Decode into a fresh map: a cleared field is omitted from the
		// response JSON, not sent as null.
		cr = map[string]interface{}{}
		resp.JSON(&cr)
		if cr["expiryDate"] == nil || cr["notes"] != "Annual" {
			t.Errorf("omitted fields should be unchanged, got expiryDate=%v notes=%v", cr["expiryDate"], cr["notes"])
		}

		// Explicit null clears the fields, including a nullable date.
		resp = c.PATCH(fmt.Sprintf("/credentials/%s", credID), map[string]interface{}{
			"expiryDate": nil, "notes": nil, "credentialNumber": nil,
		})
		requireStatus(t, resp, http.StatusOK)
		cr = map[string]interface{}{}
		resp.JSON(&cr)
		if cr["expiryDate"] != nil {
			t.Errorf("expected expiryDate cleared to null, got %v", cr["expiryDate"])
		}
		if cr["notes"] != nil {
			t.Errorf("expected notes cleared to null, got %v", cr["notes"])
		}
		if cr["credentialNumber"] != nil {
			t.Errorf("expected credentialNumber cleared to null, got %v", cr["credentialNumber"])
		}
	})

	t.Run("class rating", func(t *testing.T) {
		resp := c.POST("/licenses", map[string]interface{}{
			"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "NUL-001",
			"issueDate": today(), "issuingAuthority": "LBA",
		})
		requireStatus(t, resp, http.StatusCreated)
		var lic map[string]interface{}
		resp.JSON(&lic)
		lid := lic["id"].(string)

		resp = c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
			"classType": "SEP_LAND", "issueDate": "2023-01-15",
			"expiryDate": futureDate(365), "notes": "Initial checkout",
		})
		requireStatus(t, resp, http.StatusCreated)
		var cr map[string]interface{}
		resp.JSON(&cr)
		rid := cr["id"].(string)

		// Updating one field leaves issueDate/expiryDate unchanged.
		resp = c.PATCH(fmt.Sprintf("/licenses/%s/ratings/%s", lid, rid), map[string]interface{}{
			"notes": "Renewed",
		})
		requireStatus(t, resp, http.StatusOK)
		// Decode into a fresh map: a cleared field is omitted from the
		// response JSON, not sent as null.
		cr = map[string]interface{}{}
		resp.JSON(&cr)
		if !strings.HasPrefix(fmt.Sprint(cr["issueDate"]), "2023-01-15") {
			t.Errorf("omitted issueDate should be unchanged, got %v", cr["issueDate"])
		}
		if cr["expiryDate"] == nil {
			t.Errorf("omitted expiryDate should be unchanged, got nil")
		}
		if cr["notes"] != "Renewed" {
			t.Errorf("expected notes updated, got %v", cr["notes"])
		}

		resp = c.PATCH(fmt.Sprintf("/licenses/%s/ratings/%s", lid, rid), map[string]interface{}{
			"expiryDate": nil, "notes": nil,
		})
		requireStatus(t, resp, http.StatusOK)
		cr = map[string]interface{}{}
		resp.JSON(&cr)
		if cr["expiryDate"] != nil {
			t.Errorf("expected expiryDate cleared to null, got %v", cr["expiryDate"])
		}
		if cr["notes"] != nil {
			t.Errorf("expected notes cleared to null, got %v", cr["notes"])
		}
		if !strings.HasPrefix(fmt.Sprint(cr["issueDate"]), "2023-01-15") {
			t.Errorf("issueDate should survive an unrelated clear, got %v", cr["issueDate"])
		}
	})
}
