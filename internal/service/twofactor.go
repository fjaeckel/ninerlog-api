package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/pkg/hash"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/pquerna/otp/totp"
)

var (
	ErrTwoFactorAlreadyEnabled = errors.New("two-factor authentication is already enabled")
	ErrTwoFactorNotEnabled     = errors.New("two-factor authentication is not enabled")
	ErrInvalidTOTPCode         = errors.New("invalid TOTP code")
	ErrInvalid2FAToken         = errors.New("invalid two-factor token")
)

type TwoFactorService struct {
	userRepo   repository.UserRepository
	jwtManager *jwt.Manager
}

func NewTwoFactorService(userRepo repository.UserRepository, jwtManager *jwt.Manager) *TwoFactorService {
	return &TwoFactorService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
	}
}

// SetupTOTP generates a new TOTP secret for a user (does not enable 2FA yet)
func (s *TwoFactorService) SetupTOTP(ctx context.Context, userID uuid.UUID) (string, string, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", "", err
	}

	if user.TwoFactorEnabled {
		return "", "", ErrTwoFactorAlreadyEnabled
	}

	// Generate TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "NinerLog",
		AccountName: user.Email,
		Period:      30,
		Digits:      6,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// Store the secret (not yet enabled)
	secret := key.Secret()
	user.TwoFactorSecret = &secret
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return "", "", err
	}

	return secret, key.URL(), nil
}

// VerifyAndEnable verifies a TOTP code and enables 2FA, returning recovery codes
func (s *TwoFactorService) VerifyAndEnable(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.TwoFactorEnabled {
		return nil, ErrTwoFactorAlreadyEnabled
	}

	if user.TwoFactorSecret == nil {
		return nil, errors.New("2FA setup not started — call setup first")
	}

	// Verify the code against the stored secret
	valid := totp.Validate(code, *user.TwoFactorSecret)
	if !valid {
		return nil, ErrInvalidTOTPCode
	}

	// Generate recovery codes
	recoveryCodes, hashedCodes, err := generateRecoveryCodes(8)
	if err != nil {
		return nil, err
	}

	// Enable 2FA
	user.TwoFactorEnabled = true
	user.RecoveryCodes = pq.StringArray(hashedCodes)
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	return recoveryCodes, nil
}

// Disable disables 2FA for a user after password verification
func (s *TwoFactorService) Disable(ctx context.Context, userID uuid.UUID, password string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if !user.TwoFactorEnabled {
		return ErrTwoFactorNotEnabled
	}

	// Verify password
	if err := hash.ComparePassword(user.PasswordHash, password); err != nil {
		return ErrInvalidCredentials
	}

	// Disable 2FA
	user.TwoFactorEnabled = false
	user.TwoFactorSecret = nil
	user.RecoveryCodes = nil
	user.UpdatedAt = time.Now()
	return s.userRepo.Update(ctx, user)
}

// ValidateTOTP validates a TOTP code or recovery code for a user
func (s *TwoFactorService) ValidateTOTP(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}

	if !user.TwoFactorEnabled || user.TwoFactorSecret == nil {
		return false, ErrTwoFactorNotEnabled
	}

	// Try TOTP code first
	if totp.Validate(code, *user.TwoFactorSecret) {
		return true, nil
	}

	// Try recovery codes
	code = strings.TrimSpace(strings.ToLower(code))
	for i, hashedCode := range user.RecoveryCodes {
		if hash.ComparePassword(hashedCode, code) == nil {
			// Remove used recovery code
			user.RecoveryCodes = append(user.RecoveryCodes[:i], user.RecoveryCodes[i+1:]...)
			user.UpdatedAt = time.Now()
			_ = s.userRepo.Update(ctx, user)
			return true, nil
		}
	}

	return false, nil
}

// IsEnabled checks if 2FA is enabled for a user
func (s *TwoFactorService) IsEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return user.TwoFactorEnabled, nil
}

func generateRecoveryCodes(count int) ([]string, []string, error) {
	plainCodes := make([]string, count)
	hashedCodes := make([]string, count)

	for i := 0; i < count; i++ {
		bytes := make([]byte, 5)
		if _, err := rand.Read(bytes); err != nil {
			return nil, nil, err
		}
		code := hex.EncodeToString(bytes)
		plainCodes[i] = code[:5] + "-" + code[5:]

		hashed, err := hash.HashPassword(plainCodes[i])
		if err != nil {
			return nil, nil, err
		}
		hashedCodes[i] = hashed
	}

	return plainCodes, hashedCodes, nil
}
