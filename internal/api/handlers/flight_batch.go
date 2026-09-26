package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
)

// CreateFlightBatch implements POST /flights/batch
// (POST /flights/batch)
func (h *APIHandler) CreateFlightBatch(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.CreateFlightBatchJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.Legs) == 0 || len(req.Legs) > service.MaxFlightBatchLegs {
		h.sendError(c, http.StatusBadRequest, fmt.Sprintf("legs must hold between 1 and %d entries", service.MaxFlightBatchLegs))
		return
	}

	flights := make([]*models.Flight, 0, len(req.Legs))
	for i, leg := range req.Legs {
		body := batchLegCreate(req.Template, leg)
		flight, errMsg := h.flightFromCreate(c, userID, &body)
		if errMsg != "" {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Leg %d: %s", i, errMsg))
			return
		}
		flights = append(flights, flight)
	}

	if err := h.flightService.ValidateFlightBatch(c.Request.Context(), flights); err != nil {
		h.sendBatchError(c, err)
		return
	}

	if h.contactService != nil {
		for _, f := range flights {
			if len(f.CrewMembers) == 0 {
				continue
			}
			if _, err := h.contactService.LinkCrewMembers(c.Request.Context(), userID, f.CrewMembers); err != nil {
				slog.Warn("failed to link crew members to contacts", "error", err)
			}
		}
	}

	if err := h.flightService.CreateFlightBatch(c.Request.Context(), flights); err != nil {
		h.sendBatchError(c, err)
		return
	}

	warnings := h.flightService.CheckFlights(c.Request.Context(), userID, flights)
	out := make([]generated.Flight, 0, len(flights))
	for i, f := range flights {
		g := convertToGeneratedFlight(f)
		g.Warnings = convertToGeneratedWarnings(warnings[i])
		out = append(out, g)
	}
	c.JSON(http.StatusCreated, generated.FlightBatchResult{Flights: out})
}

// sendBatchError maps a batch service error to its response.
func (h *APIHandler) sendBatchError(c *gin.Context, err error) {
	var legErr *service.FlightBatchLegError
	switch {
	case errors.As(err, &legErr):
		h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Leg %d: %v", legErr.Index, legErr.Err))
	case errors.Is(err, service.ErrInvalidFlightBatch):
		h.sendError(c, http.StatusBadRequest, fmt.Sprintf("legs must hold between 1 and %d entries", service.MaxFlightBatchLegs))
	default:
		h.sendError(c, http.StatusInternalServerError, "Failed to create flights")
	}
}

// batchLegCreate returns the create body of one leg: the template with the
// leg's fields laid over it.
func batchLegCreate(template generated.FlightCreate, leg generated.FlightBatchLeg) generated.FlightCreate {
	body := template
	if leg.DepartureTime != nil {
		body.DepartureTime = leg.DepartureTime
	}
	if leg.ArrivalTime != nil {
		body.ArrivalTime = leg.ArrivalTime
	}
	switch {
	case leg.Landings != nil:
		body.Landings = leg.Landings
	case template.Landings == nil && !isSimulatorCreate(&template):
		one := 1
		body.Landings = &one
	}
	if leg.Launches != nil {
		body.Launches = leg.Launches
	}
	if leg.Remarks != nil {
		body.Remarks = leg.Remarks
	}
	return body
}
