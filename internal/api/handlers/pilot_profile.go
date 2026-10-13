package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/pilotprofile"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// SetPilotProfileService wires the pilot profile service.
func (h *APIHandler) SetPilotProfileService(s *pilotprofile.Service) {
	h.pilotProfileService = s
}

// GetPilotProfile implements GET /users/me/pilot-profile
func (h *APIHandler) GetPilotProfile(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	profile, err := h.pilotProfileService.Get(c.Request.Context(), userID)
	if err != nil {
		slog.Error("get pilot profile", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to get pilot profile")
		return
	}
	c.JSON(http.StatusOK, convertPilotProfile(profile))
}

// UpdatePilotProfile implements PATCH /users/me/pilot-profile
func (h *APIHandler) UpdatePilotProfile(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req generated.UpdatePilotProfileJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	var upd pilotprofile.Update
	if req.Mode != nil {
		mode := models.PilotProfileMode(*req.Mode)
		upd.Mode = &mode
	}
	if req.Intents != nil {
		upd.Intents = make(map[models.Discipline]models.DisciplineIntent, len(*req.Intents))
		for d, i := range *req.Intents {
			upd.Intents[models.Discipline(d)] = models.DisciplineIntent(i)
		}
	}
	if req.Acknowledge != nil {
		for _, d := range *req.Acknowledge {
			upd.Acknowledge = append(upd.Acknowledge, models.Discipline(d))
		}
	}

	profile, err := h.pilotProfileService.Update(c.Request.Context(), userID, upd)
	if err != nil {
		if errors.Is(err, models.ErrInvalidPilotProfile) {
			h.sendError(c, http.StatusBadRequest, "Unknown mode, discipline or intent")
			return
		}
		slog.Error("update pilot profile", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to update pilot profile")
		return
	}
	c.JSON(http.StatusOK, convertPilotProfile(profile))
}

func convertPilotProfile(p *models.DerivedPilotProfile) generated.PilotProfile {
	out := generated.PilotProfile{
		Mode:                   generated.PilotProfileMode(p.Mode),
		Disciplines:            make([]generated.DisciplineState, 0, len(p.Disciplines)),
		PendingAcknowledgement: make([]generated.Discipline, 0, len(p.PendingAcknowledgement)),
	}
	for _, d := range p.PendingAcknowledgement {
		out.PendingAcknowledgement = append(out.PendingAcknowledgement, generated.Discipline(d))
	}
	for _, s := range p.Disciplines {
		state := generated.DisciplineState{
			Discipline:     generated.Discipline(s.Discipline),
			Status:         generated.DisciplineStatus(s.Status),
			Intent:         generated.DisciplineIntent(s.Intent),
			Evidence:       make([]generated.DisciplineEvidence, 0, len(s.Evidence)),
			UlKinds:        make([]generated.ULKind, 0, len(s.ULKinds)),
			AcknowledgedAt: s.AcknowledgedAt,
		}
		for _, ev := range s.Evidence {
			e := generated.DisciplineEvidence{
				Source:   generated.DisciplineEvidenceSource(ev.Source),
				Strength: generated.DisciplineEvidenceStrength(ev.Strength),
				Ref:      ev.Ref,
				RefId:    ev.RefID,
			}
			if ev.LastSeen != nil {
				e.LastSeen = &openapi_types.Date{Time: *ev.LastSeen}
			}
			state.Evidence = append(state.Evidence, e)
		}
		for _, k := range s.ULKinds {
			state.UlKinds = append(state.UlKinds, generated.ULKind(k))
		}
		out.Disciplines = append(out.Disciplines, state)
	}
	return out
}
