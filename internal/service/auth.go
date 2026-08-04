package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
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
	ErrEmailRequired      = errors.New("email is required")
	ErrPasswordRequired   = errors.New("password is required")
	ErrNameRequired       = errors.New("name is required")
	ErrInvalidEmail       = errors.New("invalid email format")
	ErrEmailTooLong       = errors.New("email must not exceed 255 characters")
	// ErrPasswordNotSet marks an account that has no local password — an OIDC
	// account. Password-based operations are refused outright rather than
	// compared against an empty hash.
	ErrPasswordNotSet = errors.New("account has no local password")

	// ErrTwoFactorRequired is returned by ResetPassword when the account has 2FA
	// enabled and the caller supplied no code. The reset token is left unused so
	// the caller can retry with a code.
	ErrTwoFactorRequired = errors.New("two-factor authentication code required")
	// ErrTwoFactorUnavailable is returned when a 2FA-protected reset is attempted
	// but no validator is wired. It fails the reset closed rather than falling
	// back to bypassing the second factor.
	ErrTwoFactorUnavailable = errors.New("two-factor verification is unavailable")
)

const (
	maxFailedLoginAttempts         = 5
	accountLockDuration            = 15 * time.Minute
	emailVerificationTokenLifetime = 24 * time.Hour
)

// TwoFactorValidator is the part of TwoFactorService that the password-reset
// flow depends on. It is an interface so AuthService does not have to own the
// TOTP/recovery-code implementation, and so tests can substitute a stub.
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
}

// NewAuthService constructs the service. twoFactor may be nil, in which case a
// password reset for an account with 2FA enabled fails with
// ErrTwoFactorUnavailable — the second factor is never silently skipped.
func NewAuthService(
	userRepo repository.UserRepository,
	refreshTokenRepo repository.RefreshTokenRepository,
	passwordResetRepo repository.PasswordResetTokenRepository,
	emailVerificationRepo repository.EmailVerificationTokenRepository,
	jwtManager *jwt.Manager,
	twoFactor TwoFactorValidator,
) *AuthService {
	return &AuthService{
		userRepo:              userRepo,
		refreshTokenRepo:      refreshTokenRepo,
		passwordResetRepo:     passwordResetRepo,
		emailVerificationRepo: emailVerificationRepo,
		jwtManager:            jwtManager,
		twoFactor:             twoFactor,
	}
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
}

// Register creates a new user account. The account is created with
// EmailVerified=false, an email-verification token is generated and stored,
// and the plaintext token is returned to the caller so the handler can
// deliver it via email. No JWT tokens are issued at this stage — the user
// must consume the verification token (see VerifyEmail) before they can
// log in.
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
	if len(input.Password) < 12 {
		return nil, "", ErrPasswordTooShort
	}
	if len(input.Password) > 72 {
		return nil, "", ErrPasswordTooLong
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
// email as verified and returns a fresh access/refresh token pair so the
// frontend can log the user in.
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

	tokens, err := s.generateTokenPair(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}

	return user, tokens, nil
}

// ResendVerification issues a fresh verification token for the given email if
// (and only if) the address belongs to a known, not-yet-verified user. The
// returned values are empty when nothing should be sent — callers should not
// distinguish "unknown email" from "already verified" to avoid enumeration.
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
			// Burn a comparable amount of CPU so the response timing of an
			// unknown account matches that of a wrong password. Combined with
			// the identical error below, this prevents user enumeration.
			hash.DummyCompare()
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}

	// Enforce account lockout before checking the password so a locked account
	// cannot be probed further.
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return nil, nil, ErrAccountLocked
	}

	// Accounts provisioned through OIDC have no local password. The password
	// endpoints are already disabled while OIDC mode is on, so this is defence
	// in depth for a deployment that switched modes with such accounts in the
	// database. It burns the same CPU as a real comparison so the absence of a
	// password is not detectable by response timing.
	if !user.HasPassword() {
		hash.DummyCompare()
		return nil, nil, ErrInvalidCredentials
	}

	// Verify the password BEFORE surfacing any account-state details. Returning
	// "disabled" / "email not verified" prior to authentication would let an
	// unauthenticated attacker enumerate which addresses are registered and in
	// what state (CWE-204). A wrong password always yields the same generic
	// ErrInvalidCredentials regardless of whether the account exists.
	if err := hash.ComparePassword(user.PasswordHash, input.Password); err != nil {
		// Increment failed attempts
		_ = s.userRepo.IncrementFailedLoginAttempts(ctx, user.ID)

		// Lock account after maxFailedLoginAttempts consecutive failures
		if user.FailedLoginAttempts+1 >= maxFailedLoginAttempts {
			_ = s.userRepo.LockAccount(ctx, user.ID, time.Now().Add(accountLockDuration))
		}

		return nil, nil, ErrInvalidCredentials
	}

	// Password is correct — from here on it is safe to reveal account state,
	// since only the legitimate owner reaches this point.
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

	// Update last login timestamp
	now := time.Now()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	_ = s.userRepo.Update(ctx, user)

	// Delete all existing refresh tokens for this user to avoid constraint violations
	// This ensures only one active session per user
	if err := s.refreshTokenRepo.DeleteForUser(ctx, user.ID); err != nil {
		// Log error but don't fail the login
	}

	// Generate tokens
	tokens, err := s.generateTokenPair(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}

	return user, tokens, nil
}

