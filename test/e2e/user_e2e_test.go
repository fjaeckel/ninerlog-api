//go:build e2e

package e2e_test

import (
	"net/http"
	"reflect"
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

	t.Run("default timeDisplayFormat is hm", func(t *testing.T) {
		c2 := NewE2EClient(t)
		registerAndLogin(t, c2, uniqueEmail("tdf-default"), "SecurePass123!", "TDF Default")
		resp := c2.GET("/users/me")
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["timeDisplayFormat"] != "hm" {
			t.Errorf("Expected default timeDisplayFormat 'hm', got %v", u["timeDisplayFormat"])
		}
	})

	t.Run("update timeDisplayFormat to decimal", func(t *testing.T) {
		c2 := NewE2EClient(t)
		registerAndLogin(t, c2, uniqueEmail("tdf-dec"), "SecurePass123!", "TDF Decimal")
		resp := c2.PATCH("/users/me", map[string]string{"timeDisplayFormat": "decimal"})
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["timeDisplayFormat"] != "decimal" {
			t.Errorf("Expected timeDisplayFormat 'decimal', got %v", u["timeDisplayFormat"])
		}
		// Verify it persists on GET
		resp = c2.GET("/users/me")
		requireStatus(t, resp, http.StatusOK)
		resp.JSON(&u)
		if u["timeDisplayFormat"] != "decimal" {
			t.Errorf("Expected persisted timeDisplayFormat 'decimal', got %v", u["timeDisplayFormat"])
		}
	})

	t.Run("recency preferences default and update", func(t *testing.T) {
		c2 := NewE2EClient(t)
		registerAndLogin(t, c2, uniqueEmail("recency-pref"), "SecurePass123!", "Recency Pref")

		// Defaults: per-model on, per-registration off
		resp := c2.GET("/users/me")
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["recencyPerModel"] != true {
			t.Errorf("Expected default recencyPerModel true, got %v", u["recencyPerModel"])
		}
		if u["recencyPerRegistration"] != false {
			t.Errorf("Expected default recencyPerRegistration false, got %v", u["recencyPerRegistration"])
		}

		// Flip both and verify persistence
		resp = c2.PATCH("/users/me", map[string]interface{}{
			"recencyPerModel": false, "recencyPerRegistration": true,
		})
		requireStatus(t, resp, http.StatusOK)
		resp = c2.GET("/users/me")
		requireStatus(t, resp, http.StatusOK)
		resp.JSON(&u)
		if u["recencyPerModel"] != false {
			t.Errorf("Expected recencyPerModel false after update, got %v", u["recencyPerModel"])
		}
		if u["recencyPerRegistration"] != true {
			t.Errorf("Expected recencyPerRegistration true after update, got %v", u["recencyPerRegistration"])
		}
	})

	t.Run("update timeDisplayFormat back to hm", func(t *testing.T) {
		c2 := NewE2EClient(t)
		registerAndLogin(t, c2, uniqueEmail("tdf-hm"), "SecurePass123!", "TDF HM")
		c2.PATCH("/users/me", map[string]string{"timeDisplayFormat": "decimal"})
		resp := c2.PATCH("/users/me", map[string]string{"timeDisplayFormat": "hm"})
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["timeDisplayFormat"] != "hm" {
			t.Errorf("Expected timeDisplayFormat 'hm', got %v", u["timeDisplayFormat"])
		}
	})

	t.Run("invalid timeDisplayFormat ignored", func(t *testing.T) {
		c2 := NewE2EClient(t)
		registerAndLogin(t, c2, uniqueEmail("tdf-inv"), "SecurePass123!", "TDF Invalid")
		resp := c2.PATCH("/users/me", map[string]string{"timeDisplayFormat": "invalid_value"})
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		// Should remain 'hm' (default), invalid value ignored
		if u["timeDisplayFormat"] != "hm" {
			t.Errorf("Expected timeDisplayFormat 'hm' after invalid update, got %v", u["timeDisplayFormat"])
		}
	})
}

