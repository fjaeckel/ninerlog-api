package handlers

import (
	"context"
	"log/slog"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// persistCrewMembers links a flight's crew to the user's contacts — creating
// contacts for names that are new — and writes the crew rows. Failures are
// logged and swallowed. See ContactService.LinkCrewMembers for the linking
// rules.
func (h *APIHandler) persistCrewMembers(c *gin.Context, userID uuid.UUID, flight *models.Flight) {
	if h.flightCrewRepo == nil {
		return
	}
	ctx := c.Request.Context()

	if len(flight.CrewMembers) > 0 && h.contactService != nil {
		if _, err := h.contactService.LinkCrewMembers(ctx, userID, flight.CrewMembers); err != nil {
			slog.Warn("failed to link crew members to contacts",
				"flightId", flight.ID, "error", err)
		}
	}

	// An empty list is written too, clearing the crew.
	if err := h.flightCrewRepo.SetCrewMembers(ctx, flight.ID, flight.CrewMembers); err != nil {
		slog.Warn("failed to save crew members", "flightId", flight.ID, "error", err)
	}
}

// attachCrewMembers populates flight.CrewMembers for each flight in `flights`
// from the flight_crew_members table. No-op when the handler has no
// flightCrewRepo wired up. Errors are logged and swallowed. Exporters must
// call this before invoking flightrules.DisplayPICName.
func (h *APIHandler) attachCrewMembers(ctx context.Context, flights []*models.Flight) {
	if h == nil || h.flightCrewRepo == nil || len(flights) == 0 {
		return
	}
	ids := make([]uuid.UUID, 0, len(flights))
	for _, f := range flights {
		if f != nil {
			ids = append(ids, f.ID)
		}
	}
	byID, err := h.flightCrewRepo.GetByFlightIDs(ctx, ids)
	if err != nil {
		return
	}
	for _, f := range flights {
		if f == nil {
			continue
		}
		if cm, ok := byID[f.ID]; ok {
			f.CrewMembers = cm
		}
	}
}

// AttachCrewMembers is the exported wrapper used by the cloud-backup
// JSON builder so it can reuse the same crew enrichment pathway as the
// HTTP export handler.
func (h *APIHandler) AttachCrewMembers(ctx context.Context, flights []*models.Flight) {
	h.attachCrewMembers(ctx, flights)
}
