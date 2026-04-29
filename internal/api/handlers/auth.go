package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
)

// RegisterUser implements POST /auth/register
// (POST /auth/register)
func (h *APIHandler) RegisterUser(c *gin.Context) {
	var req generated.RegisterUserJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, tokens, err := h.authService.Register(c.Request.Context(), service.RegisterInput{
		Email:    string(req.Email),
		Password: req.Password,
		Name:     req.Name,
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

	c.JSON(http.StatusCreated, h.convertAuthResponse(user, tokens))
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

	token, err := h.authService.RequestPasswordReset(c.Request.Context(), string(req.Email))
	if err != nil {
		// Don't reveal internal errors to the client
		c.Status(http.StatusNoContent)
		return
	}

	// Send reset email if token was generated (user exists)
	if token != "" && h.emailSender != nil {
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

		_ = h.emailSender.Send(string(req.Email), subject, body)
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
