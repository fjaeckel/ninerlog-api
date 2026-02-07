package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// ListLicenses implements GET /licenses
// (GET /licenses)
func (h *APIHandler) ListLicenses(c *gin.Context, params generated.ListLicensesParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	isActive := params.IsActive != nil && *params.IsActive
	licenses, err := h.licenseService.ListLicenses(c.Request.Context(), userID, isActive)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve licenses")
		return
	}

	// Convert to OpenAPI License type
	response := make([]generated.License, 0, len(licenses))
	for _, lic := range licenses {
		response = append(response, convertToGeneratedLicense(lic))
	}

	c.JSON(http.StatusOK, response)
}

// CreateLicense implements POST /licenses
// (POST /licenses)
func (h *APIHandler) CreateLicense(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.CreateLicenseJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	issueDate, err := time.Parse("2006-01-02", req.IssueDate.String())
	if err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid issueDate format")
		return
	}

	license := models.License{
		UserID:           userID,
		LicenseType:      models.LicenseType(req.LicenseType),
		LicenseNumber:    req.LicenseNumber,
		IssueDate:        issueDate,
		IssuingAuthority: req.IssuingAuthority,
		IsActive:         true,
	}

	if req.ExpiryDate != nil {
		expiryDate, err := time.Parse("2006-01-02", req.ExpiryDate.String())
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Invalid expiryDate format")
			return
		}
		license.ExpiryDate = &expiryDate
	}

	if err := h.licenseService.CreateLicense(c.Request.Context(), &license); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to create license")
		return
	}

	c.JSON(http.StatusCreated, convertToGeneratedLicense(&license))
}

