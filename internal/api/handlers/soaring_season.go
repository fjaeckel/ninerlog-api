package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
)

// SetSoaringSeasonService wires the soaring season service.
func (h *APIHandler) SetSoaringSeasonService(s *service.SoaringSeasonService) {
	h.soaringSeasonService = s
}

// GetSoaringSeason implements GET /reports/soaring-season.
func (h *APIHandler) GetSoaringSeason(c *gin.Context, params generated.GetSoaringSeasonParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	season, err := h.soaringSeasonService.Season(c.Request.Context(), userID, params.Year)
	if err != nil {
		if errors.Is(err, service.ErrInvalidSeasonYear) {
			h.sendError(c, http.StatusBadRequest, "year must be between 1900 and next year")
			return
		}
		slog.Error("[soaring-season] internal error", "error", err)
		h.sendError(c, http.StatusInternalServerError, "Failed to load soaring season")
		return
	}
	c.JSON(http.StatusOK, season)
}
