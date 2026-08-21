package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/pkg/hash"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/google/uuid"
)

var (
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenRevoked       = errors.New("token revoked")
	ErrTokenUsed          = errors.New("token already used")
	ErrAccountLocked      = errors.New("account temporarily locked due to too many failed login attempts")
	ErrAccountDisabled    = errors.New("account disabled by administrator")
	ErrEmailNotVerified   = errors.New("email address not verified")
	ErrPasswordTooShort   = errors.New("password must be at least 12 characters")
	ErrPasswordTooLong    = errors.New("password must not exceed 72 characters")
	ErrPasswordTooWeak    = errors.New("password must contain at least one lowercase letter, one uppercase letter, one digit and one special character")
	ErrEmailRequired      = errors.New("email is required")
	ErrPasswordRequired   = errors.New("password is required")
	ErrNameRequired       = errors.New("name is required")
	ErrInvalidEmail       = errors.New("invalid email format")
	ErrEmailTooLong       = errors.New("email must not exceed 255 characters")
	// ErrPasswordNotSet marks an account that has no local password (an OIDC
	// account).
	ErrPasswordNotSet = errors.New("account has no local password")

	// ErrTwoFactorRequired is returned by ResetPassword when the account has 2FA
	// enabled and the caller supplied no code; the reset token is left unused.
	ErrTwoFactorRequired = errors.New("two-factor authentication code required")
	// ErrTwoFactorUnavailable is returned when a 2FA-protected reset is attempted
	// but no validator is wired.
	ErrTwoFactorUnavailable = errors.New("two-factor verification is unavailable")

	// ErrTokenReuseDetected marks a refresh token presented after its reuse
	// grace elapsed. The session it belonged to is revoked.
	ErrTokenReuseDetected = errors.New("refresh token reuse detected")
	// ErrSessionNotFound marks a session that does not exist, is already
	// revoked, or belongs to another user.
	ErrSessionNotFound = errors.New("session not found")
)

const (
	maxFailedLoginAttempts         = 5
	accountLockDuration            = 15 * time.Minute
	emailVerificationTokenLifetime = 24 * time.Hour
)

// TwoFactorValidator is the part of TwoFactorService that the password-reset
// flow depends on.
type TwoFactorValidator interface {
	// ValidateTOTP reports whether code is a valid TOTP code or an unused
	// recovery code for the user. Recovery codes are consumed on success, and
	// failures count toward the account lockout.
	ValidateTOTP(ctx context.Context, userID uuid.UUID, code string) (bool, error)
}

type AuthService struct {
	userRepo              repository.UserRepository
	refreshTokenRepo      repository.RefreshTokenRepository
	passwordResetRepo     repository.PasswordResetTokenRepository
	emailVerificationRepo repository.EmailVerificationTokenRepository
	jwtManager            *jwt.Manager
	twoFactor             TwoFactorValidator
	sessionPolicy         SessionPolicy
}

// NewAuthService constructs the service. twoFactor may be nil, in which case a
// password reset for an account with 2FA enabled fails with
// ErrTwoFactorUnavailable. Non-positive fields of policy take their defaults.
func NewAuthService(
	userRepo repository.UserRepository,
	refreshTokenRepo repository.RefreshTokenRepository,
	passwordResetRepo repository.PasswordResetTokenRepository,
	emailVerificationRepo repository.EmailVerificationTokenRepository,
	jwtManager *jwt.Manager,
	twoFactor TwoFactorValidator,
	policy SessionPolicy,
) *AuthService {
	return &AuthService{
		userRepo:              userRepo,
		refreshTokenRepo:      refreshTokenRepo,
		passwordResetRepo:     passwordResetRepo,
		emailVerificationRepo: emailVerificationRepo,
		jwtManager:            jwtManager,
		twoFactor:             twoFactor,
		sessionPolicy:         policy.normalized(),
	}
}

// SessionPolicy reports the policy the service was constructed with.
func (s *AuthService) SessionPolicy() SessionPolicy {
	return s.sessionPolicy
}

type RegisterInput struct {
	Email           string
	Password        string
	Name            string
	PreferredLocale string
}

type LoginInput struct {
	Email    string
	Password string
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	// SessionID is the session the pair belongs to.
	SessionID uuid.UUID
}

