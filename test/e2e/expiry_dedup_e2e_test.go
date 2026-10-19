//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// utcDate returns the UTC calendar date `days` from today.
func utcDate(days int) string { return time.Now().UTC().AddDate(0, 0, days).Format("2006-01-02") }

func TestExpiryDateIsLastValidDay(t *testing.T) {
	tests := []struct {
		name    string
		class   string
		expiry  string
		expired bool
	}{
		{"SEP rating valid on its expiry date", "SEP_LAND", utcDate(0), false},
		{"SEP rating expired the day after", "SEP_LAND", utcDate(-1), true},
		{"IR valid on its expiry date", "IR", utcDate(0), false},
		{"IR expired the day after", "IR", utcDate(-1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := setupCurrencyUser(t, "expiry-last-day")
			licID := createLicenseCur(t, c, "EASA", "PPL")
			expiry := tt.expiry
			createRatingCur(t, c, licID, tt.class, &expiry)

			rc := findRatingCur(getCurrencyStatus(t, c), tt.class)
			if rc == nil {
				t.Fatalf("%s rating currency not found", tt.class)
			}
			if got := rc["status"] == "expired"; got != tt.expired {
				t.Errorf("status = %v, want expired=%v", rc["status"], tt.expired)
			}
		})
	}
}

func TestNotificationEmailNearestThresholdOnly(t *testing.T) {
	ac := getAdminClient(t)

	c := NewE2EClient(t)
	email := uniqueEmail("notif-nearest")
	registerAndLogin(t, c, email, "SecurePass123!", "Nearest")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{30, 14, 7},
	})
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-NEAREST-001",
		"issueDate":        utcDate(-365),
		"expiryDate":       utcDate(5),
		"issuingAuthority": "AME Test",
	})

	mailpitDeleteAll(t)
	for i := 0; i < 2; i++ {
		resp := ac.POST("/admin/maintenance/trigger-notifications", nil)
		requireStatus(t, resp, http.StatusOK)
	}
	time.Sleep(2 * time.Second)

	result := mailpitSearchByRecipient(t, email)
	if result.MessagesCount != 1 {
		t.Fatalf("emails = %d, want 1 (the 7-day warning only)", result.MessagesCount)
	}
	if !strings.Contains(result.Messages[0].Subject, "expires in 5 days") {
		t.Errorf("subject = %q, want the 5-days-remaining warning", result.Messages[0].Subject)
	}

	resp := c.GET("/users/me/notifications/history")
	requireStatus(t, resp, http.StatusOK)
	var history map[string]interface{}
	resp.JSON(&history)
	if total := int(history["total"].(float64)); total != 1 {
		t.Errorf("history total = %d, want 1 (skipped thresholds are not listed)", total)
	}
	items := history["items"].([]interface{})
	if len(items) == 1 {
		if days, _ := items[0].(map[string]interface{})["daysBeforeExpiry"].(float64); int(days) != 7 {
			t.Errorf("history daysBeforeExpiry = %v, want 7", items[0].(map[string]interface{})["daysBeforeExpiry"])
		}
	}
}

func TestNotificationEmailOnExpiryDate(t *testing.T) {
	ac := getAdminClient(t)

	c := NewE2EClient(t)
	email := uniqueEmail("notif-expiry-day")
	registerAndLogin(t, c, email, "SecurePass123!", "ExpiryDay")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{7},
	})
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-LASTDAY-001",
		"issueDate":        utcDate(-365),
		"expiryDate":       utcDate(0),
		"issuingAuthority": "AME Test",
	})

	mailpitDeleteAll(t)
	resp := ac.POST("/admin/maintenance/trigger-notifications", nil)
	requireStatus(t, resp, http.StatusOK)
	time.Sleep(2 * time.Second)

	if got := mailpitSearchByRecipient(t, email).MessagesCount; got != 1 {
		t.Errorf("emails on the expiry date = %d, want 1", got)
	}
}
