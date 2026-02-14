package handlers

import (
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
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

	twoFA := user.TwoFactorEnabled
	response := generated.User{
		Id:               openapi_types.UUID(user.ID),
		Email:            openapi_types.Email(user.Email),
		Name:             user.Name,
		TwoFactorEnabled: &twoFA,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
	}
	c.JSON(http.StatusOK, response)
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

	// Update user
	if err := h.authService.UpdateUser(c.Request.Context(), user); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to update user")
		return
	}

	twoFA2 := user.TwoFactorEnabled
	response := generated.User{
		Id:               openapi_types.UUID(user.ID),
		Email:            openapi_types.Email(user.Email),
		Name:             user.Name,
		TwoFactorEnabled: &twoFA2,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
	}
	c.JSON(http.StatusOK, response)
}
