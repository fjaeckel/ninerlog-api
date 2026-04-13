package handlers

import (
	"net/http"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
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
	if req.EnabledCategories != nil {
		categories := make(pq.StringArray, len(*req.EnabledCategories))
		for i, c := range *req.EnabledCategories {
			categories[i] = string(c)
		}
		prefs.EnabledCategories = categories
	}
	if req.WarningDays != nil {
		days := make(pq.Int64Array, len(*req.WarningDays))
		for i, d := range *req.WarningDays {
			days[i] = int64(d)
		}
		prefs.WarningDays = days
	}
	if req.CheckHour != nil {
		if *req.CheckHour < 0 || *req.CheckHour > 23 {
			h.sendError(c, http.StatusBadRequest, "Check hour must be between 0 and 23")
			return
		}
		prefs.CheckHour = *req.CheckHour
	}

	if err := h.notificationService.UpdatePreferences(c.Request.Context(), prefs); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to update notification preferences")
		return
	}

	c.JSON(http.StatusOK, convertToGeneratedPrefs(prefs))
}

// GetNotificationHistory implements GET /users/me/notifications/history
func (h *APIHandler) GetNotificationHistory(c *gin.Context, params generated.GetNotificationHistoryParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	limit := 20
	offset := 0
	if params.Limit != nil {
		limit = *params.Limit
		if limit <= 0 || limit > 100 {
			limit = 20
		}
	}
	if params.Offset != nil {
		offset = *params.Offset
		if offset < 0 {
			offset = 0
		}
	}

	logs, total, err := h.notificationService.GetNotificationHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to get notification history")
		return
	}

	items := make([]generated.NotificationHistoryEntry, len(logs))
	for i, l := range logs {
		entry := generated.NotificationHistoryEntry{
			Id:       l.ID,
			Category: generated.NotificationCategory(l.NotificationType),
			SentAt:   l.SentAt,
		}
		if l.Subject != nil {
			entry.Subject = *l.Subject
		} else {
			entry.Subject = l.NotificationType
		}
		if l.ReferenceType != nil {
			entry.ReferenceType = l.ReferenceType
		}
		if l.DaysBeforeExpiry != nil {
			entry.DaysBeforeExpiry = l.DaysBeforeExpiry
		}
		items[i] = entry
	}

	c.JSON(http.StatusOK, generated.NotificationHistoryResponse{
		Items: items,
		Total: total,
	})
}

func convertToGeneratedPrefs(p *models.NotificationPreferences) generated.NotificationPreferences {
	days := make([]int, len(p.WarningDays))
	for i, d := range p.WarningDays {
		days[i] = int(d)
	}
	categories := make([]generated.NotificationCategory, len(p.EnabledCategories))
	for i, c := range p.EnabledCategories {
		categories[i] = generated.NotificationCategory(c)
	}
	return generated.NotificationPreferences{
		EmailEnabled:      p.EmailEnabled,
		EnabledCategories: categories,
		WarningDays:       days,
		CheckHour:         p.CheckHour,
	}
}
