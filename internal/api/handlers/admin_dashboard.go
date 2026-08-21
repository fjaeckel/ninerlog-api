package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/updatecheck"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// prefixesReviewed is registration.LastReviewed parsed as a date.
var prefixesReviewed, _ = time.Parse("2006-01-02", registration.LastReviewed)

// GetAdminStats implements GET /admin/stats
func (h *APIHandler) GetAdminStats(c *gin.Context) {
	_, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	adminStats, err := h.adminRepo.GetStats(c.Request.Context(), time.Now())
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to load admin stats")
		return
	}

	var stats generated.AdminStats
	stats.TotalUsers = adminStats.TotalUsers
	stats.TotalFlights = adminStats.TotalFlights
	stats.TotalAircraft = adminStats.TotalAircraft
	stats.TotalContacts = adminStats.TotalContacts
	stats.TotalCredentials = adminStats.TotalCredentials
	stats.TotalImports = adminStats.TotalImports
	stats.FlightsThisMonth = adminStats.FlightsThisMonth
	stats.NewUsersThisWeek = adminStats.NewUsersThisWeek
	stats.LockedAccounts = adminStats.LockedAccounts
	stats.DisabledAccounts = adminStats.DisabledAccounts
	stats.ActiveSessions = adminStats.ActiveSessions
	stats.ImportsByFormat = adminStats.ImportsByFormat
	stats.CloudBackupDestinations.ByProvider = adminStats.BackupDestinationsByProvider
	for _, count := range adminStats.BackupDestinationsByProvider {
		stats.CloudBackupDestinations.Total += count
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
	refreshDeleted, err := h.adminRepo.DeleteExpiredRefreshTokens(c.Request.Context(), now)
	if err != nil {
		slog.Error("cleanup tokens: refresh token sweep failed", "error", err)
	}

	// Delete expired/used password reset tokens
	resetDeleted, err := h.adminRepo.DeleteExpiredPasswordResetTokens(c.Request.Context(), now)
	if err != nil {
		slog.Error("cleanup tokens: password reset token sweep failed", "error", err)
	}

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

	// Get migration version (0 when it cannot be read — the config page is
	// more useful degraded than dead).
	migrationVersion, err := h.adminRepo.MigrationVersion(c.Request.Context())
	if err != nil {
		slog.Error("admin config: migration version query failed", "error", err)
		migrationVersion = 0
	}

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
	var oidcNativeRedirect *string
	if h.oidcService != nil {
		authMode = generated.AdminConfigAuthModeOidc
		issuer := h.oidcService.Config().Issuer
		oidcIssuer = &issuer
		nativeRedirect := h.oidcService.NativePostLoginRedirect()
		oidcNativeRedirect = &nativeRedirect
	}

	documentFilesEnabled := h.documentFileService != nil && h.documentFileService.Enabled()

	var airportsUpdatedAt *time.Time
	if t := airports.LoadedAt(); !t.IsZero() {
		airportsUpdatedAt = &t
	}

	sessionPolicy := h.authService.SessionPolicy()
	reuseGrace := sessionPolicy.ReuseGrace.String()

	config := generated.AdminConfig{
		AuthMode:                     &authMode,
		OidcIssuer:                   oidcIssuer,
		OidcNativeRedirect:           oidcNativeRedirect,
		GoVersion:                    runtime.Version(),
		ServerUptime:                 uptimeStr,
		MigrationVersion:             migrationVersion,
		AirportDatabaseSize:          airports.Count(),
		AirportDatabaseUpdatedAt:     airportsUpdatedAt,
		RegistrationPrefixCount:      registration.Count(),
		RegistrationPrefixesReviewed: openapi_types.Date{Time: prefixesReviewed},
		CorsOrigins:                  h.corsOrigins,
		RateLimitAuth:                "10 req/min",
		RateLimitAdmin:               "30 req/min",
		MaxSessionsPerUser:           &sessionPolicy.MaxPerUser,
		RefreshReuseGrace:            &reuseGrace,
		SmtpConfigured:               smtpConfigured,
		AdminEmailConfigured:         adminEmailConfigured,
		CloudBackupsConfigured:       cloudBackupsConfigured,
		CloudBackupProviders:         cloudBackupProviders,
		DocumentFilesEnabled:         &documentFilesEnabled,
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

	appVersion := updatecheck.RunningVersion()
	config.AppVersion = &appVersion
	updateCheckEnabled := h.updateChecker != nil && h.updateChecker.Enabled()
	config.UpdateCheckEnabled = &updateCheckEnabled
	if updateCheckEnabled && h.updateChecker.Interval() > 0 {
		interval := h.updateChecker.Interval().String()
		config.UpdateCheckInterval = &interval
	}

	c.JSON(http.StatusOK, config)
}
