package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
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

// sanitizeLogValue strips newlines and control characters to prevent log injection.
func sanitizeLogValue(s string) string {
	return strings.NewReplacer("\n", "", "\r", "", "\t", " ").Replace(s)
}

func main() {
	log.Println("🚀 Starting NinerLog API...")

	// Load environment variables
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://ninerlog:changeme@localhost:5432/ninerlog?sslmode=disable"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "change-this-secret-key-in-production"
	}
	refreshSecret := os.Getenv("REFRESH_SECRET")
	if refreshSecret == "" {
		refreshSecret = "change-this-refresh-secret-in-production"
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
		log.Printf("🔑 Admin email configured: %s", sanitizeLogValue(adminEmail)) // #nosec G706 -- sanitized
	}

	// Connect to database
	log.Println("📦 Connecting to database...")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("✅ Database connected")

	// Run database migrations
	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "db/migrations"
	}
	log.Printf("📦 Running database migrations from %s...", sanitizeLogValue(migrationsPath)) // #nosec G706 -- sanitized
	driver, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		log.Fatalf("Failed to create migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres", driver)
	if err != nil {
		log.Fatalf("Failed to initialize migrations: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("✅ Database migrations applied")

	// Load airport database from OurAirports (async-safe, cached)
	airports.Init()

	// Initialize JWT manager
	jwtManager := jwt.NewManager(jwtSecret, refreshSecret, 15*time.Minute, 7*24*time.Hour)

	// Initialize repositories
	userRepo := postgres.NewUserRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	passwordResetRepo := postgres.NewPasswordResetTokenRepository(db)
	licenseRepo := postgres.NewLicenseRepository(db)
	flightRepo := postgres.NewFlightRepository(db)

	// Initialize services
	authService := service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, jwtManager)
	licenseService := service.NewLicenseService(licenseRepo)
	flightService := service.NewFlightService(flightRepo)
	credentialRepo := postgres.NewCredentialRepository(db)
	credentialService := service.NewCredentialService(credentialRepo)
	aircraftRepo := postgres.NewAircraftRepository(db)
	aircraftService := service.NewAircraftService(aircraftRepo)
	notifRepo := postgres.NewNotificationRepository(db)
	smtpConfig := email.LoadSMTPConfig()
	emailSender := email.NewSender(smtpConfig)
	twoFactorService := service.NewTwoFactorService(userRepo, jwtManager)
	contactRepo := postgres.NewContactRepository(db)
	contactService := service.NewContactService(contactRepo)
	classRatingRepo := postgres.NewClassRatingRepository(db)
	classRatingService := service.NewClassRatingService(classRatingRepo, licenseRepo)
	flightCrewRepo := postgres.NewFlightCrewRepository(db)

	// Initialize currency evaluation
	flightDataProvider := currency.NewFlightDataProvider(db)
	currencyRegistry := currency.NewRegistry()
	currencyRegistry.Register(currency.NewEASAEvaluator())
	currencyRegistry.Register(currency.NewFAAEvaluator())
	currencyRegistry.Register(currency.NewOtherEvaluator())
	ulEval := currency.NewGermanULEvaluator()
	currencyRegistry.RegisterMulti(ulEval, ulEval.Authorities()...)
	currencyService := currency.NewService(currencyRegistry, licenseRepo, classRatingRepo, flightDataProvider)

	// Notification service depends on currency service for two-tier evaluation
	notificationService := service.NewNotificationService(notifRepo, credentialRepo, flightRepo, licenseRepo, userRepo, emailSender, currencyService)

	// Initialize unified API handler that implements the OpenAPI ServerInterface
	apiHandler := handlers.NewAPIHandler(authService, licenseService, flightService, credentialService, aircraftService, notificationService, twoFactorService, contactService, classRatingService, currencyService, jwtManager, flightCrewRepo, adminEmail)
	apiHandler.SetDB(db)
	apiHandler.SetEmailSender(emailSender)
	startedAt := time.Now()
	apiHandler.SetStartedAt(startedAt)
	apiHandler.SetCORSOrigins(corsOrigins)

	// Setup router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New() // Use gin.New() instead of gin.Default() for custom recovery

	// Metrics & recovery — first in the middleware chain so all requests are instrumented
	metricsEnabled := os.Getenv("METRICS_ENABLED") != "false" // default: true
	if metricsEnabled {
		router.Use(middleware.MetricsMiddleware())
	}
	router.Use(middleware.RecoveryWithMetrics())
	router.Use(gin.Logger())

	// Trust proxy headers (X-Real-IP, X-Forwarded-For) from nginx
	// so that c.ClientIP() returns the real client IP, not the proxy's address.
	if err := router.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}); err != nil {
		log.Fatalf("Failed to set trusted proxies: %v", err)
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
		log.Println("✅ Prometheus metrics enabled at /metrics")
	}

	// pprof debug server (separate port, opt-in via PPROF_ENABLED=true)
	if os.Getenv("PPROF_ENABLED") == "true" {
		pprofPort := os.Getenv("PPROF_PORT")
		if pprofPort == "" {
			pprofPort = "6060"
		}
		go func() {
			log.Printf("🔍 pprof debug server listening on :%s/debug/pprof/", sanitizeLogValue(pprofPort)) // #nosec G706 -- sanitized
			pprofSrv := &http.Server{
				Addr:              ":" + pprofPort,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			if err := pprofSrv.ListenAndServe(); err != nil {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

	// Rate limiting for auth endpoints: 10 requests per minute per IP
	// Disabled via DISABLE_RATE_LIMIT=true for e2e test environments

	// Register OpenAPI-generated routes
	// This automatically maps all routes to the correct handlers with proper parameter extraction
	api := router.Group("/api/v1")

	// Centralized auth middleware — all routes require auth except explicit public paths
	api.Use(middleware.AuthMiddleware(jwtManager, []string{
		"/auth/register",
		"/auth/login",
		"/auth/refresh",
		"/auth/2fa/login",
		"/airports/search",
		"/airports/:icaoCode",
	}))

	if os.Getenv("DISABLE_RATE_LIMIT") != "true" {
		authRateLimit := middleware.NewRateLimitMiddleware(10, 1*time.Minute)

		// Apply rate limiter to sensitive auth endpoints via path-matching middleware
		api.Use(middleware.RateLimitByPath(authRateLimit,
			"/auth/register",
			"/auth/login",
			"/auth/refresh",
			"/auth/2fa/login",
			"/auth/password-reset-request",
			"/auth/password-reset",
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

	log.Println("✅ Routes registered from OpenAPI specification")

	// Start background notification checker (configurable via NOTIFICATION_CHECK_INTERVAL, defaults to 1h)
	notifCtx, notifCancel := context.WithCancel(context.Background())
	defer notifCancel()
	notificationService.StartBackgroundChecker(notifCtx, service.GetCheckInterval())

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
		log.Printf("✅ Server starting on :%s", sanitizeLogValue(port)) // #nosec G706 -- sanitized
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("✅ Server exited")
}
