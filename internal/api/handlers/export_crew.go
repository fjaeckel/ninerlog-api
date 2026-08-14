package handlers

import (
	"context"
	"log/slog"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// persistCrewMembers links a flight's crew to the user's contacts — creating
// contacts for names that are new — and writes the crew rows.
//
// Every handler that stores crew goes through here so that logging a flight is
// what fills the address book, regardless of entry point. See
// ContactService.LinkCrewMembers for the linking rules.
//
// Failures are logged and swallowed rather than failing the request: the flight
// itself is already written at this point, and reporting an error for a saved
// flight would push clients into retrying a create that already succeeded.
func (h *APIHandler) persistCrewMembers(c *gin.Context, userID uuid.UUID, flight *models.Flight) {
	if h.flightCrewRepo == nil {
		return
	}
	ctx := c.Request.Context()

	if len(flight.CrewMembers) > 0 && h.contactService != nil {
		if _, err := h.contactService.LinkCrewMembers(ctx, userID, flight.CrewMembers); err != nil {
			// Non-fatal: the crew rows are still worth writing with the names
			// they were given, just without the contact links.
			slog.Warn("failed to link crew members to contacts",
				"flightId", flight.ID, "error", err)
		}
	}

	// An empty list is written too — that is how a client clears the crew.
	if err := h.flightCrewRepo.SetCrewMembers(ctx, flight.ID, flight.CrewMembers); err != nil {
		slog.Warn("failed to save crew members", "flightId", flight.ID, "error", err)
	}
}

// attachCrewMembers populates flight.CrewMembers for each flight in `flights`
// from the flight_crew_members table. It is a no-op when the handler does not
// have a flightCrewRepo wired up (test setups). Errors are logged-and-swallowed
// rather than propagated so an export still renders something usable; callers
// that need strict guarantees should call h.flightCrewRepo.GetByFlightIDs
// directly.
//
// Exporters MUST call this before invoking flightrules.DisplayPICName so the
// crew-table fallback (the canonical PIC-of-record source for Dual flights
// imported via ForeFlight or written by the modern FE) actually fires.
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
		// Best-effort: leave CrewMembers nil and let exporters fall back to
		// the legacy InstructorName column / "SELF" placeholder.
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
