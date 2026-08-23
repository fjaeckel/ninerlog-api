package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RecalculateFlights recalculates all auto-computed fields for every flight
func (h *APIHandler) RecalculateFlights(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// The fleet is canonicalised first; AircraftService repoints its flights.
	aircraftNormalized, aircraftConflicts := h.normalizeFleetRegistrations(c, userID)

	flights, err := h.flightService.ListFlights(c.Request.Context(), userID, nil)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve flights")
		return
	}

	// Resolve user display name once for self-vs-third-party instructor logic.
	userName := ""
	if user, err := h.authService.GetUserByID(c.Request.Context(), userID); err == nil && user != nil {
		userName = user.Name
	}

	// The fleet decides whether co-pilot time may be logged; resolve it once
	// for the whole pass.
	fleet := h.aircraftService.AircraftFactsIndexFor(c.Request.Context(), userID)

	updated := 0
	failed := 0
	for _, flight := range flights {
		// Load crew members so PIC/Dual calculation is correct
		if h.flightCrewRepo != nil {
			crew, err := h.flightCrewRepo.GetByFlightID(c.Request.Context(), flight.ID)
			if err == nil {
				flight.CrewMembers = crew
			}
		}
		flightcalc.ApplyAutoCalculations(flight, userName, fleet[service.NormalizeRegistrationKey(flight.AircraftReg)])
		// UpdateFlight canonicalises AircraftReg; signed flights are rejected
		// as locked and counted as errors.
		if err := h.flightService.UpdateFlight(c.Request.Context(), flight, userID); err != nil {
			failed++
			continue
		}
		updated++
	}

	c.JSON(http.StatusOK, gin.H{
		"updated":            updated,
		"errors":             failed,
		"total":              len(flights),
		"aircraftNormalized": aircraftNormalized,
		"aircraftConflicts":  aircraftConflicts,
	})
}

// normalizeFleetRegistrations rewrites the user's aircraft whose registration
// is not in canonical notation. It returns the number rewritten and the number
// whose canonical spelling is already held by another aircraft in the same
// fleet; those are left untouched.
func (h *APIHandler) normalizeFleetRegistrations(c *gin.Context, userID uuid.UUID) (normalized, conflicts int) {
	fleet, err := h.aircraftService.ListAircraft(c.Request.Context(), userID)
	if err != nil {
		return 0, 0
	}
	for _, ac := range fleet {
		if registration.Canonical(ac.Registration) == ac.Registration {
			continue
		}
		if _, err := h.aircraftService.UpdateAircraft(c.Request.Context(), ac, userID, false); err != nil {
			if errors.Is(err, service.ErrDuplicateRegistration) {
				conflicts++
			}
			continue
		}
		normalized++
	}
	return normalized, conflicts
}
