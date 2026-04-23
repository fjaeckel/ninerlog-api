package handlers

import (
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
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

	// 1. Fetch active system announcements
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, message, severity, expires_at, created_at
		FROM system_announcements
		WHERE expires_at IS NULL OR expires_at > $1
		ORDER BY created_at DESC
	`, now)

	var announcements []generated.Announcement
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var message, severity string
			var expiresAt *time.Time
			var createdAt time.Time
			if err := rows.Scan(&id, &message, &severity, &expiresAt, &createdAt); err != nil {
				continue
			}
			a := generated.Announcement{
				Id:        id.String(),
				Message:   message,
				Severity:  generated.AnnouncementSeverity(severity),
				CreatedAt: &createdAt,
			}
			if expiresAt != nil {
				a.ExpiresAt = expiresAt
			}
			announcements = append(announcements, a)
		}
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
		var flightCount int
		scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM flights WHERE user_id = $1", userID), &flightCount)
		if flightCount == 0 {
			hints = append(hints, generated.Announcement{
				Id:       "hint-add-first-flight",
				Message:  "Welcome! Log your first flight to get started.",
				Severity: "info",
			})
		}

		// Hint: No aircraft
		var aircraftCount int
		scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM aircraft WHERE user_id = $1", userID), &aircraftCount)
		if aircraftCount == 0 && flightCount > 0 {
			hints = append(hints, generated.Announcement{
				Id:       "hint-add-aircraft",
				Message:  "Add your aircraft to auto-fill registrations when logging flights.",
				Severity: "info",
			})
		}

		// Hint: No credentials
		var credentialCount int
		scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM credentials WHERE user_id = $1", userID), &credentialCount)
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

	_, err := h.db.ExecContext(c.Request.Context(), `
		INSERT INTO system_announcements (id, message, severity, expires_at, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.Message, req.Severity, req.ExpiresAt, adminUserID, now)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to create announcement")
		return
	}

	h.logAdminAction(c, adminUserID, "create_announcement", nil, req.Message)

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
	result, err := h.db.ExecContext(c.Request.Context(),
		"DELETE FROM system_announcements WHERE id = $1", targetID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to delete announcement")
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		h.sendError(c, http.StatusNotFound, "Announcement not found")
		return
	}

	h.logAdminAction(c, adminUserID, "delete_announcement", nil, targetID.String())
	c.Status(http.StatusNoContent)
}
