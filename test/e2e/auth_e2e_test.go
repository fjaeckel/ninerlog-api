//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestAuthRegistration(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("register with valid data", func(t *testing.T) {
		email := uniqueEmail("reg-valid")
		auth := registerUser(t, c, email, "SecurePass123!", "Test User")
		if auth.AccessToken == "" {
			t.Error("Expected accessToken")
		}
		if auth.RefreshToken == "" {
			t.Error("Expected refreshToken")
		}
		if auth.User.Email != email {
			t.Errorf("Expected email %s, got %s", email, auth.User.Email)
		}
	})

	t.Run("register duplicate email returns 409", func(t *testing.T) {
		email := uniqueEmail("reg-dup")
		registerUser(t, c, email, "SecurePass123!", "First")
		resp := c.POST("/auth/register", map[string]string{"email": email, "password": "SecurePass123!", "name": "Second"})
		assertStatus(t, resp, http.StatusConflict)
	})

	// Fixed: API now validates password length (12-72 chars)
	t.Run("register with short password should return 400", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{"email": uniqueEmail("short-pw"), "password": "short", "name": "Test"})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("register with 11-char password should return 400", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{"email": uniqueEmail("11char-pw"), "password": "Abcdefghij1", "name": "Test"})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("register with 12-char password should succeed", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{"email": uniqueEmail("12char-pw"), "password": "Abcdefghij12", "name": "Test"})
		requireStatus(t, resp, http.StatusCreated)
	})

	t.Run("register with empty email returns 400", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{"email": "", "password": "SecurePass123!", "name": "Test"})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// Fixed: API now rejects empty name
	t.Run("register with empty name should return 400", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{"email": uniqueEmail("no-name"), "password": "SecurePass123!", "name": ""})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// Fixed: API now validates required fields
	t.Run("register with missing fields should return 400", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{"email": uniqueEmail("missing")})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// Fixed: Email now normalized to lowercase
	t.Run("register email should be case insensitive", func(t *testing.T) {
		email := uniqueEmail("case")
		registerUser(t, c, email, "SecurePass123!", "Test")
		resp := c.POST("/auth/register", map[string]string{"email": strings.ToUpper(email), "password": "SecurePass123!", "name": "Other"})
		assertStatus(t, resp, http.StatusConflict)
	})
}

