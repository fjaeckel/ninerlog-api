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

	// Canonicalise the fleet before touching flights. AircraftService repoints
	// a canonicalised registration's flights as part of the update, so doing
	// this first means the flight loop below sees registrations that already
	// agree with the fleet.
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
		flightcalc.ApplyAutoCalculations(flight, userName)
		// UpdateFlight canonicalises AircraftReg, so a flight logged as
		// "DEABC" is rewritten to "D-EABC" here even when nothing else about
		// it changed. Signed flights are rejected as locked and land in the
		// error count, as they do for any other recalculated field.
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

// normalizeFleetRegistrations rewrites every one of the user's aircraft whose
// registration is not in canonical notation, and reports how many were
// rewritten and how many could not be.
//
// A rewrite fails when the canonical spelling is already taken by another
// aircraft in the same fleet — the user holds both "DEABC" and "D-EABC", which
// are one aircraft entered twice. Merging those means deciding which entry's
// make, model and notes survive, so this reports the count and leaves both in
// place rather than guessing.
func (h *APIHandler) normalizeFleetRegistrations(c *gin.Context, userID uuid.UUID) (normalized, conflicts int) {
	fleet, err := h.aircraftService.ListAircraft(c.Request.Context(), userID)
	if err != nil {
		return 0, 0
	}
	for _, ac := range fleet {
		if registration.Canonical(ac.Registration) == ac.Registration {
			continue
		}
		// renameFlights is false: AircraftService repoints the flights on its
		// own for a canonicalisation, which this is by construction.
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
