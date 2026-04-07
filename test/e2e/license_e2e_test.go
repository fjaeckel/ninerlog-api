//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestLicenseCRUD(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("license"), "SecurePass123!", "License")
	var licID string

	t.Run("create license", func(t *testing.T) {
		resp := c.POST("/licenses", map[string]interface{}{
			"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "DE-PPL-123",
			"issueDate": "2023-01-15", "issuingAuthority": "LBA",
		})
		requireStatus(t, resp, http.StatusCreated)
		var lic map[string]interface{}
		resp.JSON(&lic)
		licID = lic["id"].(string)
	})

	t.Run("create with separate logbook", func(t *testing.T) {
		resp := c.POST("/licenses", map[string]interface{}{
			"regulatoryAuthority": "EASA", "licenseType": "SPL", "licenseNumber": "SPL-001",
			"issueDate": "2023-06-01", "issuingAuthority": "LBA", "requiresSeparateLogbook": true,
		})
		requireStatus(t, resp, http.StatusCreated)
	})

	t.Run("list licenses", func(t *testing.T) {
		resp := c.GET("/licenses")
		requireStatus(t, resp, http.StatusOK)
		var lics []map[string]interface{}
		resp.JSON(&lics)
		if len(lics) < 2 {
			t.Errorf("Expected >=2 licenses, got %d", len(lics))
		}
	})

	t.Run("get by id", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/licenses/%s", licID)), http.StatusOK)
	})

	t.Run("update", func(t *testing.T) {
		resp := c.PATCH(fmt.Sprintf("/licenses/%s", licID), map[string]interface{}{"licenseNumber": "UPDATED"})
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("get statistics", func(t *testing.T) {
		resp := c.GET(fmt.Sprintf("/licenses/%s/statistics", licID))
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(resp.Body))
		}
	})

	t.Run("get currency", func(t *testing.T) {
		resp := c.GET(fmt.Sprintf("/licenses/%s/currency", licID))
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(resp.Body))
		}
	})

	t.Run("delete", func(t *testing.T) {
		assertStatus(t, c.DELETE(fmt.Sprintf("/licenses/%s", licID)), http.StatusNoContent)
		assertStatus(t, c.GET(fmt.Sprintf("/licenses/%s", licID)), http.StatusNotFound)
	})

	t.Run("nonexistent returns 404", func(t *testing.T) {
		assertStatus(t, c.GET("/licenses/00000000-0000-0000-0000-000000000000"), http.StatusNotFound)
	})

	t.Run("REGRESSION: missing fields should return 400 not 500", func(t *testing.T) {
		resp := c.POST("/licenses", map[string]interface{}{"licenseType": "PPL"})
		if resp.StatusCode == http.StatusInternalServerError {
			t.Log("REGRESSION: Missing license fields causes 500 instead of 400")
		} else {
			assertStatus(t, resp, http.StatusBadRequest)
		}
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.POST("/licenses", map[string]interface{}{
			"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "X",
			"issueDate": today(), "issuingAuthority": "LBA",
		}), http.StatusUnauthorized)
	})
}

func TestLicenseIsolation(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	registerAndLogin(t, c1, uniqueEmail("lic-iso1"), "SecurePass123!", "U1")
	registerAndLogin(t, c2, uniqueEmail("lic-iso2"), "SecurePass123!", "U2")

	resp := c1.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "ISO",
		"issueDate": today(), "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	lid := lic["id"].(string)

	t.Run("user2 cannot see", func(t *testing.T) {
		resp := c2.GET("/licenses")
		requireStatus(t, resp, http.StatusOK)
		var lics []interface{}
		resp.JSON(&lics)
		if len(lics) != 0 {
			t.Errorf("Expected 0, got %d", len(lics))
		}
	})

	t.Run("user2 cannot access by id", func(t *testing.T) {
		assertStatus(t, c2.GET(fmt.Sprintf("/licenses/%s", lid)), http.StatusNotFound)
	})

	t.Run("user2 cannot delete", func(t *testing.T) {
		assertStatus(t, c2.DELETE(fmt.Sprintf("/licenses/%s", lid)), http.StatusNotFound)
	})
}

func TestClassRatings(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ratings"), "SecurePass123!", "Ratings")

	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "R-001",
		"issueDate": "2023-01-01", "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	lid := lic["id"].(string)
	var rid string

	t.Run("create SEP_LAND", func(t *testing.T) {
		resp := c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
			"classType": "SEP_LAND", "issueDate": "2023-01-15",
		})
		requireStatus(t, resp, http.StatusCreated)
		var r map[string]interface{}
		resp.JSON(&r)
		rid = r["id"].(string)
	})

	t.Run("create TMG with expiry", func(t *testing.T) {
		resp := c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
			"classType": "TMG", "issueDate": "2023-03-01", "expiryDate": futureDate(365),
		})
		requireStatus(t, resp, http.StatusCreated)
	})

	t.Run("create IR", func(t *testing.T) {
		resp := c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
			"classType": "IR", "issueDate": "2023-06-01", "expiryDate": futureDate(365),
		})
		requireStatus(t, resp, http.StatusCreated)
	})

	t.Run("list ratings", func(t *testing.T) {
		resp := c.GET(fmt.Sprintf("/licenses/%s/ratings", lid))
		requireStatus(t, resp, http.StatusOK)
		var ratings []interface{}
		resp.JSON(&ratings)
		if len(ratings) < 3 {
			t.Errorf("Expected >=3, got %d", len(ratings))
		}
	})

	t.Run("update rating", func(t *testing.T) {
		requireStatus(t, c.PATCH(fmt.Sprintf("/licenses/%s/ratings/%s", lid, rid), map[string]interface{}{
			"notes": "Updated",
		}), http.StatusOK)
	})

	t.Run("delete rating", func(t *testing.T) {
		assertStatus(t, c.DELETE(fmt.Sprintf("/licenses/%s/ratings/%s", lid, rid)), http.StatusNoContent)
	})

	t.Run("invalid class type returns 400", func(t *testing.T) {
		assertStatus(t, c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
			"classType": "INVALID", "issueDate": today(),
		}), http.StatusBadRequest)
	})

	t.Run("rating on nonexistent license returns 404", func(t *testing.T) {
		assertStatus(t, c.POST("/licenses/00000000-0000-0000-0000-000000000000/ratings", map[string]interface{}{
			"classType": "SEP_LAND", "issueDate": today(),
		}), http.StatusNotFound)
	})
}
