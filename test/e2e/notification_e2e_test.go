//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNotificationPreferencesDefaults(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif-defaults"), "SecurePass123!", "NotifDefaults")

	t.Run("get defaults returns all categories enabled", func(t *testing.T) {
		resp := c.GET("/users/me/notifications")
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		if prefs["emailEnabled"] != true {
			t.Errorf("Default emailEnabled should be true, got %v", prefs["emailEnabled"])
		}

		categories, ok := prefs["enabledCategories"].([]interface{})
		if !ok {
			t.Fatalf("enabledCategories should be an array, got %T", prefs["enabledCategories"])
		}
		if len(categories) != 10 {
			t.Errorf("Default enabledCategories should have 10 entries, got %d", len(categories))
		}

		warningDays, ok := prefs["warningDays"].([]interface{})
		if !ok {
			t.Fatalf("warningDays should be an array, got %T", prefs["warningDays"])
		}
		if len(warningDays) != 3 {
			t.Errorf("Default warningDays should have 3 entries, got %d", len(warningDays))
		}

		checkHour, ok := prefs["checkHour"].(float64)
		if !ok {
			t.Fatalf("checkHour should be a number, got %T", prefs["checkHour"])
		}
		if int(checkHour) != 8 {
			t.Errorf("Default checkHour should be 8, got %d", int(checkHour))
		}
	})
}

func TestNotificationPreferencesUpdate(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif-update"), "SecurePass123!", "NotifUpdate")

	t.Run("update enabledCategories to subset", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"enabledCategories": []string{"credential_medical", "rating_expiry"},
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		categories := prefs["enabledCategories"].([]interface{})
		if len(categories) != 2 {
			t.Errorf("Expected 2 categories, got %d", len(categories))
		}

		found := map[string]bool{}
		for _, c := range categories {
			found[c.(string)] = true
		}
		if !found["credential_medical"] {
			t.Error("Expected credential_medical in categories")
		}
		if !found["rating_expiry"] {
			t.Error("Expected rating_expiry in categories")
		}
	})

	t.Run("update checkHour", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"checkHour": 14,
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		if int(prefs["checkHour"].(float64)) != 14 {
			t.Errorf("Expected checkHour 14, got %v", prefs["checkHour"])
		}
	})

	t.Run("update warningDays", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"warningDays": []int{60, 30, 7, 1},
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		days := prefs["warningDays"].([]interface{})
		if len(days) != 4 {
			t.Errorf("Expected 4 warning days, got %d", len(days))
		}
	})

	t.Run("disable email entirely", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled": false,
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		if prefs["emailEnabled"] != false {
			t.Errorf("Expected emailEnabled false, got %v", prefs["emailEnabled"])
		}
	})

	t.Run("re-enable email keeps other settings", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled": true,
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		if prefs["emailEnabled"] != true {
			t.Errorf("Expected emailEnabled true, got %v", prefs["emailEnabled"])
		}
		categories := prefs["enabledCategories"].([]interface{})
		if len(categories) != 2 {
			t.Errorf("Expected categories to persist as 2, got %d", len(categories))
		}
		if int(prefs["checkHour"].(float64)) != 14 {
			t.Errorf("Expected checkHour to persist as 14, got %v", prefs["checkHour"])
		}
	})

	t.Run("update all fields at once", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled": true,
			"enabledCategories": []string{
				"credential_medical", "credential_language",
				"currency_passenger", "currency_night",
				"currency_flight_review",
			},
			"warningDays": []int{30, 14, 7},
			"checkHour":   6,
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		if prefs["emailEnabled"] != true {
			t.Error("Expected emailEnabled true")
		}
		categories := prefs["enabledCategories"].([]interface{})
		if len(categories) != 5 {
			t.Errorf("Expected 5 categories, got %d", len(categories))
		}
		if int(prefs["checkHour"].(float64)) != 6 {
			t.Errorf("Expected checkHour 6, got %v", prefs["checkHour"])
		}
	})
}

