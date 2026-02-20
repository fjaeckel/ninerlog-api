package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
)

// Setup2FA implements POST /auth/2fa/setup
func (h *APIHandler) Setup2FA(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	secret, qrURI, err := h.twoFactorService.SetupTOTP(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrTwoFactorAlreadyEnabled) {
			h.sendError(c, http.StatusConflict, "2FA is already enabled")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to setup 2FA")
		return
	}

	c.JSON(http.StatusOK, generated.TwoFactorSetup{
		Secret: secret,
		QrUri:  qrURI,
	})
}

// Verify2FA implements POST /auth/2fa/verify
func (h *APIHandler) Verify2FA(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.Verify2FAJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	recoveryCodes, err := h.twoFactorService.VerifyAndEnable(c.Request.Context(), userID, req.Code)
	if err != nil {
		if errors.Is(err, service.ErrInvalidTOTPCode) {
			h.sendError(c, http.StatusBadRequest, "Invalid TOTP code. Check your authenticator app and try again.")
			return
		}
		if errors.Is(err, service.ErrTwoFactorAlreadyEnabled) {
			h.sendError(c, http.StatusConflict, "2FA is already enabled")
			return
		}
		h.sendError(c, http.StatusBadRequest, "Failed to verify 2FA code")
		return
	}

	c.JSON(http.StatusOK, generated.TwoFactorEnabled{
		RecoveryCodes: recoveryCodes,
	})
}

// Disable2FA implements POST /auth/2fa/disable
func (h *APIHandler) Disable2FA(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.Disable2FAJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.twoFactorService.Disable(c.Request.Context(), userID, req.Password); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			h.sendError(c, http.StatusUnauthorized, "Password is incorrect")
			return
		}
		h.sendError(c, http.StatusBadRequest, "Failed to disable 2FA")
		return
	}

	c.Status(http.StatusNoContent)
}

// Login2FA implements POST /auth/2fa/login
func (h *APIHandler) Login2FA(c *gin.Context) {
	var req generated.Login2FAJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate the temporary 2FA token
	claims, err := h.jwtManager.Validate2FAToken(req.TwoFactorToken)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Invalid or expired 2FA token. Please login again.")
		return
	}

	// Validate the TOTP code
	valid, err := h.twoFactorService.ValidateTOTP(c.Request.Context(), claims.UserID, req.Code)
	if err != nil || !valid {
		h.sendError(c, http.StatusUnauthorized, "Invalid 2FA code")
		return
	}

	// 2FA passed — generate real tokens
	tokens, err := h.authService.GenerateTokensForUser(c.Request.Context(), claims.UserID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to generate tokens")
		return
	}

	user, err := h.authService.GetUserByID(c.Request.Context(), claims.UserID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to get user")
		return
	}

	c.JSON(http.StatusOK, h.convertAuthResponse(user, tokens))
}
