//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestCredentialCRUD(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cred"), "SecurePass123!", "Cred User")
	var credID string

	t.Run("create medical", func(t *testing.T) {
		resp := c.POST("/credentials", map[string]interface{}{
			"credentialType": "EASA_CLASS2_MEDICAL", "credentialNumber": "MED-001",
			"issueDate": "2024-01-15", "expiryDate": futureDate(365),
			"issuingAuthority": "AME Smith", "notes": "Annual",
		})
		requireStatus(t, resp, http.StatusCreated)
		var cr map[string]interface{}
		resp.JSON(&cr)
		credID = cr["id"].(string)
	})

	t.Run("create language proficiency", func(t *testing.T) {
		requireStatus(t, c.POST("/credentials", map[string]interface{}{
			"credentialType": "LANG_ICAO_LEVEL4", "issueDate": "2023-06-01",
			"expiryDate": futureDate(730), "issuingAuthority": "LBA",
		}), http.StatusCreated)
	})

	t.Run("create security clearance", func(t *testing.T) {
		requireStatus(t, c.POST("/credentials", map[string]interface{}{
			"credentialType": "SEC_CLEARANCE_ZUP", "credentialNumber": "ZUP-123",
			"issueDate": "2024-03-01", "issuingAuthority": "LBA",
		}), http.StatusCreated)
	})

	t.Run("create OTHER type", func(t *testing.T) {
		requireStatus(t, c.POST("/credentials", map[string]interface{}{
			"credentialType": "OTHER", "issueDate": today(), "issuingAuthority": "Custom",
		}), http.StatusCreated)
	})

	t.Run("list credentials", func(t *testing.T) {
		resp := c.GET("/credentials")
		requireStatus(t, resp, http.StatusOK)
		var creds []interface{}
		resp.JSON(&creds)
		if len(creds) < 4 {
			t.Errorf("Expected >=4, got %d", len(creds))
		}
	})

	t.Run("get by id", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/credentials/%s", credID)), http.StatusOK)
	})

	t.Run("update", func(t *testing.T) {
		resp := c.PATCH(fmt.Sprintf("/credentials/%s", credID), map[string]interface{}{"notes": "Updated"})
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("delete", func(t *testing.T) {
		assertStatus(t, c.DELETE(fmt.Sprintf("/credentials/%s", credID)), http.StatusNoContent)
		assertStatus(t, c.GET(fmt.Sprintf("/credentials/%s", credID)), http.StatusNotFound)
	})

	t.Run("nonexistent returns 404", func(t *testing.T) {
		assertStatus(t, c.GET("/credentials/00000000-0000-0000-0000-000000000000"), http.StatusNotFound)
	})

	t.Run("invalid type returns 400", func(t *testing.T) {
		assertStatus(t, c.POST("/credentials", map[string]interface{}{
			"credentialType": "INVALID", "issueDate": today(), "issuingAuthority": "X",
		}), http.StatusBadRequest)
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.POST("/credentials", map[string]interface{}{
			"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": today(), "issuingAuthority": "X",
		}), http.StatusUnauthorized)
	})
}

func TestCredentialIsolation(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	registerAndLogin(t, c1, uniqueEmail("cred-iso1"), "SecurePass123!", "U1")
	registerAndLogin(t, c2, uniqueEmail("cred-iso2"), "SecurePass123!", "U2")

	resp := c1.POST("/credentials", map[string]interface{}{
		"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": today(), "issuingAuthority": "AME",
	})
	requireStatus(t, resp, http.StatusCreated)
	var cr map[string]interface{}
	resp.JSON(&cr)
	cid := cr["id"].(string)

	t.Run("user2 cannot see", func(t *testing.T) {
		resp := c2.GET("/credentials")
		requireStatus(t, resp, http.StatusOK)
		var creds []interface{}
		resp.JSON(&creds)
		if len(creds) != 0 {
			t.Errorf("Expected 0, got %d", len(creds))
		}
	})

	t.Run("user2 cannot access by id", func(t *testing.T) {
		assertStatus(t, c2.GET(fmt.Sprintf("/credentials/%s", cid)), http.StatusNotFound)
	})
}