func TestNotificationPreferencesValidation(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif-valid"), "SecurePass123!", "NotifValid")

	t.Run("checkHour below 0 rejected", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"checkHour": -1,
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("checkHour above 23 rejected", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"checkHour": 24,
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("empty enabledCategories accepted", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"enabledCategories": []string{},
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		categories := prefs["enabledCategories"].([]interface{})
		if len(categories) != 0 {
			t.Errorf("Expected 0 categories, got %d", len(categories))
		}
	})

	t.Run("empty warningDays accepted", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"warningDays": []int{},
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		days := prefs["warningDays"].([]interface{})
		if len(days) != 0 {
			t.Errorf("Expected 0 warning days, got %d", len(days))
		}
	})
}

func TestNotificationPreferencesAuth(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("unauthenticated GET returns 401", func(t *testing.T) {
		resp := c.GET("/users/me/notifications")
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("unauthenticated PATCH returns 401", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled": false,
		})
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("unauthenticated history returns 401", func(t *testing.T) {
		resp := c.GET("/users/me/notifications/history")
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

func TestNotificationHistory(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif-history"), "SecurePass123!", "NotifHistory")

	t.Run("empty history returns empty array", func(t *testing.T) {
		resp := c.GET("/users/me/notifications/history")
		requireStatus(t, resp, http.StatusOK)

		var history map[string]interface{}
		resp.JSON(&history)

		items := history["items"].([]interface{})
		if len(items) != 0 {
			t.Errorf("Expected 0 history items, got %d", len(items))
		}

		total := int(history["total"].(float64))
		if total != 0 {
			t.Errorf("Expected total 0, got %d", total)
		}
	})

	t.Run("history with limit and offset", func(t *testing.T) {
		resp := c.GET("/users/me/notifications/history?limit=5&offset=0")
		requireStatus(t, resp, http.StatusOK)

		var history map[string]interface{}
		resp.JSON(&history)

		if _, ok := history["items"]; !ok {
			t.Error("Response should have 'items' field")
		}
		if _, ok := history["total"]; !ok {
			t.Error("Response should have 'total' field")
		}
	})
}

func TestNotificationPreferencesIsolation(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	registerAndLogin(t, c1, uniqueEmail("notif-iso1"), "SecurePass123!", "IsoUser1")
	registerAndLogin(t, c2, uniqueEmail("notif-iso2"), "SecurePass123!", "IsoUser2")

	t.Run("user1 changes do not affect user2", func(t *testing.T) {
		resp := c1.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled":      false,
			"enabledCategories": []string{"credential_medical"},
			"checkHour":         22,
		})
		requireStatus(t, resp, http.StatusOK)

		resp = c2.GET("/users/me/notifications")
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		if prefs["emailEnabled"] != true {
			t.Error("User2 emailEnabled should still be true (defaults)")
		}
		categories := prefs["enabledCategories"].([]interface{})
		if len(categories) != 10 {
			t.Errorf("User2 should still have 10 default categories, got %d", len(categories))
		}
		if int(prefs["checkHour"].(float64)) != 8 {
			t.Errorf("User2 checkHour should still be 8, got %v", prefs["checkHour"])
		}
	})
}

func TestNotificationAllCategories(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif-allcat"), "SecurePass123!", "AllCat")

	allCategories := []string{
		"credential_medical", "credential_language", "credential_security", "credential_other",
		"rating_expiry", "currency_passenger", "currency_night", "currency_instrument",
		"currency_flight_review", "currency_revalidation",
	}

	t.Run("set each category individually and verify", func(t *testing.T) {
		for _, cat := range allCategories {
			resp := c.PATCH("/users/me/notifications", map[string]interface{}{
				"enabledCategories": []string{cat},
			})
			requireStatus(t, resp, http.StatusOK)

			var prefs map[string]interface{}
			resp.JSON(&prefs)

			categories := prefs["enabledCategories"].([]interface{})
			if len(categories) != 1 {
				t.Errorf("Category %s: expected 1 category, got %d", cat, len(categories))
			}
			if categories[0].(string) != cat {
				t.Errorf("Category %s: expected %s, got %s", cat, cat, categories[0])
			}
		}
	})

	t.Run("restore all categories", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"enabledCategories": allCategories,
		})
		requireStatus(t, resp, http.StatusOK)

		var prefs map[string]interface{}
		resp.JSON(&prefs)

		categories := prefs["enabledCategories"].([]interface{})
		if len(categories) != 10 {
			t.Errorf("Expected 10 categories, got %d", len(categories))
		}
	})
}

