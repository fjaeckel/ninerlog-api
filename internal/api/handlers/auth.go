package handlers

import (
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
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

	// Match AuthResponse schema from OpenAPI spec
	response := generated.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    900, // 15 minutes in seconds
		User: generated.User{
			Id:        openapi_types.UUID(user.ID),
			Email:     openapi_types.Email(user.Email),
			Name:      user.Name,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}

	c.JSON(http.StatusCreated, response)
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

	// Match AuthResponse schema from OpenAPI spec
	response := generated.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    900, // 15 minutes in seconds
		User: generated.User{
			Id:        openapi_types.UUID(user.ID),
			Email:     openapi_types.Email(user.Email),
			Name:      user.Name,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}

	c.JSON(http.StatusOK, response)
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
	response := map[string]interface{}{
		"accessToken":  tokens.AccessToken,
		"refreshToken": tokens.RefreshToken,
		"expiresIn":    900, // 15 minutes in seconds
	}

	c.JSON(http.StatusOK, response)
}
