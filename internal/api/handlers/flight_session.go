package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// GetCurrentFlightSession implements GET /flight-sessions/current
// (GET /flight-sessions/current)
func (h *APIHandler) GetCurrentFlightSession(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	session, err := h.flightSessionService.GetCurrent(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, models.ErrNoOpenFlightSession) {
			h.sendError(c, http.StatusNotFound, "No open flight session")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flight session")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedFlightSession(session))
}

// DiscardCurrentFlightSession implements DELETE /flight-sessions/current
// (DELETE /flight-sessions/current)
func (h *APIHandler) DiscardCurrentFlightSession(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.flightSessionService.Discard(c.Request.Context(), userID); err != nil {
		if errors.Is(err, models.ErrNoOpenFlightSession) {
			h.sendError(c, http.StatusNotFound, "No open flight session")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to discard flight session")
		return
	}

	c.Status(http.StatusNoContent)
}

// RecordFlightSessionEvent implements POST /flight-sessions/current/events
// (POST /flight-sessions/current/events)
func (h *APIHandler) RecordFlightSessionEvent(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.FlightSessionEvent
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	input := service.FlightSessionEventInput{
		Type:        string(req.Type),
		OccurredAt:  req.OccurredAt,
		AircraftReg: req.AircraftReg,
		ICAO:        req.Icao,
		Lat:         req.Lat,
		Lon:         req.Lon,
		UserName:    h.getUserNameFromContext(c),
	}

	session, created, err := h.flightSessionService.RecordEvent(c.Request.Context(), userID, input)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrNoOpenFlightSession):
			h.sendError(c, http.StatusNotFound, "No open flight session — record an offblock event first")
		case errors.Is(err, service.ErrInvalidSessionEvent):
			h.sendError(c, http.StatusBadRequest, "Invalid event type")
		case errors.Is(err, models.ErrInvalidFlightSessionData):
			h.sendError(c, http.StatusBadRequest, "Event timestamp lies in the future")
		case errors.Is(err, models.ErrFlightSessionTimeOrder):
			h.sendError(c, http.StatusBadRequest, "Event times are out of order (expected off block ≤ takeoff ≤ landing ≤ on block)")
		case errors.Is(err, models.ErrFlightSessionTooLong):
			h.sendError(c, http.StatusBadRequest, "Session exceeds 24 hours — discard it and log the flight manually")
		case errors.Is(err, models.ErrFlightSessionMissingReg):
			h.sendError(c, http.StatusBadRequest, "Aircraft registration is required to complete the flight — resend onblock with aircraftReg")
		default:
			h.sendError(c, http.StatusInternalServerError, "Failed to record flight session event")
		}
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, convertToGeneratedFlightSession(session))
}

func convertToGeneratedFlightSession(s *models.FlightSession) generated.FlightSession {
	resp := generated.FlightSession{
		Id:            openapi_types.UUID(s.ID),
		UserId:        openapi_types.UUID(s.UserID),
		Status:        generated.FlightSessionStatus(s.Status),
		AircraftReg:   s.AircraftReg,
		DepartureIcao: s.DepartureICAO,
		ArrivalIcao:   s.ArrivalICAO,
		OffBlockAt:    s.OffBlockAt,
		TakeoffAt:     s.TakeoffAt,
		LandingAt:     s.LandingAt,
		OnBlockAt:     s.OnBlockAt,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
	if s.FlightID != nil {
		id := openapi_types.UUID(*s.FlightID)
		resp.FlightId = &id
	}
	return resp
}
