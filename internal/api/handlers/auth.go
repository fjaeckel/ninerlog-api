package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/service"
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
		h.sendError(c, http.StatusInternalServerError, "Registration failed")
		return
	}

	c.JSON(http.StatusCreated, convertAuthResponse(user, tokens))
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
			h.sendError(c, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Login failed")
		return
	}

	// Check if 2FA is enabled
	if user.TwoFactorEnabled {
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

	c.JSON(http.StatusOK, convertAuthResponse(user, tokens))
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
		h.sendError(c, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

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
		h.sendError(c, http.StatusBadRequest, err.Error())
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

func convertAuthResponse(user *models.User, tokens *service.TokenPair) generated.AuthResponse {
	twoFA := user.TwoFactorEnabled
	userResp := generated.User{
		Id:               openapi_types.UUID(user.ID),
		Email:            openapi_types.Email(user.Email),
		Name:             user.Name,
		TwoFactorEnabled: &twoFA,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
	}
	if user.DefaultLicenseID != nil {
		dlid := openapi_types.UUID(*user.DefaultLicenseID)
		userResp.DefaultLicenseId = &dlid
	}
	return generated.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    900,
		User:         userResp,
	}
}
