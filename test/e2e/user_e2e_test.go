//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

func TestUserProfile(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("profile")
	registerAndLogin(t, c, email, "SecurePass123!", "Profile User")

	t.Run("get current user", func(t *testing.T) {
		resp := c.GET("/users/me")
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["email"] != email {
			t.Errorf("Expected %s, got %v", email, u["email"])
		}
	})

	t.Run("update name", func(t *testing.T) {
		resp := c.PATCH("/users/me", map[string]string{"name": "Updated"})
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["name"] != "Updated" {
			t.Errorf("Expected Updated, got %v", u["name"])
		}
	})

	t.Run("update email", func(t *testing.T) {
		ne := uniqueEmail("profile-new")
		resp := c.PATCH("/users/me", map[string]string{"email": ne})
		requireStatus(t, resp, http.StatusOK)
	})
}

func TestUserDeletion(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("delete account with password", func(t *testing.T) {
		email := uniqueEmail("del-user")
		pw := "SecurePass123!"
		registerAndLogin(t, c, email, pw, "DelMe")
		resp := c.DELETEWithBody("/users/me", map[string]string{"password": pw})
		// API returns 200 with message body instead of 204
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected 200 or 204, got %d: %s", resp.StatusCode, string(resp.Body))
		}
		resp = c.POST("/auth/login", map[string]string{"email": email, "password": pw})
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("delete without password fails", func(t *testing.T) {
		registerAndLogin(t, c, uniqueEmail("del-nopw"), "SecurePass123!", "Keep")
		resp := c.DELETEWithBody("/users/me", map[string]string{})
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 400/401, got %d", resp.StatusCode)
		}
	})

	t.Run("delete with wrong password fails", func(t *testing.T) {
		registerAndLogin(t, c, uniqueEmail("del-wrongpw"), "SecurePass123!", "Keep2")
		resp := c.DELETEWithBody("/users/me", map[string]string{"password": "Wrong!"})
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected 401/403, got %d", resp.StatusCode)
		}
	})
}

func TestNotificationPreferences(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif"), "SecurePass123!", "Notif")

	t.Run("get defaults", func(t *testing.T) {
		resp := c.GET("/users/me/notifications")
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("update preferences", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled": false, "currencyWarnings": false, "credentialWarnings": true, "warningDays": []int{30, 14, 7},
		})
		requireStatus(t, resp, http.StatusOK)
	})
}

func TestDeleteAllUserData(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("del-data"), "SecurePass123!", "Data")
	c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "DEL-001",
		"issueDate": today(), "issuingAuthority": "LBA",
	})

	t.Run("delete data keeps account", func(t *testing.T) {
		resp := c.DELETE("/users/me/data")
		// API returns 200 with message body instead of 204
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected 200 or 204, got %d: %s", resp.StatusCode, string(resp.Body))
		}
		assertStatus(t, c.GET("/users/me"), http.StatusOK)
		resp = c.GET("/licenses")
		requireStatus(t, resp, http.StatusOK)
		var lics []interface{}
		resp.JSON(&lics)
		if len(lics) != 0 {
			t.Errorf("Expected 0 licenses, got %d", len(lics))
		}
	})
}
