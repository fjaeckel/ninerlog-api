package handlers

import (
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
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
		user.Name = *req.Name
	}
	if req.Email != nil {
		user.Email = string(*req.Email)
	}
	if req.TimeDisplayFormat != nil {
		format := string(*req.TimeDisplayFormat)
		if format == "hm" || format == "decimal" {
			user.TimeDisplayFormat = format
		}
	}

	// Update user
	if err := h.authService.UpdateUser(c.Request.Context(), user); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to update user")
		return
	}

	c.JSON(http.StatusOK, h.buildUserResponse(user))
}
