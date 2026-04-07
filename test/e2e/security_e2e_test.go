//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	c := NewE2EClient(t)
	resp := c.DoRaw("GET", "/health", nil)
	requireStatus(t, resp, http.StatusOK)

	if resp.Headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("Expected X-Content-Type-Options: nosniff")
	}
	if resp.Headers.Get("X-Frame-Options") != "DENY" {
		t.Error("Expected X-Frame-Options: DENY")
	}
}

func TestSQLInjection(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("sqli"), "SecurePass123!", "SQLi")

	tests := []struct {
		name string
		path string
	}{
		{"flight search", "/flights?search=' OR 1=1 --"},
		{"aircraft reg", "/flights?aircraftReg='; DROP TABLE flights; --"},
		{"contact search", "/contacts/search?q=' OR ''='"},
		{"airport search", "/airports/search?q=ED' OR '1'='1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := c.GET(tt.path)
			if resp.StatusCode == http.StatusInternalServerError {
				t.Errorf("SQL injection caused 500 on %s", tt.path)
			}
		})
	}

	t.Run("email field", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{
			"email": "'; DROP TABLE users; --@h.com", "password": "SecurePass123!", "name": "X",
		})
		if resp.StatusCode == http.StatusInternalServerError {
			t.Error("SQL injection in email caused 500")
		}
	})
}

func TestAuthorizationBoundaries(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	registerAndLogin(t, c1, uniqueEmail("authz1"), "SecurePass123!", "U1")
	registerAndLogin(t, c2, uniqueEmail("authz2"), "SecurePass123!", "U2")

	resp := c1.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-EONE", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
	})
	requireStatus(t, resp, http.StatusCreated)
	var f map[string]interface{}
	resp.JSON(&f)
	fid := f["id"].(string)

	resp = c1.POST("/credentials", map[string]interface{}{
		"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": today(), "issuingAuthority": "AME",
	})
	requireStatus(t, resp, http.StatusCreated)
	var cr map[string]interface{}
	resp.JSON(&cr)
	crid := cr["id"].(string)

	t.Run("user2 cannot update user1 flight", func(t *testing.T) {
		assertStatus(t, c2.PUT("/flights/"+fid, map[string]interface{}{
			"date": today(), "aircraftReg": "D-EHCK", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		}), http.StatusNotFound)
	})

	t.Run("user2 cannot update user1 credential", func(t *testing.T) {
		assertStatus(t, c2.PATCH("/credentials/"+crid, map[string]interface{}{"notes": "Hacked"}), http.StatusNotFound)
	})

	t.Run("user2 export excludes user1 data", func(t *testing.T) {
		resp := c2.GET("/exports/json")
		requireStatus(t, resp, http.StatusOK)
		var data map[string]interface{}
		resp.JSON(&data)
		if flights, ok := data["flights"].([]interface{}); ok {
			for _, fl := range flights {
				if fl.(map[string]interface{})["id"] == fid {
					t.Error("Data leak: user2 export contains user1 flight")
				}
			}
		}
	})
}

func TestInvalidUUIDs(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("uuid"), "SecurePass123!", "UUID")

	invalid := []string{"not-a-uuid", "12345", "../../etc/passwd"}
	endpoints := []string{"/flights/", "/licenses/", "/aircraft/", "/credentials/", "/contacts/"}

	for _, id := range invalid {
		for _, ep := range endpoints {
			t.Run(fmt.Sprintf("%s%s", ep, id), func(t *testing.T) {
				resp := c.GET(ep + id)
				if resp.StatusCode == http.StatusInternalServerError {
					t.Errorf("500 for invalid UUID at %s%s", ep, id)
				}
			})
		}
	}
}
