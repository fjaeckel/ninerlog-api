package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetCurrentUser implements GET /users/me
// (GET /users/me)
func (h *APIHandler) GetCurrentUser(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, h.buildUserResponse(user))
}

// UpdateCurrentUser implements PATCH /users/me
// (PATCH /users/me)
func (h *APIHandler) UpdateCurrentUser(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.UpdateCurrentUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get current user
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	// Apply updates
	if req.Name != nil {
		user.Name = strings.TrimSpace(*req.Name)
	}
	if req.Email != nil {
		user.Email = strings.ToLower(strings.TrimSpace(string(*req.Email)))
	}
	if req.TimeDisplayFormat != nil {
		format := string(*req.TimeDisplayFormat)
		if format == "hm" || format == "decimal" {
			user.TimeDisplayFormat = format
		}
	}
	if req.PreferredLocale != nil {
		locale := string(*req.PreferredLocale)
		if locale == "en" || locale == "de" {
			user.PreferredLocale = locale
		}
	}

	// Update user
	if err := h.authService.UpdateUser(c.Request.Context(), user); err != nil {
		if errors.Is(err, service.ErrUserAlreadyExists) {
			h.sendError(c, http.StatusConflict, "This email is already in use by another account")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to update user")
		return
	}

	c.JSON(http.StatusOK, h.buildUserResponse(user))
}
