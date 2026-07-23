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
// Used for unrecoverable startup failures where the process must fail closed.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func main() {
	logging.Setup()
	slog.Info("🚀 Starting NinerLog API...")

	// Load environment variables
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Local development fallback for passwordless local Postgres; deployed environments should set DATABASE_URL with the required TLS settings.
		dbURL = "postgresql://localhost:5432/ninerlog?sslmode=disable"
	}
	// Server-side backstop: kill any query that runs longer than this,
	// regardless of client behavior, so it can't hold its pool connection
	// indefinitely. Defense in depth alongside RequestTimeoutMiddleware.
	// This DSN is also used below for running migrations, so any migration
	// expected to run longer than this needs its own
	// "SET LOCAL statement_timeout = 0;" to opt out for that transaction.
	dbURL = withStatementTimeout(dbURL, 10*time.Second)
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	// JWT signing secrets are mandatory and must be strong. Fail closed at
	// startup rather than silently falling back to a public placeholder value,
	// which would let anyone forge tokens for any user.
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
		slog.Info("🔑 Admin email configured", "email", adminEmail)
	}

	// Connect to database
	slog.Info("📦 Connecting to database...")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fatal("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fatal("failed to ping database", "error", err)
	}

	// Bound the connection pool. database/sql defaults to an unlimited
	// MaxOpenConns, so without this a burst of slow requests (or a slow
	// query with no statement timeout) can open connections until
	// Postgres's own max_connections is exhausted, taking the database
	// down for every tenant. Excess load queues (and eventually times out)
	// instead.
	db.SetMaxOpenConns(25) // tune to Postgres max_connections / replica count
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	slog.Info("✅ Database connected")

	// Run database migrations
	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "db/migrations"
	}
	slog.Info("📦 Running database migrations", "path", migrationsPath)
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
	slog.Info("✅ Database migrations applied")

	// Load airport database from OurAirports (async-safe, cached)
	airports.Init()

	// Initialize JWT manager
	jwtManager := jwt.NewManager(jwtSecret, refreshSecret, 15*time.Minute, 7*24*time.Hour)

	// Initialize repositories
	userRepo := postgres.NewUserRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	passwordResetRepo := postgres.NewPasswordResetTokenRepository(db)
	emailVerificationRepo := postgres.NewEmailVerificationTokenRepository(db)
	licenseRepo := postgres.NewLicenseRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	flightBaselineRepo := postgres.NewFlightBaselineRepository(db)
	// Initialize services
	authService := service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, emailVerificationRepo, jwtManager)
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

	// TOTP secrets are encrypted at rest when TOTP_ENCRYPTION_KEY (base64,
	// 32 bytes) is set. Without it, secrets are stored as plaintext; warn so
	// operators enable encryption in production.
	var totpAEAD *cryptoutil.AEAD
	if totpKey := os.Getenv("TOTP_ENCRYPTION_KEY"); totpKey != "" {
		totpAEAD, err = cryptoutil.NewFromBase64(totpKey)
		if err != nil {
			fatal("invalid TOTP_ENCRYPTION_KEY", "error", err)
		}
		slog.Info("✅ TOTP secrets encrypted at rest")
	} else {
		slog.Warn("⚠️  TOTP_ENCRYPTION_KEY not set — 2FA secrets are stored unencrypted")
	}
	twoFactorService := service.NewTwoFactorService(userRepo, jwtManager, totpAEAD)
	contactRepo := postgres.NewContactRepository(db)
	contactService := service.NewContactService(contactRepo)
	classRatingRepo := postgres.NewClassRatingRepository(db)
	classRatingService := service.NewClassRatingService(classRatingRepo, licenseRepo)
	flightCrewRepo := postgres.NewFlightCrewRepository(db)
	webauthnCredRepo := postgres.NewWebAuthnCredentialRepository(db)
	webauthnSessionRepo := postgres.NewWebAuthnSessionRepository(db)

	// Initialize currency evaluation
	flightDataProvider := currency.NewFlightDataProvider(db)
	currencyRegistry := currency.NewRegistry()
	currencyRegistry.Register(currency.NewEASAEvaluator())
	currencyRegistry.Register(currency.NewFAAEvaluator())
	currencyRegistry.Register(currency.NewOtherEvaluator())
	ulEval := currency.NewGermanULEvaluator()
	currencyRegistry.RegisterMulti(ulEval, ulEval.Authorities()...)
	currencyService := currency.NewService(currencyRegistry, licenseRepo, classRatingRepo, flightDataProvider)

	// Custom (user-authored) currency rules
	customCurrencyRepo := postgres.NewCustomCurrencyRuleRepository(db)
	customCurrencyEvaluator := currency.NewCustomEvaluator(db)
	customCurrencyService := currency.NewCustomService(customCurrencyRepo, customCurrencyEvaluator)
	customCurrencyHandler := handlers.NewCustomCurrencyHandler(customCurrencyService)

	// Notification service depends on currency service for two-tier evaluation
	notificationService := service.NewNotificationService(notifRepo, credentialRepo, flightRepo, licenseRepo, userRepo, emailSender, currencyService, customCurrencyService)

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
	var webauthnService *service.WebAuthnService
	if webauthnRPID != "" {
		webauthnService, err = service.NewWebAuthnService(webauthnRPID, webauthnRPName, webauthnOrigins, webauthnCredRepo, webauthnSessionRepo, userRepo, authService)
		if err != nil {
			slog.Warn("⚠️  WebAuthn disabled", "error", err)
			webauthnService = nil
		} else {
			slog.Info("✅ WebAuthn enabled", "rp_id", webauthnRPID)
		}
	} else {
		slog.Info("ℹ️  WebAuthn disabled (set WEBAUTHN_RP_ID to enable)")
		_ = webauthnCredRepo
		_ = webauthnSessionRepo
	}

	// Initialize unified API handler that implements the OpenAPI ServerInterface
	apiHandler := handlers.NewAPIHandler(authService, licenseService, flightService, credentialService, aircraftService, notificationService, twoFactorService, contactService, classRatingService, currencyService, webauthnService, jwtManager, flightCrewRepo, adminEmail)
	apiHandler.SetDB(db)
	apiHandler.SetEmailSender(emailSender)
	flightSessionService := service.NewFlightSessionService(flightSessionRepo, aircraftRepo, flightService)
	apiHandler.SetFlightSessionService(flightSessionService)
	flightSignatureRepo := postgres.NewFlightSignatureRepository(db)
	flightSignatureService := service.NewFlightSignatureService(flightSignatureRepo, flightRepo, userRepo)
	apiHandler.SetFlightSignatureService(flightSignatureService)
	startedAt := time.Now()
	apiHandler.SetStartedAt(startedAt)
	apiHandler.SetCORSOrigins(corsOrigins)

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
		slog.Info("✅ Cloud backups enabled (S3, SFTP, WebDAV providers)")
	} else {
		slog.Info("ℹ️  Cloud backups disabled (set BACKUP_CREDENTIALS_KEY to enable)")
	}

	// Setup router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New() // Use gin.New() instead of gin.Default() for custom recovery

	// Metrics & recovery — first in the middleware chain so all requests are instrumented
	metricsEnabled := os.Getenv("METRICS_ENABLED") != "false" // default: true
	if metricsEnabled {
		router.Use(middleware.MetricsMiddleware())
	}
	router.Use(middleware.RecoveryWithMetrics())

	// Structured access log: one JSON line per request with request ID,
	// method, path, status, latency, client IP, and (when authenticated) the
	// user ID. Registered ahead of AuthMiddleware, but its post-handler
	// section runs after auth has populated the user ID in the context.
	// nil → the JSON default logger configured by logging.Setup above.
	router.Use(middleware.LoggerMiddleware(nil))

	// Trust proxy headers (X-Real-IP, X-Forwarded-For) from nginx
	// so that c.ClientIP() returns the real client IP, not the proxy's address.
	if err := router.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}); err != nil {
		fatal("failed to set trusted proxies", "error", err)
	}
	router.ForwardedByClientIP = true
	router.RemoteIPHeaders = []string{"X-Real-IP", "X-Forwarded-For"}

	// CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Security headers on all responses
	router.Use(middleware.SecurityHeadersMiddleware())

	// Request body size limit for multipart uploads (10 MB)
	router.MaxMultipartMemory = 10 << 20

	// Cap non-multipart request bodies (JSON endpoints via ShouldBindJSON
	// read the body in full before any application-level check runs).
	// Multipart requests are exempt — they're already bounded by
	// MaxMultipartMemory above plus the explicit CSV size check.
	// POST /imports/json restores a full logbook backup and legitimately
	// needs more room than a single-entity JSON body.
	router.Use(middleware.MaxBodyBytesMiddleware(1<<20, map[string]int64{
		"/imports/json": 50 << 20,
	}))

	// Health check with DB connectivity
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
		appVersion := os.Getenv("APP_VERSION")
		if appVersion == "" {
			appVersion = "dev"
		}
		middleware.RegisterAppMetrics(appVersion, startedAt)
		prometheus.MustRegister(middleware.NewDBStatsCollector(db))

		router.GET("/metrics", gin.WrapH(promhttp.Handler()))
		slog.Info("✅ Prometheus metrics enabled at /metrics")
	}

	// pprof debug server (separate port, opt-in via PPROF_ENABLED=true)
	if os.Getenv("PPROF_ENABLED") == "true" {
		pprofPort := os.Getenv("PPROF_PORT")
		if pprofPort == "" {
			pprofPort = "6060"
		}
		go func() {
			slog.Info("🔍 pprof debug server listening", "addr", ":"+pprofPort+"/debug/pprof/")
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

	// Rate limiting for auth endpoints: 10 requests per minute per IP
	// Disabled via DISABLE_RATE_LIMIT=true for e2e test environments

	// Register OpenAPI-generated routes
	// This automatically maps all routes to the correct handlers with proper parameter extraction
	api := router.Group("/api/v1")

	// Bound every request's context so a slow/unbounded query can't hold its
	// DB connection forever; releases the connection even if the query
	// itself has no deadline. Defense in depth alongside statement_timeout.
	api.Use(middleware.RequestTimeoutMiddleware(15 * time.Second))

	// Centralized auth middleware — all routes require auth except explicit public paths
	api.Use(middleware.AuthMiddleware(jwtManager, []string{
		"/auth/register",
		"/auth/login",
		"/auth/refresh",
		"/auth/2fa/login",
		"/auth/password-reset-request",
		"/auth/password-reset",
		"/auth/verify-email",
		"/auth/verify-email/resend",
		"/auth/webauthn/login/options",
		"/auth/webauthn/login/verify",
		"/airports/search",
		// Instructor signing links: unauthenticated by design (the signer
		// has no NinerLog account). "/airports/:icaoCode" is deliberately
		// NOT listed here — the OpenAPI spec requires bearerAuth for it, and
		// unlike this literal-path allowlist, "/sign/:token" is a gin route
		// *pattern* that AuthMiddleware matches via c.FullPath().
		"/sign/:token",
	}))

	if os.Getenv("DISABLE_RATE_LIMIT") != "true" {
		// Coarse global limiter on every authenticated route. Previously only
		// /auth/* and /admin/* were rate-limited at all — /flights, search,
		// /exports/pdf, /imports/*, etc. were open to unlimited repetition by
		// an authenticated user (or a stolen token). Keyed by user ID so it
		// can't be inflated by users sharing a NAT/office IP.
		generalRateLimit := middleware.NewUserRateLimitMiddleware(120, 1*time.Minute)
		api.Use(generalRateLimit)

		// Tighter limits for specifically expensive endpoints, layered on top
		// of the general limiter above.
		expensiveRateLimit := middleware.NewUserRateLimitMiddleware(15, 1*time.Minute)
		api.Use(middleware.RateLimitByPath(expensiveRateLimit,
			"/exports/pdf",
			// Custom-currency preview evaluates an arbitrary user-supplied rule
			// (aggregate + per-flight lapse queries) without persisting it, so
			// it is the most repeatable heavy path in this feature.
			"/custom-currency/preview",
		))
		api.Use(middleware.RateLimitByPathPrefix(expensiveRateLimit, "/imports"))
		// Advanced search ("q") drives up to 50 leading-wildcard ILIKE scans
		// plus a correlated crew subquery per request; plain /flights listing
		// (no "q") stays under only the general limiter above.
		api.Use(middleware.RateLimitByPathWithQueryParam(expensiveRateLimit, "/flights", "q"))

		authRateLimit := middleware.NewRateLimitMiddleware(10, 1*time.Minute)

		// Apply rate limiter to sensitive auth endpoints via path-matching middleware
		api.Use(middleware.RateLimitByPath(authRateLimit,
			"/auth/register",
			"/auth/login",
			"/auth/refresh",
			"/auth/2fa/login",
			"/auth/password-reset-request",
			"/auth/password-reset",
			"/auth/verify-email",
			"/auth/verify-email/resend",
			"/auth/webauthn/login/options",
			"/auth/webauthn/login/verify",
		))

		// Stricter rate limiting for admin endpoints: 30 requests per minute per IP
		adminRateLimit := middleware.NewRateLimitMiddleware(30, 1*time.Minute)
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

		// Public signing links are unauthenticated and token-guessing-prone
		// by nature (though the 256-bit token itself makes brute force
		// infeasible); rate-limit by path prefix since the trailing token
		// segment defeats RateLimitByPath's suffix matching.
		signRateLimit := middleware.NewRateLimitMiddleware(20, 1*time.Minute)
		api.Use(middleware.RateLimitByPathPrefix(signRateLimit, "/sign/"))

		// Authenticated signature actions that trigger outbound email.
		signatureEmailRateLimit := middleware.NewRateLimitMiddleware(10, 1*time.Minute)
		api.Use(middleware.RateLimitByPath(signatureEmailRateLimit,
			"/signatures",
			"/resend",
		))
	} // end DISABLE_RATE_LIMIT check

	generated.RegisterHandlersWithOptions(api, apiHandler, generated.GinServerOptions{
		ErrorHandler: func(c *gin.Context, err error, statusCode int) {
			// Sanitize generated wrapper errors — never expose raw error messages
			c.JSON(statusCode, gin.H{"error": "Invalid request parameters"})
		},
	})

	// Register custom reports routes (not in OpenAPI spec)
	handlers.RegisterReportsRoutes(api, apiHandler, db)

	// Register flight utility routes
	handlers.RegisterFlightUtilRoutes(api, apiHandler)

	// Register custom currency rule routes (not in OpenAPI spec)
	handlers.RegisterCustomCurrencyRoutes(api, customCurrencyHandler)

	slog.Info("✅ Routes registered from OpenAPI specification")

	// Start background notification checker (configurable via NOTIFICATION_CHECK_INTERVAL, defaults to 1h)
	notifCtx, notifCancel := context.WithCancel(context.Background())
	defer notifCancel()
	notificationService.StartBackgroundChecker(notifCtx, service.GetCheckInterval())

	// Start cloud-backup scheduler if configured
	if backupScheduler != nil {
		backupScheduler.Start(notifCtx)
		slog.Info("✅ Cloud backup scheduler started")
	}

	// Start server with timeouts to prevent slow-loris and resource exhaustion
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
		slog.Info("✅ Server starting", "addr", ":"+port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("failed to start server", "error", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("🛑 Shutting down...")
	if backupScheduler != nil {
		backupScheduler.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fatal("server forced to shutdown", "error", err)
	}

	slog.Info("✅ Server exited")
}
