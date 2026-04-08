//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestDeleteUserCascade(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("cascade")
	pw := "SecurePass123!"
	registerAndLogin(t, c, email, pw, "Cascade User")

	// Create a rich data set
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "CAS-001",
		"issueDate": today(), "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	licID := lic["id"].(string)

	resp = c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
		"classType": "SEP_LAND", "issueDate": today(),
	})
	requireStatus(t, resp, http.StatusCreated)

	resp = c.POST("/aircraft", map[string]interface{}{
		"registration": "D-ECAS", "type": "C172", "make": "Cessna", "model": "172",
	})
	requireStatus(t, resp, http.StatusCreated)

	resp = c.POST("/contacts", map[string]interface{}{"name": "Cascade Pilot"})
	requireStatus(t, resp, http.StatusCreated)

	resp = c.POST("/credentials", map[string]interface{}{
		"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": today(), "issuingAuthority": "AME",
	})
	requireStatus(t, resp, http.StatusCreated)

	resp = c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-ECAS", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
	})
	requireStatus(t, resp, http.StatusCreated)

	t.Run("delete user cascades all data", func(t *testing.T) {
		resp := c.DELETEWithBody("/users/me", map[string]string{"password": pw})
		assertStatus(t, resp, http.StatusNoContent)

		// Can't login anymore
		resp = c.POST("/auth/login", map[string]string{"email": email, "password": pw})
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

func TestDeleteAllDataKeepsAccount(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("deldata")
	pw := "SecurePass123!"
	registerAndLogin(t, c, email, pw, "Data Wipe User")

	// Create data across all entities
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "WIPE-001",
		"issueDate": today(), "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)

	resp = c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EWIP", "type": "C172", "make": "Cessna", "model": "172",
	})
	requireStatus(t, resp, http.StatusCreated)

	resp = c.POST("/contacts", map[string]interface{}{"name": "Wipe Contact"})
	requireStatus(t, resp, http.StatusCreated)

	resp = c.POST("/credentials", map[string]interface{}{
		"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": today(), "issuingAuthority": "AME",
	})
	requireStatus(t, resp, http.StatusCreated)

	for i := 0; i < 3; i++ {
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": pastDate(i), "aircraftReg": "D-EWIP", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		}), http.StatusCreated)
	}

	t.Run("delete all data", func(t *testing.T) {
		resp := c.DELETE("/users/me/data")
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Fatalf("Expected 200/204, got %d: %s", resp.StatusCode, string(resp.Body))
		}
	})

	t.Run("account still exists", func(t *testing.T) {
		resp := c.GET("/users/me")
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["email"] != email {
			t.Errorf("Expected email %s, got %v", email, u["email"])
		}
	})

	t.Run("flights wiped", func(t *testing.T) {
		resp := c.GET("/flights")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		if len(r["data"].([]interface{})) != 0 {
			t.Error("Expected 0 flights")
		}
	})

	t.Run("licenses wiped", func(t *testing.T) {
		resp := c.GET("/licenses")
		requireStatus(t, resp, http.StatusOK)
		var lics []interface{}
		resp.JSON(&lics)
		if len(lics) != 0 {
			t.Error("Expected 0 licenses")
		}
	})

	t.Run("aircraft wiped", func(t *testing.T) {
		resp := c.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		if len(r["data"].([]interface{})) != 0 {
			t.Error("Expected 0 aircraft")
		}
	})

	t.Run("credentials wiped", func(t *testing.T) {
		resp := c.GET("/credentials")
		requireStatus(t, resp, http.StatusOK)
		var creds []interface{}
		resp.JSON(&creds)
		if len(creds) != 0 {
			t.Error("Expected 0 credentials")
		}
	})

	t.Run("contacts wiped", func(t *testing.T) {
		resp := c.GET("/contacts")
		requireStatus(t, resp, http.StatusOK)
		var contacts []interface{}
		resp.JSON(&contacts)
		if len(contacts) != 0 {
			t.Error("Expected 0 contacts")
		}
	})
}

func TestDeleteLicenseCascadesRatings(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cas-lic"), "SecurePass123!", "LicCas")

	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "LICAS-001",
		"issueDate": today(), "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	lid := lic["id"].(string)

	// Add ratings
	for _, ct := range []string{"SEP_LAND", "TMG", "IR"} {
		requireStatus(t, c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
			"classType": ct, "issueDate": today(),
		}), http.StatusCreated)
	}

	// Verify ratings exist
	resp = c.GET(fmt.Sprintf("/licenses/%s/ratings", lid))
	requireStatus(t, resp, http.StatusOK)
	var ratings []interface{}
	resp.JSON(&ratings)
	if len(ratings) != 3 {
		t.Fatalf("Expected 3 ratings, got %d", len(ratings))
	}

	// Delete license
	assertStatus(t, c.DELETE(fmt.Sprintf("/licenses/%s", lid)), http.StatusNoContent)

	// Ratings should be gone
	resp = c.GET(fmt.Sprintf("/licenses/%s/ratings", lid))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestDeleteAircraftFlightsRetainData(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cas-ac"), "SecurePass123!", "AcCas")

	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EDEL", "type": "C172", "make": "Cessna", "model": "172",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)
	acID := ac["id"].(string)

	// Create flight referencing this aircraft
	resp = c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-EDEL", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
	})
	requireStatus(t, resp, http.StatusCreated)
	var flt map[string]interface{}
	resp.JSON(&flt)
	fltID := flt["id"].(string)

	// Delete aircraft
	assertStatus(t, c.DELETE(fmt.Sprintf("/aircraft/%s", acID)), http.StatusNoContent)

	// Flight should still exist with registration as text
	resp = c.GET(fmt.Sprintf("/flights/%s", fltID))
	requireStatus(t, resp, http.StatusOK)
	resp.JSON(&flt)
	if flt["aircraftReg"] != "D-EDEL" {
		t.Errorf("Flight should retain aircraft reg, got %v", flt["aircraftReg"])
	}
}
