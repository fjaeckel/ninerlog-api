//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

// The e2e stack runs in the default local-credential mode (no OIDC_ISSUER),
// which is the configuration ninerlog.com and every fresh self-hosted install
// use. These tests lock in that default: the capability probe must advertise
// local sign-in, the OIDC endpoints must exist but stay closed, and none of
// them may become a way in.
//
// The OIDC flow itself is covered at unit level against a fake provider
// (internal/service/oidc_test.go) — an e2e stack cannot mint ID tokens for a
// real issuer.

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
		// A disabled feature must not be a token oracle.
		if body := string(resp.Body); strings.Contains(body, "accessToken") {
			t.Errorf("exchange returned token material while disabled: %s", body)
		}
	})
}

func TestLocalAuthStillWorksWithoutOIDC(t *testing.T) {
	// The mirror of the OIDC-mode gate: with no provider configured, the
	// ordinary login path must be untouched by any of this.
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("oidc-local-mode"), "SecurePass123!", "Local Mode")

	resp := c.GET("/users/me")
	assertStatus(t, resp, http.StatusOK)
}
