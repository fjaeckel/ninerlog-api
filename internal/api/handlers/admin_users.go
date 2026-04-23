package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
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
	if err != nil || !h.isAdminUser(user.Email) {
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

	// Build query with optional search
	countQuery := "SELECT COUNT(*) FROM users"
	dataQuery := `
		SELECT u.id, u.email, u.name, u.created_at, u.last_login_at,
		       u.two_factor_enabled, u.disabled, u.failed_login_attempts,
		       u.locked_until,
		       (SELECT COUNT(*) FROM flights WHERE user_id = u.id) as flight_count,
		       (SELECT COUNT(*) FROM aircraft WHERE user_id = u.id) as aircraft_count
		FROM users u
	`
	var args []interface{}
	argIdx := 1

	if params.Search != nil && *params.Search != "" {
		searchClause := fmt.Sprintf(" WHERE LOWER(email) LIKE LOWER($%d) OR LOWER(name) LIKE LOWER($%d)", argIdx, argIdx)
		pattern := "%" + *params.Search + "%"
		args = append(args, pattern)
		argIdx++
		countQuery += searchClause
		dataQuery += searchClause
	}

	dataQuery += fmt.Sprintf(" ORDER BY u.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	// Count total
	var total int
	if len(args) > 2 {
		scanCount(h.db.QueryRowContext(c.Request.Context(), countQuery, args[0]), &total)
	} else {
		scanCount(h.db.QueryRowContext(c.Request.Context(), countQuery), &total)
	}

	// Query users
	rows, err := h.db.QueryContext(c.Request.Context(), dataQuery, args...)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to query users")
		return
	}
	defer rows.Close()

	var users []generated.AdminUser
	for rows.Next() {
		var id uuid.UUID
		var email, name string
		var createdAt time.Time
		var lastLoginAt *time.Time
		var twoFactorEnabled, disabled bool
		var failedAttempts int
		var lockedUntil *time.Time
		var flightCount, aircraftCount int

		if err := rows.Scan(&id, &email, &name, &createdAt, &lastLoginAt,
			&twoFactorEnabled, &disabled, &failedAttempts, &lockedUntil,
			&flightCount, &aircraftCount); err != nil {
			continue
		}

		isLocked := lockedUntil != nil && lockedUntil.After(time.Now())
		adminUser := generated.AdminUser{
			Id:               openapi_types.UUID(id),
			Email:            openapi_types.Email(email),
			Name:             name,
			CreatedAt:        createdAt,
			TwoFactorEnabled: twoFactorEnabled,
			Disabled:         disabled,
			Locked:           &isLocked,
			FlightCount:      flightCount,
			AircraftCount:    aircraftCount,
		}
		if lastLoginAt != nil {
			adminUser.LastLoginAt = lastLoginAt
		}
		if lockedUntil != nil && lockedUntil.After(time.Now()) {
			adminUser.LockedUntil = lockedUntil
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
	_, _ = h.db.ExecContext(c.Request.Context(),
		"DELETE FROM refresh_tokens WHERE user_id = $1", targetID)

	h.logAdminAction(c, adminUserID, "disable_user", &targetID,
		fmt.Sprintf(`{"email":"%s"}`, strings.ReplaceAll(user.Email, `"`, `\"`)))

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

	h.logAdminAction(c, adminUserID, "enable_user", &targetID,
		fmt.Sprintf(`{"email":"%s"}`, strings.ReplaceAll(user.Email, `"`, `\"`)))

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
	_, _ = h.db.ExecContext(c.Request.Context(),
		"UPDATE users SET failed_login_attempts = 0, locked_until = NULL, updated_at = $1 WHERE id = $2",
		time.Now(), targetID)

	h.logAdminAction(c, adminUserID, "unlock_user", &targetID,
		fmt.Sprintf(`{"email":"%s"}`, strings.ReplaceAll(user.Email, `"`, `\"`)))

	c.JSON(http.StatusOK, gin.H{"message": "User unlocked"})
}

// ResetUser2fa implements POST /admin/users/{userId}/reset-2fa
func (h *APIHandler) ResetUser2fa(c *gin.Context, userId openapi_types.UUID) {
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

	h.logAdminAction(c, adminUserID, "reset_2fa", &targetID,
		fmt.Sprintf(`{"email":"%s"}`, strings.ReplaceAll(user.Email, `"`, `\"`)))

	c.JSON(http.StatusOK, gin.H{"message": "2FA reset for user"})
}
