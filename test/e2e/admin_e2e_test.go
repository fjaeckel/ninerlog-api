//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func getAdminClient(t *testing.T) *E2EClient {
	t.Helper()
	ac := NewE2EClient(t)
	const adminEmail = "admin@ninerlog-test.com"
	const adminPw = "AdminPass123!"
	// Try to register admin; if already exists, just login
	resp := ac.POST("/auth/register", map[string]string{
		"email": adminEmail, "password": adminPw, "name": "Admin",
	})
	if resp.StatusCode == http.StatusConflict {
		auth := loginUser(t, ac, adminEmail, adminPw)
		ac.SetToken(auth.AccessToken)
	} else {
		requireStatus(t, resp, http.StatusCreated)
		// Email verification is required before login: pull token from mailpit
		// and exchange it for an auth response.
		token := extractVerificationToken(t, adminEmail)
		verifyResp := ac.POST("/auth/verify-email", map[string]string{"token": token})
		requireStatus(t, verifyResp, http.StatusOK)
		var auth AuthResponseBody
		verifyResp.JSON(&auth)
		ac.SetToken(auth.AccessToken)
	}
	return ac
}

func TestAdminEndpoints(t *testing.T) {
	ac := getAdminClient(t)

	uc := NewE2EClient(t)
	ue := uniqueEmail("admin-target")
	ua := registerAndLogin(t, uc, ue, "UserPass123!", "Target")

	t.Run("admin stats", func(t *testing.T) {
		resp := ac.GET("/admin/stats")
		requireStatus(t, resp, http.StatusOK)
		var s map[string]interface{}
		resp.JSON(&s)
		if s["totalUsers"] == nil {
			t.Error("Expected totalUsers")
		}
		cbd, ok := s["cloudBackupDestinations"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected cloudBackupDestinations object, got %T", s["cloudBackupDestinations"])
		}
		if _, ok := cbd["total"]; !ok {
			t.Error("Expected cloudBackupDestinations.total")
		}
		if _, ok := cbd["byProvider"].(map[string]interface{}); !ok {
			t.Errorf("Expected cloudBackupDestinations.byProvider map, got %T", cbd["byProvider"])
		}
	})

	t.Run("admin config", func(t *testing.T) {
		resp := ac.GET("/admin/config")
		requireStatus(t, resp, http.StatusOK)
		var cfg map[string]interface{}
		resp.JSON(&cfg)
		if cfg["jwtSecret"] != nil {
			t.Error("Should not expose JWT secret")
		}
		if _, ok := cfg["cloudBackupsConfigured"]; !ok {
			t.Error("Expected cloudBackupsConfigured field")
		}
		if _, ok := cfg["cloudBackupProviders"]; !ok {
			t.Error("Expected cloudBackupProviders field")
		}
	})

	t.Run("admin list users", func(t *testing.T) {
		requireStatus(t, ac.GET("/admin/users"), http.StatusOK)
	})

	t.Run("admin search users", func(t *testing.T) {
		requireStatus(t, ac.GET(fmt.Sprintf("/admin/users?search=%s", ue)), http.StatusOK)
	})

	t.Run("admin disable user", func(t *testing.T) {
		requireStatus(t, ac.POST(fmt.Sprintf("/admin/users/%s/disable", ua.User.ID), nil), http.StatusOK)
		resp := uc.POST("/auth/login", map[string]string{"email": ue, "password": "UserPass123!"})
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 403/401, got %d", resp.StatusCode)
		}
	})

	t.Run("admin enable user", func(t *testing.T) {
		requireStatus(t, ac.POST(fmt.Sprintf("/admin/users/%s/enable", ua.User.ID), nil), http.StatusOK)
		requireStatus(t, uc.POST("/auth/login", map[string]string{"email": ue, "password": "UserPass123!"}), http.StatusOK)
	})

	t.Run("admin unlock", func(t *testing.T) {
		requireStatus(t, ac.POST(fmt.Sprintf("/admin/users/%s/unlock", ua.User.ID), nil), http.StatusOK)
	})

	t.Run("admin reset 2fa", func(t *testing.T) {
		resp := ac.POST(fmt.Sprintf("/admin/users/%s/reset-2fa", ua.User.ID), nil)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 200 or 400, got %d: %s", resp.StatusCode, string(resp.Body))
		}
	})

	t.Run("admin audit log", func(t *testing.T) {
		requireStatus(t, ac.GET("/admin/audit-log"), http.StatusOK)
	})

	t.Run("admin delete user removes account and content", func(t *testing.T) {
		// Create a fresh target user.
		dc := NewE2EClient(t)
		de := uniqueEmail("admin-delete-target")
		da := registerAndLogin(t, dc, de, "UserPass123!", "ToDelete")

		// Self-delete must be rejected. Look up the admin's own ID via search.
		ar := getAdminClient(t)
		selfResp := ar.GET("/admin/users?search=admin@ninerlog-test.com")
		requireStatus(t, selfResp, http.StatusOK)
		var selfList struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		selfResp.JSON(&selfList)
		if len(selfList.Data) > 0 {
			assertStatus(t, ar.DELETE(fmt.Sprintf("/admin/users/%s", selfList.Data[0].ID)), http.StatusBadRequest)
		}

		// Delete the target user. Cascading FKs remove all owned content.
		requireStatus(t, ar.DELETE(fmt.Sprintf("/admin/users/%s", da.User.ID)), http.StatusOK)

		// Login should now fail (account no longer exists).
		login := dc.POST("/auth/login", map[string]string{"email": de, "password": "UserPass123!"})
		if login.StatusCode == http.StatusOK {
			t.Errorf("Expected login to fail after delete, got %d", login.StatusCode)
		}

		// Second delete should 404.
		assertStatus(t, ar.DELETE(fmt.Sprintf("/admin/users/%s", da.User.ID)), http.StatusNotFound)
	})

	t.Run("admin cleanup tokens", func(t *testing.T) {
		requireStatus(t, ac.POST("/admin/maintenance/cleanup-tokens", nil), http.StatusOK)
	})

	t.Run("admin create announcement", func(t *testing.T) {
		requireStatus(t, ac.POST("/admin/announcements", map[string]interface{}{
			"message": "Test", "severity": "warning",
		}), http.StatusCreated)
	})
}

