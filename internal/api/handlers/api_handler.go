package handlers

import (
	"database/sql"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// APIHandler implements the generated.ServerInterface from OpenAPI spec
type APIHandler struct {
	authService            *service.AuthService
	licenseService         *service.LicenseService
	flightService          *service.FlightService
	flightSessionService   *service.FlightSessionService
	flightSignatureService *service.FlightSignatureService
	credentialService      *service.CredentialService
	aircraftService        *service.AircraftService
	notificationService    *service.NotificationService
	twoFactorService       *service.TwoFactorService
	contactService         *service.ContactService
	classRatingService     *service.ClassRatingService
	currencyService        *currency.Service
	webauthnService        *service.WebAuthnService
	// oidcService is nil unless OIDC_ISSUER is configured. Non-nil means the
	// server runs in OIDC mode, which also switches every local credential
	// path off — see requireLocalAuth.
	oidcService     *service.OIDCService
	jwtManager      *jwt.Manager
	db              *sql.DB
	flightCrewRepo  repository.FlightCrewRepository
	adminEmail      string
	emailSender     *email.Sender
	startedAt       time.Time
	corsOrigins     []string
	backupService   *cloudbackup.Service
	deletionService *service.DeletionService
	// documentImageService is nil only if the subsystem was never wired up;
	// the operator-facing off switch lives inside the service itself so that
	// GET /features can still report it accurately.
	documentImageService *service.DocumentImageService
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
	webauthnService *service.WebAuthnService,
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
		webauthnService:     webauthnService,
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

// getUserNameFromContext returns the authenticated user's display name, used
// by flight auto-calculations to resolve whether an Instructor crew member is
// the user themselves (Dual given) or a third party (Dual received). Returns
// "" if the user cannot be resolved; flightcalc treats an empty name
// conservatively (any Instructor → Dual received).
func (h *APIHandler) getUserNameFromContext(c *gin.Context) string {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return ""
	}
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil || user == nil {
		return ""
	}
	return user.Name
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

// isAdminUser reports whether the user holds the configured admin address.
//
// The email must be VERIFIED. Admin status is derived from a user-settable
// field, so without the verification requirement any authenticated user could
// PATCH /users/me to the configured ADMIN_EMAIL and inherit full admin rights
// whenever no account currently holds that address (fresh deployment, rotated
// admin address, deleted admin account). Requiring email_verified means the
// caller must additionally prove control of the mailbox — and UpdateCurrentUser
// clears email_verified on every address change (see updateUserEmail).
func (h *APIHandler) isAdminUser(user *models.User) bool {
	if user == nil || h.adminEmail == "" {
		return false
	}
	return user.EmailVerified && strings.EqualFold(user.Email, h.adminEmail)
}

// buildUserResponse creates a generated.User from a models.User, including isAdmin
func (h *APIHandler) buildUserResponse(user *models.User) generated.User {
	twoFA := user.TwoFactorEnabled
	emailVerified := user.EmailVerified
	isAdmin := h.isAdminUser(user)
	tdf := generated.UserTimeDisplayFormat(user.TimeDisplayFormat)
	locale := generated.UserPreferredLocale(user.PreferredLocale)
	df := generated.UserDateFormat(user.DateFormat)
	ds := generated.UserDecimalSeparator(user.DecimalSeparator)
	recencyPerModel := user.RecencyPerModel
	recencyPerRegistration := user.RecencyPerRegistration
	columnMode := generated.UserFlightListColumnMode(models.NormalizeFlightListColumnMode(user.FlightListColumnMode))
	// Always an array, never null: the client treats a missing list as "no
	// optional columns", which is a different answer from "auto".
	columns := make([]generated.FlightListColumn, 0, len(user.FlightListColumns))
	for _, c := range user.FlightListColumns {
		columns = append(columns, generated.FlightListColumn(c))
	}
	return generated.User{
		Id:                     openapi_types.UUID(user.ID),
		Email:                  openapi_types.Email(user.Email),
		Name:                   user.Name,
		EmailVerified:          &emailVerified,
		TwoFactorEnabled:       &twoFA,
		IsAdmin:                &isAdmin,
		TimeDisplayFormat:      &tdf,
		DateFormat:             &df,
		DecimalSeparator:       &ds,
		PreferredLocale:        &locale,
		RecencyPerModel:        &recencyPerModel,
		RecencyPerRegistration: &recencyPerRegistration,
		FlightListColumnMode:   &columnMode,
		FlightListColumns:      &columns,
		CreatedAt:              user.CreatedAt,
		UpdatedAt:              user.UpdatedAt,
	}
}