// Register creates a new user account with EmailVerified=false, stores an
// email-verification token, and returns the plaintext token for the handler
// to deliver via email. No JWT tokens are issued; the user must consume the
// verification token (see VerifyEmail) before logging in.
func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*models.User, string, error) {
	// Normalize and validate input
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)

	if input.Email == "" {
		return nil, "", ErrEmailRequired
	}
	if input.Password == "" {
		return nil, "", ErrPasswordRequired
	}
	if input.Name == "" {
		return nil, "", ErrNameRequired
	}
	if len(input.Email) > 255 {
		return nil, "", ErrEmailTooLong
	}
	if _, err := mail.ParseAddress(input.Email); err != nil {
		return nil, "", ErrInvalidEmail
	}
	if err := validatePassword(input.Password); err != nil {
		return nil, "", err
	}

	// Normalize preferred locale; fall back to the default for unknown values.
	locale := strings.ToLower(strings.TrimSpace(input.PreferredLocale))
	if locale != "de" {
		locale = "en"
	}

	// Check if user already exists
	existingUser, err := s.userRepo.GetByEmail(ctx, input.Email)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, "", err
	}
	if existingUser != nil {
		return nil, "", ErrUserAlreadyExists
	}

	// Hash password
	hashedPassword, err := hash.HashPassword(input.Password)
	if err != nil {
		return nil, "", err
	}

	// Create user (unverified)
	now := time.Now()
	user := &models.User{
		Email:           input.Email,
		PasswordHash:    hashedPassword,
		Name:            input.Name,
		EmailVerified:   false,
		PreferredLocale: locale,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			return nil, "", ErrUserAlreadyExists
		}
		return nil, "", err
	}

	// Generate verification token
	token, err := s.createEmailVerificationToken(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}

	return user, token, nil
}

// createEmailVerificationToken removes any existing verification tokens for the
// user, then mints, stores, and returns a fresh single-use token.
func (s *AuthService) createEmailVerificationToken(ctx context.Context, userID uuid.UUID) (string, error) {
	if err := s.emailVerificationRepo.DeleteForUser(ctx, userID); err != nil {
		return "", err
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	rec := &models.EmailVerificationToken{
		UserID:    userID,
		TokenHash: hash.HashToken(token),
		ExpiresAt: time.Now().Add(emailVerificationTokenLifetime),
		Used:      false,
	}
	if err := s.emailVerificationRepo.Create(ctx, rec); err != nil {
		return "", err
	}
	return token, nil
}

// VerifyEmail consumes a verification token and, on success, marks the user's
// email as verified and returns a fresh access/refresh token pair.
func (s *AuthService) VerifyEmail(ctx context.Context, token string) (*models.User, *TokenPair, error) {
	if token == "" {
		return nil, nil, ErrInvalidToken
	}
	tokenHash := hash.HashToken(token)
	rec, err := s.emailVerificationRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrInvalidToken
		}
		return nil, nil, err
	}
	if rec.Used {
		return nil, nil, ErrTokenUsed
	}
	if rec.ExpiresAt.Before(time.Now()) {
		return nil, nil, ErrTokenExpired
	}

	user, err := s.userRepo.GetByID(ctx, rec.UserID)
	if err != nil {
		return nil, nil, err
	}

	if !user.EmailVerified {
		if err := s.userRepo.MarkEmailVerified(ctx, user.ID); err != nil {
			return nil, nil, err
		}
		user.EmailVerified = true
	}

	if err := s.emailVerificationRepo.MarkAsUsed(ctx, tokenHash); err != nil {
		return nil, nil, err
	}

	tokens, err := s.startSession(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}

	// Verifying counts as a login.
	s.RecordLogin(ctx, user)

	return user, tokens, nil
}

// ResendVerification issues a fresh verification token for the given email if
// (and only if) the address belongs to a known, not-yet-verified user. The
// returned values are empty when nothing should be sent; "unknown email" and
// "already verified" are indistinguishable to callers.
func (s *AuthService) ResendVerification(ctx context.Context, email string) (token, userEmail, userName, locale string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", "", "", "", nil
		}
		return "", "", "", "", err
	}
	if user.EmailVerified {
		return "", "", "", "", nil
	}
	token, err = s.createEmailVerificationToken(ctx, user.ID)
	if err != nil {
		return "", "", "", "", err
	}
	return token, user.Email, user.Name, user.PreferredLocale, nil
}

// MarkEmailVerified marks a user as verified and clears any outstanding
// verification tokens. Used for environments where email delivery is disabled.
func (s *AuthService) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !user.EmailVerified {
		if err := s.userRepo.MarkEmailVerified(ctx, userID); err != nil {
			return err
		}
	}
	if err := s.emailVerificationRepo.DeleteForUser(ctx, userID); err != nil {
		return err
	}
	return nil
}