func TestAdminAccessControl(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("nonadmin"), "SecurePass123!", "Regular")

	t.Run("non-admin stats 403", func(t *testing.T) { assertStatus(t, c.GET("/admin/stats"), http.StatusForbidden) })
	t.Run("non-admin users 403", func(t *testing.T) { assertStatus(t, c.GET("/admin/users"), http.StatusForbidden) })
	t.Run("non-admin config 403", func(t *testing.T) { assertStatus(t, c.GET("/admin/config"), http.StatusForbidden) })
	t.Run("non-admin announce 403", func(t *testing.T) {
		assertStatus(t, c.POST("/admin/announcements", map[string]interface{}{"message": "X", "severity": "info"}), http.StatusForbidden)
	})
	t.Run("non-admin disable 403", func(t *testing.T) {
		assertStatus(t, c.POST("/admin/users/00000000-0000-0000-0000-000000000000/disable", nil), http.StatusForbidden)
	})
	t.Run("non-admin delete 403", func(t *testing.T) {
		assertStatus(t, c.DELETE("/admin/users/00000000-0000-0000-0000-000000000000"), http.StatusForbidden)
	})
	t.Run("unauth admin 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/admin/stats"), http.StatusUnauthorized)
	})
}

func TestAnnouncements(t *testing.T) {
	ac := getAdminClient(t)

	resp := ac.POST("/admin/announcements", map[string]interface{}{"message": "For listing", "severity": "info"})
	requireStatus(t, resp, http.StatusCreated)
	var ann map[string]interface{}
	resp.JSON(&ann)
	aid := ann["id"].(string)

	uc := NewE2EClient(t)
	registerAndLogin(t, uc, uniqueEmail("ann-viewer"), "SecurePass123!", "Viewer")

	t.Run("user sees announcements", func(t *testing.T) {
		requireStatus(t, uc.GET("/announcements"), http.StatusOK)
	})

	t.Run("admin deletes announcement", func(t *testing.T) {
		assertStatus(t, ac.DELETE(fmt.Sprintf("/admin/announcements/%s", aid)), http.StatusNoContent)
	})
}

