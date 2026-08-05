package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// SetDeletionService stores the tombstone feed service.
func (h *APIHandler) SetDeletionService(s *service.DeletionService) {
	h.deletionService = s
}

// ListDeletions implements GET /sync/deletions
func (h *APIHandler) ListDeletions(c *gin.Context, params generated.ListDeletionsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.deletionService == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Deletion sync is not available")
		return
	}

	var entity *models.DeletionEntityType
	if params.Entity != nil {
		e := models.DeletionEntityType(*params.Entity)
		entity = &e
	}

	page, pageSize := 1, service.DefaultDeletionPageSize
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	if params.PageSize != nil && *params.PageSize > 0 {
		pageSize = *params.PageSize
	}

	result, err := h.deletionService.ListDeletions(
		c.Request.Context(), userID, params.Since, entity, page, pageSize,
	)
	if err != nil {
		if errors.Is(err, service.ErrInvalidDeletionEntity) {
			h.sendError(c, http.StatusBadRequest, "Unknown entity type")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve deletions")
		return
	}

	data := make([]generated.Deletion, 0, len(result.Deletions))
	for _, d := range result.Deletions {
		data = append(data, generated.Deletion{
			Entity:    generated.DeletionEntity(d.EntityType),
			Id:        openapi_types.UUID(d.EntityID),
			DeletedAt: d.DeletedAt,
		})
	}

	// pageSize is echoed from the clamped value the service actually used, so
	// a client asking for more than the maximum is not told it got it.
	effectivePageSize := pageSize
	if effectivePageSize > service.MaxDeletionPageSize {
		effectivePageSize = service.MaxDeletionPageSize
	}
	totalPages := 0
	if effectivePageSize > 0 {
		totalPages = (result.Total + effectivePageSize - 1) / effectivePageSize
	}

	c.JSON(http.StatusOK, generated.DeletionFeed{
		Data: data,
		Pagination: struct {
			Page       int `json:"page"`
			PageSize   int `json:"pageSize"`
			Total      int `json:"total"`
			TotalPages int `json:"totalPages"`
		}{
			Page:       page,
			PageSize:   effectivePageSize,
			Total:      result.Total,
			TotalPages: totalPages,
		},
		RetentionDays:    int(h.deletionService.Retention().Hours() / 24),
		WatermarkExpired: result.WatermarkExpired,
	})
}
