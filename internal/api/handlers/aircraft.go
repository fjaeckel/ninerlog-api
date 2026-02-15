package handlers

import (
	"errors"
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// ListAircraft implements GET /aircraft
// (GET /aircraft)
func (h *APIHandler) ListAircraft(c *gin.Context, params generated.ListAircraftParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	aircraft, err := h.aircraftService.ListAircraft(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve aircraft")
		return
	}

	// Pagination
	page := 1
	pageSize := 20
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	if params.PageSize != nil && *params.PageSize > 0 {
		pageSize = *params.PageSize
		if pageSize > 100 {
			pageSize = 100
		}
	}

	total := len(aircraft)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	pageAircraft := aircraft[start:end]
	result := make([]generated.Aircraft, 0, len(pageAircraft))
	for _, a := range pageAircraft {
		result = append(result, convertToGeneratedAircraft(a))
	}

	c.JSON(http.StatusOK, generated.PaginatedAircraft{
		Data: result,
		Pagination: struct {
			Page       int `json:"page"`
			PageSize   int `json:"pageSize"`
			Total      int `json:"total"`
			TotalPages int `json:"totalPages"`
		}{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// CreateAircraft implements POST /aircraft
// (POST /aircraft)
func (h *APIHandler) CreateAircraft(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.CreateAircraftJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	aircraft := &models.Aircraft{
		UserID:       userID,
		Registration: req.Registration,
		Type:         req.Type,
		Make:         req.Make,
		Model:        req.Model,
		IsActive:     true,
	}
	if req.IsComplex != nil {
		aircraft.IsComplex = *req.IsComplex
	}
	if req.IsHighPerformance != nil {
		aircraft.IsHighPerformance = *req.IsHighPerformance
	}
	if req.IsTailwheel != nil {
		aircraft.IsTailwheel = *req.IsTailwheel
	}
	if req.Notes != nil {
		aircraft.Notes = req.Notes
	}
	if req.AircraftClass != nil {
		s := string(*req.AircraftClass)
		aircraft.AircraftClass = &s
	}

	if err := h.aircraftService.CreateAircraft(c.Request.Context(), aircraft); err != nil {
		if errors.Is(err, service.ErrDuplicateRegistration) {
			h.sendError(c, http.StatusConflict, "Aircraft registration already exists")
			return
		}
		h.sendError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusCreated, convertToGeneratedAircraft(aircraft))
}

// GetAircraft implements GET /aircraft/{aircraftId}
// (GET /aircraft/{aircraftId})
func (h *APIHandler) GetAircraft(c *gin.Context, aircraftId generated.AircraftId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	aircraft, err := h.aircraftService.GetAircraft(c.Request.Context(), uuid.UUID(aircraftId), userID)
	if err != nil {
		if errors.Is(err, service.ErrAircraftNotFound) || errors.Is(err, service.ErrUnauthorizedAircraft) {
			h.sendError(c, http.StatusNotFound, "Aircraft not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve aircraft")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedAircraft(aircraft))
}

// UpdateAircraft implements PATCH /aircraft/{aircraftId}
// (PATCH /aircraft/{aircraftId})
func (h *APIHandler) UpdateAircraft(c *gin.Context, aircraftId generated.AircraftId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.UpdateAircraftJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	aircraft, err := h.aircraftService.GetAircraft(c.Request.Context(), uuid.UUID(aircraftId), userID)
	if err != nil {
		if errors.Is(err, service.ErrAircraftNotFound) || errors.Is(err, service.ErrUnauthorizedAircraft) {
			h.sendError(c, http.StatusNotFound, "Aircraft not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve aircraft")
		return
	}

	if req.Registration != nil {
		aircraft.Registration = *req.Registration
	}
	if req.Type != nil {
		aircraft.Type = *req.Type
	}
	if req.Make != nil {
		aircraft.Make = *req.Make
	}
	if req.Model != nil {
		aircraft.Model = *req.Model
	}
	if req.IsComplex != nil {
		aircraft.IsComplex = *req.IsComplex
	}
	if req.IsHighPerformance != nil {
		aircraft.IsHighPerformance = *req.IsHighPerformance
	}
	if req.IsTailwheel != nil {
		aircraft.IsTailwheel = *req.IsTailwheel
	}
	if req.Notes != nil {
		aircraft.Notes = req.Notes
	}
	if req.IsActive != nil {
		aircraft.IsActive = *req.IsActive
	}
	if req.AircraftClass != nil {
		s := string(*req.AircraftClass)
		aircraft.AircraftClass = &s
	}

	if err := h.aircraftService.UpdateAircraft(c.Request.Context(), aircraft, userID); err != nil {
		if errors.Is(err, service.ErrDuplicateRegistration) {
			h.sendError(c, http.StatusConflict, "Aircraft registration already exists")
			return
		}
		h.sendError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedAircraft(aircraft))
}

// DeleteAircraft implements DELETE /aircraft/{aircraftId}
// (DELETE /aircraft/{aircraftId})
func (h *APIHandler) DeleteAircraft(c *gin.Context, aircraftId generated.AircraftId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.aircraftService.DeleteAircraft(c.Request.Context(), uuid.UUID(aircraftId), userID); err != nil {
		if errors.Is(err, service.ErrAircraftNotFound) || errors.Is(err, service.ErrUnauthorizedAircraft) {
			h.sendError(c, http.StatusNotFound, "Aircraft not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete aircraft")
		return
	}

	c.Status(http.StatusNoContent)
}

func convertToGeneratedAircraft(a *models.Aircraft) generated.Aircraft {
	ac := generated.Aircraft{
		Id:                openapi_types.UUID(a.ID),
		UserId:            openapi_types.UUID(a.UserID),
		Registration:      a.Registration,
		Type:              a.Type,
		Make:              a.Make,
		Model:             a.Model,
		IsComplex:         &a.IsComplex,
		IsHighPerformance: &a.IsHighPerformance,
		IsTailwheel:       &a.IsTailwheel,
		Notes:             a.Notes,
		IsActive:          &a.IsActive,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
	}
	if a.AircraftClass != nil {
		ac.AircraftClass = a.AircraftClass
	}
	return ac
}
