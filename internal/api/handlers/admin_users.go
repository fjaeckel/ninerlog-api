package handlers

import (
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// requireAdmin checks admin privileges and returns the admin user ID. Returns false if not admin.
func (h *APIHandler) requireAdmin(c *gin.Context) (uuid.UUID, bool) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return uuid.Nil, false
	}
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil || !h.isAdminUser(user) {
		h.sendError(c, http.StatusForbidden, "Admin access required")
		return uuid.Nil, false
	}
	return userID, true
}

// ListAdminUsers implements GET /admin/users
func (h *APIHandler) ListAdminUsers(c *gin.Context, params generated.ListAdminUsersParams) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	_ = adminUserID

	page := 1
	pageSize := 20
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	if params.PageSize != nil && *params.PageSize > 0 {
		pageSize = *params.PageSize
	}

	search := ""
	if params.Search != nil {
		search = *params.Search
	}
	rows, total, err := h.adminRepo.ListUsers(c.Request.Context(), search, pageSize, (page-1)*pageSize)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query users")
		return
	}

	var users []generated.AdminUser
	for _, row := range rows {
		isLocked := row.LockedUntil != nil && row.LockedUntil.After(time.Now())
		emailSuppressed := row.EmailSuppressed
		adminUser := generated.AdminUser{
			Id:               openapi_types.UUID(row.ID),
			Email:            openapi_types.Email(row.Email),
			Name:             row.Name,
			CreatedAt:        row.CreatedAt,
			EmailVerified:    row.EmailVerified,
			TwoFactorEnabled: row.TwoFactorEnabled,
			Disabled:         row.Disabled,
			Locked:           &isLocked,
			EmailSuppressed:  &emailSuppressed,
			FlightCount:      row.FlightCount,
			AircraftCount:    row.AircraftCount,
		}
		if row.LastLoginAt != nil {
			adminUser.LastLoginAt = row.LastLoginAt
		}
		// The reminder timestamp is what the retention clock counts from, so
		// the deletion date is derived rather than stored. Both are reported
		// only while the account is still unverified: on a verified account the
		// stamp is a historical footnote, not a pending deletion.
		if !row.EmailVerified && row.VerificationReminderSentAt != nil {
			adminUser.VerificationReminderSentAt = row.VerificationReminderSentAt
			if h.unverifiedAccountService != nil {
				scheduled := row.VerificationReminderSentAt.Add(h.unverifiedAccountService.Config().Retention)
				adminUser.ScheduledDeletionAt = &scheduled
			}
		}
		if isLocked {
			adminUser.LockedUntil = row.LockedUntil
		}
		users = append(users, adminUser)
	}
	if users == nil {
		users = []generated.AdminUser{}
	}

	totalPages := (total + pageSize - 1) / pageSize
	c.JSON(http.StatusOK, generated.PaginatedAdminUsers{
		Data: users,
		Pagination: struct {
			Page       int `json:"page"`
			PageSize   int `json:"pageSize"`
			Total      int `json:"total"`
			TotalPages int `json:"totalPages"`
		}{Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages},
	})
}

// DisableUser implements POST /admin/users/{userId}/disable
func (h *APIHandler) DisableUser(c *gin.Context, userId openapi_types.UUID) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	targetID := uuid.UUID(userId)
	if targetID == adminUserID {
		h.sendError(c, http.StatusBadRequest, "Cannot disable your own account")
		return
	}

	user, err := h.authService.GetUserByID(c.Request.Context(), targetID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	user.Disabled = true
	user.UpdatedAt = time.Now()
	if err := h.authService.UpdateUser(c.Request.Context(), user); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to disable user")
		return
	}

	// Revoke all tokens
	_ = h.authService.RevokeAllSessions(c.Request.Context(), targetID)

	h.logAdminAction(c, adminUserID, "disable_user", &targetID, map[string]any{"email": user.Email})

	c.JSON(http.StatusOK, gin.H{"message": "User disabled"})
}

