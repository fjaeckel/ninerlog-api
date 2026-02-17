package handlers

import (
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/service/flightcalc"
	"github.com/gin-gonic/gin"
)

// RegisterFlightUtilRoutes registers utility routes for flights
func RegisterFlightUtilRoutes(api *gin.RouterGroup, h *APIHandler) {
	// Routes now registered via generated interface
}

// RecalculateFlights recalculates all auto-computed fields for every flight
func (h *APIHandler) RecalculateFlights(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	flights, err := h.flightService.ListFlights(c.Request.Context(), userID, nil)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flights")
		return
	}

	updated := 0
	errors := 0
	for _, flight := range flights {
		flightcalc.ApplyAutoCalculations(flight)
		if err := h.flightService.UpdateFlight(c.Request.Context(), flight, userID); err != nil {
			errors++
			continue
		}
		updated++
	}

	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"errors":  errors,
		"total":   len(flights),
	})
}
