package handlers

import (
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
)

// ListAircraft implements GET /aircraft
// (GET /aircraft)
func (h *APIHandler) ListAircraft(c *gin.Context, params generated.ListAircraftParams) {
	h.sendError(c, http.StatusNotImplemented, "Aircraft management not yet implemented")
}

// CreateAircraft implements POST /aircraft
// (POST /aircraft)
func (h *APIHandler) CreateAircraft(c *gin.Context) {
	h.sendError(c, http.StatusNotImplemented, "Aircraft management not yet implemented")
}

// GetAircraft implements GET /aircraft/{aircraftId}
// (GET /aircraft/{aircraftId})
func (h *APIHandler) GetAircraft(c *gin.Context, aircraftId generated.AircraftId) {
	h.sendError(c, http.StatusNotImplemented, "Aircraft management not yet implemented")
}

// UpdateAircraft implements PATCH /aircraft/{aircraftId}
// (PATCH /aircraft/{aircraftId})
func (h *APIHandler) UpdateAircraft(c *gin.Context, aircraftId generated.AircraftId) {
	h.sendError(c, http.StatusNotImplemented, "Aircraft management not yet implemented")
}

// DeleteAircraft implements DELETE /aircraft/{aircraftId}
// (DELETE /aircraft/{aircraftId})
func (h *APIHandler) DeleteAircraft(c *gin.Context, aircraftId generated.AircraftId) {
	h.sendError(c, http.StatusNotImplemented, "Aircraft management not yet implemented")
}
