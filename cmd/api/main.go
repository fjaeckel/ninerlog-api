package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // #nosec G108 -- pprof is opt-in via PPROF_ENABLED and runs on a separate port
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/api/handlers"
	"github.com/fjaeckel/ninerlog-api/internal/api/middleware"
	"github.com/fjaeckel/ninerlog-api/internal/logging"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/s3"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/sftp"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider/webdav"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/internal/updatecheck"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// fatal logs a structured error and exits. Structured attributes (e.g. an
// "error" key) may be passed after the message, matching slog's variadic API.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// envInt reads a positive integer from the environment, falling back to def
// when the variable is unset, unparseable, or non-positive. A bad value logs
// a warning and returns the default.
func envInt(key string, def int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		slog.Warn("Ignoring invalid environment value, using default",
			"key", key, "value", raw, "default", def)
		return def
	}
	return v
}

// envIntNarrow is envInt for knobs consumed as a plain int, parsed at 32-bit
// width. Out-of-range, unparseable, and non-positive values keep the default.
func envIntNarrow(key string, def int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || v <= 0 {
		slog.Warn("Ignoring invalid environment value, using default",
			"key", key, "value", raw, "default", def)
		return def
	}
	return int(v)
}

// envDuration reads a Go duration (e.g. "24h", "60s") from the environment,
// keeping the default when the variable is unset, unparseable, or non-positive.
// envBoolWithLegacy reads a boolean feature switch, honouring a previous name
// for the same knob. Only the exact string "false" disables; the current name
// wins when both are set.
func envBoolWithLegacy(key, legacyKey string, def bool) bool {
	for _, k := range []string{key, legacyKey} {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			if k == legacyKey {
				slog.Warn("environment variable is deprecated, use the current name",
					"deprecated", legacyKey, "use", key)
			}
			return v != "false"
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v <= 0 {
		slog.Warn("Ignoring invalid environment value, using default",
			"key", key, "value", raw, "default", def.String())
		return def
	}
	return v
}

func main() {
	logging.Setup()
	slog.Info("Starting NinerLog API...")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Local development fallback.
		dbURL = "postgresql://localhost:5432/ninerlog?sslmode=disable"
	}
	// 10s server-side statement timeout on the DSN (migrations included).
	dbURL = withStatementTimeout(dbURL, 10*time.Second)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	refreshSecret := os.Getenv("REFRESH_SECRET")
	if err := validateJWTSecrets(jwtSecret, refreshSecret); err != nil {
		fatal("invalid JWT configuration", "error", err)
	}
	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:5173,http://localhost:80,http://192.168.148.1"
	}
	corsOrigins := strings.Split(corsOrigin, ",")
	for i := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(corsOrigins[i])
	}

	// Admin email (optional — designates admin user)
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail != "" {
		slog.Info("Admin email configured", "email", adminEmail)
	}

	slog.Info("Connecting to database...")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fatal("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fatal("failed to ping database", "error", err)
	}

	// Bound the connection pool.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	slog.Info("Database connected")

	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "db/migrations"
	}
	slog.Info("Running database migrations", "path", migrationsPath)
	driver, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		fatal("failed to create migration driver", "error", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres", driver)
	if err != nil {
		fatal("failed to initialize migrations", "error", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		fatal("failed to run migrations", "error", err)
	}
	slog.Info("Database migrations applied")

	// Load the in-memory airport database (OurAirports + mwgg, merged);
	// a failure here is non-fatal.
	airports.Init()

	jwtManager := jwt.NewManager(jwtSecret, refreshSecret, 15*time.Minute, 7*24*time.Hour)

	userRepo := postgres.NewUserRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	passwordResetRepo := postgres.NewPasswordResetTokenRepository(db)
	emailVerificationRepo := postgres.NewEmailVerificationTokenRepository(db)
	licenseRepo := postgres.NewLicenseRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	flightBaselineRepo := postgres.NewFlightBaselineRepository(db)
	// TOTP secrets are encrypted at rest only when TOTP_ENCRYPTION_KEY
	// (base64, 32 bytes) is set.
	var totpAEAD *cryptoutil.AEAD
	if totpKey := os.Getenv("TOTP_ENCRYPTION_KEY"); totpKey != "" {
		totpAEAD, err = cryptoutil.NewFromBase64(totpKey)
		if err != nil {
			fatal("invalid TOTP_ENCRYPTION_KEY", "error", err)
		}
		slog.Info("TOTP secrets encrypted at rest")
	} else {
		slog.Warn("TOTP_ENCRYPTION_KEY not set — 2FA secrets are stored unencrypted")
	}

	twoFactorService := service.NewTwoFactorService(userRepo, jwtManager, totpAEAD)
	sessionPolicy := service.SessionPolicy{
		MaxPerUser: envIntNarrow("MAX_SESSIONS_PER_USER", service.DefaultMaxSessionsPerUser),
		ReuseGrace: envDuration("REFRESH_REUSE_GRACE", service.DefaultRefreshReuseGrace),
	}
	slog.Info("Session policy configured",
		"max_per_user", sessionPolicy.MaxPerUser, "reuse_grace", sessionPolicy.ReuseGrace.String())
	authService := service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, emailVerificationRepo, jwtManager, twoFactorService, sessionPolicy)
	licenseService := service.NewLicenseService(licenseRepo)
	flightService := service.NewFlightService(flightRepo, flightBaselineRepo)
	flightSessionRepo := postgres.NewFlightSessionRepository(db)
	credentialRepo := postgres.NewCredentialRepository(db)
	credentialService := service.NewCredentialService(credentialRepo)
	aircraftRepo := postgres.NewAircraftRepository(db)
	aircraftService := service.NewAircraftService(aircraftRepo)
	notifRepo := postgres.NewNotificationRepository(db)
	smtpConfig := email.LoadSMTPConfig()
	emailSender := email.NewSender(smtpConfig)

	// The delivery recorder is attached after construction.
	emailDeliveryService := service.NewEmailDeliveryService(postgres.NewEmailDeliveryRepository(db))
	emailSender.SetDeliveryRecorder(emailDeliveryService)
	emailDeliveryService.RefreshSuppressionGauge(context.Background())

	contactRepo := postgres.NewContactRepository(db)
	contactService := service.NewContactService(contactRepo)
	classRatingRepo := postgres.NewClassRatingRepository(db)
	classRatingService := service.NewClassRatingService(classRatingRepo, licenseRepo)
	flightCrewRepo := postgres.NewFlightCrewRepository(db)
	webauthnCredRepo := postgres.NewWebAuthnCredentialRepository(db)
	webauthnSessionRepo := postgres.NewWebAuthnSessionRepository(db)

	flightDataProvider := postgres.NewCurrencyFlightDataProvider(db)
	currencyRegistry := currency.NewRegistry()
	currencyRegistry.Register(currency.NewEASAEvaluator())
	currencyRegistry.Register(currency.NewFAAEvaluator())
	currencyRegistry.Register(currency.NewOtherEvaluator())
	ulEval := currency.NewGermanULEvaluator()
	currencyRegistry.RegisterMulti(ulEval, ulEval.Authorities()...)
	currencyService := currency.NewService(currencyRegistry, licenseRepo, classRatingRepo, flightDataProvider)

	// Custom (user-authored) currency rules
	customCurrencyRepo := postgres.NewCustomCurrencyRuleRepository(db)
	customCurrencyEvaluator := currency.NewCustomEvaluator(postgres.NewCustomCurrencyDataProvider(db))
	customCurrencyService := currency.NewCustomService(customCurrencyRepo, customCurrencyEvaluator)
	customCurrencyHandler := handlers.NewCustomCurrencyHandler(customCurrencyService)

	notificationService := service.NewNotificationService(notifRepo, credentialRepo, flightRepo, licenseRepo, userRepo, emailSender, currencyService, customCurrencyService)

	// OIDC single sign-on (optional — enabled by setting OIDC_ISSUER). When
	// configured, every local credential path (passwords, registration, email
	// verification, TOTP, passkeys) is switched off. A malformed configuration
	// is fatal.
	oidcConfig, err := service.LoadOIDCConfig()
	if err != nil {
		fatal("invalid OIDC configuration", "error", err)
	}
	var oidcService *service.OIDCService
	if oidcConfig.Enabled() {
		oidcIdentityRepo := postgres.NewOIDCIdentityRepository(db)
		oidcService, err = service.NewOIDCService(oidcConfig, userRepo, oidcIdentityRepo, authService)
		if err != nil {
			fatal("failed to initialize OIDC", "error", err)
		}
		slog.Info("OIDC mode enabled — local passwords, registration, 2FA and passkeys are disabled",
			"issuer", oidcConfig.Issuer,
			"provider_name", oidcConfig.ProviderName,
			"scopes", strings.Join(oidcConfig.Scopes, " "),
			"link_by_verified_email", oidcConfig.LinkByVerifiedEmail,
			"trust_email_verified", oidcConfig.TrustEmailVerified)
	} else {
		slog.Info("Local authentication mode (set OIDC_ISSUER to use single sign-on)")
	}

	// WebAuthn / passkey service (optional — disabled if WEBAUTHN_RP_ID is not set).
	webauthnRPID := os.Getenv("WEBAUTHN_RP_ID")
	webauthnRPName := os.Getenv("WEBAUTHN_RP_NAME")
	if webauthnRPName == "" {
		webauthnRPName = "NinerLog"
	}
	webauthnOriginsRaw := os.Getenv("WEBAUTHN_RP_ORIGINS")
	if webauthnOriginsRaw == "" {
		webauthnOriginsRaw = corsOrigin
	}
	webauthnOrigins := strings.Split(webauthnOriginsRaw, ",")
	for i := range webauthnOrigins {
		webauthnOrigins[i] = strings.TrimSpace(webauthnOrigins[i])
	}
	// TTL for stored ceremony state; also drives the client-side WebAuthn
	// timeout.
	webauthnSessionTTL := service.DefaultWebAuthnSessionTTL
	if raw := os.Getenv("WEBAUTHN_SESSION_TTL"); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil || parsed <= 0 {
			slog.Warn("Ignoring invalid WEBAUTHN_SESSION_TTL, using default",
				"value", raw, "default", webauthnSessionTTL.String())
		} else {
			webauthnSessionTTL = parsed
		}
	}
	webauthnMaxOpenCeremonies := envIntNarrow("WEBAUTHN_MAX_OPEN_CEREMONIES",
		service.DefaultWebAuthnMaxOpenCeremonies)

	var webauthnService *service.WebAuthnService
	switch {
	case oidcConfig.Enabled():
		slog.Info("WebAuthn disabled (OIDC mode owns authentication)")
	case webauthnRPID != "":
		webauthnService, err = service.NewWebAuthnService(webauthnRPID, webauthnRPName, webauthnOrigins,
			webauthnCredRepo, webauthnSessionRepo, userRepo, authService,
			webauthnSessionTTL, webauthnMaxOpenCeremonies)
		if err != nil {
			slog.Warn("WebAuthn disabled", "error", err)
			webauthnService = nil
		} else {
			slog.Info("WebAuthn enabled", "rp_id", webauthnRPID,
				"session_ttl", webauthnSessionTTL.String(),
				"max_open_ceremonies", webauthnMaxOpenCeremonies)
		}
	default:
		slog.Info("WebAuthn disabled (set WEBAUTHN_RP_ID to enable)")
		_ = webauthnCredRepo
		_ = webauthnSessionRepo
	}

	apiHandler := handlers.NewAPIHandler(authService, licenseService, flightService, credentialService, aircraftService, notificationService, twoFactorService, contactService, classRatingService, currencyService, webauthnService, jwtManager, flightCrewRepo, adminEmail)
	apiHandler.SetOIDCService(oidcService)
	// Repositories the handler uses directly (admin console, reports, import
	// history, announcements, bulk wipes) — no raw *sql.DB reaches a handler.
	apiHandler.SetAdminRepository(postgres.NewAdminRepository(db))
	apiHandler.SetAnnouncementRepository(postgres.NewAnnouncementRepository(db))
	apiHandler.SetFlightImportRepository(postgres.NewFlightImportRepository(db))
	apiHandler.SetReportsRepository(postgres.NewReportsRepository(db))
	apiHandler.SetUserContentRepository(postgres.NewUserContentRepository(db))
	apiHandler.SetEmailSender(emailSender)
	flightSessionService := service.NewFlightSessionService(flightSessionRepo, aircraftRepo, flightService)
	apiHandler.SetFlightSessionService(flightSessionService)
	flightSignatureRepo := postgres.NewFlightSignatureRepository(db)
	flightSignatureService := service.NewFlightSignatureService(flightSignatureRepo, flightRepo, userRepo)
	apiHandler.SetFlightSignatureService(flightSignatureService)
	// Licence/credential reference files (JPEG, PNG, PDF).
	// DOCUMENT_FILES_ENABLED=false turns the feature off: every /files
	// endpoint answers 403, GET /features reports it, stored rows are kept.
	// DOCUMENT_IMAGES_ENABLED is the legacy name for the same knob.
	documentFilesEnabled := envBoolWithLegacy("DOCUMENT_FILES_ENABLED", "DOCUMENT_IMAGES_ENABLED", true)
	documentFileService := service.NewDocumentFileService(
		postgres.NewDocumentFileRepository(db), licenseRepo, credentialRepo, documentFilesEnabled)
	apiHandler.SetDocumentFileService(documentFileService)

	startedAt := time.Now()
	apiHandler.SetStartedAt(startedAt)
	apiHandler.SetCORSOrigins(corsOrigins)

	// Release check against GitHub (UPDATE_CHECK_ENABLED=false opts out).
	updateChecker := updatecheck.New(updatecheck.FromEnv())
	apiHandler.SetUpdateChecker(updateChecker)

	// Cloud backup service (optional — enabled only when BACKUP_CREDENTIALS_KEY is set).
	var backupScheduler *cloudbackup.Scheduler
	if backupKey := os.Getenv("BACKUP_CREDENTIALS_KEY"); backupKey != "" {
		aead, err := cryptoutil.NewFromBase64(backupKey)
		if err != nil {
			fatal("invalid BACKUP_CREDENTIALS_KEY", "error", err)
		}
		backupDestRepo := postgres.NewBackupDestinationRepository(db)
		backupRunRepo := postgres.NewBackupRunRepository(db)
		registry := provider.NewRegistry()
		registry.Register(s3.New())
		registry.Register(sftp.New())
		registry.Register(webdav.New())
		builder := &cloudbackup.DefaultJSONBuilder{
			Flights:     flightService,
			Aircraft:    aircraftService,
			Licenses:    licenseService,
			Credentials: credentialService,
			ClassRating: classRatingService,
			AttachCrew:  apiHandler.AttachCrewMembers,
		}
		backupSvc, err := cloudbackup.New(cloudbackup.Options{
			DestinationRepo: backupDestRepo,
			RunRepo:         backupRunRepo,
			Registry:        registry,
			Crypto:          aead,
			Builder:         builder,
		})
		if err != nil {
			fatal("failed to initialize cloud backup service", "error", err)
		}
		apiHandler.SetBackupService(backupSvc)
		backupScheduler = cloudbackup.NewScheduler(backupSvc, 0, nil)
		slog.Info("Cloud backups enabled (S3, SFTP, WebDAV providers)")
	} else {
		slog.Info("Cloud backups disabled (set BACKUP_CREDENTIALS_KEY to enable)")
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Metrics & recovery — first in the middleware chain.
	metricsEnabled := os.Getenv("METRICS_ENABLED") != "false" // default: true
	if metricsEnabled {
		router.Use(middleware.MetricsMiddleware())
	}
	router.Use(middleware.RecoveryWithMetrics())

	// Structured access log: one JSON line per request. nil selects the
	// default logger configured by logging.Setup.
	router.Use(middleware.LoggerMiddleware(nil))

	// Trust proxy headers (X-Real-IP, X-Forwarded-For) from nginx.
	if err := router.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}); err != nil {
		fatal("failed to set trusted proxies", "error", err)
	}
	router.ForwardedByClientIP = true
	router.RemoteIPHeaders = []string{"X-Real-IP", "X-Forwarded-For"}

	router.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", middleware.HeaderIdempotencyKey},
		ExposeHeaders:    []string{"Content-Length", middleware.HeaderIdempotencyReplayed, "Retry-After", handlers.HeaderCrewEntriesRenamed},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.Use(middleware.SecurityHeadersMiddleware())

	// Request body size limit for multipart uploads (10 MB)
	router.MaxMultipartMemory = 10 << 20

	// Cap request bodies: 1 MB non-multipart, 12 MB multipart, 50 MB for
	// POST /imports/json.
	router.Use(middleware.MaxBodyBytesMiddleware(1<<20, 12<<20, map[string]int64{
		"/imports/json": 50 << 20,
	}))

	// Health check with DB connectivity.
	router.GET("/health", func(c *gin.Context) {
		if err := db.Ping(); err != nil {
			if metricsEnabled {
				middleware.HealthCheckStatus.Set(0)
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":   "unhealthy",
				"database": "unreachable",
			})
			return
		}
		if metricsEnabled {
			middleware.HealthCheckStatus.Set(1)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Prometheus metrics endpoint (no auth required, alongside /health)
	if metricsEnabled {
		middleware.RegisterAppMetrics(updatecheck.RunningVersion(), startedAt)
		prometheus.MustRegister(middleware.NewDBStatsCollector(db))

		router.GET("/metrics", gin.WrapH(promhttp.Handler()))
		slog.Info("Prometheus metrics enabled at /metrics")
	}

	// pprof debug server (separate port, opt-in via PPROF_ENABLED=true)
	if os.Getenv("PPROF_ENABLED") == "true" {
		pprofPort := os.Getenv("PPROF_PORT")
		if pprofPort == "" {
			pprofPort = "6060"
		}
		go func() {
			slog.Info("pprof debug server listening", "addr", ":"+pprofPort+"/debug/pprof/")
			pprofSrv := &http.Server{
				Addr:              ":" + pprofPort,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			if err := pprofSrv.ListenAndServe(); err != nil {
				slog.Error("pprof server error", "error", err)
			}
		}()
	}

	api := router.Group("/api/v1")

	// 15s deadline on every request context.
	api.Use(middleware.RequestTimeoutMiddleware(15 * time.Second))

	// Client details for session records.
	api.Use(middleware.DeviceContext())

	// Centralized auth middleware — all routes require auth except explicit public paths
	api.Use(middleware.AuthMiddleware(jwtManager, []string{
		"/auth/register",
		"/auth/login",
		"/auth/providers",
		// OIDC login flow.
		"/auth/oidc/authorize",
		"/auth/oidc/callback",
		"/auth/oidc/exchange",
		"/auth/refresh",
		"/auth/logout",
		"/auth/2fa/login",
		"/auth/password-reset-request",
		"/auth/password-reset",
		"/auth/verify-email",
		"/auth/verify-email/resend",
		"/auth/webauthn/login/options",
		"/auth/webauthn/login/verify",
		"/airports/search",
		// Unauthenticated instructor signing links; a gin route *pattern*
		// matched via c.FullPath(), unlike the literal paths above.
		"/sign/:token",
	}))

	if os.Getenv("DISABLE_RATE_LIMIT") != "true" {
		// Coarse global limiter on every authenticated route, keyed by user ID.
		generalRateLimit := middleware.NewUserRateLimitMiddleware("general", 120, 1*time.Minute)
		api.Use(generalRateLimit)

		// Tighter limits for expensive endpoints, layered on the general
		// limiter.
		expensiveRateLimit := middleware.NewUserRateLimitMiddleware("expensive", 15, 1*time.Minute)
		api.Use(middleware.RateLimitByPath(expensiveRateLimit,
			"/exports/pdf",
			"/custom-currency/preview",
		))
		// GET /imports/templates is exempt: it serves a static catalogue and is
		// read every time the import screen opens, so it must not spend the
		// budget reserved for actual imports.
		api.Use(middleware.RateLimitByPathPrefixExcept(expensiveRateLimit,
			[]string{"/imports/templates"}, "/imports"))

		// Advanced search ("q") gets its own per-user bucket, tunable via
		// SEARCH_RATE_LIMIT_PER_MINUTE; plain /flights listing stays under
		// the general limiter only.
		searchRateLimit := middleware.NewUserRateLimitMiddleware(
			"search", envInt("SEARCH_RATE_LIMIT_PER_MINUTE", 60), 1*time.Minute)
		api.Use(middleware.RateLimitByPathWithQueryParam(searchRateLimit, "/flights", "q"))

		authRateLimit := middleware.NewRateLimitMiddleware("auth", 10, 1*time.Minute)
		api.Use(middleware.RateLimitByPath(authRateLimit,
			"/auth/register",
			"/auth/login",
			"/auth/refresh",
			"/auth/logout",
			"/auth/2fa/login",
			"/auth/password-reset-request",
			"/auth/password-reset",
			"/auth/verify-email",
			"/auth/verify-email/resend",
			"/auth/webauthn/login/options",
			"/auth/webauthn/login/verify",
		))

		// OIDC login and the capability probe get their own budget.
		oidcRateLimit := middleware.NewRateLimitMiddleware("oidc", 60, 1*time.Minute)
		api.Use(middleware.RateLimitByPath(oidcRateLimit,
			"/auth/providers",
			"/auth/oidc/authorize",
			"/auth/oidc/callback",
			"/auth/oidc/exchange",
		))

		// Admin endpoints: 30 requests per minute per IP.
		adminRateLimit := middleware.NewRateLimitMiddleware("admin", 30, 1*time.Minute)
		api.Use(middleware.RateLimitByPath(adminRateLimit,
			"/admin/stats",
			"/admin/users",
			"/admin/audit-log",
			"/admin/maintenance/cleanup-tokens",
			"/admin/maintenance/smtp-test",
			"/admin/announcements",
			"/admin/config",
			"/disable",
			"/enable",
			"/unlock",
			"/reset-2fa",
		))

		// Public signing links, rate-limited by path prefix.
		signRateLimit := middleware.NewRateLimitMiddleware("sign", 20, 1*time.Minute)
		api.Use(middleware.RateLimitByPathPrefix(signRateLimit, "/sign/"))

		// Licence/credential files: uploads share the "expensive" budget;
		// reads get their own per-user bucket, tunable via
		// FILE_READ_RATE_LIMIT_PER_MINUTE.
		fileReadRateLimit := middleware.NewUserRateLimitMiddleware(
			"file_read", envInt("FILE_READ_RATE_LIMIT_PER_MINUTE", 90), 1*time.Minute)
		api.Use(middleware.RateLimitByPathSegmentForMethods(
			fileReadRateLimit, []string{http.MethodGet}, "/files"))
		api.Use(middleware.RateLimitByPathSegmentForMethods(
			expensiveRateLimit, []string{http.MethodPost, http.MethodDelete}, "/files"))

		// Authenticated signature actions that trigger outbound email.
		signatureEmailRateLimit := middleware.NewRateLimitMiddleware("signature_email", 10, 1*time.Minute)
		api.Use(middleware.RateLimitByPath(signatureEmailRateLimit,
			"/signatures",
			"/resend",
		))
	} // end DISABLE_RATE_LIMIT check

	// Idempotent writes; a no-op for requests without an Idempotency-Key
	// header.
	idempotencyService := service.NewIdempotencyService(
		postgres.NewIdempotencyRepository(db),
		envDuration("IDEMPOTENCY_TTL", service.DefaultIdempotencyTTL),
		envDuration("IDEMPOTENCY_LEASE", service.DefaultIdempotencyLease),
		envIntNarrow("IDEMPOTENCY_MAX_RESPONSE_BYTES", service.DefaultIdempotencyMaxResponseBytes),
	)
	api.Use(middleware.IdempotencyMiddleware(idempotencyService))

	// Deletion tombstones. The rows are written by database triggers
	// (migration 000054); this serves and sweeps them.
	deletionService := service.NewDeletionService(
		postgres.NewDeletionRepository(db),
		envDuration("TOMBSTONE_RETENTION", service.DefaultTombstoneRetention),
	)
	apiHandler.SetDeletionService(deletionService)
	apiHandler.SetEmailDeliveryService(emailDeliveryService)

	// Unverified-account lifecycle. Constructed only when
	// UnverifiedCleanupDisabledReason returns empty; a non-empty reason means
	// neither the worker nor the admin endpoint exists.
	var unverifiedAccountService *service.UnverifiedAccountService
	if reason := service.UnverifiedCleanupDisabledReason(emailSender.IsConfigured(), oidcService != nil); reason == "" {
		unverifiedAccountService = service.NewUnverifiedAccountService(
			userRepo,
			authService,
			emailSender,
			handlers.FrontendBaseURL,
			service.LoadUnverifiedAccountConfig(),
		)
		apiHandler.SetUnverifiedAccountService(unverifiedAccountService)
	} else {
		slog.Info("Unverified account cleanup disabled", "reason", reason)
	}

	generated.RegisterHandlersWithOptions(api, apiHandler, generated.GinServerOptions{
		ErrorHandler: func(c *gin.Context, err error, statusCode int) {
			// Sanitize generated wrapper errors — never expose raw error messages
			c.JSON(statusCode, gin.H{"error": "Invalid request parameters"})
		},
	})

	// Register flight utility routes
	handlers.RegisterFlightUtilRoutes(api, apiHandler)

	// Register custom currency rule routes (not in OpenAPI spec)
	handlers.RegisterCustomCurrencyRoutes(api, customCurrencyHandler)

	// OIDC authorize/callback browser redirects, registered manually and
	// unconditionally; they answer 503 when OIDC is off.
	handlers.RegisterOIDCRoutes(api, apiHandler)

	slog.Info("Routes registered from OpenAPI specification")

	// Start background notification checker (configurable via NOTIFICATION_CHECK_INTERVAL, defaults to 1h)
	notifCtx, notifCancel := context.WithCancel(context.Background())
	defer notifCancel()
	notificationService.StartBackgroundChecker(notifCtx, service.GetCheckInterval())

	// Refetch the airport database on a timer (AIRPORT_REFRESH_INTERVAL,
	// default 24h). A failed refresh keeps the snapshot already in memory.
	airports.StartRefresher(notifCtx, airports.RefreshInterval())

	// Look up the newest published releases at startup and on a timer
	// (UPDATE_CHECK_INTERVAL, default 24h).
	updateChecker.Start(notifCtx)

	// Evict expired CSV upload sessions on a timer.
	handlers.StartImportSessionReaper(notifCtx, time.Minute)

	// Sweep expired idempotency records.
	idempotencyService.StartReaper(notifCtx, time.Hour)

	// Sweep deletion tombstones past the retention horizon.
	deletionService.StartReaper(notifCtx, time.Hour)

	// Sweep expired WebAuthn ceremony rows.
	if webauthnService != nil {
		webauthnService.StartSessionReaper(notifCtx, 5*time.Minute)
	}

	// Sweep pending OIDC logins and unredeemed handoff codes.
	if oidcService != nil {
		oidcService.StartStateReaper(notifCtx, 5*time.Minute)
	}

	// Remind, then reap, accounts that never confirmed their address.
	if unverifiedAccountService != nil {
		unverifiedAccountService.Start(notifCtx)
	}

	if backupScheduler != nil {
		backupScheduler.Start(notifCtx)
		slog.Info("Cloud backup scheduler started")
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		Handler:           router,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	go func() {
		slog.Info("Server starting", "addr", ":"+port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("failed to start server", "error", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down...")
	if backupScheduler != nil {
		backupScheduler.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fatal("server forced to shutdown", "error", err)
	}

	slog.Info("Server exited")
}
