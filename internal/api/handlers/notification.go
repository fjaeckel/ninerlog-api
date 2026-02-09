package handlers

import (
	"net/http"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// GetNotificationPreferences implements GET /users/me/notifications
func (h *APIHandler) GetNotificationPreferences(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	prefs, err := h.notificationService.GetPreferences(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to get notification preferences")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedPrefs(prefs))
}

// UpdateNotificationPreferences implements PATCH /users/me/notifications
func (h *APIHandler) UpdateNotificationPreferences(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.UpdateNotificationPreferencesJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get current preferences
	prefs, err := h.notificationService.GetPreferences(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to get notification preferences")
		return
	}

	prefs.UserID = userID

	// Apply updates
	if req.EmailEnabled != nil {
		prefs.EmailEnabled = *req.EmailEnabled
	}
	if req.CurrencyWarnings != nil {
		prefs.CurrencyWarnings = *req.CurrencyWarnings
	}
	if req.CredentialWarnings != nil {
		prefs.CredentialWarnings = *req.CredentialWarnings
	}
	if req.WarningDays != nil {
		days := make(pq.Int64Array, len(*req.WarningDays))
		for i, d := range *req.WarningDays {
			days[i] = int64(d)
		}
		prefs.WarningDays = days
	}

	if err := h.notificationService.UpdatePreferences(c.Request.Context(), prefs); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to update notification preferences")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedPrefs(prefs))
}

func convertToGeneratedPrefs(p *models.NotificationPreferences) generated.NotificationPreferences {
	days := make([]int, len(p.WarningDays))
	for i, d := range p.WarningDays {
		days[i] = int(d)
	}
	return generated.NotificationPreferences{
		EmailEnabled:       p.EmailEnabled,
		CurrencyWarnings:   p.CurrencyWarnings,
		CredentialWarnings: p.CredentialWarnings,
		WarningDays:        days,
	}
}
