//go:build e2e

package e2e_test

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestPasswordResetRequest(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("request reset for existing user returns 204", func(t *testing.T) {
		email := uniqueEmail("pwreset-req")
		registerUser(t, c, email, "SecurePass123!", "Reset User")
		c.ClearToken()

		mailpitDeleteAll(t)

		resp := c.POST("/auth/password-reset-request", map[string]string{"email": email})
		assertStatus(t, resp, http.StatusNoContent)
	})

	t.Run("request reset for non-existent email returns 204", func(t *testing.T) {
		resp := c.POST("/auth/password-reset-request", map[string]string{"email": "nobody@example.com"})
		assertStatus(t, resp, http.StatusNoContent)
	})

	t.Run("request reset sends email", func(t *testing.T) {
		email := uniqueEmail("pwreset-email")
		registerUser(t, c, email, "SecurePass123!", "Email User")
		c.ClearToken()

		mailpitDeleteAll(t)

		resp := c.POST("/auth/password-reset-request", map[string]string{"email": email})
		requireStatus(t, resp, http.StatusNoContent)

		// Wait briefly for email delivery
		time.Sleep(500 * time.Millisecond)

		msg := mailpitRequireEmail(t, email, "Password Reset")
		if msg.HTML == "" {
			t.Error("Expected HTML body in reset email")
		}
		if !regexp.MustCompile(`/new-password\?token=`).MatchString(msg.HTML) {
			t.Errorf("Expected reset link in email body, got: %s", msg.HTML[:200])
		}
	})

	t.Run("request reset with invalid body returns 400", func(t *testing.T) {
		resp := c.POST("/auth/password-reset-request", "not-json")
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

func TestPasswordReset(t *testing.T) {
	c := NewE2EClient(t)

	// Helper: request a password reset and extract the token from the email
	requestResetToken := func(t *testing.T, email string) string {
		t.Helper()
		mailpitDeleteAll(t)

		resp := c.POST("/auth/password-reset-request", map[string]string{"email": email})
		requireStatus(t, resp, http.StatusNoContent)

		time.Sleep(500 * time.Millisecond)

		msg := mailpitRequireEmail(t, email, "Password Reset")
		re := regexp.MustCompile(`token=([A-Za-z0-9_\-=]+)`)
		matches := re.FindStringSubmatch(msg.HTML)
		if len(matches) < 2 {
			t.Fatalf("Could not extract token from email body")
		}
		return matches[1]
	}

	t.Run("reset password with valid token", func(t *testing.T) {
		email := uniqueEmail("pwreset-ok")
		oldPw := "OldPassword123!"
		newPw := "NewPassword456!"

		registerUser(t, c, email, oldPw, "Reset OK")
		c.ClearToken()

		token := requestResetToken(t, email)

		resp := c.POST("/auth/password-reset", map[string]string{
			"token":       token,
			"newPassword": newPw,
		})
		assertStatus(t, resp, http.StatusNoContent)

		// Login with new password should succeed
		auth := loginUser(t, c, email, newPw)
		if auth.AccessToken == "" {
			t.Error("Expected accessToken after login with new password")
		}

		// Login with old password should fail
		resp = c.POST("/auth/login", map[string]string{"email": email, "password": oldPw})
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("reset with invalid token returns 400", func(t *testing.T) {
		resp := c.POST("/auth/password-reset", map[string]string{
			"token":       "invalid-token",
			"newPassword": "NewPassword456!",
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("reset with already used token returns 400", func(t *testing.T) {
		email := uniqueEmail("pwreset-used")
		registerUser(t, c, email, "SecurePass123!", "Used Token")
		c.ClearToken()

		token := requestResetToken(t, email)

		// First use: should succeed
		resp := c.POST("/auth/password-reset", map[string]string{
			"token":       token,
			"newPassword": "FirstReset123!",
		})
		requireStatus(t, resp, http.StatusNoContent)

		// Second use: should fail
		resp = c.POST("/auth/password-reset", map[string]string{
			"token":       token,
			"newPassword": "SecondReset123!",
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("reset with short password returns 400", func(t *testing.T) {
		email := uniqueEmail("pwreset-short")
		registerUser(t, c, email, "SecurePass123!", "Short PW")
		c.ClearToken()

		token := requestResetToken(t, email)

		resp := c.POST("/auth/password-reset", map[string]string{
			"token":       token,
			"newPassword": "short",
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("reset revokes all refresh tokens", func(t *testing.T) {
		email := uniqueEmail("pwreset-revoke")
		oldPw := "SecurePass123!"
		newPw := "NewPassword456!"

		auth := registerUser(t, c, email, oldPw, "Revoke Test")
		refreshToken := auth.RefreshToken
		c.ClearToken()

		token := requestResetToken(t, email)

		resp := c.POST("/auth/password-reset", map[string]string{
			"token":       token,
			"newPassword": newPw,
		})
		requireStatus(t, resp, http.StatusNoContent)

		// Old refresh token should be invalidated
		resp = c.POST("/auth/refresh", map[string]string{"refreshToken": refreshToken})
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("reset with missing fields returns 400", func(t *testing.T) {
		resp := c.POST("/auth/password-reset", map[string]string{})
		// Empty token is treated as invalid token
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("reset sends a password-changed notice", func(t *testing.T) {
		email := uniqueEmail("pwreset-notice")
		newPw := "NewPassword456!"

		registerUser(t, c, email, "SecurePass123!", "Notice User")
		c.ClearToken()

		token := requestResetToken(t, email)
		resp := c.POST("/auth/password-reset", map[string]string{
			"token":       token,
			"newPassword": newPw,
		})
		requireStatus(t, resp, http.StatusNoContent)

		time.Sleep(500 * time.Millisecond)

		msg := mailpitRequireEmail(t, email, "Your password was changed")
		if msg.HTML == "" {
			t.Error("Expected HTML body in the password-changed notice")
		}
	})

	// A reset must NOT be a way around 2FA. Control of the mailbox gets an
	// attacker the reset link; if that link also stripped the second factor,
	// the mailbox alone would be enough for a full account takeover.
	t.Run("reset requires 2FA and keeps it enabled", func(t *testing.T) {
		email := uniqueEmail("pwreset-2fa")
		pw := "SecurePass123!"
		newPw := "NewPassword456!"

		// Register and setup 2FA
		auth := registerUser(t, c, email, pw, "2FA Reset")
		c.SetToken(auth.AccessToken)

		// Setup 2FA
		resp := c.POST("/auth/2fa/setup", nil)
		requireStatus(t, resp, http.StatusOK)
		var setup map[string]interface{}
		resp.JSON(&setup)
		secret := setup["secret"].(string)

		// Verify 2FA with valid TOTP code
		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("Failed to generate TOTP: %v", err)
		}
		resp = c.POST("/auth/2fa/verify", map[string]string{"code": code})
		requireStatus(t, resp, http.StatusOK)

		// Confirm login now requires 2FA
		c.ClearToken()
		resp = c.POST("/auth/login", map[string]string{"email": email, "password": pw})
		requireStatus(t, resp, http.StatusOK)
		var loginResp AuthResponseBody
		resp.JSON(&loginResp)
		if !loginResp.RequiresTwoFactor {
			t.Fatal("Expected 2FA to be required before reset")
		}

		token := requestResetToken(t, email)

		// Without a code the reset is refused, and the token is not consumed.
		resp = c.POST("/auth/password-reset", map[string]string{
			"token":       token,
			"newPassword": newPw,
		})
		assertStatus(t, resp, http.StatusUnauthorized)
		var errBody struct {
			Code string `json:"code"`
		}
		resp.JSON(&errBody)
		if errBody.Code != "two_factor_required" {
			t.Errorf("Expected code two_factor_required, got %q", errBody.Code)
		}

		// A wrong code is refused too.
		resp = c.POST("/auth/password-reset", map[string]string{
			"token":         token,
			"newPassword":   newPw,
			"twoFactorCode": "000000",
		})
		assertStatus(t, resp, http.StatusUnauthorized)
		resp.JSON(&errBody)
		if errBody.Code != "invalid_two_factor_code" {
			t.Errorf("Expected code invalid_two_factor_code, got %q", errBody.Code)
		}

		// The old password must still work at this point.
		resp = c.POST("/auth/login", map[string]string{"email": email, "password": pw})
		requireStatus(t, resp, http.StatusOK)

		// With a valid TOTP code the reset goes through.
		resetCode, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("Failed to generate TOTP: %v", err)
		}
		resp = c.POST("/auth/password-reset", map[string]string{
			"token":         token,
			"newPassword":   newPw,
			"twoFactorCode": resetCode,
		})
		requireStatus(t, resp, http.StatusNoContent)

		// The new password works — and 2FA is still in force.
		resp = c.POST("/auth/login", map[string]string{"email": email, "password": newPw})
		requireStatus(t, resp, http.StatusOK)
		var afterReset AuthResponseBody
		resp.JSON(&afterReset)
		if !afterReset.RequiresTwoFactor {
			t.Error("Expected 2FA to still be required after the password reset")
		}
		if afterReset.AccessToken != "" {
			t.Error("Expected no accessToken before the 2FA challenge is answered")
		}
	})

	// The self-service escape hatch when the authenticator is gone: a recovery
	// code stands in for the TOTP code, so no admin has to intervene.
	t.Run("reset accepts a recovery code", func(t *testing.T) {
		email := uniqueEmail("pwreset-recovery")
		pw := "SecurePass123!"
		newPw := "NewPassword456!"

		auth := registerUser(t, c, email, pw, "Recovery Reset")
		c.SetToken(auth.AccessToken)

		resp := c.POST("/auth/2fa/setup", nil)
		requireStatus(t, resp, http.StatusOK)
		var setup map[string]interface{}
		resp.JSON(&setup)
		secret := setup["secret"].(string)

		code, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("Failed to generate TOTP: %v", err)
		}
		resp = c.POST("/auth/2fa/verify", map[string]string{"code": code})
		requireStatus(t, resp, http.StatusOK)
		var verify struct {
			RecoveryCodes []string `json:"recoveryCodes"`
		}
		resp.JSON(&verify)
		if len(verify.RecoveryCodes) == 0 {
			t.Fatal("Expected recovery codes after enabling 2FA")
		}
		c.ClearToken()

		token := requestResetToken(t, email)
		resp = c.POST("/auth/password-reset", map[string]string{
			"token":         token,
			"newPassword":   newPw,
			"twoFactorCode": verify.RecoveryCodes[0],
		})
		requireStatus(t, resp, http.StatusNoContent)

		// 2FA survives, and the used recovery code is spent.
		resp = c.POST("/auth/login", map[string]string{"email": email, "password": newPw})
		requireStatus(t, resp, http.StatusOK)
		var afterReset AuthResponseBody
		resp.JSON(&afterReset)
		if !afterReset.RequiresTwoFactor {
			t.Error("Expected 2FA to still be required after a recovery-code reset")
		}

		resp = c.POST("/auth/2fa/login", map[string]string{
			"twoFactorToken": afterReset.TwoFactorToken,
			"code":           verify.RecoveryCodes[0],
		})
		assertStatus(t, resp, http.StatusUnauthorized)
	})
}