// TestEmailDeliveryAdminEndpoints covers the deliverability surface: the send
// log, the suppression list, and the on-demand unverified-account sweep.
func TestEmailDeliveryAdminEndpoints(t *testing.T) {
	ac := getAdminClient(t)

	t.Run("delivery log lists recent sends", func(t *testing.T) {
		// Registering sends a verification email, which must appear in the log.
		uc := NewE2EClient(t)
		email := uniqueEmail("delivery-log")
		resp := uc.POST("/auth/register", map[string]string{
			"email": email, "password": "UserPass123!", "name": "Delivery Target",
		})
		requireStatus(t, resp, http.StatusCreated)

		logResp := ac.GET(fmt.Sprintf("/admin/email/deliveries?recipient=%s", email))
		requireStatus(t, logResp, http.StatusOK)

		var body struct {
			Data []struct {
				Recipient string `json:"recipient"`
				EmailType string `json:"emailType"`
				Status    string `json:"status"`
			} `json:"data"`
		}
		logResp.JSON(&body)

		if len(body.Data) == 0 {
			t.Fatalf("Expected a delivery event for %s", email)
		}
		found := false
		for _, e := range body.Data {
			if e.Recipient != email {
				t.Errorf("Recipient filter leaked %q into results for %q", e.Recipient, email)
			}
			if e.EmailType == "verify_email" {
				found = true
			}
			if e.Status == "" {
				t.Error("Expected a delivery status on every event")
			}
		}
		if !found {
			t.Errorf("Expected a verify_email event for %s, got %+v", email, body.Data)
		}
	})

	t.Run("delivery log honours the limit", func(t *testing.T) {
		resp := ac.GET("/admin/email/deliveries?limit=1")
		requireStatus(t, resp, http.StatusOK)
		var body struct {
			Data []map[string]interface{} `json:"data"`
		}
		resp.JSON(&body)
		if len(body.Data) > 1 {
			t.Errorf("Expected at most 1 event, got %d", len(body.Data))
		}
	})

	t.Run("suppression list is readable", func(t *testing.T) {
		resp := ac.GET("/admin/email/suppressions")
		requireStatus(t, resp, http.StatusOK)
		var body struct {
			Data []map[string]interface{} `json:"data"`
		}
		resp.JSON(&body)
		if body.Data == nil {
			t.Error("Expected a data array, even when empty")
		}
	})

	t.Run("lifting a suppression that does not exist returns 404", func(t *testing.T) {
		resp := ac.DELETE("/admin/email/suppressions/never-bounced@example.test")
		requireStatus(t, resp, http.StatusNotFound)
	})

	t.Run("unverified sweep is callable and reports counts", func(t *testing.T) {
		resp := ac.POST("/admin/maintenance/cleanup-unverified", nil)
		// 503 is the honest answer on a deployment that runs without the
		// reaper; anything else must be a well-formed result.
		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Skip("Unverified account cleanup is disabled on this deployment")
		}
		requireStatus(t, resp, http.StatusOK)

		var body struct {
			RemindersSent   int `json:"remindersSent"`
			AccountsDeleted int `json:"accountsDeleted"`
		}
		resp.JSON(&body)
		if body.RemindersSent < 0 || body.AccountsDeleted < 0 {
			t.Errorf("Nonsensical sweep result: %+v", body)
		}
	})

	t.Run("a freshly registered account is never swept away", func(t *testing.T) {
		// The reminder is only due a day after signup and deletion 30 days
		// after that, so a sweep immediately after registration must leave the
		// account able to verify and log in.
		uc := NewE2EClient(t)
		email := uniqueEmail("sweep-survivor")
		requireStatus(t, uc.POST("/auth/register", map[string]string{
			"email": email, "password": "UserPass123!", "name": "Survivor",
		}), http.StatusCreated)

		if resp := ac.POST("/admin/maintenance/cleanup-unverified", nil); resp.StatusCode == http.StatusServiceUnavailable {
			t.Skip("Unverified account cleanup is disabled on this deployment")
		}

		// Still registered: a duplicate signup is refused because the account
		// survived the sweep.
		dup := uc.POST("/auth/register", map[string]string{
			"email": email, "password": "UserPass123!", "name": "Survivor",
		})
		if dup.StatusCode == http.StatusCreated {
			t.Error("Account was reaped by an immediate sweep — the reminder delay is not being honoured")
		}
	})

	t.Run("admin email endpoints reject non-admins", func(t *testing.T) {
		uc := NewE2EClient(t)
		registerAndLogin(t, uc, uniqueEmail("email-nonadmin"), "UserPass123!", "Not Admin")

		for _, path := range []string{"/admin/email/deliveries", "/admin/email/suppressions"} {
			resp := uc.GET(path)
			if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("GET %s as non-admin: expected 401/403, got %d", path, resp.StatusCode)
			}
		}
		resp := uc.POST("/admin/maintenance/cleanup-unverified", nil)
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Sweep as non-admin: expected 401/403, got %d", resp.StatusCode)
		}
	})
}