// RefreshToken generates a new access token using a refresh token
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	// Validate refresh token
	claims, err := s.jwtManager.ValidateRefreshToken(refreshToken)
	if err != nil {
		if errors.Is(err, jwt.ErrExpiredToken) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	// Check if token exists and is not revoked
	tokenHash := hash.HashToken(refreshToken)
	storedToken, err := s.refreshTokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	if storedToken.Revoked {
		return nil, ErrTokenRevoked
	}

	if storedToken.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	// Revoke the old refresh token (rotation: old token becomes invalid immediately)
	if err := s.refreshTokenRepo.RevokeByTokenHash(ctx, tokenHash); err != nil {
		return nil, err
	}

	// Generate new tokens
	return s.generateTokenPair(ctx, claims.UserID)
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
	// account matched — callers must stay silent in that case so the response
	// does not reveal whether the address exists.
	Token string
	Email string
	Name  string
	// Locale is the account's preferred locale, for template selection.
	Locale string
	// TwoFactorEnabled tells the mail to warn the user up front that they will
	// also need their authenticator (or a recovery code) to finish the reset.
	TwoFactorEnabled bool
}

// RequestPasswordReset creates a password reset token.
//
// On success it returns the reset token together with the recipient details
// loaded from the database. The handler uses the database-sourced email
// (rather than the HTTP request) when sending the reset mail, which keeps
// untrusted input out of the SMTP message (CWE-640).
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) (*PasswordResetRequest, error) {
	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Get user
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Don't reveal if user exists
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

// PasswordResetResult describes a completed password reset, so the handler can
// send the confirmation mail without loading the user again.
type PasswordResetResult struct {
	UserID uuid.UUID
	Email  string
	Name   string
	Locale string
	// TwoFactorEnabled reports whether the account still has 2FA active. A
	// reset never turns it off, so this is simply the account's current state.
	TwoFactorEnabled bool
}

// ResetPassword resets a user's password using a reset token.
//
// A reset does NOT disable two-factor authentication. It used to: control of
// the mailbox alone was enough to strip the second factor and take the account
// over, and the owner was never told. Instead, an account with 2FA enabled must
// prove the second factor here too — twoFactorCode is a TOTP code or one of the
// account's recovery codes, which is the self-service path for a lost
// authenticator. Only a user who has lost the authenticator AND every recovery
// code needs an admin (POST /admin/users/{userId}/reset-2fa).
//
// The reset token is consumed only on success, so a wrong or missing code can
// be retried with the same link. Code guessing is bounded by the shared account
// lockout applied inside the validator.
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
	if len(newPassword) < 12 {
		return nil, ErrPasswordTooShort
	}
	if len(newPassword) > 72 {
		return nil, ErrPasswordTooLong
	}

	if user.TwoFactorEnabled {
		if strings.TrimSpace(twoFactorCode) == "" {
			return nil, ErrTwoFactorRequired
		}
		if s.twoFactor == nil {
			// No validator wired — refuse rather than reset without the factor.
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
		// cleared the failed-attempt counter. Writing the copy loaded above back
		// would restore the consumed code and make it usable a second time.
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

	// Update the password only. 2FA enrolment is deliberately left untouched.
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

// generateTokenPair creates both access and refresh tokens
func (s *AuthService) generateTokenPair(ctx context.Context, userID uuid.UUID) (*TokenPair, error) {
	// Generate access token
	accessToken, err := s.jwtManager.GenerateAccessToken(userID)
	if err != nil {
		return nil, err
	}

	// Generate refresh token
	refreshToken, err := s.jwtManager.GenerateRefreshToken(userID)
	if err != nil {
		return nil, err
	}

	// Store refresh token
	tokenHash := hash.HashToken(refreshToken)
	storedToken := &models.RefreshToken{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.jwtManager.GetRefreshTokenExpiry()),
		Revoked:   false,
	}

	if err := s.refreshTokenRepo.Create(ctx, storedToken); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// GetUserByID retrieves a user by ID
func (s *AuthService) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}

// GenerateTokensForUser generates access and refresh tokens for a user (used after 2FA verification)
func (s *AuthService) GenerateTokensForUser(ctx context.Context, userID uuid.UUID) (*TokenPair, error) {
	return s.generateTokenPair(ctx, userID)
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

// VerifyPassword checks a plaintext password against the stored hash for the
// given user. Used to re-authenticate before security-sensitive profile
// changes (e.g. changing the email address, which is both the account recovery
// channel and the basis for admin authorization).
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
// whose address has just changed, so the new address can be proven.
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
	if len(newPassword) < 12 {
		return ErrPasswordTooShort
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
// confirmed by means other than a password.
//
// In OIDC mode there is no password to re-enter, so the handler confirms the
// destructive action by requiring the caller to type their own email address
// instead. The confirmation itself happens in the handler; this method is the
// deletion once that check has passed.
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
