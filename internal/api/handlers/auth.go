package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	emailpkg "github.com/fjaeckel/ninerlog-api/pkg/email"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// RegisterUser implements POST /auth/register
// (POST /auth/register)
func (h *APIHandler) RegisterUser(c *gin.Context) {
	if !h.requireLocalAuth(c) {
		return
	}

	var req generated.RegisterUserJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, verificationToken, err := h.authService.Register(c.Request.Context(), service.RegisterInput{
		Email:           string(req.Email),
		Password:        req.Password,
		Name:            req.Name,
		PreferredLocale: preferredLocaleString(req.PreferredLocale),
	})

	if err != nil {
		if err == service.ErrUserAlreadyExists {
			h.sendError(c, http.StatusConflict, "Email already exists")
			return
		}
		if err == service.ErrPasswordTooShort || err == service.ErrPasswordTooLong ||
			err == service.ErrPasswordTooWeak ||
			err == service.ErrEmailRequired || err == service.ErrPasswordRequired ||
			err == service.ErrNameRequired || err == service.ErrInvalidEmail ||
			err == service.ErrEmailTooLong {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Registration failed")
		return
	}

	verificationRequired := h.emailSender != nil && h.emailSender.IsConfigured()
	message := "A verification email has been sent. Please check your inbox to complete registration."

	if verificationRequired {
		// Deliver the verification email; failures are logged, not fatal.
		h.sendVerificationEmail(c.Request.Context(), user.Email, user.Name, user.PreferredLocale, verificationToken)
	} else {
		if err := h.authService.MarkEmailVerified(c.Request.Context(), user.ID); err != nil {
			h.sendError(c, http.StatusInternalServerError, "Registration failed")
			return
		}
		user.EmailVerified = true
		message = "Account created successfully. You can now sign in."
	}

	c.JSON(http.StatusCreated, generated.RegistrationResponse{
		Email:                openapi_types.Email(user.Email),
		Message:              message,
		VerificationRequired: verificationRequired,
	})
}

// preferredLocaleString converts the optional generated locale enum into a
// plain string, returning an empty value when omitted.
func preferredLocaleString(locale *generated.RegisterUserJSONBodyPreferredLocale) string {
	if locale == nil {
		return ""
	}
	return string(*locale)
}

// VerifyEmail implements POST /auth/verify-email
// (POST /auth/verify-email)
func (h *APIHandler) VerifyEmail(c *gin.Context) {
	if !h.requireLocalAuth(c) {
		return
	}

	var req generated.VerifyEmailJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, tokens, err := h.authService.VerifyEmail(c.Request.Context(), req.Token)
	if err != nil {
		if errors.Is(err, service.ErrInvalidToken) || errors.Is(err, service.ErrTokenUsed) || errors.Is(err, service.ErrTokenExpired) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Email verification failed")
		return
	}

	c.JSON(http.StatusOK, h.convertAuthResponse(user, tokens))
}

// ResendVerificationEmail implements POST /auth/verify-email/resend
// (POST /auth/verify-email/resend)
func (h *APIHandler) ResendVerificationEmail(c *gin.Context) {
	if !h.requireLocalAuth(c) {
		return
	}

	var req generated.ResendVerificationEmailJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	parsed, err := mail.ParseAddress(string(req.Email))
	if err != nil {
		// Indistinguishable from the success path.
		c.Status(http.StatusNoContent)
		return
	}

	token, userEmail, userName, locale, err := h.authService.ResendVerification(c.Request.Context(), parsed.Address)
	if err != nil {
		// Don't leak internal errors.
		c.Status(http.StatusNoContent)
		return
	}

	if token != "" && userEmail != "" {
		h.sendVerificationEmail(c.Request.Context(), userEmail, userName, locale, token)
	}

	c.Status(http.StatusNoContent)
}

// sendVerificationEmail delivers the email-verification message. Errors are
// swallowed.
func (h *APIHandler) sendVerificationEmail(ctx context.Context, toEmail, userName, locale, token string) {
	if h.emailSender == nil || toEmail == "" || token == "" {
		return
	}
	link := fmt.Sprintf("%s/verify-email?token=%s", frontendBaseURL(), token)

	tmpl := emailpkg.Templates(locale)
	subject, body := tmpl.VerifyEmail(emailpkg.VerifyEmailParams{
		UserName: userName,
		Link:     link,
	})
	_ = h.emailSender.SendMessage(ctx, emailpkg.Message{
		To: toEmail, Subject: subject, HTMLBody: body, Type: emailpkg.TypeVerifyEmail,
	})
}

// LoginUser implements POST /auth/login
// (POST /auth/login)
func (h *APIHandler) LoginUser(c *gin.Context) {
	if !h.requireLocalAuth(c) {
		return
	}

	var req generated.LoginUserJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, tokens, err := h.authService.Login(c.Request.Context(), service.LoginInput{
		Email:    string(req.Email),
		Password: req.Password,
	})

	if err != nil {
		if err == service.ErrInvalidCredentials {
			AuthLoginAttemptsTotal.WithLabelValues("invalid_credentials").Inc()
			h.sendError(c, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		if err == service.ErrAccountLocked {
			AuthLoginAttemptsTotal.WithLabelValues("account_locked").Inc()
			h.sendError(c, http.StatusTooManyRequests, "Account temporarily locked due to too many failed login attempts. Please try again later.")
			return
		}
		if err == service.ErrAccountDisabled {
			AuthLoginAttemptsTotal.WithLabelValues("account_disabled").Inc()
			h.sendError(c, http.StatusForbidden, "Account disabled. Contact the administrator.")
			return
		}
		if err == service.ErrEmailNotVerified {
			AuthLoginAttemptsTotal.WithLabelValues("email_not_verified").Inc()
			code := "email_not_verified"
			c.JSON(http.StatusForbidden, generated.Error{
				Error: "Email address not verified. Please check your inbox for the verification link.",
				Code:  &code,
			})
			return
		}
		AuthLoginAttemptsTotal.WithLabelValues("error").Inc()
		h.sendError(c, http.StatusInternalServerError, "Login failed")
		return
	}

	if user.TwoFactorEnabled {
		AuthLoginAttemptsTotal.WithLabelValues("2fa_required").Inc()
		twoFactorToken, err := h.jwtManager.Generate2FAToken(user.ID)
		if err != nil {
			h.sendError(c, http.StatusInternalServerError, "Failed to generate 2FA token")
			return
		}

		// One of the two shapes the spec's 200 allows.
		c.JSON(http.StatusOK, generated.TwoFactorLoginRequired{
			RequiresTwoFactor: true,
			TwoFactorToken:    twoFactorToken,
		})
		return
	}

	AuthLoginAttemptsTotal.WithLabelValues("success").Inc()
	c.JSON(http.StatusOK, h.convertAuthResponse(user, tokens))
}

// RefreshToken implements POST /auth/refresh
// (POST /auth/refresh)
func (h *APIHandler) RefreshToken(c *gin.Context) {
	var req generated.RefreshTokenJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	tokens, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		AuthTokenRefreshTotal.WithLabelValues("invalid").Inc()
		h.sendError(c, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	AuthTokenRefreshTotal.WithLabelValues("success").Inc()
	c.JSON(http.StatusOK, map[string]interface{}{
		"accessToken":  tokens.AccessToken,
		"refreshToken": tokens.RefreshToken,
		"expiresIn":    900,
	})
}

// LogoutUser implements POST /auth/logout. It revokes the presented refresh
// token server-side and always answers 204; revocation is idempotent.
func (h *APIHandler) LogoutUser(c *gin.Context) {
	var req generated.LogoutUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.RefreshToken != "" {
		if err := h.authService.Logout(c.Request.Context(), req.RefreshToken); err != nil {
			// An unknown or already-revoked token is indistinguishable from a
			// successful revocation.
			slog.Warn("logout revocation failed", "error", err)
		}
	}
	c.Status(http.StatusNoContent)
}

// ChangePassword implements POST /auth/change-password
// (POST /auth/change-password)
func (h *APIHandler) ChangePassword(c *gin.Context) {
	if !h.requireLocalAuth(c) {
		return
	}
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.ChangePasswordJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.authService.ChangePassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			h.sendError(c, http.StatusUnauthorized, "Current password is incorrect")
			return
		}
		if err == service.ErrPasswordTooShort || err == service.ErrPasswordTooLong ||
			err == service.ErrPasswordTooWeak {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(c, http.StatusBadRequest, "Failed to change password")
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteCurrentUser implements DELETE /users/me
// (DELETE /users/me)
func (h *APIHandler) DeleteCurrentUser(c *gin.Context) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req generated.DeleteCurrentUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Deletion requires re-confirming identity: a password where one exists,
	// the account's own address typed out in OIDC mode.
	if h.OIDCEnabled() {
		user, err := h.authService.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			h.sendError(c, http.StatusInternalServerError, "Failed to delete account")
			return
		}
		if req.ConfirmEmail == nil || !strings.EqualFold(strings.TrimSpace(string(*req.ConfirmEmail)), user.Email) {
			h.sendError(c, http.StatusBadRequest,
				"Type your account email address to confirm deletion")
			return
		}
		if err := h.authService.DeleteUserConfirmed(c.Request.Context(), userID); err != nil {
			h.sendError(c, http.StatusInternalServerError, "Failed to delete account")
			return
		}
		c.Status(http.StatusNoContent)
		return
	}

	if req.Password == nil || *req.Password == "" {
		h.sendError(c, http.StatusBadRequest, "Password is required to confirm deletion")
		return
	}

	if err := h.authService.DeleteUser(c.Request.Context(), userID, *req.Password); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) || errors.Is(err, service.ErrPasswordNotSet) {
			h.sendError(c, http.StatusUnauthorized, "Password is incorrect")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Failed to delete account")
		return
	}

	c.Status(http.StatusNoContent)
}

// RequestPasswordReset implements POST /auth/password-reset-request
// (POST /auth/password-reset-request)
func (h *APIHandler) RequestPasswordReset(c *gin.Context) {
	if !h.requireLocalAuth(c) {
		return
	}

	var req generated.RequestPasswordResetJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	parsedEmail, err := mail.ParseAddress(string(req.Email))
	if err != nil {
		// Keep response indistinguishable to prevent user enumeration.
		c.Status(http.StatusNoContent)
		return
	}

	reset, err := h.authService.RequestPasswordReset(c.Request.Context(), parsedEmail.Address)
	if err != nil {
		// Don't reveal internal errors to the client
		c.Status(http.StatusNoContent)
		return
	}

	// Send the reset email if a token was generated. The recipient is the
	// canonical address loaded from the database, not the HTTP request body.
	if reset.Token != "" && reset.Email != "" && h.emailSender != nil {
		resetLink := fmt.Sprintf("%s/new-password?token=%s", frontendBaseURL(), reset.Token)
		tmpl := emailpkg.Templates(reset.Locale)
		subject, body := tmpl.PasswordReset(emailpkg.PasswordResetParams{
			UserName:         reset.Name,
			Link:             resetLink,
			TwoFactorEnabled: reset.TwoFactorEnabled,
		})
		_ = h.emailSender.SendMessage(c.Request.Context(), emailpkg.Message{
			To: reset.Email, Subject: subject, HTMLBody: body, Type: emailpkg.TypePasswordReset,
		})
	}

	// Always 204.
	c.Status(http.StatusNoContent)
}

// ResetPassword implements POST /auth/password-reset
// (POST /auth/password-reset)
//
// A reset does not disable 2FA: an account with 2FA enabled must supply a
// TOTP or recovery code alongside the reset token. Either way the owner is
// told by mail that their password changed.
func (h *APIHandler) ResetPassword(c *gin.Context) {
	if !h.requireLocalAuth(c) {
		return
	}

	var req generated.ResetPasswordJSONRequestBody

	if err := c.ShouldBindJSON(&req); err != nil {
		h.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	twoFactorCode := ""
	if req.TwoFactorCode != nil {
		twoFactorCode = *req.TwoFactorCode
	}

	result, err := h.authService.ResetPassword(c.Request.Context(), req.Token, req.NewPassword, twoFactorCode)
	if err != nil {
		if errors.Is(err, service.ErrInvalidToken) || errors.Is(err, service.ErrTokenUsed) || errors.Is(err, service.ErrTokenExpired) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, service.ErrPasswordTooShort) || errors.Is(err, service.ErrPasswordTooLong) ||
			errors.Is(err, service.ErrPasswordTooWeak) {
			h.sendError(c, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, service.ErrTwoFactorRequired) {
			code := "two_factor_required"
			c.JSON(http.StatusUnauthorized, generated.Error{
				Error: "This account uses two-factor authentication. Enter a code from your authenticator app or one of your recovery codes to reset the password.",
				Code:  &code,
			})
			return
		}
		if errors.Is(err, service.ErrInvalidTOTPCode) {
			code := "invalid_two_factor_code"
			c.JSON(http.StatusUnauthorized, generated.Error{
				Error: "Invalid two-factor code",
				Code:  &code,
			})
			return
		}
		if errors.Is(err, service.ErrAccountLocked) {
			h.sendError(c, http.StatusTooManyRequests, "Account temporarily locked due to too many failed attempts. Please try again later.")
			return
		}
		h.sendError(c, http.StatusInternalServerError, "Password reset failed")
		return
	}

	h.sendPasswordChangedEmail(c.Request.Context(), result)

	c.Status(http.StatusNoContent)
}

// sendPasswordChangedEmail tells the account owner that their password was
// reset. Errors are swallowed.
func (h *APIHandler) sendPasswordChangedEmail(ctx context.Context, result *service.PasswordResetResult) {
	if h.emailSender == nil || result == nil || result.Email == "" {
		return
	}
	tmpl := emailpkg.Templates(result.Locale)
	subject, body := tmpl.PasswordChanged(emailpkg.PasswordChangedParams{
		UserName:         result.Name,
		TwoFactorEnabled: result.TwoFactorEnabled,
	})
	_ = h.emailSender.SendMessage(ctx, emailpkg.Message{
		To: result.Email, Subject: subject, HTMLBody: body, Type: emailpkg.TypePasswordChanged,
	})
}

func (h *APIHandler) convertAuthResponse(user *models.User, tokens *service.TokenPair) generated.AuthResponse {
	return generated.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    900,
		User:         h.buildUserResponse(user),
	}
}
