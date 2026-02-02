//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/api/handlers"
	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository/postgres"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/fjaeckel/pilotlog-api/internal/testutil"
	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func setupLicenseFlightTestServer(t *testing.T) (*gin.Engine, *testutil.APITestClient) {
	t.Helper()

	db := testutil.SetupTestDB(t)
	t.Cleanup(func() {
		testutil.TeardownTestDB(t, db)
	})

	jwtManager := jwt.NewManager(
		"test_access_secret_key_for_testing",
		"test_refresh_secret_key_for_testing",
		15*time.Minute,
		7*24*time.Hour,
	)

	// Repositories
	userRepo := postgres.NewUserRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	passwordResetRepo := postgres.NewPasswordResetTokenRepository(db)
	licenseRepo := postgres.NewLicenseRepository(db)
	flightRepo := postgres.NewFlightRepository(db)

	// Services
	authService := service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, jwtManager)
	licenseService := service.NewLicenseService(licenseRepo)
	flightService := service.NewFlightService(flightRepo, licenseRepo)

	// Handlers
	licenseHandler := handlers.NewLicenseHandler(licenseService, jwtManager)
	flightHandler := handlers.NewFlightHandler(flightService, jwtManager)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	v1 := router.Group("/api/v1")
	{
		// Auth routes
		auth := v1.Group("/auth")
		{
			auth.POST("/register", func(c *gin.Context) {
				var input service.RegisterInput
				if err := c.ShouldBindJSON(&input); err != nil {
					c.JSON(400, gin.H{"error": err.Error()})
					return
				}
				user, tokens, err := authService.Register(c.Request.Context(), input)
				if err != nil {
					if err == service.ErrUserAlreadyExists {
						c.JSON(409, gin.H{"error": "user already exists"})
						return
					}
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				c.JSON(201, gin.H{
					"user":         user,
					"accessToken":  tokens.AccessToken,
					"refreshToken": tokens.RefreshToken,
				})
			})
		}

		// License routes
		licenses := v1.Group("/licenses")
		{
			licenses.POST("", licenseHandler.CreateLicense)
			licenses.GET("", licenseHandler.ListLicenses)
			licenses.GET("/:id", licenseHandler.GetLicense)
			licenses.PUT("/:id", licenseHandler.UpdateLicense)
			licenses.DELETE("/:id", licenseHandler.DeleteLicense)
		}

		// Flight routes
		flights := v1.Group("/flights")
		{
			flights.POST("", flightHandler.CreateFlight)
			flights.GET("", flightHandler.ListFlights)
			flights.GET("/:id", flightHandler.GetFlight)
			flights.PUT("/:id", flightHandler.UpdateFlight)
			flights.DELETE("/:id", flightHandler.DeleteFlight)
		}
	}

	return router, testutil.NewAPITestClient(t, router)
}

func TestLicenseManagementE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test")
	}

	_, client := setupLicenseFlightTestServer(t)

	// Register and get auth token
	registerResp := client.POST("/api/v1/auth/register", map[string]string{
		"email":    "license-e2e@example.com",
		"password": "Password123!",
		"name":     "License Test User",
	})

	if registerResp.Code != http.StatusCreated {
		t.Fatalf("Failed to register user: %d - %s", registerResp.Code, registerResp.Body.String())
	}

	var registerData map[string]interface{}
	json.Unmarshal(registerResp.Body.Bytes(), &registerData)
	accessToken := registerData["accessToken"].(string)
	client.SetAuthToken(accessToken)

	var createdLicenseID string

	t.Run("Create license", func(t *testing.T) {
		expiryDate := time.Now().AddDate(2, 0, 0)
		resp := client.POST("/api/v1/licenses", map[string]interface{}{
			"licenseType":      "EASA_PPL",
			"licenseNumber":    "PPL-12345",
			"issueDate":        time.Now().Format(time.RFC3339),
			"expiryDate":       expiryDate.Format(time.RFC3339),
			"issuingAuthority": "EASA",
			"isActive":         true,
		})

		if resp.Code != http.StatusCreated {
			t.Fatalf("Failed to create license: %d - %s", resp.Code, resp.Body.String())
		}

		var license models.License
		json.Unmarshal(resp.Body.Bytes(), &license)
		createdLicenseID = license.ID.String()

		if license.LicenseType != models.LicenseTypeEASAPPL {
			t.Errorf("Expected license type EASA_PPL, got %v", license.LicenseType)
		}
		if license.LicenseNumber != "PPL-12345" {
			t.Errorf("Expected license number PPL-12345, got %s", license.LicenseNumber)
		}
	})

	t.Run("List licenses", func(t *testing.T) {
		resp := client.GET("/api/v1/licenses")

		if resp.Code != http.StatusOK {
			t.Fatalf("Failed to list licenses: %d - %s", resp.Code, resp.Body.String())
		}

		var licenses []models.License
		json.Unmarshal(resp.Body.Bytes(), &licenses)

		if len(licenses) != 1 {
			t.Errorf("Expected 1 license, got %d", len(licenses))
		}
	})

	t.Run("Get license by ID", func(t *testing.T) {
		resp := client.GET(fmt.Sprintf("/api/v1/licenses/%s", createdLicenseID))

		if resp.Code != http.StatusOK {
			t.Fatalf("Failed to get license: %d - %s", resp.Code, resp.Body.String())
		}

		var license models.License
		json.Unmarshal(resp.Body.Bytes(), &license)

		if license.ID.String() != createdLicenseID {
			t.Errorf("Expected license ID %s, got %s", createdLicenseID, license.ID.String())
		}
	})

	t.Run("Update license", func(t *testing.T) {
		resp := client.PUT(fmt.Sprintf("/api/v1/licenses/%s", createdLicenseID), map[string]interface{}{
			"isActive": false,
		})

		if resp.Code != http.StatusOK {
			t.Fatalf("Failed to update license: %d - %s", resp.Code, resp.Body.String())
		}
	})

	t.Run("Delete license", func(t *testing.T) {
		resp := client.DELETE(fmt.Sprintf("/api/v1/licenses/%s", createdLicenseID))

		if resp.Code != http.StatusOK {
			t.Fatalf("Failed to delete license: %d - %s", resp.Code, resp.Body.String())
		}
	})
}

