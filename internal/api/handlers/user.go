package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetCurrentUser implements GET /users/me
// (GET /users/me)
func (h *APIHandler) GetCurrentUser(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, h.buildUserResponse(user))
}

// UpdateCurrentUser implements PATCH /users/me
// (PATCH /users/me)
func (h *APIHandler) UpdateCurrentUser(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.UpdateCurrentUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get current user
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		h.sendError(c, http.StatusNotFound, "User not found")
		return
	}

	// Apply updates
	if req.Name != nil {
		user.Name = strings.TrimSpace(*req.Name)
	}

	// Changing the email address is a security-sensitive operation: the address
	// is the account's recovery channel and (via ADMIN_EMAIL) the basis for
	// admin authorization. It therefore requires the current password, and the
	// new address must be re-verified before it is trusted again.
	emailChanged := false
	if req.Email != nil {
		newEmail := strings.ToLower(strings.TrimSpace(string(*req.Email)))
		if !strings.EqualFold(newEmail, user.Email) {
			// Re-validate the normalized address. mail.ParseAddress accepts
			// quoted local-parts that it then re-emits unquoted (e.g.
			// "back\\slash"@x -> back\slash@x); such a value round-trips back
			// through openapi_types.Email as invalid and breaks every response
			// that serializes this user (including the admin user list).
			if _, err := mail.ParseAddress(newEmail); err != nil {
				h.sendError(c, http.StatusBadRequest, "Invalid email address")
				return
			}
			if req.CurrentPassword == nil || *req.CurrentPassword == "" {
				h.sendError(c, http.StatusBadRequest, "currentPassword is required to change the email address")
				return
			}
			if err := h.authService.VerifyPassword(c.Request.Context(), userID, *req.CurrentPassword); err != nil {
				h.sendError(c, http.StatusUnauthorized, "Password is incorrect")
				return
			}
			user.Email = newEmail
			// The new address is unproven until its verification link is used.
			user.EmailVerified = false
			emailChanged = true
		}
	}
	if req.TimeDisplayFormat != nil {
		format := string(*req.TimeDisplayFormat)
		if format == "hm" || format == "decimal" {
			user.TimeDisplayFormat = format
		}
	}
	if req.DateFormat != nil {
		df := string(*req.DateFormat)
		if df == "DD.MM.YYYY" || df == "MM/DD/YYYY" || df == "YYYY-MM-DD" {
			user.DateFormat = df
		}
	}
	if req.DecimalSeparator != nil {
		ds := string(*req.DecimalSeparator)
		if ds == "comma" || ds == "dot" {
			user.DecimalSeparator = ds
		}
	}
	if req.PreferredLocale != nil {
		locale := string(*req.PreferredLocale)
		if locale == "en" || locale == "de" {
			user.PreferredLocale = locale
		}
	}
	if req.RecencyPerModel != nil {
		user.RecencyPerModel = *req.RecencyPerModel
	}
	if req.RecencyPerRegistration != nil {
		user.RecencyPerRegistration = *req.RecencyPerRegistration
	}
	if req.FlightListColumnMode != nil {
		mode := string(*req.FlightListColumnMode)
		if mode == models.FlightListColumnModeAuto || mode == models.FlightListColumnModeCustom {
			user.FlightListColumnMode = mode
		}
	}
	// An empty array is meaningful here — in custom mode it means "none of the
	// optional columns" — so the list is replaced whenever the field is present.
	if req.FlightListColumns != nil {
		columns := make([]string, 0, len(*req.FlightListColumns))
		for _, c := range *req.FlightListColumns {
			columns = append(columns, string(c))
		}
		user.FlightListColumns = models.NormalizeFlightListColumns(columns)
	}

	// Update user
	if err := h.authService.UpdateUser(c.Request.Context(), user); err != nil {
		if errors.Is(err, service.ErrUserAlreadyExists) {
			h.sendError(c, http.StatusConflict, "This email is already in use by another account")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to update user")
		return
	}

	// Send a verification link to the NEW address so the user can prove control
	// of it. Failures are non-fatal — the address is already marked unverified,
	// and the user can request a fresh link via /auth/verify-email/resend.
	if emailChanged {
		if token, err := h.authService.CreateEmailVerificationToken(c.Request.Context(), userID); err == nil {
			h.sendVerificationEmail(user.Email, user.Name, user.PreferredLocale, token)
		}
	}

	c.JSON(http.StatusOK, h.buildUserResponse(user))
}

// GetMyStatistics implements GET /users/me/statistics
func (h *APIHandler) GetMyStatistics(c *gin.Context, params generated.GetMyStatisticsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var startDate, endDate *time.Time
	if params.StartDate != nil {
		t := params.StartDate.Time
		startDate = &t
	}
	if params.EndDate != nil {
		t := params.EndDate.Time
		endDate = &t
	}

	stats, baseline, err := h.flightService.GetStatsByUserID(c.Request.Context(), userID, startDate, endDate, true)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to calculate statistics")
		return
	}

	statistics := generated.Statistics{
		TotalFlights:        stats.TotalFlights,
		TotalMinutes:        stats.TotalMinutes,
		PicMinutes:          stats.PICMinutes,
		DualMinutes:         stats.DualMinutes,
		NightMinutes:        stats.NightMinutes,
		IfrMinutes:          stats.IFRMinutes,
		SoloMinutes:         ptrInt(stats.SoloMinutes),
		CrossCountryMinutes: ptrInt(stats.CrossCountryMinutes),
		LandingsDay:         stats.LandingsDay,
		LandingsNight:       stats.LandingsNight,
	}
	if baseline != nil {
		statistics.Baseline = baselineContribution(baseline)
	}

	c.JSON(http.StatusOK, statistics)
}
