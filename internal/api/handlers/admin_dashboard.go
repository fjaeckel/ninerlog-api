package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
)

// scanCount scans a single count value from a query row, defaulting to 0 on error.
func scanCount(row *sql.Row, dest *int) {
	if err := row.Scan(dest); err != nil {
		log.Printf("admin stats: count query failed: %v", err)
		*dest = 0
	}
}

// GetAdminStats implements GET /admin/stats
func (h *APIHandler) GetAdminStats(c *gin.Context) {
	_, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	var stats generated.AdminStats

	scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM users"), &stats.TotalUsers)
	scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM flights"), &stats.TotalFlights)
	scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM aircraft"), &stats.TotalAircraft)
	scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM credentials"), &stats.TotalCredentials)
	scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM flight_imports"), &stats.TotalImports)

	// Flights this month
	monthStart := time.Now().Format("2006-01") + "-01"
	scanCount(h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM flights WHERE created_at >= $1", monthStart,
	), &stats.FlightsThisMonth)

	// New users this week
	weekAgo := time.Now().AddDate(0, 0, -7)
	scanCount(h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM users WHERE created_at >= $1", weekAgo,
	), &stats.NewUsersThisWeek)

	// Locked accounts (locked_until in the future)
	scanCount(h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM users WHERE locked_until IS NOT NULL AND locked_until > $1", time.Now(),
	), &stats.LockedAccounts)

	// Disabled accounts
	scanCount(h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM users WHERE disabled = true",
	), &stats.DisabledAccounts)

	// Cloud backup destinations: total count + breakdown by provider.
	// Always queryable since the table is part of the standard schema; an
	// empty result simply yields total=0 and an empty byProvider map.
	stats.CloudBackupDestinations.ByProvider = map[string]int{}
	rows, err := h.db.QueryContext(c.Request.Context(),
		"SELECT provider, COUNT(*) FROM backup_destinations GROUP BY provider")
	if err != nil {
		log.Printf("admin stats: backup_destinations query failed: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var provider string
			var count int
			if err := rows.Scan(&provider, &count); err != nil {
				log.Printf("admin stats: backup_destinations scan failed: %v", err)
				continue
			}
			stats.CloudBackupDestinations.ByProvider[provider] = count
			stats.CloudBackupDestinations.Total += count
		}
	}

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

	// Soft-expire past-due pending signature requests (not hard-deleted —
	// flight_signatures is an append-only audit trail).
	var signaturesExpired int64
	if h.flightSignatureService != nil {
		signaturesExpired, _ = h.flightSignatureService.ExpirePendingRequests(c.Request.Context())
	}

	h.logAdminAction(c, adminUserID, "cleanup_tokens", nil,
		fmt.Sprintf(`{"refreshTokensDeleted":%d,"resetTokensDeleted":%d,"signatureRequestsExpired":%d}`, refreshDeleted, resetDeleted, signaturesExpired))

	c.JSON(http.StatusOK, gin.H{
		"refreshTokensDeleted":     refreshDeleted,
		"resetTokensDeleted":       resetDeleted,
		"signatureRequestsExpired": signaturesExpired,
		"message":                  fmt.Sprintf("Cleaned up %d refresh tokens, %d reset tokens, and expired %d signature requests", refreshDeleted, resetDeleted, signaturesExpired),
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

// TriggerNotifications implements POST /admin/maintenance/trigger-notifications
func (h *APIHandler) TriggerNotifications(c *gin.Context) {
	adminUserID, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	h.notificationService.TriggerCheck(c.Request.Context())

	h.logAdminAction(c, adminUserID, "trigger_notifications", nil, `{}`)

	c.JSON(http.StatusOK, gin.H{
		"message": "Notification check triggered for all users",
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
	scanCount(h.db.QueryRowContext(c.Request.Context(),
		"SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = false",
	), &migrationVersion)

	// SMTP configured?
	smtpConfigured := h.emailSender != nil

	// Admin email configured?
	adminEmailConfigured := h.adminEmail != ""

	// Cloud backups configured? (set when BACKUP_CREDENTIALS_KEY is provided)
	cloudBackupsConfigured := h.backupService != nil
	cloudBackupProviders := []string{}
	if cloudBackupsConfigured {
		for _, p := range h.backupService.ListProviders() {
			cloudBackupProviders = append(cloudBackupProviders, p.Name())
		}
	}

	config := generated.AdminConfig{
		GoVersion:              runtime.Version(),
		ServerUptime:           uptimeStr,
		MigrationVersion:       migrationVersion,
		AirportDatabaseSize:    airports.Count(),
		CorsOrigins:            h.corsOrigins,
		RateLimitAuth:          "10 req/min",
		RateLimitAdmin:         "30 req/min",
		SmtpConfigured:         smtpConfigured,
		AdminEmailConfigured:   adminEmailConfigured,
		CloudBackupsConfigured: cloudBackupsConfigured,
		CloudBackupProviders:   cloudBackupProviders,
	}

	c.JSON(http.StatusOK, config)
}