// Login authenticates a user and returns tokens
func (s *AuthService) Login(ctx context.Context, input LoginInput) (*models.User, *TokenPair, error) {
	// Normalize email
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))

	// Get user by email
	user, err := s.userRepo.GetByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Burn CPU comparable to a real password comparison.
			hash.DummyCompare()
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}

	// Enforce account lockout before the password check.
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return nil, nil, ErrAccountLocked
	}

	// No local password (OIDC account): burn a comparison and refuse.
	if !user.HasPassword() {
		hash.DummyCompare()
		return nil, nil, ErrInvalidCredentials
	}

	// Verify the password before surfacing any account-state details; a wrong
	// password always yields the same generic ErrInvalidCredentials.
	if err := hash.ComparePassword(user.PasswordHash, input.Password); err != nil {
		_ = s.userRepo.IncrementFailedLoginAttempts(ctx, user.ID)

		// Lock account after maxFailedLoginAttempts consecutive failures
		if user.FailedLoginAttempts+1 >= maxFailedLoginAttempts {
			_ = s.userRepo.LockAccount(ctx, user.ID, time.Now().Add(accountLockDuration))
		}

		return nil, nil, ErrInvalidCredentials
	}

	if user.Disabled {
		return nil, nil, ErrAccountDisabled
	}
	if !user.EmailVerified {
		return nil, nil, ErrEmailNotVerified
	}

	// Successful login — reset failed attempts
	if user.FailedLoginAttempts > 0 {
		_ = s.userRepo.ResetFailedLoginAttempts(ctx, user.ID)
	}

	// For a 2FA account, Login2FA stamps the login and starts the session
	// after the second factor.
	if user.TwoFactorEnabled {
		return user, nil, nil
	}
	s.RecordLogin(ctx, user)

	tokens, err := s.startSession(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}

	return user, tokens, nil
}

// RefreshToken rotates a refresh token, returning a new pair on the same
// session. A token superseded within the policy's reuse grace is still
// accepted; presenting one after the grace revokes the whole session and
// returns ErrTokenReuseDetected.
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	if _, err := s.jwtManager.ValidateRefreshToken(refreshToken); err != nil {
		if errors.Is(err, jwt.ErrExpiredToken) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	tokenHash := hash.HashToken(refreshToken)
	storedToken, err := s.refreshTokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	if storedToken.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	rotate := false
	if storedToken.Revoked {
		// Revoked outright — a logout, a password change, or the owner ending
		// the session. No grace applies.
		if storedToken.RotatedAt == nil {
			return nil, ErrTokenRevoked
		}
		if !s.withinReuseGrace(storedToken) {
			if err := s.refreshTokenRepo.RevokeSession(ctx, storedToken.UserID, storedToken.SessionID); err != nil &&
				!errors.Is(err, repository.ErrNotFound) {
				slog.Warn("failed to revoke session after refresh token replay",
					"user_id", storedToken.UserID, "session_id", storedToken.SessionID, "error", err)
			}
			RefreshReuseDetectedTotal.Inc()
			return nil, ErrTokenReuseDetected
		}
		RefreshGraceTotal.Inc()
	} else {
		rotate = true
	}

	// The replacement row is written before the presented token is superseded.
	pair, err := s.generateTokenPair(ctx, storedToken.UserID, storedToken.SessionID, deviceForRotation(ctx, storedToken))
	if err != nil {
		return nil, err
	}

	if rotate {
		if err := s.refreshTokenRepo.MarkRotated(ctx, tokenHash); err != nil {
			return nil, err
		}
	}

	return pair, nil
}

// withinReuseGrace reports whether a rotated token was superseded recently
// enough to still be served.
func (s *AuthService) withinReuseGrace(token *models.RefreshToken) bool {
	if token.RotatedAt == nil {
		return false
	}
	return time.Since(*token.RotatedAt) <= s.sessionPolicy.normalized().ReuseGrace
}

// Logout revokes a refresh token
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	tokenHash := hash.HashToken(refreshToken)
	return s.refreshTokenRepo.RevokeByTokenHash(ctx, tokenHash)
}

// PasswordResetRequest describes the recipient of a password-reset mail. All
// fields are loaded from the database, never from the HTTP request.
type PasswordResetRequest struct {
	// Token is the plaintext reset token to embed in the mail. Empty when no
	// account matched; callers must stay silent in that case.
	Token string
	Email string
	Name  string
	// Locale is the account's preferred locale, for template selection.
	Locale string
	// TwoFactorEnabled tells the mail to warn the user up front that they will
	// also need their authenticator (or a recovery code) to finish the reset.
	TwoFactorEnabled bool
}

