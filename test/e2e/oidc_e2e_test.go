//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

// The e2e stack runs in local-credential mode (no OIDC_ISSUER). These tests
// cover that default: the capability probe advertises local sign-in and the
// OIDC endpoints exist but stay closed.

func TestAuthProvidersProbe(t *testing.T) {
	c := NewE2EClient(t)
	c.ClearToken()

	resp := c.GET("/auth/providers")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Mode                 string `json:"mode"`
		PasswordLoginEnabled bool   `json:"passwordLoginEnabled"`
		RegistrationEnabled  bool   `json:"registrationEnabled"`
		TwoFactorEnabled     bool   `json:"twoFactorEnabled"`
		WebauthnEnabled      bool   `json:"webauthnEnabled"`
		OIDC                 struct {
			Enabled      bool   `json:"enabled"`
			Name         string `json:"name"`
			AuthorizeURL string `json:"authorizeUrl"`
		} `json:"oidc"`
	}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Mode != "local" {
		t.Fatalf("mode = %q, want local — the e2e stack configures no identity provider", body.Mode)
	}
	if !body.PasswordLoginEnabled || !body.RegistrationEnabled || !body.TwoFactorEnabled {
		t.Errorf("local mode must advertise password login, registration and 2FA: %+v", body)
	}
	if body.OIDC.Enabled {
		t.Error("oidc.enabled must be false without OIDC_ISSUER")
	}
}

func TestOIDCEndpointsClosedInLocalMode(t *testing.T) {
	c := NewE2EClient(t)
	c.ClearToken()

	t.Run("authorize", func(t *testing.T) {
		resp := c.GET("/auth/oidc/authorize")
		assertStatus(t, resp, http.StatusServiceUnavailable)
	})

	t.Run("callback", func(t *testing.T) {
		resp := c.GET("/auth/oidc/callback?code=x&state=y")
		assertStatus(t, resp, http.StatusServiceUnavailable)
	})

	t.Run("exchange", func(t *testing.T) {
		resp := c.POST("/auth/oidc/exchange", map[string]any{"code": "anything"})
		assertStatus(t, resp, http.StatusServiceUnavailable)
		// A disabled exchange returns no token material.
		if body := string(resp.Body); strings.Contains(body, "accessToken") {
			t.Errorf("exchange returned token material while disabled: %s", body)
		}
	})
}

func TestLocalAuthStillWorksWithoutOIDC(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("oidc-local-mode"), "SecurePass123!", "Local Mode")

	resp := c.GET("/users/me")
	assertStatus(t, resp, http.StatusOK)
}
