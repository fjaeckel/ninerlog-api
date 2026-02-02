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

	"github.com/fjaeckel/pilotlog-api/internal/api/handlers"
	"github.com/fjaeckel/pilotlog-api/internal/repository/postgres"
	"github.com/fjaeckel/pilotlog-api/internal/service"
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
		corsOrigin = "http://localhost:5173,http://localhost:80"
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
	flightService := service.NewFlightService(flightRepo, licenseRepo)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	licenseHandler := handlers.NewLicenseHandler(licenseService, jwtManager)
	flightHandler := handlers.NewFlightHandler(flightService, jwtManager)

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

	// API routes
	api := router.Group("/api/v1")
	{
		// Auth routes
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.RegisterUser)
			auth.POST("/login", authHandler.LoginUser)
			auth.POST("/refresh", authHandler.RefreshToken)
		}

		// License routes (protected)
		licenses := api.Group("/licenses")
		{
			licenses.GET("", licenseHandler.ListLicenses)
			licenses.POST("", licenseHandler.CreateLicense)
			licenses.GET("/:id", licenseHandler.GetLicense)
			licenses.PATCH("/:id", licenseHandler.UpdateLicense)
			licenses.DELETE("/:id", licenseHandler.DeleteLicense)
		}

		// Flight routes (protected)
		flights := api.Group("/flights")
		{
			flights.GET("", flightHandler.ListFlights)
			flights.POST("", flightHandler.CreateFlight)
			flights.GET("/:id", flightHandler.GetFlight)
			flights.PUT("/:id", flightHandler.UpdateFlight)
			flights.DELETE("/:id", flightHandler.DeleteFlight)
		}
	}

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