func TestAuthLogin(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("login")
	pw := "SecurePass123!"
	registerUser(t, c, email, pw, "Login Test")

	t.Run("login correct credentials", func(t *testing.T) {
		auth := loginUser(t, c, email, pw)
		if auth.AccessToken == "" {
			t.Error("Expected accessToken")
		}
	})

	t.Run("login wrong password returns 401", func(t *testing.T) {
		resp := c.POST("/auth/login", map[string]string{"email": email, "password": "Wrong123!"})
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("login nonexistent email returns 401", func(t *testing.T) {
		resp := c.POST("/auth/login", map[string]string{"email": "noone@x.com", "password": pw})
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

func TestAuthTokenRefresh(t *testing.T) {
	c := NewE2EClient(t)
	auth := registerUser(t, c, uniqueEmail("refresh"), "SecurePass123!", "Refresh")

	t.Run("refresh with valid token", func(t *testing.T) {
		resp := c.POST("/auth/refresh", map[string]string{"refreshToken": auth.RefreshToken})
		requireStatus(t, resp, http.StatusOK)
		var newAuth AuthResponseBody
		resp.JSON(&newAuth)
		if newAuth.AccessToken == "" {
			t.Error("Expected new accessToken")
		}
		if newAuth.RefreshToken == auth.RefreshToken {
			t.Error("Expected token rotation")
		}
	})

	t.Run("refresh already used token fails", func(t *testing.T) {
		resp := c.POST("/auth/refresh", map[string]string{"refreshToken": auth.RefreshToken})
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 401/400, got %d", resp.StatusCode)
		}
	})

	t.Run("refresh invalid token fails", func(t *testing.T) {
		resp := c.POST("/auth/refresh", map[string]string{"refreshToken": "invalid.token"})
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

func TestAuthChangePassword(t *testing.T) {
	c := NewE2EClient(t)
	email := uniqueEmail("chpw")
	oldPw := "OldPassword123!"
	newPw := "NewPassword456!"
	auth := registerUser(t, c, email, oldPw, "PW Change")
	c.SetToken(auth.AccessToken)

	t.Run("change password success", func(t *testing.T) {
		resp := c.POST("/auth/change-password", map[string]string{"currentPassword": oldPw, "newPassword": newPw})
		assertStatus(t, resp, http.StatusNoContent)
	})

	t.Run("login with new password", func(t *testing.T) {
		a := loginUser(t, c, email, newPw)
		if a.AccessToken == "" {
			t.Error("Expected accessToken")
		}
	})

	t.Run("login with old password fails", func(t *testing.T) {
		resp := c.POST("/auth/login", map[string]string{"email": email, "password": oldPw})
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("change password with wrong current fails", func(t *testing.T) {
		a := loginUser(t, c, email, newPw)
		c.SetToken(a.AccessToken)
		resp := c.POST("/auth/change-password", map[string]string{"currentPassword": "Wrong!", "newPassword": "Another789!"})
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 401/400, got %d", resp.StatusCode)
		}
	})

	t.Run("change password without auth returns 401", func(t *testing.T) {
		c.ClearToken()
		resp := c.POST("/auth/change-password", map[string]string{"currentPassword": newPw, "newPassword": "X"})
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}

func TestAuthProtectedEndpoints(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("no token returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.GET("/users/me"), http.StatusUnauthorized)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		c.SetToken("invalid.jwt.token")
		assertStatus(t, c.GET("/users/me"), http.StatusUnauthorized)
	})

	t.Run("public endpoints work without auth", func(t *testing.T) {
		c.ClearToken()
		resp := c.GET("/airports/search?q=EDNY")
		if resp.StatusCode == http.StatusUnauthorized {
			t.Error("Airport search should be public")
		}
	})
}

func TestEmailVerification(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("register sends verification email", func(t *testing.T) {
		email := uniqueEmail("verify-send")
		mailpitDeleteAll(t)
		resp := c.POST("/auth/register", map[string]string{
			"email": email, "password": "SecurePass123!", "name": "Verify Send",
		})
		requireStatus(t, resp, http.StatusCreated)
		// Body should not contain tokens — only registration confirmation.
		var body map[string]any
		resp.JSON(&body)
		if _, hasAccess := body["accessToken"]; hasAccess {
			t.Errorf("Expected no accessToken in register response, got %v", body)
		}
		// A verification email must be delivered.
		_ = mailpitRequireEmail(t, email, "Confirm your email")
	})

	t.Run("login before verification returns 403 with code email_not_verified", func(t *testing.T) {
		email := uniqueEmail("verify-block")
		pw := "SecurePass123!"
		resp := c.POST("/auth/register", map[string]string{
			"email": email, "password": pw, "name": "Blocked",
		})
		requireStatus(t, resp, http.StatusCreated)

		loginResp := c.POST("/auth/login", map[string]string{"email": email, "password": pw})
		assertStatus(t, loginResp, http.StatusForbidden)
		var errBody struct {
			Code string `json:"code"`
		}
		loginResp.JSON(&errBody)
		if errBody.Code != "email_not_verified" {
			t.Errorf("Expected error code 'email_not_verified', got %q (body: %s)", errBody.Code, string(loginResp.Body))
		}
	})

	t.Run("verify-email returns auth tokens and unlocks login", func(t *testing.T) {
		email := uniqueEmail("verify-ok")
		pw := "SecurePass123!"
		resp := c.POST("/auth/register", map[string]string{
			"email": email, "password": pw, "name": "Verify OK",
		})
		requireStatus(t, resp, http.StatusCreated)

		token := extractVerificationToken(t, email)
		verifyResp := c.POST("/auth/verify-email", map[string]string{"token": token})
		requireStatus(t, verifyResp, http.StatusOK)
		var auth AuthResponseBody
		verifyResp.JSON(&auth)
		if auth.AccessToken == "" || auth.RefreshToken == "" {
			t.Errorf("Expected tokens after verify-email, got %+v", auth)
		}

		// Login is now permitted.
		ok := loginUser(t, c, email, pw)
		if ok.AccessToken == "" {
			t.Error("Expected login to succeed after verification")
		}
	})

	t.Run("verify-email with invalid token returns 400", func(t *testing.T) {
		resp := c.POST("/auth/verify-email", map[string]string{"token": "not-a-real-token"})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("verify-email rejects already-used token", func(t *testing.T) {
		email := uniqueEmail("verify-reuse")
		resp := c.POST("/auth/register", map[string]string{
			"email": email, "password": "SecurePass123!", "name": "Reuse",
		})
		requireStatus(t, resp, http.StatusCreated)
		token := extractVerificationToken(t, email)
		first := c.POST("/auth/verify-email", map[string]string{"token": token})
		requireStatus(t, first, http.StatusOK)
		second := c.POST("/auth/verify-email", map[string]string{"token": token})
		assertStatus(t, second, http.StatusBadRequest)
	})

	t.Run("resend verification always returns 204", func(t *testing.T) {
		// Unverified user: should send a new email.
		email := uniqueEmail("verify-resend")
		mailpitDeleteAll(t)
		regResp := c.POST("/auth/register", map[string]string{
			"email": email, "password": "SecurePass123!", "name": "Resend",
		})
		requireStatus(t, regResp, http.StatusCreated)
		// Drain the original verification email so the next assertion
		// only sees the resent one.
		_ = mailpitRequireEmail(t, email, "Confirm your email")
		mailpitDeleteAll(t)

		resendResp := c.POST("/auth/verify-email/resend", map[string]string{"email": email})
		assertStatus(t, resendResp, http.StatusNoContent)
		_ = mailpitRequireEmail(t, email, "Confirm your email")

		// Unknown email: still 204, no enumeration.
		unknownResp := c.POST("/auth/verify-email/resend", map[string]string{
			"email": uniqueEmail("verify-unknown"),
		})
		assertStatus(t, unknownResp, http.StatusNoContent)
	})
}
