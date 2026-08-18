package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// GetAnnouncements implements GET /announcements
// Returns active system announcements + auto-generated user hints
func (h *APIHandler) GetAnnouncements(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	now := time.Now()

	// 1. Fetch active system announcements; a failure yields an empty list.
	var announcements []generated.Announcement
	if active, err := h.announcementRepo.ListActive(c.Request.Context(), now); err == nil {
		for _, sa := range active {
			a := generated.Announcement{
				Id:        sa.ID.String(),
				Message:   sa.Message,
				Severity:  generated.AnnouncementSeverity(sa.Severity),
				CreatedAt: &sa.CreatedAt,
			}
			if sa.ExpiresAt != nil {
				a.ExpiresAt = sa.ExpiresAt
			}
			announcements = append(announcements, a)
		}
	} else {
		slog.Error("announcements: listing failed", "error", err)
	}
	if announcements == nil {
		announcements = []generated.Announcement{}
	}

	// 2. Generate automatic user hints
	var hints []generated.Announcement
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err == nil {
		// Hint: Enable 2FA
		if !user.TwoFactorEnabled {
			hints = append(hints, generated.Announcement{
				Id:       "hint-enable-2fa",
				Message:  "Secure your account — enable two-factor authentication in Profile Settings.",
				Severity: "success",
			})
		}

		// Hint: No flights yet
		flightCount, err := h.flightService.CountFlights(c.Request.Context(), userID, nil)
		if err != nil {
			slog.Error("announcements: flight count failed", "error", err)
			flightCount = 0
		}
		if flightCount == 0 {
			hints = append(hints, generated.Announcement{
				Id:       "hint-add-first-flight",
				Message:  "Welcome! Log your first flight to get started.",
				Severity: "info",
			})
		}

		// Hint: No aircraft
		aircraftCount, err := h.aircraftService.CountAircraft(c.Request.Context(), userID)
		if err != nil {
			slog.Error("announcements: aircraft count failed", "error", err)
			aircraftCount = 0
		}
		if aircraftCount == 0 && flightCount > 0 {
			hints = append(hints, generated.Announcement{
				Id:       "hint-add-aircraft",
				Message:  "Add your aircraft to auto-fill registrations when logging flights.",
				Severity: "info",
			})
		}

		// Hint: No credentials
		credentialCount := 0
		if credentials, err := h.credentialService.ListCredentials(c.Request.Context(), userID); err == nil {
			credentialCount = len(credentials)
		} else {
			slog.Error("announcements: credential listing failed", "error", err)
		}
		if credentialCount == 0 && flightCount > 3 {
			hints = append(hints, generated.Announcement{
				Id:       "hint-add-credentials",
				Message:  "Track your medical and language proficiency — add credentials to get expiry reminders.",
				Severity: "info",
			})
		}
	}
	if hints == nil {
		hints = []generated.Announcement{}
	}

	c.JSON(http.StatusOK, gin.H{
		"announcements": announcements,
		"hints":         hints,
	})
}

// CreateAnnouncement implements POST /admin/announcements
func (h *APIHandler) CreateAnnouncement(c *gin.Context) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	var req struct {
		Message   string     `json:"message" binding:"required"`
		Severity  string     `json:"severity" binding:"required"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate severity
	switch req.Severity {
	case "info", "success", "warning", "critical":
	default:
		h.sendError(c, http.StatusBadRequest, "Severity must be info, success, warning, or critical")
		return
	}

	id := uuid.New()
	now := time.Now()

	if err := h.announcementRepo.Create(c.Request.Context(), &models.SystemAnnouncement{
		ID:        id,
		Message:   req.Message,
		Severity:  req.Severity,
		ExpiresAt: req.ExpiresAt,
		CreatedBy: adminUserID,
		CreatedAt: now,
	}); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to create announcement")
		return
	}

	h.logAdminAction(c, adminUserID, "create_announcement", nil, map[string]any{"message": req.Message, "severity": req.Severity})

	announcement := generated.Announcement{
		Id:        id.String(),
		Message:   req.Message,
		Severity:  generated.AnnouncementSeverity(req.Severity),
		CreatedAt: &now,
	}
	if req.ExpiresAt != nil {
		announcement.ExpiresAt = req.ExpiresAt
	}

	c.JSON(http.StatusCreated, announcement)
}

// DeleteAnnouncement implements DELETE /admin/announcements/{announcementId}
func (h *APIHandler) DeleteAnnouncement(c *gin.Context, announcementId openapi_types.UUID) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	targetID := uuid.UUID(announcementId)
	if err := h.announcementRepo.Delete(c.Request.Context(), targetID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.sendError(c, http.StatusNotFound, "Announcement not found")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete announcement")
		return
	}

	h.logAdminAction(c, adminUserID, "delete_announcement", nil, map[string]any{"announcementId": targetID.String()})
	c.Status(http.StatusNoContent)
}