// RequestPasswordReset creates a password reset token. On success it returns
// the reset token together with the recipient details loaded from the
// database, never from the HTTP request.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) (*PasswordResetRequest, error) {
	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Get user
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Unknown address: empty request.
		if errors.Is(err, repository.ErrNotFound) {
			return &PasswordResetRequest{}, nil
		}
		return nil, err
	}

	// Delete any existing reset tokens for this user
	if err := s.passwordResetRepo.DeleteForUser(ctx, user.ID); err != nil {
		return nil, err
	}

	// Generate reset token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Store token
	resetToken := &models.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: hash.HashToken(token),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Used:      false,
	}

	if err := s.passwordResetRepo.Create(ctx, resetToken); err != nil {
		return nil, err
	}

	return &PasswordResetRequest{
		Token:            token,
		Email:            user.Email,
		Name:             user.Name,
		Locale:           user.PreferredLocale,
		TwoFactorEnabled: user.TwoFactorEnabled,
	}, nil
}

// PasswordResetResult describes a completed password reset.
type PasswordResetResult struct {
	UserID uuid.UUID
	Email  string
	Name   string
	Locale string
	// TwoFactorEnabled reports whether the account still has 2FA active; a
	// reset never turns it off.
	TwoFactorEnabled bool
}

// ResetPassword resets a user's password using a reset token. A reset does NOT
// disable two-factor authentication: an account with 2FA enabled must prove
// the second factor here too — twoFactorCode is a TOTP code or one of the
// account's recovery codes. The reset token is consumed only on success, so a
// wrong or missing code can be retried with the same link.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword, twoFactorCode string) (*PasswordResetResult, error) {
	// Get token
	tokenHash := hash.HashToken(token)
	resetToken, err := s.passwordResetRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	// Check if token is valid
	if resetToken.Used {
		return nil, ErrTokenUsed
	}

	if resetToken.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, resetToken.UserID)
	if err != nil {
		return nil, err
	}

	// Validate new password
	if err := validatePassword(newPassword); err != nil {
		return nil, err
	}

	if user.TwoFactorEnabled {
		if strings.TrimSpace(twoFactorCode) == "" {
			return nil, ErrTwoFactorRequired
		}
		if s.twoFactor == nil {
			return nil, ErrTwoFactorUnavailable
		}
		valid, err := s.twoFactor.ValidateTOTP(ctx, user.ID, twoFactorCode)
		if err != nil {
			return nil, err
		}
		if !valid {
			return nil, ErrInvalidTOTPCode
		}

		// Re-read the user: validating may have consumed a recovery code and
		// cleared the failed-attempt counter.
		user, err = s.userRepo.GetByID(ctx, resetToken.UserID)
		if err != nil {
			return nil, err
		}
	}

	// Hash new password
	hashedPassword, err := hash.HashPassword(newPassword)
	if err != nil {
		return nil, err
	}

	// Update the password only; 2FA enrolment is untouched.
	user.PasswordHash = hashedPassword
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	// Mark token as used
	if err := s.passwordResetRepo.MarkAsUsed(ctx, tokenHash); err != nil {
		return nil, err
	}

	// Revoke all refresh tokens for this user
	if err := s.refreshTokenRepo.RevokeAllForUser(ctx, user.ID); err != nil {
		return nil, err
	}

	return &PasswordResetResult{
		UserID:           user.ID,
		Email:            user.Email,
		Name:             user.Name,
		Locale:           user.PreferredLocale,
		TwoFactorEnabled: user.TwoFactorEnabled,
	}, nil
}

