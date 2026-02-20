package handlers

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
)

// GetAdminStats implements GET /admin/stats
func (h *APIHandler) GetAdminStats(c *gin.Context) {
	_, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	var stats generated.AdminStats

	h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM users").Scan(&stats.TotalUsers)
	h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM flights").Scan(&stats.TotalFlights)
	h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM aircraft").Scan(&stats.TotalAircraft)
	h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM credentials").Scan(&stats.TotalCredentials)
	h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM flight_imports").Scan(&stats.TotalImports)

	// Flights this month
	monthStart := time.Now().Format("2006-01") + "-01"
	h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM flights WHERE created_at >= $1", monthStart,
	).Scan(&stats.FlightsThisMonth)

	// New users this week
	weekAgo := time.Now().AddDate(0, 0, -7)
	h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM users WHERE created_at >= $1", weekAgo,
	).Scan(&stats.NewUsersThisWeek)

	// Locked accounts (locked_until in the future)
	h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM users WHERE locked_until IS NOT NULL AND locked_until > $1", time.Now(),
	).Scan(&stats.LockedAccounts)

	// Disabled accounts
	h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM users WHERE disabled = true",
	).Scan(&stats.DisabledAccounts)

	c.JSON(http.StatusOK, stats)
}

// CleanupTokens implements POST /admin/maintenance/cleanup-tokens
func (h *APIHandler) CleanupTokens(c *gin.Context) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	now := time.Now()

	// Delete expired refresh tokens
	result1, _ := h.db.ExecContext(c.Request.Context(),
		"DELETE FROM refresh_tokens WHERE expires_at < $1 OR revoked = true", now)
	refreshDeleted, _ := result1.RowsAffected()

	// Delete expired/used password reset tokens
	result2, _ := h.db.ExecContext(c.Request.Context(),
		"DELETE FROM password_reset_tokens WHERE expires_at < $1 OR used = true", now)
	resetDeleted, _ := result2.RowsAffected()

	h.logAdminAction(c, adminUserID, "cleanup_tokens", nil,
		fmt.Sprintf(`{"refreshTokensDeleted":%d,"resetTokensDeleted":%d}`, refreshDeleted, resetDeleted))

	c.JSON(http.StatusOK, gin.H{
		"refreshTokensDeleted": refreshDeleted,
		"resetTokensDeleted":   resetDeleted,
		"message":              fmt.Sprintf("Cleaned up %d refresh tokens and %d reset tokens", refreshDeleted, resetDeleted),
	})
}

// SmtpTest implements POST /admin/maintenance/smtp-test
func (h *APIHandler) SmtpTest(c *gin.Context) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	user, err := h.authService.GetUserByID(c.Request.Context(), adminUserID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to get admin user")
		return
	}

	if h.emailSender == nil {
		h.sendError(c, http.StatusBadRequest, "Email sender not configured")
		return
	}

	subject := "NinerLog SMTP Test"
	body := fmt.Sprintf(`<h2>SMTP Test Successful</h2>
<p>This is a test email from the NinerLog admin console.</p>
<p>Sent at: %s</p>
<p>If you received this email, your SMTP configuration is working correctly.</p>`,
		time.Now().Format(time.RFC3339))

	if err := h.emailSender.Send(user.Email, subject, body); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to send test email")
		return
	}

	h.logAdminAction(c, adminUserID, "smtp_test", nil,
		fmt.Sprintf(`{"sentTo":"%s"}`, user.Email))

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Test email sent to %s", user.Email),
	})
}

// GetAdminConfig implements GET /admin/config
func (h *APIHandler) GetAdminConfig(c *gin.Context) {
	_, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	// Calculate uptime
	uptime := time.Since(h.startedAt)
	days := int(uptime.Hours()) / 24
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60
	uptimeStr := fmt.Sprintf("%dd %dh %dm", days, hours, minutes)

	// Get migration version
	var migrationVersion int
	h.db.QueryRowContext(c.Request.Context(),
		"SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = false",
	).Scan(&migrationVersion)

	// SMTP configured?
	smtpConfigured := h.emailSender != nil

	// Admin email configured?
	adminEmailConfigured := h.adminEmail != ""

	config := generated.AdminConfig{
		GoVersion:            runtime.Version(),
		ServerUptime:         uptimeStr,
		MigrationVersion:     migrationVersion,
		AirportDatabaseSize:  airports.Count(),
		CorsOrigins:          h.corsOrigins,
		RateLimitAuth:        "10 req/min",
		RateLimitAdmin:       "30 req/min",
		SmtpConfigured:       smtpConfigured,
		AdminEmailConfigured: adminEmailConfigured,
	}

	c.JSON(http.StatusOK, config)
}
