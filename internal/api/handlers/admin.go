package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
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
	if err != nil || !h.isAdminUser(user) {
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

	total, err := h.adminRepo.CountAuditLog(c.Request.Context())
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to count audit log entries")
		return
	}

	logEntries, err := h.adminRepo.ListAuditLog(c.Request.Context(), pageSize, (page-1)*pageSize)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query audit log")
		return
	}

	var entries []generated.AdminAuditLogEntry
	for _, le := range logEntries {
		entry := generated.AdminAuditLogEntry{
			Id:          openapi_types.UUID(le.ID),
			AdminUserId: openapi_types.UUID(le.AdminUserID),
			Action:      le.Action,
			CreatedAt:   le.CreatedAt,
		}
		if le.AdminEmail != nil {
			e := openapi_types.Email(*le.AdminEmail)
			entry.AdminEmail = &e
		}
		if le.AdminName != nil {
			entry.AdminName = le.AdminName
		}
		if le.TargetUserID != nil {
			tid := openapi_types.UUID(*le.TargetUserID)
			entry.TargetUserId = &tid
		}
		if le.TargetUserEmail != nil {
			e := openapi_types.Email(*le.TargetUserEmail)
			entry.TargetUserEmail = &e
		}
		if le.TargetUserName != nil {
			entry.TargetUserName = le.TargetUserName
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

// logAdminAction records an admin action to the audit log. details is
// marshalled with encoding/json; the column is JSONB.
func (h *APIHandler) logAdminAction(c *gin.Context, adminUserID uuid.UUID, action string, targetUserID *uuid.UUID, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	payload, err := json.Marshal(details)
	if err != nil {
		slog.Error("audit log: marshal details", "action", action, "error", err)
		payload = []byte(`{}`)
	}
	if err := h.adminRepo.InsertAuditLog(c.Request.Context(), &repository.AdminAuditLogEntry{
		ID:           uuid.New(),
		AdminUserID:  adminUserID,
		Action:       action,
		TargetUserID: targetUserID,
		Details:      payload,
		CreatedAt:    time.Now(),
	}); err != nil {
		slog.Error("audit log: insert failed", "action", action, "admin_user_id", adminUserID, "error", err)
	}
}