func TestFlightLoggingE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test")
	}

	_, client := setupLicenseFlightTestServer(t)

	// Register and get auth token
	registerResp := client.POST("/api/v1/auth/register", map[string]string{
		"email":    "flight-e2e@example.com",
		"password": "Password123!",
		"name":     "Flight Test User",
	})

	if registerResp.Code != http.StatusCreated {
		t.Fatalf("Failed to register user: %d - %s", registerResp.Code, registerResp.Body.String())
	}

	var registerData map[string]interface{}
	json.Unmarshal(registerResp.Body.Bytes(), &registerData)
	accessToken := registerData["accessToken"].(string)
	client.SetAuthToken(accessToken)

	// Create a license first
	licenseResp := client.POST("/api/v1/licenses", map[string]interface{}{
		"licenseType":      "EASA_PPL",
		"licenseNumber":    "PPL-67890",
		"issueDate":        time.Now().Format(time.RFC3339),
		"expiryDate":       time.Now().AddDate(2, 0, 0).Format(time.RFC3339),
		"issuingAuthority": "EASA",
		"isActive":         true,
	})

	if licenseResp.Code != http.StatusCreated {
		t.Fatalf("Failed to create license: %d - %s", licenseResp.Code, licenseResp.Body.String())
	}

	var license models.License
	json.Unmarshal(licenseResp.Body.Bytes(), &license)
	licenseID := license.ID.String()

	var createdFlightID string

	t.Run("Create flight", func(t *testing.T) {
		resp := client.POST("/api/v1/flights", map[string]interface{}{
			"licenseId":     licenseID,
			"date":          time.Now().Format(time.RFC3339),
			"aircraftReg":   "D-EXXX",
			"aircraftType":  "C172",
			"departure":     "EDNY",
			"arrival":       "EDNY",
			"totalTime":     1.5,
			"picTime":       1.5,
			"dualTime":      0.0,
			"nightTime":     0.0,
			"ifrTime":       0.0,
			"landingsDay":   2,
			"landingsNight": 0,
			"remarks":       "Pattern work",
		})

		if resp.Code != http.StatusCreated {
			t.Fatalf("Failed to create flight: %d - %s", resp.Code, resp.Body.String())
		}

		var flight models.Flight
		json.Unmarshal(resp.Body.Bytes(), &flight)
		createdFlightID = flight.ID.String()

		if flight.AircraftReg != "D-EXXX" {
			t.Errorf("Expected aircraft reg D-EXXX, got %s", flight.AircraftReg)
		}
	})

	t.Run("List flights", func(t *testing.T) {
		resp := client.GET("/api/v1/flights")

		if resp.Code != http.StatusOK {
			t.Fatalf("Failed to list flights: %d - %s", resp.Code, resp.Body.String())
		}

		var flights []models.Flight
		json.Unmarshal(resp.Body.Bytes(), &flights)

		if len(flights) != 1 {
			t.Errorf("Expected 1 flight, got %d", len(flights))
		}
	})

	t.Run("Get flight by ID", func(t *testing.T) {
		resp := client.GET(fmt.Sprintf("/api/v1/flights/%s", createdFlightID))

		if resp.Code != http.StatusOK {
			t.Fatalf("Failed to get flight: %d - %s", resp.Code, resp.Body.String())
		}

		var flight models.Flight
		json.Unmarshal(resp.Body.Bytes(), &flight)

		if flight.ID.String() != createdFlightID {
			t.Errorf("Expected flight ID %s, got %s", createdFlightID, flight.ID.String())
		}
	})

	t.Run("Update flight", func(t *testing.T) {
		resp := client.PUT(fmt.Sprintf("/api/v1/flights/%s", createdFlightID), map[string]interface{}{
			"remarks": "Updated remarks",
		})

		if resp.Code != http.StatusOK {
			t.Fatalf("Failed to update flight: %d - %s", resp.Code, resp.Body.String())
		}
	})

	t.Run("Delete flight", func(t *testing.T) {
		resp := client.DELETE(fmt.Sprintf("/api/v1/flights/%s", createdFlightID))

		if resp.Code != http.StatusOK {
			t.Fatalf("Failed to delete flight: %d - %s", resp.Code, resp.Body.String())
		}
	})
}
