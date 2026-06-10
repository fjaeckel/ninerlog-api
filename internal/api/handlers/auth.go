package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// RegisterUser implements POST /auth/register
// (POST /auth/register)
func (h *APIHandler) RegisterUser(c *gin.Context) {
	var req generated.RegisterUserJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, verificationToken, err := h.authService.Register(c.Request.Context(), service.RegisterInput{
		Email:           string(req.Email),
		Password:        req.Password,
		Name:            req.Name,
		PreferredLocale: preferredLocaleString(req.PreferredLocale),
	})

	if err != nil {
		if err == service.ErrUserAlreadyExists {
			h.sendError(c, http.StatusConflict, "Email already exists")
			return
		}
		if err == service.ErrPasswordTooShort || err == service.ErrPasswordTooLong ||
			err == service.ErrEmailRequired || err == service.ErrPasswordRequired ||
			err == service.ErrNameRequired || err == service.ErrInvalidEmail ||
			err == service.ErrEmailTooLong {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Registration failed")
		return
	}

	verificationRequired := h.emailSender != nil && h.emailSender.IsConfigured()
	message := "A verification email has been sent. Please check your inbox to complete registration."

	if verificationRequired {
		// Deliver the verification email. Failures here are logged but do not
		// fail the request — the user can request a fresh email via /auth/verify-email/resend.
		h.sendVerificationEmail(user.Email, user.Name, user.PreferredLocale, verificationToken)
	} else {
		if err := h.authService.MarkEmailVerified(c.Request.Context(), user.ID); err != nil {
			h.sendError(c, http.StatusInternalServerError, "Registration failed")
			return
		}
		user.EmailVerified = true
		message = "Account created successfully. You can now sign in."
	}

	c.JSON(http.StatusCreated, generated.RegistrationResponse{
		Email:                openapi_types.Email(user.Email),
		Message:              message,
		VerificationRequired: verificationRequired,
	})
}

// preferredLocaleString converts the optional generated locale enum into a
// plain string, returning an empty value when omitted so the service can
// apply its default.
func preferredLocaleString(locale *generated.RegisterUserJSONBodyPreferredLocale) string {
	if locale == nil {
		return ""
	}
	return string(*locale)
}

// VerifyEmail implements POST /auth/verify-email
// (POST /auth/verify-email)
func (h *APIHandler) VerifyEmail(c *gin.Context) {
	var req generated.VerifyEmailJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, tokens, err := h.authService.VerifyEmail(c.Request.Context(), req.Token)
	if err != nil {
		if errors.Is(err, service.ErrInvalidToken) || errors.Is(err, service.ErrTokenUsed) || errors.Is(err, service.ErrTokenExpired) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Email verification failed")
		return
	}

	c.JSON(http.StatusOK, h.convertAuthResponse(user, tokens))
}

// ResendVerificationEmail implements POST /auth/verify-email/resend
// (POST /auth/verify-email/resend)
func (h *APIHandler) ResendVerificationEmail(c *gin.Context) {
	var req generated.ResendVerificationEmailJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	parsed, err := mail.ParseAddress(string(req.Email))
	if err != nil {
		// Stay indistinguishable to prevent enumeration.
		c.Status(http.StatusNoContent)
		return
	}

	token, userEmail, userName, locale, err := h.authService.ResendVerification(c.Request.Context(), parsed.Address)
	if err != nil {
		// Don't leak internal errors.
		c.Status(http.StatusNoContent)
		return
	}

	if token != "" && userEmail != "" {
		h.sendVerificationEmail(userEmail, userName, locale, token)
	}

	c.Status(http.StatusNoContent)
}

// sendVerificationEmail delivers the email-verification message. Errors are
// swallowed: the operator can inspect SMTP logs and the user can resend.
func (h *APIHandler) sendVerificationEmail(toEmail, userName, locale, token string) {
	if h.emailSender == nil || toEmail == "" || token == "" {
		return
	}
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = os.Getenv("CORS_ORIGIN")
	}
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}
	if idx := strings.Index(frontendURL, ","); idx > 0 {
		frontendURL = frontendURL[:idx]
	}
	frontendURL = strings.TrimSpace(frontendURL)
	link := fmt.Sprintf("%s/verify-email?token=%s", frontendURL, token)

	tmpl := emailpkg.Templates(locale)
	subject, body := tmpl.VerifyEmail(emailpkg.VerifyEmailParams{
		UserName: userName,
		Link:     link,
	})
	_ = h.emailSender.Send(toEmail, subject, body)
}

