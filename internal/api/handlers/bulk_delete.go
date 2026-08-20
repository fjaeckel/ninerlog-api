package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DeleteAllFlights implements DELETE /flights/delete-all
func (h *APIHandler) DeleteAllFlights(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Delete import history first
	_ = h.flightImportRepo.DeleteByUserID(c.Request.Context(), userID)

	deleted, err := h.flightService.DeleteAllFlights(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to delete flights")
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// DeleteAllUserData implements DELETE /users/me/data
func (h *APIHandler) DeleteAllUserData(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.userContentRepo.DeleteAllContent(c.Request.Context(), userID); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to delete user data")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All user data deleted successfully"})
}