func TestUserFlightListColumnPreferences(t *testing.T) {
	columnsOf := func(t *testing.T, u map[string]interface{}) []string {
		t.Helper()
		raw, ok := u["flightListColumns"]
		if !ok || raw == nil {
			t.Fatalf("flightListColumns missing from the user payload: %v", u)
		}
		list, ok := raw.([]interface{})
		if !ok {
			t.Fatalf("flightListColumns = %v, want an array", raw)
		}
		out := make([]string, 0, len(list))
		for _, v := range list {
			out = append(out, v.(string))
		}
		return out
	}

	t.Run("defaults to auto with an empty column list", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("flc-default"), "SecurePass123!", "Columns Default")
		resp := c.GET("/users/me")
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["flightListColumnMode"] != "auto" {
			t.Errorf("Expected default flightListColumnMode 'auto', got %v", u["flightListColumnMode"])
		}
		if got := columnsOf(t, u); len(got) != 0 {
			t.Errorf("Expected no columns by default, got %v", got)
		}
	})

	t.Run("custom selection round-trips in canonical order", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("flc-custom"), "SecurePass123!", "Columns Custom")
		resp := c.PATCH("/users/me", map[string]interface{}{
			"flightListColumnMode": "custom",
			// Deliberately out of order, with a duplicate and an unknown key.
			"flightListColumns": []string{"remarks", "picTime", "picTime", "offOnBlock", "notAColumn"},
		})
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["flightListColumnMode"] != "custom" {
			t.Errorf("Expected flightListColumnMode 'custom', got %v", u["flightListColumnMode"])
		}
		want := []string{"offOnBlock", "picTime", "remarks"}
		if got := columnsOf(t, u); !reflect.DeepEqual(got, want) {
			t.Errorf("flightListColumns = %v, want %v", got, want)
		}

		// And it survives a reload.
		resp = c.GET("/users/me")
		requireStatus(t, resp, http.StatusOK)
		resp.JSON(&u)
		if got := columnsOf(t, u); !reflect.DeepEqual(got, want) {
			t.Errorf("Persisted flightListColumns = %v, want %v", got, want)
		}
	})

	t.Run("an empty custom selection is honoured, not treated as auto", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("flc-none"), "SecurePass123!", "Columns None")
		c.PATCH("/users/me", map[string]interface{}{
			"flightListColumnMode": "custom",
			"flightListColumns":    []string{"ifrTime"},
		})
		resp := c.PATCH("/users/me", map[string]interface{}{"flightListColumns": []string{}})
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["flightListColumnMode"] != "custom" {
			t.Errorf("Expected flightListColumnMode to stay 'custom', got %v", u["flightListColumnMode"])
		}
		if got := columnsOf(t, u); len(got) != 0 {
			t.Errorf("Expected the column list to be cleared, got %v", got)
		}
	})

	t.Run("switching back to auto keeps the saved selection", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("flc-back"), "SecurePass123!", "Columns Back")
		c.PATCH("/users/me", map[string]interface{}{
			"flightListColumnMode": "custom",
			"flightListColumns":    []string{"nightTime", "landings"},
		})
		resp := c.PATCH("/users/me", map[string]interface{}{"flightListColumnMode": "auto"})
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["flightListColumnMode"] != "auto" {
			t.Errorf("Expected flightListColumnMode 'auto', got %v", u["flightListColumnMode"])
		}
		want := []string{"nightTime", "landings"}
		if got := columnsOf(t, u); !reflect.DeepEqual(got, want) {
			t.Errorf("flightListColumns = %v, want the selection to be kept (%v)", got, want)
		}
	})

	t.Run("an unknown mode is ignored", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("flc-mode"), "SecurePass123!", "Columns Mode")
		resp := c.PATCH("/users/me", map[string]interface{}{"flightListColumnMode": "whatever"})
		requireStatus(t, resp, http.StatusOK)
		var u map[string]interface{}
		resp.JSON(&u)
		if u["flightListColumnMode"] != "auto" {
			t.Errorf("Expected flightListColumnMode to stay 'auto', got %v", u["flightListColumnMode"])
		}
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

// TestNotificationPreferences — basic smoke test (comprehensive tests in notification_e2e_test.go)
func TestNotificationPreferencesSmoke(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif"), "SecurePass123!", "Notif")

	t.Run("get defaults", func(t *testing.T) {
		resp := c.GET("/users/me/notifications")
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("update preferences", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled": false, "enabledCategories": []string{"credential_medical", "rating_expiry"}, "warningDays": []int{30, 14, 7}, "checkHour": 10,
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
