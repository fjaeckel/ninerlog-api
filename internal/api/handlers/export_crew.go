package handlers

import (
	"context"
	"log/slog"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// checkCrewContacts clears every member's contactId that is not one of
// userID's contacts, or every contactId without a contact service, and
// returns the linker that checked them.
func (h *APIHandler) checkCrewContacts(ctx context.Context, userID uuid.UUID, members []models.FlightCrewMember) (*service.CrewLinker, error) {
	if h.contactService == nil {
		for i := range members {
			members[i].ContactID = nil
		}
		return nil, nil
	}
	linker := h.contactService.NewCrewLinker(userID)
	if err := linker.KeepOwned(ctx, members); err != nil {
		return nil, err
	}
	return linker, nil
}

// persistCrewMembers links a flight's crew to the user's contacts with
// linker — creating contacts for names that are new — and writes the crew
// rows. A member keeps its checked contactId when linking fails; on a failed
// write the stored crew is loaded into flight. See
// ContactService.LinkCrewMembers for the linking rules.
func (h *APIHandler) persistCrewMembers(ctx context.Context, linker *service.CrewLinker, flight *models.Flight) {
	if h.flightCrewRepo == nil {
		return
	}
	if linker != nil && len(flight.CrewMembers) > 0 {
		if err := linker.Link(ctx, flight.CrewMembers); err != nil {
			slog.Warn("failed to link crew members to contacts", "flightId", flight.ID, "error", err)
		}
	}

	// An empty list is written too, clearing the crew.
	if err := h.flightCrewRepo.SetCrewMembers(ctx, flight.ID, flight.CrewMembers); err != nil {
		slog.Warn("failed to save crew members", "flightId", flight.ID, "error", err)
		h.reloadCrewMembers(ctx, flight)
	}
}

// reloadCrewMembers replaces flight.CrewMembers with the stored crew rows.
func (h *APIHandler) reloadCrewMembers(ctx context.Context, flight *models.Flight) {
	flight.CrewMembers = nil
	h.attachCrewMembers(ctx, []*models.Flight{flight})
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
		slog.Warn("failed to load crew members", "flights", len(ids), "error", err)
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
