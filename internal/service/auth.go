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
)

const (
	maxFailedLoginAttempts         = 5
	accountLockDuration            = 15 * time.Minute
	emailVerificationTokenLifetime = 24 * time.Hour
)

type AuthService struct {
	userRepo              repository.UserRepository
	refreshTokenRepo      repository.RefreshTokenRepository
	passwordResetRepo     repository.PasswordResetTokenRepository
	emailVerificationRepo repository.EmailVerificationTokenRepository
	jwtManager            *jwt.Manager
}

func NewAuthService(
	userRepo repository.UserRepository,
	refreshTokenRepo repository.RefreshTokenRepository,
	passwordResetRepo repository.PasswordResetTokenRepository,
	emailVerificationRepo repository.EmailVerificationTokenRepository,
	jwtManager *jwt.Manager,
) *AuthService {
	return &AuthService{
		userRepo:              userRepo,
		refreshTokenRepo:      refreshTokenRepo,
		passwordResetRepo:     passwordResetRepo,
		emailVerificationRepo: emailVerificationRepo,
		jwtManager:            jwtManager,
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

// Login authenticates a user and returns tokens
func (s *AuthService) Login(ctx context.Context, input LoginInput) (*models.User, *TokenPair, error) {
	// Normalize email
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))

	// Get user by email
	user, err := s.userRepo.GetByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}

	// Check if account is disabled
	if user.Disabled {
		return nil, nil, ErrAccountDisabled
	}

	// Block login until the user has confirmed their email address. The
	// check sits before password verification so we don't leak whether
	// the password is correct for an unverified account.
	if !user.EmailVerified {
		return nil, nil, ErrEmailNotVerified
	}

	// Check if account is locked
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return nil, nil, ErrAccountLocked
	}

	// Verify password
	if err := hash.ComparePassword(user.PasswordHash, input.Password); err != nil {
		// Increment failed attempts
		_ = s.userRepo.IncrementFailedLoginAttempts(ctx, user.ID)

		// Lock account after maxFailedLoginAttempts consecutive failures
		if user.FailedLoginAttempts+1 >= maxFailedLoginAttempts {
			_ = s.userRepo.LockAccount(ctx, user.ID, time.Now().Add(accountLockDuration))
		}

		return nil, nil, ErrInvalidCredentials
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

// RequestPasswordReset creates a password reset token.
//
// On success it returns both the reset token and the recipient email
// address loaded from the database. The handler uses the database-sourced
// email (rather than the HTTP request) when sending the reset mail, which
// keeps untrusted input out of the SMTP message (CWE-640).
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) (token, userEmail string, err error) {
	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Get user
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Don't reveal if user exists
		if errors.Is(err, repository.ErrNotFound) {
			return "", "", nil
		}
		return "", "", err
	}

	// Delete any existing reset tokens for this user
	if err := s.passwordResetRepo.DeleteForUser(ctx, user.ID); err != nil {
		return "", "", err
	}

	// Generate reset token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	token = base64.URLEncoding.EncodeToString(tokenBytes)

	// Store token
	resetToken := &models.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: hash.HashToken(token),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Used:      false,
	}

	if err := s.passwordResetRepo.Create(ctx, resetToken); err != nil {
		return "", "", err
	}

	return token, user.Email, nil
}

// ResetPassword resets a user's password using a reset token
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	// Get token
	tokenHash := hash.HashToken(token)
	resetToken, err := s.passwordResetRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrInvalidToken
		}
		return err
	}

	// Check if token is valid
	if resetToken.Used {
		return ErrTokenUsed
	}

	if resetToken.ExpiresAt.Before(time.Now()) {
		return ErrTokenExpired
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, resetToken.UserID)
	if err != nil {
		return err
	}

	// Validate new password
	if len(newPassword) < 12 {
		return ErrPasswordTooShort
	}
	if len(newPassword) > 72 {
		return ErrPasswordTooLong
	}

	// Hash new password
	hashedPassword, err := hash.HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Update user password and disable 2FA (user lost access to authenticator)
	user.PasswordHash = hashedPassword
	user.TwoFactorEnabled = false
	user.TwoFactorSecret = nil
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return err
	}

	// Mark token as used
	if err := s.passwordResetRepo.MarkAsUsed(ctx, tokenHash); err != nil {
		return err
	}

	// Revoke all refresh tokens for this user
	return s.refreshTokenRepo.RevokeAllForUser(ctx, user.ID)
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

	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			return ErrUserAlreadyExists
		}
		return err
	}
	return nil
}

// ChangePassword changes the user's password after verifying the current password
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify current password
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
	if err := hash.ComparePassword(user.PasswordHash, password); err != nil {
		return ErrInvalidCredentials
	}

	// Clean up tokens
	_ = s.refreshTokenRepo.DeleteForUser(ctx, userID)
	_ = s.passwordResetRepo.DeleteForUser(ctx, userID)

	// Delete user (cascades to licenses, flights via FK)
	return s.userRepo.Delete(ctx, userID)
}
