//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

// Ending a session ends its access token, on every path that ends a session.

func TestRevokedSessionEndsItsAccessToken(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("accesstokenrevoke")
	const password = "SecurePass123!"
	registerOnly(t, c, email, password, "Access Revoke")

	phone := loginAs(t, c, email, password, "Mozilla/5.0 (iPhone) AppleWebKit/605.1 Safari/604.1")
	laptop := loginAs(t, c, email, password, "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")

	c.SetToken(laptop.AccessToken)
	var phoneSessionID string
	for _, s := range listSessions(t, c).Sessions {
		if !s.Current {
			phoneSessionID = s.ID
		}
	}
	if phoneSessionID == "" {
		t.Fatal("Could not identify the other device's session")
	}
	requireStatus(t, c.DELETE("/auth/sessions/"+phoneSessionID), http.StatusNoContent)

	t.Run("the revoked device's access token stops working", func(t *testing.T) {
		c.SetToken(phone.AccessToken)
		assertStatus(t, c.GET("/users/me"), http.StatusUnauthorized)
	})

	t.Run("it cannot write either", func(t *testing.T) {
		c.SetToken(phone.AccessToken)
		assertStatus(t, c.POST("/aircraft", map[string]any{
			"registration": "D-EREV", "type": "C172", "manufacturer": "Cessna", "model": "172S",
		}), http.StatusUnauthorized)
	})

	t.Run("the revoking device is untouched", func(t *testing.T) {
		c.SetToken(laptop.AccessToken)
		assertStatus(t, c.GET("/users/me"), http.StatusOK)
	})
}

func TestLogoutEndsItsAccessToken(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("logoutaccess")
	const password = "SecurePass123!"
	registerOnly(t, c, email, password, "Logout Access")

	auth := loginAs(t, c, email, password, "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	c.SetToken(auth.AccessToken)
	requireStatus(t, c.GET("/users/me"), http.StatusOK)

	requireStatus(t, c.POST("/auth/logout",
		map[string]string{"refreshToken": auth.RefreshToken}), http.StatusNoContent)

	assertStatus(t, c.GET("/users/me"), http.StatusUnauthorized)
}

func TestPasswordChangeEndsEveryAccessToken(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("pwchangeaccess")
	const password = "SecurePass123!"
	const newPassword = "EvenMoreSecure456!"
	registerOnly(t, c, email, password, "Password Change Access")

	phone := loginAs(t, c, email, password, "Mozilla/5.0 (iPhone) AppleWebKit/605.1 Safari/604.1")
	laptop := loginAs(t, c, email, password, "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")

	c.SetToken(laptop.AccessToken)
	requireStatus(t, c.POST("/auth/change-password", map[string]string{
		"currentPassword": password, "newPassword": newPassword,
	}), http.StatusNoContent)

	t.Run("the other device is evicted", func(t *testing.T) {
		c.SetToken(phone.AccessToken)
		assertStatus(t, c.GET("/users/me"), http.StatusUnauthorized)
	})

	t.Run("so is the device that changed it", func(t *testing.T) {
		c.SetToken(laptop.AccessToken)
		assertStatus(t, c.GET("/users/me"), http.StatusUnauthorized)
	})

	t.Run("signing in with the new password works", func(t *testing.T) {
		fresh := loginAs(t, c, email, newPassword, "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
		c.SetToken(fresh.AccessToken)
		assertStatus(t, c.GET("/users/me"), http.StatusOK)
	})
}

// An access token issued before a rotation stays valid until it expires.
func TestRotationKeepsTheCurrentAccessTokenValid(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("rotationaccess")
	const password = "SecurePass123!"
	registerOnly(t, c, email, password, "Rotation Access")

	auth := loginAs(t, c, email, password, "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")

	resp := c.POST("/auth/refresh", map[string]string{"refreshToken": auth.RefreshToken})
	requireStatus(t, resp, http.StatusOK)
	var refreshed AuthResponseBody
	resp.JSON(&refreshed)

	t.Run("the pre-rotation access token still works", func(t *testing.T) {
		c.SetToken(auth.AccessToken)
		assertStatus(t, c.GET("/users/me"), http.StatusOK)
	})

	t.Run("so does the new one", func(t *testing.T) {
		c.SetToken(refreshed.AccessToken)
		assertStatus(t, c.GET("/users/me"), http.StatusOK)
	})
}
