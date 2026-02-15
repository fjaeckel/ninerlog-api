package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterCurrencyRoutes registers the currency status endpoint
func RegisterCurrencyRoutes(api *gin.RouterGroup, h *APIHandler) {
	api.GET("/currency", h.GetAllCurrencyStatus)
}

// GetAllCurrencyStatus returns currency status for all class ratings across all licenses
func (h *APIHandler) GetAllCurrencyStatus(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	result, err := h.currencyService.EvaluateAll(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to evaluate currency")
		return
	}

	c.JSON(http.StatusOK, result)
}
