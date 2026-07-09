package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
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

// encSecretPrefix marks a TOTP secret that is stored encrypted (AES-256-GCM).
// Secrets without this prefix are legacy plaintext and are read as-is, so an
// existing database keeps working after the encryption key is introduced.
const encSecretPrefix = "enc:v1:"

type TwoFactorService struct {
	userRepo   repository.UserRepository
	jwtManager *jwt.Manager
	// aead encrypts TOTP secrets at rest. When nil, secrets are stored as
	// plaintext (legacy behavior) — the deployment should set an encryption key.
	aead *cryptoutil.AEAD
}

// NewTwoFactorService constructs the service. aead may be nil, in which case
// TOTP secrets are stored unencrypted (backward-compatible). Provide an AEAD
// (see cryptoutil) to encrypt secrets at rest.
func NewTwoFactorService(userRepo repository.UserRepository, jwtManager *jwt.Manager, aead *cryptoutil.AEAD) *TwoFactorService {
	return &TwoFactorService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
		aead:       aead,
	}
}

// encodeSecret returns the value to persist for a TOTP secret: an encrypted,
// prefixed blob when an AEAD is configured, otherwise the plaintext (legacy).
func (s *TwoFactorService) encodeSecret(plaintext string) (string, error) {
	if s.aead == nil {
		return plaintext, nil
	}
	ciphertext, nonce, err := s.aead.Encrypt([]byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("encrypt 2FA secret: %w", err)
	}
	blob := append(append([]byte{}, nonce...), ciphertext...)
	return encSecretPrefix + base64.StdEncoding.EncodeToString(blob), nil
}

// decodeSecret returns the plaintext TOTP secret from a stored value. Values
// without encSecretPrefix are treated as legacy plaintext.
func (s *TwoFactorService) decodeSecret(stored string) (string, error) {
	if !strings.HasPrefix(stored, encSecretPrefix) {
		return stored, nil // legacy plaintext secret
	}
	if s.aead == nil {
		return "", errors.New("2FA secret is encrypted but no encryption key is configured")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encSecretPrefix))
	if err != nil {
		return "", fmt.Errorf("decode 2FA secret: %w", err)
	}
	if len(raw) < cryptoutil.NonceSize {
		return "", errors.New("malformed encrypted 2FA secret")
	}
	nonce := raw[:cryptoutil.NonceSize]
	ciphertext := raw[cryptoutil.NonceSize:]
	plaintext, err := s.aead.Decrypt(ciphertext, nonce)
	if err != nil {
		return "", fmt.Errorf("decrypt 2FA secret: %w", err)
	}
	return string(plaintext), nil
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

	// Store the secret (not yet enabled), encrypted at rest when a key is set.
	secret := key.Secret()
	stored, err := s.encodeSecret(secret)
	if err != nil {
		return "", "", err
	}
	user.TwoFactorSecret = &stored
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(ctx, user); err != nil {
		return "", "", err
	}

	// Return the plaintext secret + otpauth URL to the caller for QR display.
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

	// Verify the code against the stored secret (decrypting it first if needed).
	secret, err := s.decodeSecret(*user.TwoFactorSecret)
	if err != nil {
		return nil, err
	}
	valid := totp.Validate(code, secret)
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

// ValidateTOTP validates a TOTP code or recovery code for a user.
//
// The 2FA step is brute-force protected with the same per-account lockout used
// by password login: a locked account is rejected with ErrAccountLocked, each
// failed code counts toward the lockout, and a successful validation resets the
// counter. This prevents an attacker who has the password from grinding TOTP
// codes via repeated /auth/2fa/login calls.
func (s *TwoFactorService) ValidateTOTP(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}

	if !user.TwoFactorEnabled || user.TwoFactorSecret == nil {
		return false, ErrTwoFactorNotEnabled
	}

	// Try TOTP code first (decrypting the stored secret if needed).
	secret, err := s.decodeSecret(*user.TwoFactorSecret)
	if err != nil {
		return false, err
	}
  // Reject further attempts while the account is locked.
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return false, ErrAccountLocked
	}
  
	if totp.Validate(code, secret) {
    s.resetFailedAttempts(ctx, user)
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
			s.resetFailedAttempts(ctx, user)
			return true, nil
		}
	}

	// Neither the TOTP nor a recovery code matched — count the failure toward
	// the account lockout.
	s.recordFailedAttempt(ctx, userID)
	return false, nil
}

// recordFailedAttempt increments the shared failed-attempt counter and locks the
// account once it reaches the threshold. It re-reads the authoritative count
// after incrementing so the threshold is correct regardless of repository
// implementation.
func (s *TwoFactorService) recordFailedAttempt(ctx context.Context, userID uuid.UUID) {
	_ = s.userRepo.IncrementFailedLoginAttempts(ctx, userID)
	if u, err := s.userRepo.GetByID(ctx, userID); err == nil && u.FailedLoginAttempts >= maxFailedLoginAttempts {
		_ = s.userRepo.LockAccount(ctx, userID, time.Now().Add(accountLockDuration))
	}
}

// resetFailedAttempts clears the failed-attempt counter after a successful 2FA
// validation.
func (s *TwoFactorService) resetFailedAttempts(ctx context.Context, user *models.User) {
	if user.FailedLoginAttempts > 0 {
		_ = s.userRepo.ResetFailedLoginAttempts(ctx, user.ID)
	}
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