// --- Email delivery tests using MailPit ---

func TestNotificationEmailCredentialExpiry(t *testing.T) {
	// Register admin to trigger notifications
	ac := getAdminClient(t)

	// Register a normal user with a credential expiring soon
	c := NewE2EClient(t)
	email := uniqueEmail("notif-email-cred")
	registerAndLogin(t, c, email, "SecurePass123!", "CredExpiry")

	// Ensure notifications are enabled for credential_medical
	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{30, 14, 7},
	})

	// Create a medical credential expiring in 10 days
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-NOTIF-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(10),
		"issuingAuthority": "AME Test",
	})

	// Clear MailPit before triggering
	mailpitDeleteAll(t)

	// Trigger notification check via admin endpoint
	resp := ac.POST("/admin/maintenance/trigger-notifications", nil)
	requireStatus(t, resp, http.StatusOK)

	// Wait briefly for email delivery
	time.Sleep(2 * time.Second)

	// Check MailPit for the email
	result := mailpitSearchByRecipient(t, email)

	if result.MessagesCount == 0 {
		t.Fatalf("Expected at least 1 email to %s, got 0", email)
	}

	// Verify subject contains credential info
	found := false
	for _, msg := range result.Messages {
		if strings.Contains(msg.Subject, "EASA_CLASS2_MEDICAL") && strings.Contains(msg.Subject, "expires") {
			found = true
			break
		}
	}
	if !found {
		subjects := make([]string, len(result.Messages))
		for i, m := range result.Messages {
			subjects[i] = m.Subject
		}
		t.Errorf("Expected email about EASA_CLASS2_MEDICAL expiry, got subjects: %v", subjects)
	}
}

func TestNotificationEmailNotSentWhenDisabled(t *testing.T) {
	ac := getAdminClient(t)

	c := NewE2EClient(t)
	email := uniqueEmail("notif-email-disabled")
	registerAndLogin(t, c, email, "SecurePass123!", "Disabled")

	// Disable email notifications
	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled": false,
	})

	// Create a credential expiring soon
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS1_MEDICAL",
		"credentialNumber": "MED-DISABLED-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(5),
		"issuingAuthority": "AME Test",
	})

	// Clear MailPit and trigger
	mailpitDeleteAll(t)
	resp := ac.POST("/admin/maintenance/trigger-notifications", nil)
	requireStatus(t, resp, http.StatusOK)
	time.Sleep(2 * time.Second)

	// Should NOT have received any email
	result := mailpitSearchByRecipient(t, email)
	if result.MessagesCount != 0 {
		t.Errorf("Expected 0 emails when notifications disabled, got %d", result.MessagesCount)
	}
}

func TestNotificationEmailCategoryFiltering(t *testing.T) {
	ac := getAdminClient(t)

	c := NewE2EClient(t)
	email := uniqueEmail("notif-email-catfilt")
	registerAndLogin(t, c, email, "SecurePass123!", "CatFilter")

	// Enable only credential_language — NOT credential_medical
	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_language"},
		"warningDays":       []int{30, 14, 7},
	})

	// Create a MEDICAL credential expiring soon (not in enabled categories)
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-CATFILT-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(5),
		"issuingAuthority": "AME Test",
	})

	// Clear MailPit and trigger
	mailpitDeleteAll(t)
	resp := ac.POST("/admin/maintenance/trigger-notifications", nil)
	requireStatus(t, resp, http.StatusOK)
	time.Sleep(2 * time.Second)

	// Should NOT have received email — medical category is not enabled
	result := mailpitSearchByRecipient(t, email)
	if result.MessagesCount != 0 {
		t.Errorf("Expected 0 emails (medical category disabled), got %d", result.MessagesCount)
	}
}

