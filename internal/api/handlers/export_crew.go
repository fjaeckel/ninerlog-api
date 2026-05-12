package handlers

import (
	"context"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

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
