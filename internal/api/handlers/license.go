package handlers

import (
	"net/http"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type LicenseHandler struct {
	licenseService *service.LicenseService
	jwtManager     *jwt.Manager
}

func NewLicenseHandler(licenseService *service.LicenseService, jwtManager *jwt.Manager) *LicenseHandler {
	return &LicenseHandler{
		licenseService: licenseService,
		jwtManager:     jwtManager,
	}
}

// createLicenseRequest is the JSON request body for creating a license.
// Uses string dates (YYYY-MM-DD) matching the OpenAPI spec.
type createLicenseRequest struct {
	LicenseType      models.LicenseType `json:"licenseType" binding:"required"`
	LicenseNumber    string             `json:"licenseNumber" binding:"required"`
	IssueDate        string             `json:"issueDate" binding:"required"`
	ExpiryDate       *string            `json:"expiryDate,omitempty"`
	IssuingAuthority string             `json:"issuingAuthority" binding:"required"`
	IsActive         *bool              `json:"isActive,omitempty"`
}

func (h *LicenseHandler) CreateLicense(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	var req createLicenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	issueDate, err := time.Parse("2006-01-02", req.IssueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issueDate format, expected YYYY-MM-DD"})
		return
	}

	license := models.License{
		UserID:           userID,
		LicenseType:      req.LicenseType,
		LicenseNumber:    req.LicenseNumber,
		IssueDate:        issueDate,
		IssuingAuthority: req.IssuingAuthority,
		IsActive:         true,
	}

	if req.ExpiryDate != nil && *req.ExpiryDate != "" {
		expiryDate, err := time.Parse("2006-01-02", *req.ExpiryDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expiryDate format, expected YYYY-MM-DD"})
			return
		}
		license.ExpiryDate = &expiryDate
	}

	if req.IsActive != nil {
		license.IsActive = *req.IsActive
	}

	if err := h.licenseService.CreateLicense(c.Request.Context(), &license); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, license)
}

func (h *LicenseHandler) GetLicense(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	licenseIDStr := c.Param("id")
	licenseID, err := uuid.Parse(licenseIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid license ID"})
		return
	}

	license, err := h.licenseService.GetLicense(c.Request.Context(), licenseID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "license not found"})
		return
	}

	c.JSON(http.StatusOK, license)
}

func (h *LicenseHandler) ListLicenses(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	activeOnly := c.Query("active") == "true"
	licenses, err := h.licenseService.ListLicenses(c.Request.Context(), userID, activeOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, licenses)
}

func (h *LicenseHandler) UpdateLicense(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	licenseIDStr := c.Param("id")
	licenseID, err := uuid.Parse(licenseIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid license ID"})
		return
	}

	// Get existing license
	license, err := h.licenseService.GetLicense(c.Request.Context(), licenseID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "license not found"})
		return
	}

	// Parse update fields
	var updates struct {
		LicenseType      *string `json:"licenseType"`
		LicenseNumber    *string `json:"licenseNumber"`
		IssueDate        *string `json:"issueDate"`
		ExpiryDate       *string `json:"expiryDate"`
		IssuingAuthority *string `json:"issuingAuthority"`
		IsActive         *bool   `json:"isActive"`
	}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if updates.LicenseType != nil {
		license.LicenseType = models.LicenseType(*updates.LicenseType)
	}
	if updates.LicenseNumber != nil {
		license.LicenseNumber = *updates.LicenseNumber
	}
	if updates.IssueDate != nil {
		parsed, err := time.Parse("2006-01-02", *updates.IssueDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issueDate format, expected YYYY-MM-DD"})
			return
		}
		license.IssueDate = parsed
	}
	if updates.ExpiryDate != nil {
		if *updates.ExpiryDate == "" {
			license.ExpiryDate = nil
		} else {
			parsed, err := time.Parse("2006-01-02", *updates.ExpiryDate)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expiryDate format, expected YYYY-MM-DD"})
				return
			}
			license.ExpiryDate = &parsed
		}
	}
	if updates.IssuingAuthority != nil {
		license.IssuingAuthority = *updates.IssuingAuthority
	}
	if updates.IsActive != nil {
		license.IsActive = *updates.IsActive
	}

	if err := h.licenseService.UpdateLicense(c.Request.Context(), license, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "license updated successfully"})
}

func (h *LicenseHandler) DeleteLicense(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	licenseIDStr := c.Param("id")
	licenseID, err := uuid.Parse(licenseIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid license ID"})
		return
	}

	if err := h.licenseService.DeleteLicense(c.Request.Context(), licenseID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "license deleted successfully"})
}

func (h *LicenseHandler) getUserIDFromToken(c *gin.Context) (uuid.UUID, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) < 8 {
		return uuid.Nil, jwt.ErrInvalidToken
	}

	tokenString := authHeader[7:]
	claims, err := h.jwtManager.ValidateAccessToken(tokenString)
	if err != nil {
		return uuid.Nil, err
	}

	return claims.UserID, nil
}