func TestNotificationEmailDedup(t *testing.T) {
	ac := getAdminClient(t)

	c := NewE2EClient(t)
	email := uniqueEmail("notif-email-dedup")
	registerAndLogin(t, c, email, "SecurePass123!", "Dedup")

	// Use a single warning day to isolate dedup behavior
	// (multiple warning days would each trigger separately — that's correct behavior)
	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{14},
	})

	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-DEDUP-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(10),
		"issuingAuthority": "AME Test",
	})

	// Clear MailPit and trigger twice
	mailpitDeleteAll(t)

	resp := ac.POST("/admin/maintenance/trigger-notifications", nil)
	requireStatus(t, resp, http.StatusOK)
	time.Sleep(2 * time.Second)

	firstResult := mailpitSearchByRecipient(t, email)
	firstCount := firstResult.MessagesCount
	if firstCount == 0 {
		t.Fatal("Expected at least 1 email on first trigger")
	}

	// Trigger again — dedup should prevent duplicate emails
	resp = ac.POST("/admin/maintenance/trigger-notifications", nil)
	requireStatus(t, resp, http.StatusOK)
	time.Sleep(2 * time.Second)

	secondResult := mailpitSearchByRecipient(t, email)
	if secondResult.MessagesCount != firstCount {
		t.Errorf("Expected %d emails (dedup), got %d after second trigger", firstCount, secondResult.MessagesCount)
	}
}

func TestNotificationEmailAppearsInHistory(t *testing.T) {
	ac := getAdminClient(t)

	c := NewE2EClient(t)
	email := uniqueEmail("notif-email-hist")
	registerAndLogin(t, c, email, "SecurePass123!", "History")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_security"},
		"warningDays":       []int{30, 14, 7},
	})

	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "SEC_CLEARANCE_ZUP",
		"credentialNumber": "ZUP-HIST-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(5),
		"issuingAuthority": "LBA",
	})

	// Trigger notification check
	mailpitDeleteAll(t)
	resp := ac.POST("/admin/maintenance/trigger-notifications", nil)
	requireStatus(t, resp, http.StatusOK)
	time.Sleep(2 * time.Second)

	// Verify email was sent
	result := mailpitSearchByRecipient(t, email)
	if result.MessagesCount == 0 {
		t.Fatal("Expected at least 1 email for security credential expiry")
	}

	// Verify the notification appears in history
	resp = c.GET("/users/me/notifications/history")
	requireStatus(t, resp, http.StatusOK)

	var history map[string]interface{}
	resp.JSON(&history)

	total := int(history["total"].(float64))
	if total == 0 {
		t.Error("Expected at least 1 history entry after notification was sent")
	}

	items := history["items"].([]interface{})
	if len(items) == 0 {
		t.Fatal("Expected history items")
	}

	// Check first entry has expected fields
	entry := items[0].(map[string]interface{})
	if entry["category"] == nil {
		t.Error("History entry should have 'category'")
	}
	if entry["subject"] == nil {
		t.Error("History entry should have 'subject'")
	}
	if entry["sentAt"] == nil {
		t.Error("History entry should have 'sentAt'")
	}
}

func TestNotificationTriggerAdminOnly(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif-nonadmin"), "SecurePass123!", "NonAdmin")

	t.Run("non-admin cannot trigger notifications", func(t *testing.T) {
		resp := c.POST("/admin/maintenance/trigger-notifications", nil)
		assertStatus(t, resp, http.StatusForbidden)
	})

	t.Run("unauthenticated cannot trigger notifications", func(t *testing.T) {
		unauth := NewE2EClient(t)
		resp := unauth.POST("/admin/maintenance/trigger-notifications", nil)
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

func TestNotificationSmtpTest(t *testing.T) {
	ac := getAdminClient(t)

	mailpitDeleteAll(t)

	t.Run("admin SMTP test sends email", func(t *testing.T) {
		resp := ac.POST("/admin/maintenance/smtp-test", nil)
		requireStatus(t, resp, http.StatusOK)

		time.Sleep(2 * time.Second)

		// Check MailPit received the test email
		result := mailpitSearchByRecipient(t, "admin@ninerlog-test.com")
		if result.MessagesCount == 0 {
			// Might be sent to admin email, check all messages
			result = mailpitGetMessages(t)
		}
		if result.MessagesCount == 0 {
			t.Error("Expected SMTP test email to be delivered to MailPit")
		}

		// Verify subject
		found := false
		for _, msg := range result.Messages {
			if strings.Contains(msg.Subject, "SMTP Test") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected email with subject containing 'SMTP Test'")
		}
	})
}