// GetLicense implements GET /licenses/{licenseId}
// (GET /licenses/{licenseId})
func (h *APIHandler) GetLicense(c *gin.Context, licenseId generated.LicenseId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	license, err := h.licenseService.GetLicense(c.Request.Context(), uuid.UUID(licenseId), userID)
	if err != nil {
		if errors.Is(err, service.ErrLicenseNotFound) || errors.Is(err, service.ErrUnauthorizedAccess) {
			h.sendError(c, http.StatusNotFound, "License not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve license")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedLicense(license))
}

// UpdateLicense implements PATCH /licenses/{licenseId}
// (PATCH /licenses/{licenseId})
func (h *APIHandler) UpdateLicense(c *gin.Context, licenseId generated.LicenseId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.UpdateLicenseJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get existing license
	license, err := h.licenseService.GetLicense(c.Request.Context(), uuid.UUID(licenseId), userID)
	if err != nil {
		if errors.Is(err, service.ErrLicenseNotFound) || errors.Is(err, service.ErrUnauthorizedAccess) {
			h.sendError(c, http.StatusNotFound, "License not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve license")
		return
	}

	// Apply updates
	if req.ExpiryDate != nil {
		expiryDate, err := time.Parse("2006-01-02", req.ExpiryDate.String())
		if err != nil {
			h.sendError(c, http.StatusBadRequest, "Invalid expiryDate format")
			return
		}
		license.ExpiryDate = &expiryDate
	}
	if req.IsActive != nil {
		license.IsActive = *req.IsActive
	}

	if err := h.licenseService.UpdateLicense(c.Request.Context(), license, userID); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to update license")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedLicense(license))
}

// DeleteLicense implements DELETE /licenses/{licenseId}
// (DELETE /licenses/{licenseId})
func (h *APIHandler) DeleteLicense(c *gin.Context, licenseId generated.LicenseId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.licenseService.DeleteLicense(c.Request.Context(), uuid.UUID(licenseId), userID); err != nil {
		if errors.Is(err, service.ErrLicenseNotFound) || errors.Is(err, service.ErrUnauthorizedAccess) {
			h.sendError(c, http.StatusNotFound, "License not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete license")
		return
	}

	// OpenAPI spec requires 204 No Content for successful DELETE
	c.Status(http.StatusNoContent)
}

// GetLicenseCurrency implements GET /licenses/{licenseId}/currency
// (GET /licenses/{licenseId}/currency)
func (h *APIHandler) GetLicenseCurrency(c *gin.Context, licenseId generated.LicenseId) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Verify license ownership
	license, err := h.licenseService.GetLicense(c.Request.Context(), uuid.UUID(licenseId), userID)
	if err != nil {
		if errors.Is(err, service.ErrLicenseNotFound) || errors.Is(err, service.ErrUnauthorizedAccess) {
			h.sendError(c, http.StatusNotFound, "License not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve license")
		return
	}

	// TODO: Implement currency calculation service
	// For now, return a placeholder response matching the OpenAPI schema
	currency := generated.Currency{
		LicenseId:     openapi_types.UUID(license.ID),
		IsCurrent:     false,
		DaysCurrent:   false,
		NightsCurrent: false,
		Last90Days: struct {
			DayLandings   int "json:\"dayLandings\""
			Flights       int "json:\"flights\""
			NightLandings int "json:\"nightLandings\""
			TotalLandings int "json:\"totalLandings\""
		}{
			Flights:       0,
			TotalLandings: 0,
			DayLandings:   0,
			NightLandings: 0,
		},
	}

	if license.ExpiryDate != nil {
		expiryDate := openapi_types.Date{Time: *license.ExpiryDate}
		currency.ExpiryDate = &expiryDate
	}

	c.JSON(http.StatusOK, currency)
}

// GetLicenseStatistics implements GET /licenses/{licenseId}/statistics
// (GET /licenses/{licenseId}/statistics)
func (h *APIHandler) GetLicenseStatistics(c *gin.Context, licenseId generated.LicenseId, params generated.GetLicenseStatisticsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Verify license ownership
	_, err = h.licenseService.GetLicense(c.Request.Context(), uuid.UUID(licenseId), userID)
	if err != nil {
		if errors.Is(err, service.ErrLicenseNotFound) || errors.Is(err, service.ErrUnauthorizedAccess) {
			h.sendError(c, http.StatusNotFound, "License not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to retrieve license")
		return
	}

	// Parse optional date filters
	var startDate, endDate *time.Time
	if params.StartDate != nil {
		t := params.StartDate.Time
		startDate = &t
	}
	if params.EndDate != nil {
		t := params.EndDate.Time
		endDate = &t
	}

	// Get real statistics from database
	stats, err := h.flightService.GetStatsByLicenseID(c.Request.Context(), uuid.UUID(licenseId), userID, startDate, endDate)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to calculate statistics")
		return
	}

	statistics := generated.Statistics{
		LicenseId:     openapi_types.UUID(licenseId),
		TotalFlights:  stats.TotalFlights,
		TotalHours:    float32(stats.TotalHours),
		PicHours:      float32(stats.PICHours),
		DualHours:     float32(stats.DualHours),
		NightHours:    float32(stats.NightHours),
		IfrHours:      float32(stats.IFRHours),
		LandingsDay:   stats.LandingsDay,
		LandingsNight: stats.LandingsNight,
	}

	c.JSON(http.StatusOK, statistics)
}

// Helper function to convert models.License to generated.License
func convertToGeneratedLicense(lic *models.License) generated.License {
	license := generated.License{
		Id:               openapi_types.UUID(lic.ID),
		UserId:           openapi_types.UUID(lic.UserID),
		LicenseType:      generated.LicenseType(lic.LicenseType),
		LicenseNumber:    lic.LicenseNumber,
		IssueDate:        openapi_types.Date{Time: lic.IssueDate},
		IssuingAuthority: lic.IssuingAuthority,
		IsActive:         lic.IsActive,
		CreatedAt:        lic.CreatedAt,
		UpdatedAt:        lic.UpdatedAt,
	}

	if lic.ExpiryDate != nil {
		expiryDate := openapi_types.Date{Time: *lic.ExpiryDate}
		license.ExpiryDate = &expiryDate
	}

	return license
}