// generateTokenPair creates an access/refresh pair bound to sessionID, minting
// a session when sessionID is uuid.Nil, and records the pair against device.
func (s *AuthService) generateTokenPair(
	ctx context.Context,
	userID, sessionID uuid.UUID,
	device DeviceInfo,
) (*TokenPair, error) {
	if sessionID == uuid.Nil {
		sessionID = uuid.New()
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(userID, sessionID)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.jwtManager.GenerateRefreshToken(userID, sessionID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	storedToken := &models.RefreshToken{
		UserID:      userID,
		TokenHash:   hash.HashToken(refreshToken),
		ExpiresAt:   now.Add(s.jwtManager.GetRefreshTokenExpiry()),
		Revoked:     false,
		SessionID:   sessionID,
		DeviceLabel: DeviceLabel(device.UserAgent),
		UserAgent:   truncateUserAgent(device.UserAgent),
		IPAddress:   device.IPAddress,
		LastUsedAt:  now,
	}

	if err := s.refreshTokenRepo.Create(ctx, storedToken); err != nil {
		return nil, err
	}

	if err := s.refreshTokenRepo.TouchSession(ctx, sessionID, now); err != nil {
		slog.Warn("failed to stamp session last use", "session_id", sessionID, "error", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionID:    sessionID,
	}, nil
}

// AccessTokenState reports whether the account behind an access token is
// disabled and whether the token's session still holds a live refresh token.
// A deleted account reports live=false rather than an error.
func (s *AuthService) AccessTokenState(ctx context.Context, userID, sessionID uuid.UUID) (bool, bool, error) {
	disabled, live, err := s.refreshTokenRepo.AccessTokenState(ctx, userID, sessionID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return disabled, live, nil
}

// GetUserByID retrieves a user by ID
func (s *AuthService) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}

// RecordLogin stamps a successful sign-in on the account and mirrors the value
// onto the in-memory user. Called by every path that hands a session to a
// user: password login, the 2FA second factor, passkeys, OIDC, and the sign-up
// verification link. A refresh is not a login. Failure is logged and
// swallowed.
func (s *AuthService) RecordLogin(ctx context.Context, user *models.User) {
	now := time.Now()
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID, now); err != nil {
		slog.Warn("failed to record last login", "user_id", user.ID, "error", err)
		return
	}
	user.LastLoginAt = &now
	user.UpdatedAt = now
}

// GenerateTokensForUser starts a session for a user whose identity has already
// been proven by another factor: the 2FA step, a passkey, or an OIDC handoff.
func (s *AuthService) GenerateTokensForUser(ctx context.Context, userID uuid.UUID) (*TokenPair, error) {
	return s.startSession(ctx, userID)
}

// UpdateUser updates user information
func (s *AuthService) UpdateUser(ctx context.Context, user *models.User) error {
	// Normalize email
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))

	// Keep the stored flights-list preference canonical: a known mode, and a
	// deduplicated column list in display order with unknown keys dropped.
	user.FlightListColumnMode = models.NormalizeFlightListColumnMode(user.FlightListColumnMode)
	user.FlightListColumns = models.NormalizeFlightListColumns(user.FlightListColumns)

	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			return ErrUserAlreadyExists
		}
		return err
	}
	return nil
}

// RevokeAllSessions deletes every refresh token a user holds, forcing a fresh
// login on all devices. Used when an admin disables an account, so a disabled
// user cannot keep an existing session alive.
func (s *AuthService) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	return s.refreshTokenRepo.DeleteForUser(ctx, userID)
}

// VerifyPassword checks a plaintext password against the stored hash for the
// given user. Used to re-authenticate before security-sensitive profile
// changes.
func (s *AuthService) VerifyPassword(ctx context.Context, userID uuid.UUID, password string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := hash.ComparePassword(user.PasswordHash, password); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// CreateEmailVerificationToken mints a fresh verification token for a user
// whose address has just changed.
func (s *AuthService) CreateEmailVerificationToken(ctx context.Context, userID uuid.UUID) (string, error) {
	return s.createEmailVerificationToken(ctx, userID)
}

// ChangePassword changes the user's password after verifying the current password
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify current password
	if !user.HasPassword() {
		return ErrPasswordNotSet
	}
	if err := hash.ComparePassword(user.PasswordHash, currentPassword); err != nil {
		return ErrInvalidCredentials
	}

	// Validate new password
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	// Hash new password
	hashedPassword, err := hash.HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hashedPassword
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return err
	}

	// Revoke all refresh tokens (force re-login on all devices)
	return s.refreshTokenRepo.RevokeAllForUser(ctx, userID)
}

// DeleteUser permanently deletes a user account after verifying the password
func (s *AuthService) DeleteUser(ctx context.Context, userID uuid.UUID, password string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify password
	if !user.HasPassword() {
		return ErrPasswordNotSet
	}
	if err := hash.ComparePassword(user.PasswordHash, password); err != nil {
		return ErrInvalidCredentials
	}

	return s.deleteUserAndTokens(ctx, userID)
}

// DeleteUserConfirmed permanently deletes an account whose identity has been
// confirmed by means other than a password; the confirmation itself happens in
// the handler.
func (s *AuthService) DeleteUserConfirmed(ctx context.Context, userID uuid.UUID) error {
	if _, err := s.userRepo.GetByID(ctx, userID); err != nil {
		return err
	}
	return s.deleteUserAndTokens(ctx, userID)
}

func (s *AuthService) deleteUserAndTokens(ctx context.Context, userID uuid.UUID) error {
	// Clean up tokens
	_ = s.refreshTokenRepo.DeleteForUser(ctx, userID)
	_ = s.passwordResetRepo.DeleteForUser(ctx, userID)

	// Delete user (cascades to licenses, flights via FK)
	return s.userRepo.Delete(ctx, userID)
}
