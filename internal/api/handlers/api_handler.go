package handlers

import (
	"database/sql"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/fjaeckel/pilotlog-api/internal/service/currency"
	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// APIHandler implements the generated.ServerInterface from OpenAPI spec
type APIHandler struct {
	authService         *service.AuthService
	licenseService      *service.LicenseService
	flightService       *service.FlightService
	credentialService   *service.CredentialService
	aircraftService     *service.AircraftService
	notificationService *service.NotificationService
	twoFactorService    *service.TwoFactorService
	contactService      *service.ContactService
	classRatingService  *service.ClassRatingService
	currencyService     *currency.Service
	jwtManager          *jwt.Manager
	db                  *sql.DB
}

// NewAPIHandler creates a new unified API handler that implements the OpenAPI ServerInterface
func NewAPIHandler(
	authService *service.AuthService,
	licenseService *service.LicenseService,
	flightService *service.FlightService,
	credentialService *service.CredentialService,
	aircraftService *service.AircraftService,
	notificationService *service.NotificationService,
	twoFactorService *service.TwoFactorService,
	contactService *service.ContactService,
	classRatingService *service.ClassRatingService,
	currencyService *currency.Service,
	jwtManager *jwt.Manager,
) *APIHandler {
	return &APIHandler{
		authService:         authService,
		licenseService:      licenseService,
		flightService:       flightService,
		credentialService:   credentialService,
		aircraftService:     aircraftService,
		notificationService: notificationService,
		twoFactorService:    twoFactorService,
		contactService:      contactService,
		classRatingService:  classRatingService,
		currencyService:     currencyService,
		jwtManager:          jwtManager,
	}
}

// getUserIDFromContext extracts and validates user ID from authenticated context
func (h *APIHandler) getUserIDFromContext(c *gin.Context) (uuid.UUID, error) {
	// First check if the auth middleware already set the user ID
	if userID, exists := c.Get("userID"); exists {
		if id, ok := userID.(uuid.UUID); ok {
			return id, nil
		}
	}

	// Fallback: parse from Authorization header directly (for routes not covered by middleware)
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || len(authHeader) < 8 {
		return uuid.Nil, jwt.ErrInvalidToken
	}

	tokenString := authHeader[7:] // Remove "Bearer " prefix
	claims, err := h.jwtManager.ValidateAccessToken(tokenString)
	if err != nil {
		return uuid.Nil, err
	}

	return claims.UserID, nil
}

// sendError sends a standardized error response matching OpenAPI Error schema
func (h *APIHandler) sendError(c *gin.Context, statusCode int, message string, details ...map[string]string) {
	errorResponse := generated.Error{
		Error: message,
	}

	if len(details) > 0 {
		errorDetails := make([]struct {
			Field   *string `json:"field,omitempty"`
			Message *string `json:"message,omitempty"`
		}, 0, len(details))

		for _, detail := range details {
			field := detail["field"]
			msg := detail["message"]
			errorDetails = append(errorDetails, struct {
				Field   *string `json:"field,omitempty"`
				Message *string `json:"message,omitempty"`
			}{
				Field:   &field,
				Message: &msg,
			})
		}
		errorResponse.Details = &errorDetails
	}

	c.JSON(statusCode, errorResponse)
}

// Verify that APIHandler implements the generated.ServerInterface
var _ generated.ServerInterface = (*APIHandler)(nil)
