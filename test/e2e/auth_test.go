//go:build e2e

package e2e_test

import (
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/api/handlers"
	"github.com/fjaeckel/pilotlog-api/internal/repository/postgres"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/fjaeckel/pilotlog-api/internal/testutil"
	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func setupTestServer(t *testing.T) (*gin.Engine, *testutil.APITestClient) {
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

	userRepo := postgres.NewUserRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	passwordResetRepo := postgres.NewPasswordResetTokenRepository(db)
	licenseRepo := postgres.NewLicenseRepository(db)
	flightRepo := postgres.NewFlightRepository(db)

	authService := service.NewAuthService(
		userRepo,
		refreshTokenRepo,
		passwordResetRepo,
		jwtManager,
	)
	licenseService := service.NewLicenseService(licenseRepo)
	flightService := service.NewFlightService(flightRepo)

	// Use unified API handler that implements ServerInterface
	apiHandler := handlers.NewAPIHandler(authService, licenseService, flightService, jwtManager)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Register routes using the generated OpenAPI handler registration
	v1 := router.Group("/api/v1")
	generated.RegisterHandlers(v1, apiHandler)

	// Add test-specific endpoints for password reset (not in OpenAPI spec yet)
	auth := v1.Group("/auth")
	{
		auth.POST("/password-reset", func(c *gin.Context) {
			var input struct {
				Email string `json:"email"`
			}
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}

			token, err := authService.RequestPasswordReset(c.Request.Context(), input.Email)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}

			// In production, don't return the token - send it via email
			// For testing, we return it so tests can use it
			c.JSON(200, gin.H{
				"message": "if the email exists, a reset link will be sent",
				"token":   token, // Test-only: return token for testing
			})
		})

		auth.POST("/reset-password", func(c *gin.Context) {
			var input struct {
				Token       string `json:"token"`
				NewPassword string `json:"newPassword"`
			}
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}

			err := authService.ResetPassword(c.Request.Context(), input.Token, input.NewPassword)
			if err != nil {
				if err == service.ErrInvalidToken || err == service.ErrTokenExpired || err == service.ErrTokenUsed {
					c.JSON(400, gin.H{"error": err.Error()})
					return
				}
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}

			c.JSON(200, gin.H{"message": "password reset successful"})
		})
	}

	return router, testutil.NewAPITestClient(t, router)
}

