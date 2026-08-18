package handlers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/gin-gonic/gin"
)

// scanCount scans a single count value from a query row, defaulting to 0 on error.
func scanCount(row *sql.Row, dest *int) {
	if err := row.Scan(dest); err != nil {
		slog.Error("admin stats: count query failed", "error", err)
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
	scanCount(h.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM contacts"), &stats.TotalContacts)
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
	stats.CloudBackupDestinations.ByProvider = map[string]int{}
	rows, err := h.db.QueryContext(c.Request.Context(),
		"SELECT provider, COUNT(*) FROM backup_destinations GROUP BY provider")
	if err != nil {
		slog.Error("admin stats: backup_destinations query failed", "error", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var provider string
			var count int
			if err := rows.Scan(&provider, &count); err != nil {
				slog.Error("admin stats: backup_destinations scan failed", "error", err)
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
		map[string]any{
			"refreshTokensDeleted": refreshDeleted, "resetTokensDeleted": resetDeleted,
			"signatureRequestsExpired": signaturesExpired})

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

	if err := h.emailSender.SendMessage(c.Request.Context(), emailpkg.Message{
		To: user.Email, Subject: subject, HTMLBody: body, Type: emailpkg.TypeAdminTest,
	}); err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to send test email")
		return
	}

	h.logAdminAction(c, adminUserID, "smtp_test", nil,
		map[string]any{"sentTo": user.Email})

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

	h.logAdminAction(c, adminUserID, "trigger_notifications", nil, nil)

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

	// Authentication mode. The issuer is a public URL and safe to show an
	// admin; the client secret deliberately has no representation here.
	authMode := generated.AdminConfigAuthModeLocal
	var oidcIssuer *string
	if h.oidcService != nil {
		authMode = generated.AdminConfigAuthModeOidc
		issuer := h.oidcService.Config().Issuer
		oidcIssuer = &issuer
	}

	documentFilesEnabled := h.documentFileService != nil && h.documentFileService.Enabled()

	config := generated.AdminConfig{
		AuthMode:               &authMode,
		OidcIssuer:             oidcIssuer,
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
		DocumentFilesEnabled:   &documentFilesEnabled,
	}

	// The unverified-account lifecycle timing is reported only when running;
	// otherwise the disabled reason is reported instead.
	enabled := h.unverifiedAccountService != nil
	config.UnverifiedCleanupEnabled = &enabled
	if enabled {
		reaperCfg := h.unverifiedAccountService.Config()
		reminderAfter := reaperCfg.ReminderAfter.String()
		retention := reaperCfg.Retention.String()
		config.UnverifiedReminderAfter = &reminderAfter
		config.UnverifiedRetention = &retention
	} else if reason := service.UnverifiedCleanupDisabledReason(
		h.emailSender.IsConfigured(), h.oidcService != nil,
	); reason != "" {
		disabledReason := generated.AdminConfigUnverifiedCleanupDisabledReason(reason)
		config.UnverifiedCleanupDisabledReason = &disabledReason
	}

	if h.emailDeliveryService != nil {
		if count, err := h.emailDeliveryService.CountSuppressions(c.Request.Context()); err == nil {
			config.EmailSuppressedCount = &count
		}
	}

	c.JSON(http.StatusOK, config)
}