// LoginUser implements POST /auth/login
// (POST /auth/login)
func (h *APIHandler) LoginUser(c *gin.Context) {
	var req generated.LoginUserJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, tokens, err := h.authService.Login(c.Request.Context(), service.LoginInput{
		Email:    string(req.Email),
		Password: req.Password,
	})

	if err != nil {
		if err == service.ErrInvalidCredentials {
			AuthLoginAttemptsTotal.WithLabelValues("invalid_credentials").Inc()
			h.sendError(c, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		if err == service.ErrAccountLocked {
			AuthLoginAttemptsTotal.WithLabelValues("account_locked").Inc()
			h.sendError(c, http.StatusTooManyRequests, "Account temporarily locked due to too many failed login attempts. Please try again later.")
			return
		}
		if err == service.ErrAccountDisabled {
			AuthLoginAttemptsTotal.WithLabelValues("account_disabled").Inc()
			h.sendError(c, http.StatusForbidden, "Account disabled. Contact the administrator.")
			return
		}
		if err == service.ErrEmailNotVerified {
			AuthLoginAttemptsTotal.WithLabelValues("email_not_verified").Inc()
			code := "email_not_verified"
			c.JSON(http.StatusForbidden, generated.Error{
				Error: "Email address not verified. Please check your inbox for the verification link.",
				Code:  &code,
			})
			return
		}
		AuthLoginAttemptsTotal.WithLabelValues("error").Inc()
		h.sendError(c, http.StatusInternalServerError, "Login failed")
		return
	}

	// Check if 2FA is enabled
	if user.TwoFactorEnabled {
		AuthLoginAttemptsTotal.WithLabelValues("2fa_required").Inc()
		// Generate a short-lived 2FA challenge token
		twoFactorToken, err := h.jwtManager.Generate2FAToken(user.ID)
		if err != nil {
			h.sendError(c, http.StatusInternalServerError, "Failed to generate 2FA token")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"requiresTwoFactor": true,
			"twoFactorToken":    twoFactorToken,
		})
		return
	}

	AuthLoginAttemptsTotal.WithLabelValues("success").Inc()
	c.JSON(http.StatusOK, h.convertAuthResponse(user, tokens))
}

// RefreshToken implements POST /auth/refresh
// (POST /auth/refresh)
func (h *APIHandler) RefreshToken(c *gin.Context) {
	var req generated.RefreshTokenJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	tokens, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		AuthTokenRefreshTotal.WithLabelValues("invalid").Inc()
		h.sendError(c, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	AuthTokenRefreshTotal.WithLabelValues("success").Inc()
	// Return new access and refresh tokens with expiry
	c.JSON(http.StatusOK, map[string]interface{}{
		"accessToken":  tokens.AccessToken,
		"refreshToken": tokens.RefreshToken,
		"expiresIn":    900,
	})
}

// ChangePassword implements POST /auth/change-password
// (POST /auth/change-password)
func (h *APIHandler) ChangePassword(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.ChangePasswordJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.authService.ChangePassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			h.sendError(c, http.StatusUnauthorized, "Current password is incorrect")
			return
		}
		if err == service.ErrPasswordTooShort || err == service.ErrPasswordTooLong {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusBadRequest, "Failed to change password")
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteCurrentUser implements DELETE /users/me
// (DELETE /users/me)
func (h *APIHandler) DeleteCurrentUser(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.DeleteCurrentUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.authService.DeleteUser(c.Request.Context(), userID, req.Password); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			h.sendError(c, http.StatusUnauthorized, "Password is incorrect")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete account")
		return
	}

	c.Status(http.StatusNoContent)
}

// RequestPasswordReset implements POST /auth/password-reset-request
// (POST /auth/password-reset-request)
func (h *APIHandler) RequestPasswordReset(c *gin.Context) {
	var req generated.RequestPasswordResetJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	parsedEmail, err := mail.ParseAddress(string(req.Email))
	if err != nil {
		// Keep response indistinguishable to prevent user enumeration.
		c.Status(http.StatusNoContent)
		return
	}

	token, userEmail, err := h.authService.RequestPasswordReset(c.Request.Context(), parsedEmail.Address)
	if err != nil {
		// Don't reveal internal errors to the client
		c.Status(http.StatusNoContent)
		return
	}

	// Send reset email if token was generated (user exists). The recipient
	// is the canonical address loaded from the database, NOT the HTTP
	// request body — this keeps untrusted input out of the SMTP message
	// and resolves CodeQL go/email-injection (CWE-640).
	if token != "" && userEmail != "" && h.emailSender != nil {
		frontendURL := os.Getenv("FRONTEND_URL")
		if frontendURL == "" {
			// Fall back to CORS_ORIGIN (the canonical frontend URL in production)
			frontendURL = os.Getenv("CORS_ORIGIN")
		}
		if frontendURL == "" {
			frontendURL = "http://localhost:5173"
		}
		// CORS_ORIGIN may contain multiple comma-separated origins; use the first one
		if idx := strings.Index(frontendURL, ","); idx > 0 {
			frontendURL = frontendURL[:idx]
		}
		frontendURL = strings.TrimSpace(frontendURL)
		resetLink := fmt.Sprintf("%s/new-password?token=%s", frontendURL, token)
		subject := "NinerLog: Password Reset"
		body := fmt.Sprintf(`<h2>Password Reset</h2>
<p>You requested a password reset for your NinerLog account.</p>
<p><a href="%s">Click here to reset your password</a></p>
<p>This link expires in 1 hour. If you did not request this, you can ignore this email.</p>
<p>— NinerLog</p>`, resetLink)

		_ = h.emailSender.Send(userEmail, subject, body)
	}

	// Always return 204 to prevent user enumeration
	c.Status(http.StatusNoContent)
}

// ResetPassword implements POST /auth/password-reset
// (POST /auth/password-reset)
func (h *APIHandler) ResetPassword(c *gin.Context) {
	var req generated.ResetPasswordJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.authService.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidToken) || errors.Is(err, service.ErrTokenUsed) || errors.Is(err, service.ErrTokenExpired) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, service.ErrPasswordTooShort) || errors.Is(err, service.ErrPasswordTooLong) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Password reset failed")
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *APIHandler) convertAuthResponse(user *models.User, tokens *service.TokenPair) generated.AuthResponse {
	return generated.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    900,
		User:         h.buildUserResponse(user),
	}
}