func TestAuthFlowE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test")
	}

	// Create a fresh test server for each test run
	_, client := setupTestServer(t)

	t.Run("Register new user", func(t *testing.T) {
		registerPayload := map[string]string{
			"email":    "e2e@example.com",
			"password": "SecurePass123!",
			"name":     "E2E Test User",
		}

		w := client.POST("/api/v1/auth/register", registerPayload)

		if w.Code != 201 {
			t.Errorf("Expected status 201, got %d", w.Code)
		}

		var response map[string]interface{}
		client.ParseJSON(w, &response)

		if response["accessToken"] == nil {
			t.Error("Expected accessToken in response")
		}
		if response["refreshToken"] == nil {
			t.Error("Expected refreshToken in response")
		}
	})

	t.Run("Register duplicate email fails", func(t *testing.T) {
		registerPayload := map[string]string{
			"email":    "e2e@example.com",
			"password": "SecurePass123!",
			"name":     "E2E Test User",
		}

		w := client.POST("/api/v1/auth/register", registerPayload)

		if w.Code != 409 {
			t.Errorf("Expected status 409, got %d", w.Code)
		}
	})

	t.Run("Login with correct credentials", func(t *testing.T) {
		loginPayload := map[string]string{
			"email":    "e2e@example.com",
			"password": "SecurePass123!",
		}

		w := client.POST("/api/v1/auth/login", loginPayload)

		if w.Code != 200 {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		client.ParseJSON(w, &response)

		if response["accessToken"] == nil {
			t.Error("Expected accessToken in response")
		}

		if response["refreshToken"] != nil {
			accessToken := response["accessToken"].(string)
			client.SetAuthToken(accessToken)
		}
	})

	t.Run("Login with wrong password fails", func(t *testing.T) {
		loginPayload := map[string]string{
			"email":    "e2e@example.com",
			"password": "WrongPassword",
		}

		w := client.POST("/api/v1/auth/login", loginPayload)

		if w.Code != 401 {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Refresh token flow", func(t *testing.T) {
		// First register a user for this test
		registerPayload := map[string]string{
			"email":    "refresh-test@example.com",
			"password": "SecurePass123!",
			"name":     "Refresh Test User",
		}
		client.POST("/api/v1/auth/register", registerPayload)

		// Now login
		loginPayload := map[string]string{
			"email":    "refresh-test@example.com",
			"password": "SecurePass123!",
		}

		w := client.POST("/api/v1/auth/login", loginPayload)
		var loginResponse map[string]interface{}
		client.ParseJSON(w, &loginResponse)

		if loginResponse["refreshToken"] == nil {
			t.Fatalf("Expected refreshToken in login response, got: %+v", loginResponse)
		}

		refreshToken := loginResponse["refreshToken"].(string)

		refreshPayload := map[string]string{
			"refreshToken": refreshToken,
		}

		w = client.POST("/api/v1/auth/refresh", refreshPayload)

		if w.Code != 200 {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var refreshResponse map[string]interface{}
		client.ParseJSON(w, &refreshResponse)

		if refreshResponse["accessToken"] == nil {
			t.Error("Expected new accessToken")
		}
	})

	t.Run("Access protected endpoint with valid token", func(t *testing.T) {
		// First register and login
		registerPayload := map[string]string{
			"email":    "protected-test@example.com",
			"password": "SecurePass123!",
			"name":     "Protected Test User",
		}
		client.POST("/api/v1/auth/register", registerPayload)

		loginPayload := map[string]string{
			"email":    "protected-test@example.com",
			"password": "SecurePass123!",
		}

		w := client.POST("/api/v1/auth/login", loginPayload)
		var response map[string]interface{}
		client.ParseJSON(w, &response)

		accessToken := response["accessToken"].(string)
		client.SetAuthToken(accessToken)

		w = client.GET("/api/v1/users/me")

		if w.Code != 200 {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("Access protected endpoint without token fails", func(t *testing.T) {
		client.SetAuthToken("")

		w := client.GET("/api/v1/users/me")

		if w.Code != 401 {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})
}

func TestPasswordResetFlowE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test")
	}

	_, client := setupTestServer(t)

	registerPayload := map[string]string{
		"email":    "reset@example.com",
		"password": "OldPassword123!",
		"name":     "Reset Test User",
	}

	w := client.POST("/api/v1/auth/register", registerPayload)
	if w.Code != 201 {
		t.Fatalf("Failed to register user: status %d", w.Code)
	}

	var resetToken string

	t.Run("Request password reset", func(t *testing.T) {
		resetPayload := map[string]string{
			"email": "reset@example.com",
		}

		w := client.POST("/api/v1/auth/password-reset", resetPayload)

		if w.Code != 200 {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		client.ParseJSON(w, &response)

		// In test mode, the token is returned
		if response["token"] != nil {
			resetToken = response["token"].(string)
		}
	})

	t.Run("Request reset for non-existent email", func(t *testing.T) {
		resetPayload := map[string]string{
			"email": "nonexistent@example.com",
		}

		w := client.POST("/api/v1/auth/password-reset", resetPayload)

		if w.Code != 200 {
			t.Errorf("Expected status 200 (security), got %d", w.Code)
		}
	})

	t.Run("Login with new password after reset", func(t *testing.T) {
		if resetToken == "" {
			t.Skip("No reset token available")
		}

		// First, reset the password
		resetPasswordPayload := map[string]string{
			"token":       resetToken,
			"newPassword": "NewPassword456!",
		}

		w := client.POST("/api/v1/auth/reset-password", resetPasswordPayload)
		if w.Code != 200 {
			t.Errorf("Expected status 200 for password reset, got %d: %s", w.Code, w.Body.String())
		}

		// Now try to login with new password
		loginPayload := map[string]string{
			"email":    "reset@example.com",
			"password": "NewPassword456!",
		}

		w = client.POST("/api/v1/auth/login", loginPayload)

		if w.Code != 200 {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}
