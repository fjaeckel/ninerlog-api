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
	// Try to register admin; if already exists, just login
	resp := ac.POST("/auth/register", map[string]string{
		"email": "admin@ninerlog-test.com", "password": "AdminPass123!", "name": "Admin",
	})
	if resp.StatusCode == http.StatusConflict {
		auth := loginUser(t, ac, "admin@ninerlog-test.com", "AdminPass123!")
		ac.SetToken(auth.AccessToken)
	} else {
		requireStatus(t, resp, http.StatusCreated)
		var auth AuthResponseBody
		resp.JSON(&auth)
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
	})

	t.Run("admin config", func(t *testing.T) {
		resp := ac.GET("/admin/config")
		requireStatus(t, resp, http.StatusOK)
		var cfg map[string]interface{}
		resp.JSON(&cfg)
		if cfg["jwtSecret"] != nil {
			t.Error("Should not expose JWT secret")
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
