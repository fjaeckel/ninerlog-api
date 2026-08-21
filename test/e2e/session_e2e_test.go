//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
	"time"
)

// refreshReuseGrace mirrors REFRESH_REUSE_GRACE in docker-compose.e2e.yaml.
const refreshReuseGrace = time.Second

// maxSessionsPerUser mirrors MAX_SESSIONS_PER_USER in docker-compose.e2e.yaml.
const maxSessionsPerUser = 5

type sessionBody struct {
	ID          string `json:"id"`
	DeviceLabel string `json:"deviceLabel"`
	IPAddress   string `json:"ipAddress"`
	CreatedAt   string `json:"createdAt"`
	LastUsedAt  string `json:"lastUsedAt"`
	ExpiresAt   string `json:"expiresAt"`
	Current     bool   `json:"current"`
}

type sessionListBody struct {
	Sessions    []sessionBody `json:"sessions"`
	MaxSessions int           `json:"maxSessions"`
}

// loginAs signs in with a given User-Agent so the session gets a distinct
// device label.
func loginAs(t *testing.T, c *E2EClient, email, password, userAgent string) AuthResponseBody {
	t.Helper()
	resp := c.DoWithHeaders("POST", "/auth/login",
		map[string]string{"email": email, "password": password},
		map[string]string{"User-Agent": userAgent})
	requireStatus(t, resp, http.StatusOK)
	var auth AuthResponseBody
	resp.JSON(&auth)
	return auth
}

func listSessions(t *testing.T, c *E2EClient) sessionListBody {
	t.Helper()
	resp := c.GET("/auth/sessions")
	requireStatus(t, resp, http.StatusOK)
	var body sessionListBody
	resp.JSON(&body)
	return body
}

func TestSessionsSurviveASecondDevice(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("multisession")
	const password = "SecurePass123!"
	registerUser(t, c, email, password, "Multi Session")

	phone := loginAs(t, c, email, password, "Mozilla/5.0 (iPhone) AppleWebKit/605.1 Safari/604.1")
	laptop := loginAs(t, c, email, password, "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")

	t.Run("the first device is still signed in", func(t *testing.T) {
		c.SetToken(phone.AccessToken)
		assertStatus(t, c.GET("/users/me"), http.StatusOK)

		resp := c.POST("/auth/refresh", map[string]string{"refreshToken": phone.RefreshToken})
		requireStatus(t, resp, http.StatusOK)
		var refreshed AuthResponseBody
		resp.JSON(&refreshed)
		phone.RefreshToken = refreshed.RefreshToken
	})

	t.Run("both devices appear in the session list", func(t *testing.T) {
		c.SetToken(laptop.AccessToken)
		body := listSessions(t, c)

		if len(body.Sessions) != 2 {
			t.Fatalf("Expected 2 sessions, got %d", len(body.Sessions))
		}
		if body.MaxSessions != maxSessionsPerUser {
			t.Errorf("Expected maxSessions %d, got %d", maxSessionsPerUser, body.MaxSessions)
		}

		labels := map[string]bool{}
		current := 0
		for _, s := range body.Sessions {
			labels[s.DeviceLabel] = true
			if s.Current {
				current++
			}
		}
		if current != 1 {
			t.Errorf("Expected exactly 1 current session, got %d", current)
		}
		for _, want := range []string{"Safari on iPhone", "Chrome on macOS"} {
			if !labels[want] {
				t.Errorf("Expected a session labelled %q, got %v", want, labels)
			}
		}
	})
}

func TestSessionRevocation(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("revokesession")
	const password = "SecurePass123!"
	registerUser(t, c, email, password, "Revoke Session")

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

	t.Run("revoking a session ends it", func(t *testing.T) {
		assertStatus(t, c.DELETE("/auth/sessions/"+phoneSessionID), http.StatusNoContent)

		resp := c.POST("/auth/refresh", map[string]string{"refreshToken": phone.RefreshToken})
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("the revoking device keeps working", func(t *testing.T) {
		assertStatus(t, c.GET("/users/me"), http.StatusOK)
	})

	t.Run("revoking an unknown session returns 404", func(t *testing.T) {
		assertStatus(t, c.DELETE("/auth/sessions/00000000-0000-0000-0000-000000000000"), http.StatusNotFound)
	})

	t.Run("listing sessions without auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/auth/sessions"), http.StatusUnauthorized)
	})
}

func TestSessionsRevokeAllOthers(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("revokeothers")
	const password = "SecurePass123!"
	registerUser(t, c, email, password, "Revoke Others")

	older := loginAs(t, c, email, password, "Mozilla/5.0 (iPhone) AppleWebKit/605.1 Safari/604.1")
	loginAs(t, c, email, password, "Mozilla/5.0 (iPad) AppleWebKit/605.1 Safari/604.1")
	keeper := loginAs(t, c, email, password, "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")

	c.SetToken(keeper.AccessToken)
	resp := c.DELETE("/auth/sessions")
	requireStatus(t, resp, http.StatusOK)

	var body struct {
		Revoked int `json:"revoked"`
	}
	resp.JSON(&body)
	if body.Revoked != 2 {
		t.Errorf("Expected 2 sessions revoked, got %d", body.Revoked)
	}

	if sessions := listSessions(t, c).Sessions; len(sessions) != 1 {
		t.Errorf("Expected 1 remaining session, got %d", len(sessions))
	}

	assertStatus(t, c.POST("/auth/refresh",
		map[string]string{"refreshToken": older.RefreshToken}), http.StatusUnauthorized)
}

func TestSessionsCappedPerUser(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("sessioncap")
	const password = "SecurePass123!"
	registerUser(t, c, email, password, "Session Cap")

	first := loginAs(t, c, email, password, "Mozilla/5.0 (iPhone) AppleWebKit/605.1 Safari/604.1")

	var last AuthResponseBody
	for i := 0; i < maxSessionsPerUser; i++ {
		last = loginAs(t, c, email, password, "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	}

	c.SetToken(last.AccessToken)
	if sessions := listSessions(t, c).Sessions; len(sessions) != maxSessionsPerUser {
		t.Errorf("Expected %d sessions, got %d", maxSessionsPerUser, len(sessions))
	}

	// The cap evicts the oldest session rather than refusing the newest login.
	assertStatus(t, c.POST("/auth/refresh",
		map[string]string{"refreshToken": first.RefreshToken}), http.StatusUnauthorized)
}

func TestSessionReplayRevokesTheWholeSession(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("replay")
	const password = "SecurePass123!"
	auth := registerUser(t, c, email, password, "Replay")

	resp := c.POST("/auth/refresh", map[string]string{"refreshToken": auth.RefreshToken})
	requireStatus(t, resp, http.StatusOK)
	var rotated AuthResponseBody
	resp.JSON(&rotated)

	time.Sleep(refreshReuseGrace + 500*time.Millisecond)

	assertStatus(t, c.POST("/auth/refresh",
		map[string]string{"refreshToken": auth.RefreshToken}), http.StatusUnauthorized)

	// The live token of a replayed session is revoked with it.
	assertStatus(t, c.POST("/auth/refresh",
		map[string]string{"refreshToken": rotated.RefreshToken}), http.StatusUnauthorized)
}
