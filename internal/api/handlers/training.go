package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/training"
	"github.com/gin-gonic/gin"
)

// SetTrainingService wires the training progress service.
func (h *APIHandler) SetTrainingService(s *training.Service) {
	h.trainingService = s
}

// GetTrainingProgress implements GET /training/progress.
func (h *APIHandler) GetTrainingProgress(c *gin.Context, params generated.GetTrainingProgressParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var requested []models.TrainingProgrammeID
	if params.Programme != nil {
		for _, p := range *params.Programme {
			requested = append(requested, models.TrainingProgrammeID(p))
		}
	}
	progress, err := h.trainingService.Progress(c.Request.Context(), userID, requested)
	if err != nil {
		if errors.Is(err, models.ErrUnknownTrainingProgramme) {
			h.sendError(c, http.StatusBadRequest, "programme must be one of SPL, SPL_TMG_EXTENSION, UL_THREE_AXIS, UL_WEIGHT_SHIFT")
			return
		}
		slog.Error("[training-progress] internal error", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to load training progress")
		return
	}
	c.JSON(http.StatusOK, progress)
}
