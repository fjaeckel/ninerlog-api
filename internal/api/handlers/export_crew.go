package handlers

import (
	"context"
	"log/slog"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// linkCrewHook returns the in-transaction hook that links a flight's crew to
// the user's contacts, creating contacts for names that are new. A failure
// aborts the flight write. See ContactService.LinkCrewMembers for the linking
// rules.
func (h *APIHandler) linkCrewHook(userID uuid.UUID, flight *models.Flight) repository.TxHook {
	return func(ctx context.Context, repos repository.TxRepos) error {
		if len(flight.CrewMembers) == 0 || h.contactService == nil {
			return nil
		}
		_, err := h.contactService.WithRepository(repos.Contacts()).LinkCrewMembers(ctx, userID, flight.CrewMembers)
		return err
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
