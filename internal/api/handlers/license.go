package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// ListLicenses implements GET /licenses
// (GET /licenses)
func (h *APIHandler) ListLicenses(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	licenses, err := h.licenseService.ListLicenses(c.Request.Context(), userID)
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
		UserID:              userID,
		RegulatoryAuthority: req.RegulatoryAuthority,
		LicenseType:         req.LicenseType,
		LicenseNumber:       req.LicenseNumber,
		IssueDate:           issueDate,
		IssuingAuthority:    req.IssuingAuthority,
	}
	if req.RequiresSeparateLogbook != nil {
		license.RequiresSeparateLogbook = *req.RequiresSeparateLogbook
	}

	if err := h.licenseService.CreateLicense(c.Request.Context(), &license); err != nil {
		if errors.Is(err, service.ErrInvalidLicense) {
			h.sendError(c, http.StatusBadRequest, "Invalid license data: ensure all required fields are provided")
			return
		}
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
	if req.RegulatoryAuthority != nil {
		license.RegulatoryAuthority = *req.RegulatoryAuthority
	}
	if req.LicenseType != nil {
		license.LicenseType = *req.LicenseType
	}
	if req.LicenseNumber != nil {
		license.LicenseNumber = *req.LicenseNumber
	}
	if req.IssuingAuthority != nil {
		license.IssuingAuthority = *req.IssuingAuthority
	}
	if req.RequiresSeparateLogbook != nil {
		license.RequiresSeparateLogbook = *req.RequiresSeparateLogbook
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

	// Calculate currency from real flight data
	result, err := h.flightService.GetCurrency(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to calculate currency")
		return
	}

	reqDay := result.RequiredDay
	reqNight := result.RequiredNight

	currency := generated.Currency{
		LicenseId:     openapi_types.UUID(license.ID),
		IsCurrent:     result.IsCurrent,
		DaysCurrent:   result.DaysCurrent,
		NightsCurrent: result.NightsCurrent,
		Last90Days: struct {
			DayLandings   int "json:\"dayLandings\""
			Flights       int "json:\"flights\""
			NightLandings int "json:\"nightLandings\""
			TotalLandings int "json:\"totalLandings\""
		}{
			Flights:       result.Flights90Days,
			TotalLandings: result.TotalLandings,
			DayLandings:   result.DayLandings,
			NightLandings: result.NightLandings,
		},
		RequiredLandings: &struct {
			Day   int "json:\"day\""
			Night int "json:\"night\""
		}{
			Day:   reqDay,
			Night: reqNight,
		},
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
	stats, err := h.flightService.GetStatsByUserID(c.Request.Context(), userID, startDate, endDate)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to calculate statistics")
		return
	}

	lid := openapi_types.UUID(licenseId)
	statistics := generated.Statistics{
		LicenseId:           &lid,
		TotalFlights:        stats.TotalFlights,
		TotalMinutes:        stats.TotalMinutes,
		PicMinutes:          stats.PICMinutes,
		DualMinutes:         stats.DualMinutes,
		NightMinutes:        stats.NightMinutes,
		IfrMinutes:          stats.IFRMinutes,
		SoloMinutes:         ptrInt(stats.SoloMinutes),
		CrossCountryMinutes: ptrInt(stats.CrossCountryMinutes),
		LandingsDay:         stats.LandingsDay,
		LandingsNight:       stats.LandingsNight,
	}

	c.JSON(http.StatusOK, statistics)
}

// Helper function to convert models.License to generated.License
func convertToGeneratedLicense(lic *models.License) generated.License {
	reqSepLogbook := lic.RequiresSeparateLogbook
	license := generated.License{
		Id:                      openapi_types.UUID(lic.ID),
		UserId:                  openapi_types.UUID(lic.UserID),
		RegulatoryAuthority:     lic.RegulatoryAuthority,
		LicenseType:             lic.LicenseType,
		LicenseNumber:           lic.LicenseNumber,
		IssueDate:               openapi_types.Date{Time: lic.IssueDate},
		IssuingAuthority:        lic.IssuingAuthority,
		RequiresSeparateLogbook: &reqSepLogbook,
		CreatedAt:               lic.CreatedAt,
		UpdatedAt:               lic.UpdatedAt,
	}

	return license
}
