package handlers

import (
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// ListAdminAuditLog implements GET /admin/audit-log
func (h *APIHandler) ListAdminAuditLog(c *gin.Context, params generated.ListAdminAuditLogParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Verify admin access
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil || !h.isAdminUser(user.Email) {
		h.sendError(c, http.StatusForbidden, "Admin access required")
		return
	}

	page := 1
	pageSize := 20
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	if params.PageSize != nil && *params.PageSize > 0 {
		pageSize = *params.PageSize
	}

	// Count total
	var total int
	if err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM admin_audit_log",
	).Scan(&total); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to count audit log entries")
		return
	}

	// Query entries
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, admin_user_id, action, target_user_id, details, created_at
		FROM admin_audit_log
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, pageSize, (page-1)*pageSize)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query audit log")
		return
	}
	defer rows.Close()

	var entries []generated.AdminAuditLogEntry
	for rows.Next() {
		var id, adminUserID uuid.UUID
		var action string
		var targetUserID *uuid.UUID
		var details []byte
		var createdAt time.Time

		if err := rows.Scan(&id, &adminUserID, &action, &targetUserID, &details, &createdAt); err != nil {
			continue
		}

		entry := generated.AdminAuditLogEntry{
			Id:          openapi_types.UUID(id),
			AdminUserId: openapi_types.UUID(adminUserID),
			Action:      action,
			CreatedAt:   createdAt,
		}
		if targetUserID != nil {
			tid := openapi_types.UUID(*targetUserID)
			entry.TargetUserId = &tid
		}

		entries = append(entries, entry)
	}
	if entries == nil {
		entries = []generated.AdminAuditLogEntry{}
	}

	totalPages := (total + pageSize - 1) / pageSize
	c.JSON(http.StatusOK, generated.PaginatedAdminAuditLog{
		Data: entries,
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

// logAdminAction records an admin action to the audit log
func (h *APIHandler) logAdminAction(c *gin.Context, adminUserID uuid.UUID, action string, targetUserID *uuid.UUID, details string) {
	_, _ = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO admin_audit_log (id, admin_user_id, action, target_user_id, details, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uuid.New(), adminUserID, action, targetUserID, details, time.Now())
}
