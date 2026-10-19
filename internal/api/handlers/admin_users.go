package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
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
	h.persistBootstrapAdmin(c, user)
	return userID, true
}

// persistBootstrapAdmin sets the admin flag on the unflagged ADMIN_EMAIL holder.
func (h *APIHandler) persistBootstrapAdmin(c *gin.Context, user *models.User) {
	if h.adminRoles == nil {
		return
	}
	if err := h.adminRoles.PersistBootstrap(c.Request.Context(), user); err != nil {
		slog.Error("admin bootstrap: persist flag failed", "user_id", user.ID, "error", err)
	}
}

// sendAdminRoleError writes the response for an AdminRoleService error.
func (h *APIHandler) sendAdminRoleError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrAdminTargetNotFound):
		h.sendError(c, http.StatusNotFound, "User not found")
	case errors.Is(err, service.ErrAdminTargetUnverified):
		h.sendError(c, http.StatusConflict, "User has not verified their email address")
	case errors.Is(err, service.ErrAdminTargetDisabled):
		h.sendError(c, http.StatusConflict, "User account is disabled")
	case errors.Is(err, service.ErrBootstrapAdmin):
		h.sendError(c, http.StatusConflict, "User holds the ADMIN_EMAIL address")
	case errors.Is(err, service.ErrLastAdmin):
		h.sendError(c, http.StatusConflict, "Cannot remove the last admin")
	default:
		h.sendError(c, http.StatusInternalServerError, fallback)
	}
}

// requireAdminRoles writes 503 and returns false when the admin role service
// is not wired.
func (h *APIHandler) requireAdminRoles(c *gin.Context) bool {
	if h.adminRoles == nil {
		h.sendError(c, http.StatusServiceUnavailable, "Admin roles not configured")
		return false
	}
	return true
}

// GrantUserAdmin implements POST /admin/users/{userId}/grant-admin
func (h *APIHandler) GrantUserAdmin(c *gin.Context, userId openapi_types.UUID) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok || !h.requireAdminRoles(c) {
		return
	}

	targetID := uuid.UUID(userId)
	user, err := h.authService.GetUserByID(c.Request.Context(), targetID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}
	granted, err := h.adminRoles.Grant(c.Request.Context(), targetID)
	if err != nil {
		h.sendAdminRoleError(c, err, "Failed to grant admin rights")
		return
	}
	if granted {
		h.logAdminAction(c, adminUserID, "grant_admin", &targetID, map[string]any{"email": user.Email})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Admin rights granted"})
}

// RevokeUserAdmin implements POST /admin/users/{userId}/revoke-admin
func (h *APIHandler) RevokeUserAdmin(c *gin.Context, userId openapi_types.UUID) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok || !h.requireAdminRoles(c) {
		return
	}

	targetID := uuid.UUID(userId)
	user, err := h.authService.GetUserByID(c.Request.Context(), targetID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}
	revoked, err := h.adminRoles.Revoke(c.Request.Context(), targetID)
	if err != nil {
		h.sendAdminRoleError(c, err, "Failed to revoke admin rights")
		return
	}
	if revoked {
		h.logAdminAction(c, adminUserID, "revoke_admin", &targetID, map[string]any{"email": user.Email})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Admin rights revoked"})
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

	filter := repository.AdminUserFilter{IsAdmin: params.IsAdmin}
	if params.Search != nil {
		filter.Search = *params.Search
	}
	var rows []*repository.AdminUserRow
	var total int
	var err error
	if h.adminRoles != nil {
		rows, total, err = h.adminRoles.ListUsers(c.Request.Context(), filter, pageSize, (page-1)*pageSize)
	} else {
		rows, total, err = h.adminRepo.ListUsers(c.Request.Context(), filter, pageSize, (page-1)*pageSize)
	}
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query users")
		return
	}

	var users []generated.AdminUser
	for _, row := range rows {
		isLocked := row.LockedUntil != nil && row.LockedUntil.After(time.Now())
		emailSuppressed := row.EmailSuppressed
		rowUser := &models.User{Email: row.Email, EmailVerified: row.EmailVerified, IsAdmin: row.IsAdmin}
		adminUser := generated.AdminUser{
			Id:               openapi_types.UUID(row.ID),
			Email:            openapi_types.Email(row.Email),
			Name:             row.Name,
			CreatedAt:        row.CreatedAt,
			EmailVerified:    row.EmailVerified,
			TwoFactorEnabled: row.TwoFactorEnabled,
			Disabled:         row.Disabled,
			IsAdmin:          h.isAdminUser(rowUser),
			IsBootstrapAdmin: h.adminRoles != nil && h.adminRoles.IsBootstrapAdmin(rowUser),
			AdminFlag:        row.IsAdmin,
			Locked:           &isLocked,
			EmailSuppressed:  &emailSuppressed,
			FlightCount:      row.FlightCount,
			AircraftCount:    row.AircraftCount,
		}
		if row.LastLoginAt != nil {
			adminUser.LastLoginAt = row.LastLoginAt
		}
		if row.LastActiveAt != nil {
			adminUser.LastActiveAt = row.LastActiveAt
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
	if !h.requireAdminRoles(c) {
		return
	}
	revoked, err := h.adminRoles.Disable(c.Request.Context(), targetID)
	if err != nil {
		h.sendAdminRoleError(c, err, "Failed to disable user")
		return
	}
	if revoked {
		h.logAdminAction(c, adminUserID, "revoke_admin", &targetID, map[string]any{"email": user.Email, "reason": "disable_user"})
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

	if h.adminRoles != nil && h.adminRoles.IsBootstrapAdmin(user) {
		h.sendError(c, http.StatusConflict, "User holds the ADMIN_EMAIL address")
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

	// Removing the second factor ends every session.
	_ = h.authService.RevokeAllSessions(c.Request.Context(), targetID)

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
	if !h.requireAdminRoles(c) {
		return
	}

	// Cascading FKs remove the account's content, tokens and sessions.
	revoked, err := h.adminRoles.Delete(c.Request.Context(), targetID)
	if err != nil {
		h.sendAdminRoleError(c, err, "Failed to delete user")
		return
	}

	details := map[string]any{"email": user.Email, "name": user.Name, "user_id": targetID.String()}
	if revoked {
		h.logAdminAction(c, adminUserID, "revoke_admin", nil, map[string]any{"email": user.Email, "user_id": targetID.String(), "reason": "delete_user"})
	}
	h.logAdminAction(c, adminUserID, "delete_user", nil, details)

	c.JSON(http.StatusOK, gin.H{"message": "User and all their content deleted"})
}
