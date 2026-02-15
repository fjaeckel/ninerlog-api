package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/airports"
	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/api/handlers"
	"github.com/fjaeckel/pilotlog-api/internal/repository/postgres"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/fjaeckel/pilotlog-api/internal/service/currency"
	"github.com/fjaeckel/pilotlog-api/pkg/email"
	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func main() {
	log.Println("🚀 Starting PilotLog API...")

	// Load environment variables
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://pilotlog:changeme@localhost:5432/pilotlog?sslmode=disable"
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
	notificationService := service.NewNotificationService(notifRepo, credentialRepo, flightRepo, licenseRepo, userRepo, emailSender)
	twoFactorService := service.NewTwoFactorService(userRepo, jwtManager)
	contactRepo := postgres.NewContactRepository(db)
	contactService := service.NewContactService(contactRepo)
	classRatingRepo := postgres.NewClassRatingRepository(db)
	classRatingService := service.NewClassRatingService(classRatingRepo, licenseRepo)

	// Initialize currency evaluation
	flightDataProvider := currency.NewFlightDataProvider(db)
	currencyRegistry := currency.NewRegistry()
	currencyRegistry.Register(currency.NewEASAEvaluator())
	currencyRegistry.Register(currency.NewFAAEvaluator())
	currencyRegistry.Register(currency.NewOtherEvaluator())
	currencyService := currency.NewService(currencyRegistry, licenseRepo, classRatingRepo, flightDataProvider)

	// Initialize unified API handler that implements the OpenAPI ServerInterface
	apiHandler := handlers.NewAPIHandler(authService, licenseService, flightService, credentialService, aircraftService, notificationService, twoFactorService, contactService, classRatingService, currencyService, jwtManager)
	apiHandler.SetDB(db)

	// Setup router
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     corsOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Register OpenAPI-generated routes
	// This automatically maps all routes to the correct handlers with proper parameter extraction
	api := router.Group("/api/v1")
	generated.RegisterHandlers(api, apiHandler)

	// Register custom reports routes (not in OpenAPI spec)
	handlers.RegisterReportsRoutes(api, apiHandler, db)

	// Register contact routes
	handlers.RegisterContactRoutes(api, apiHandler)

	log.Println("✅ Routes registered from OpenAPI specification")

	// Start background notification checker (runs every hour)
	notifCtx, notifCancel := context.WithCancel(context.Background())
	defer notifCancel()
	notificationService.StartBackgroundChecker(notifCtx, 1*time.Hour)

	// Start server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: router,
	}

	go func() {
		log.Printf("✅ Server starting on :%s", port)
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
