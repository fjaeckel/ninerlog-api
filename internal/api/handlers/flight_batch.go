package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
		h.sendError(c, http.StatusBadRequest, service.ErrInvalidFlightBatch.Error())
		return
	}

	flights := make([]*models.Flight, 0, len(req.Legs))
	facts := h.aircraftService.AircraftFactsLookup(c.Request.Context(), userID)
	for i, leg := range req.Legs {
		body := batchLegCreate(req.Template, leg)
		flight, errMsg := h.flightFromCreate(c, userID, &body, facts)
		if errMsg != "" {
			h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Leg %d: %s", i, errMsg))
			return
		}
		flights = append(flights, flight)
	}

	crew := append([]models.FlightCrewMember(nil), flights[0].CrewMembers...)
	var crewLinker *service.CrewLinker
	if len(crew) > 0 {
		crewLinker, err = h.checkCrewContacts(c.Request.Context(), userID, crew)
		if err != nil {
			slog.Warn("failed to check crew contacts", "error", err)
			h.sendError(c, http.StatusInternalServerError, "Failed to create flights")
			return
		}
	}
	for _, f := range flights {
		f.CrewMembers = append([]models.FlightCrewMember(nil), crew...)
	}

	if err := h.flightService.CreateFlightBatch(c.Request.Context(), flights); err != nil {
		h.sendBatchError(c, err)
		return
	}
	h.linkBatchCrew(c.Request.Context(), crewLinker, flights, crew)

	userName := h.getUserNameFromContext(c)
	warnings := h.flightService.CheckFlights(c.Request.Context(), userID, flights)
	out := make([]generated.Flight, 0, len(flights))
	for i, f := range flights {
		g := withDisplayPICName(convertToGeneratedFlight(f), f, userName)
		g.Warnings = convertToGeneratedWarnings(warnings[i])
		out = append(out, g)
	}
	c.JSON(http.StatusCreated, generated.FlightBatchResult{Flights: out})
}

// linkBatchCrew links the template crew to the user's contacts once and,
// when that links a member, writes the linked crew to every stored leg in one
// transaction. The legs keep their crew as inserted when linking or the write
// fails.
func (h *APIHandler) linkBatchCrew(ctx context.Context, linker *service.CrewLinker, flights []*models.Flight, crew []models.FlightCrewMember) {
	if linker == nil || len(crew) == 0 || h.flightCrewRepo == nil {
		return
	}
	linked := append([]models.FlightCrewMember(nil), crew...)
	if err := linker.Link(ctx, linked); err != nil {
		slog.Warn("failed to link crew members to contacts", "error", err)
		return
	}
	if sameCrew(linked, crew) {
		return
	}
	ids := make([]uuid.UUID, len(flights))
	for i, f := range flights {
		ids[i] = f.ID
	}
	if err := h.flightCrewRepo.SetCrewMembersForFlights(ctx, ids, linked); err != nil {
		slog.Warn("failed to save crew members", "error", err)
		return
	}
	for _, f := range flights {
		f.CrewMembers = append([]models.FlightCrewMember(nil), linked...)
	}
}

// sameCrew reports whether a and b list the same names and contactIds in order.
func sameCrew(a, b []models.FlightCrewMember) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || !sameContact(a[i].ContactID, b[i].ContactID) {
			return false
		}
	}
	return true
}

func sameContact(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// sendBatchError maps a batch service error to its response.
func (h *APIHandler) sendBatchError(c *gin.Context, err error) {
	var legErr *service.FlightBatchLegError
	switch {
	case errors.As(err, &legErr):
		h.sendError(c, http.StatusBadRequest, fmt.Sprintf("Leg %d: %v", legErr.Index, legErr.Err))
	case errors.Is(err, service.ErrInvalidFlightBatch):
		h.sendError(c, http.StatusBadRequest, service.ErrInvalidFlightBatch.Error())
	default:
		h.sendError(c, http.StatusInternalServerError, "Failed to create flights")
	}
}

// batchLegCreate returns the create body of one leg: the template with the
// leg's fields laid over it, without the template's block times when the leg
// carries a take-off or landing time.
func batchLegCreate(template generated.FlightCreate, leg generated.FlightBatchLeg) generated.FlightCreate {
	body := template
	if leg.DepartureTime != nil || leg.ArrivalTime != nil {
		body.OffBlockTime = nil
		body.OnBlockTime = nil
	}
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
