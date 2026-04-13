package handlers

import (
	"database/sql"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
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
	flightCrewRepo      repository.FlightCrewRepository
	adminEmail          string
	emailSender         *email.Sender
	startedAt           time.Time
	corsOrigins         []string
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
	flightCrewRepo repository.FlightCrewRepository,
	adminEmail string,
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
		flightCrewRepo:      flightCrewRepo,
		adminEmail:          adminEmail,
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

// isAdminUser checks if the given email matches the configured admin email
func (h *APIHandler) isAdminUser(email string) bool {
	return h.adminEmail != "" && strings.EqualFold(email, h.adminEmail)
}

// buildUserResponse creates a generated.User from a models.User, including isAdmin
func (h *APIHandler) buildUserResponse(user *models.User) generated.User {
	twoFA := user.TwoFactorEnabled
	isAdmin := h.isAdminUser(user.Email)
	tdf := generated.UserTimeDisplayFormat(user.TimeDisplayFormat)
	locale := generated.UserPreferredLocale(user.PreferredLocale)
	return generated.User{
		Id:                openapi_types.UUID(user.ID),
		Email:             openapi_types.Email(user.Email),
		Name:              user.Name,
		TwoFactorEnabled:  &twoFA,
		IsAdmin:           &isAdmin,
		TimeDisplayFormat: &tdf,
		PreferredLocale:   &locale,
		CreatedAt:         user.CreatedAt,
		UpdatedAt:         user.UpdatedAt,
	}
}
