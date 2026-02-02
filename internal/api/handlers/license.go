package handlers

import (
	"net/http"

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

func (h *LicenseHandler) CreateLicense(c *gin.Context) {
	userID, err := h.getUserIDFromToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	var license models.License
	if err := c.ShouldBindJSON(&license); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	license.UserID = userID
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
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Apply updates to license (only ExpiryDate and IsActive are updatable)
	if expiryDate, ok := updates["expiry_date"].(string); ok {
		if expiryDate != "" {
			parsed, err := uuid.Parse(expiryDate)
			if err == nil {
				// Handle time parsing if needed
				_ = parsed
			}
		}
	}
	if isActive, ok := updates["is_active"].(bool); ok {
		license.IsActive = isActive
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