// EnableUser implements POST /admin/users/{userId}/enable
func (h *APIHandler) EnableUser(c *gin.Context, userId openapi_types.UUID) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	targetID := uuid.UUID(userId)
	user, err := h.authService.GetUserByID(c.Request.Context(), targetID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	user.Disabled = false
	user.UpdatedAt = time.Now()
	if err := h.authService.UpdateUser(c.Request.Context(), user); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to enable user")
		return
	}

	h.logAdminAction(c, adminUserID, "enable_user", &targetID, map[string]any{"email": user.Email})

	c.JSON(http.StatusOK, gin.H{"message": "User enabled"})
}

// UnlockUser implements POST /admin/users/{userId}/unlock
func (h *APIHandler) UnlockUser(c *gin.Context, userId openapi_types.UUID) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	targetID := uuid.UUID(userId)
	user, err := h.authService.GetUserByID(c.Request.Context(), targetID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	// Reset failed login attempts and lockout
	if err := h.adminRepo.UnlockUser(c.Request.Context(), targetID); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to unlock user")
		return
	}

	h.logAdminAction(c, adminUserID, "unlock_user", &targetID, map[string]any{"email": user.Email})

	c.JSON(http.StatusOK, gin.H{"message": "User unlocked"})
}

// ResetUser2fa implements POST /admin/users/{userId}/reset-2fa
func (h *APIHandler) ResetUser2fa(c *gin.Context, userId openapi_types.UUID) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	// TOTP does not exist in OIDC mode — second factors are the provider's
	// responsibility there — so there is nothing for an admin to reset.
	if !h.requireLocalAuth(c) {
		return
	}

	targetID := uuid.UUID(userId)
	user, err := h.authService.GetUserByID(c.Request.Context(), targetID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	if !user.TwoFactorEnabled {
		h.sendError(c, http.StatusBadRequest, "2FA is not enabled for this user")
		return
	}

	// Disable 2FA
	user.TwoFactorEnabled = false
	user.TwoFactorSecret = nil
	user.RecoveryCodes = nil
	user.UpdatedAt = time.Now()
	if err := h.authService.UpdateUser(c.Request.Context(), user); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to reset 2FA")
		return
	}

	h.logAdminAction(c, adminUserID, "reset_2fa", &targetID, map[string]any{"email": user.Email})

	// Tell the account owner their second factor is gone. Without this the
	// removal is invisible to them until the next sign-in silently skips the
	// 2FA challenge.
	if h.emailSender != nil && user.Email != "" {
		tmpl := email.Templates(user.PreferredLocale)
		subject, body := tmpl.TwoFactorReset(email.TwoFactorResetParams{UserName: user.Name})
		_ = h.emailSender.Send(user.Email, subject, body)
	}

	c.JSON(http.StatusOK, gin.H{"message": "2FA reset for user"})
}

// DeleteUser implements DELETE /admin/users/{userId}
// Permanently deletes a user and all of their content via FK ON DELETE CASCADE.
func (h *APIHandler) DeleteUser(c *gin.Context, userId openapi_types.UUID) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	targetID := uuid.UUID(userId)
	if targetID == adminUserID {
		h.sendError(c, http.StatusBadRequest, "Cannot delete your own account")
		return
	}

	user, err := h.authService.GetUserByID(c.Request.Context(), targetID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	// Log the action BEFORE deleting so the audit log captures the email in
	// metadata. The audit row's target_user_id will be set to NULL by the FK
	// cascade, which is the intended retention behaviour.
	h.logAdminAction(c, adminUserID, "delete_user", &targetID,
		map[string]any{"email": user.Email, "name": user.Name})

	// Cascading FKs handle removal of flights, aircraft, licenses, contacts,
	// credentials, refresh tokens, backups, notifications, etc.
	if err := h.authService.DeleteUserConfirmed(c.Request.Context(), targetID); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User and all their content deleted"})
}
