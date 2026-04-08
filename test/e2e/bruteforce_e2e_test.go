//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

func TestBruteForceProtection(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("brute")
	pw := "SecurePass123!"
	registerUser(t, c, email, pw, "Brute Force Target")

	t.Run("5 failed logins then lockout", func(t *testing.T) {
		// Fail 5 times
		for i := 1; i <= 5; i++ {
			resp := c.POST("/auth/login", map[string]string{"email": email, "password": "WrongPass!"})
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Attempt %d: expected 401, got %d", i, resp.StatusCode)
			}
		}

		// 6th attempt: should be locked even with correct password
		resp := c.POST("/auth/login", map[string]string{"email": email, "password": pw})
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 403/429/401 after lockout, got %d: %s", resp.StatusCode, string(resp.Body))
		}
		t.Logf("Lockout response: %d %s", resp.StatusCode, string(resp.Body))
	})

	t.Run("admin can unlock locked user", func(t *testing.T) {
		ac := getAdminClient(t)

		// Find the user ID
		resp := ac.GET("/admin/users?search=" + email)
		requireStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		resp.JSON(&result)
		data := result["data"].([]interface{})
		if len(data) == 0 {
			t.Fatal("Could not find locked user")
		}
		userID := data[0].(map[string]interface{})["id"].(string)

		// Unlock
		resp = ac.POST("/admin/users/"+userID+"/unlock", nil)
		requireStatus(t, resp, http.StatusOK)

		// Should be able to login now
		resp = c.POST("/auth/login", map[string]string{"email": email, "password": pw})
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("successful login resets failed counter", func(t *testing.T) {
		email2 := uniqueEmail("brute-reset")
		registerUser(t, c, email2, pw, "Reset Counter")

		// Fail 3 times
		for i := 0; i < 3; i++ {
			c.POST("/auth/login", map[string]string{"email": email2, "password": "Wrong!"})
		}

		// Succeed
		resp := c.POST("/auth/login", map[string]string{"email": email2, "password": pw})
		requireStatus(t, resp, http.StatusOK)

		// Fail 3 more times — counter should have reset, so still not locked
		for i := 0; i < 3; i++ {
			c.POST("/auth/login", map[string]string{"email": email2, "password": "Wrong!"})
		}

		// Should still work (3 < 5)
		resp = c.POST("/auth/login", map[string]string{"email": email2, "password": pw})
		requireStatus(t, resp, http.StatusOK)
	})
}
