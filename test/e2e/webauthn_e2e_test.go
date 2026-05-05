//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

// TestWebAuthnEndpoints exercises the contract / wiring of the new
// /auth/webauthn/* endpoints without performing a full ceremony — full
// register/login flows are exercised from the frontend Playwright suite using
// Chromium's virtual authenticator. The server must be configured with
// WEBAUTHN_RP_ID for these tests to be meaningful.
func TestWebAuthnEndpoints(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("register/options requires authentication", func(t *testing.T) {
		c.ClearToken()
		resp := c.POST("/auth/webauthn/register/options", nil)
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("credentials list requires authentication", func(t *testing.T) {
		c.ClearToken()
		resp := c.GET("/auth/webauthn/credentials")
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("delete credential requires authentication", func(t *testing.T) {
		c.ClearToken()
		resp := c.DELETE("/auth/webauthn/credentials/00000000-0000-0000-0000-000000000000")
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("login/options is public and returns a discoverable challenge", func(t *testing.T) {
		c.ClearToken()
		resp := c.POST("/auth/webauthn/login/options", map[string]any{})
		// Either the server has WebAuthn configured (200) or it doesn't (503).
		// Both states are acceptable in CI; we only assert it is *not* 401/404.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("unexpected status %d (body=%s)", resp.StatusCode, string(resp.Body))
		}
		if resp.StatusCode == http.StatusOK {
			var body struct {
				SessionID string         `json:"sessionId"`
				PublicKey map[string]any `json:"publicKey"`
			}
			if err := resp.JSON(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.SessionID == "" {
				t.Errorf("expected sessionId, got empty")
			}
			if _, ok := body.PublicKey["challenge"]; !ok {
				t.Errorf("expected publicKey.challenge in response, got %v", body.PublicKey)
			}
		}
	})

	t.Run("login/options with unknown email does not enumerate users", func(t *testing.T) {
		c.ClearToken()
		resp := c.POST("/auth/webauthn/login/options", map[string]string{
			"email": "definitely-not-registered-" + uniqueEmail("noenum"),
		})
		// Same as above — must return options or 503, never 404 / 401.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("unexpected status %d (body=%s) — would leak account existence",
				resp.StatusCode, string(resp.Body))
		}
	})

	t.Run("login/verify with malformed body is 400", func(t *testing.T) {
		c.ClearToken()
		resp := c.POST("/auth/webauthn/login/verify", map[string]any{
			"sessionId": "not-a-uuid",
			"response":  map[string]any{"id": "garbage"},
		})
		// Either 503 (disabled) or 400 (bad request — invalid response).
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("unexpected status %d (body=%s)", resp.StatusCode, string(resp.Body))
		}
	})

	t.Run("authenticated user can list (initially empty) and start registration", func(t *testing.T) {
		client := NewE2EClient(t)
		registerAndLogin(t, client, uniqueEmail("webauthn"), "SecurePass123!", "WebAuthn User")

		listResp := client.GET("/auth/webauthn/credentials")
		// 200 with empty array if WebAuthn is enabled, 503 otherwise.
		if listResp.StatusCode != http.StatusOK && listResp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("unexpected list status %d (body=%s)", listResp.StatusCode, string(listResp.Body))
		}
		if listResp.StatusCode == http.StatusOK {
			var creds []map[string]any
			if err := listResp.JSON(&creds); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			if len(creds) != 0 {
				t.Errorf("expected no passkeys for new user, got %d", len(creds))
			}
		}

		optsResp := client.POST("/auth/webauthn/register/options", nil)
		if optsResp.StatusCode != http.StatusOK && optsResp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("unexpected register/options status %d (body=%s)", optsResp.StatusCode, string(optsResp.Body))
		}
		if optsResp.StatusCode == http.StatusOK {
			var body struct {
				SessionID string         `json:"sessionId"`
				PublicKey map[string]any `json:"publicKey"`
			}
			if err := optsResp.JSON(&body); err != nil {
				t.Fatalf("decode options: %v", err)
			}
			if body.SessionID == "" {
				t.Errorf("expected sessionId in registration options")
			}
			if _, ok := body.PublicKey["challenge"]; !ok {
				t.Errorf("expected publicKey.challenge in registration options, got %v", body.PublicKey)
			}
		}

		// Deleting a non-existent credential as the authenticated user should 404 (not 401/500).
		delResp := client.DELETE("/auth/webauthn/credentials/00000000-0000-0000-0000-000000000000")
		if delResp.StatusCode != http.StatusNotFound && delResp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("unexpected delete status %d (body=%s)", delResp.StatusCode, string(delResp.Body))
		}
	})
}
