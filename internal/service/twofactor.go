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

	// ErrTwoFactorKeyMissing means the service was built without an encryption
	// key. A running server cannot reach this — ENCRYPTION_KEY is required at
	// startup — so it exists to make the failure explicit in tests and in any
	// future wiring that forgets to pass one, instead of silently degrading to
	// plaintext seeds.
	ErrTwoFactorKeyMissing = errors.New("two-factor authentication is unavailable: no encryption key is configured")
)

// encSecretPrefix marks a TOTP secret that is stored encrypted (AES-256-GCM).
// Every stored secret carries it: migration 61 cleared the enrolments that
// predate mandatory encryption, so an unprefixed value is a corrupt row rather
// than an old one.
const encSecretPrefix = "enc:v1:"

type TwoFactorService struct {
	userRepo   repository.UserRepository
	jwtManager *jwt.Manager
	// aead encrypts TOTP secrets at rest, derived from ENCRYPTION_KEY. It is
	// never nil in a running server — the key is required at startup — and a
	// nil one fails enrolment and verification closed rather than falling back
	// to storing seeds in the clear.
	aead *cryptoutil.AEAD
}

// NewTwoFactorService constructs the service. aead comes from
// cryptoutil.DeriveAEAD(masterKey, PurposeTOTPSecrets); without it, 2FA cannot
// be set up or verified.
func NewTwoFactorService(userRepo repository.UserRepository, jwtManager *jwt.Manager, aead *cryptoutil.AEAD) *TwoFactorService {
	return &TwoFactorService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
		aead:       aead,
	}
}

// encodeSecret returns the value to persist for a TOTP secret: an encrypted,
// prefixed blob. There is no unencrypted form — a seed is a bearer credential
// for someone's second factor, and storing one in the clear because a key was
// missing was never a mode worth having.
func (s *TwoFactorService) encodeSecret(plaintext string) (string, error) {
	if s.aead == nil {
		return "", ErrTwoFactorKeyMissing
	}
	ciphertext, nonce, err := s.aead.Encrypt([]byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("encrypt 2FA secret: %w", err)
	}
	blob := append(append([]byte{}, nonce...), ciphertext...)
	return encSecretPrefix + base64.StdEncoding.EncodeToString(blob), nil
}

// decodeSecret returns the plaintext TOTP secret from a stored value.
//
// An unprefixed value is refused rather than read as a legacy plaintext seed.
// Nothing writes one any more and migration 61 cleared the ones that existed,
// so a value arriving here without the marker is a corrupt or hand-edited row —
// and accepting it would mean an attacker who can write to the column could
// choose a victim's TOTP seed by storing it unencrypted.
func (s *TwoFactorService) decodeSecret(stored string) (string, error) {
	if s.aead == nil {
		return "", ErrTwoFactorKeyMissing
	}
	if !strings.HasPrefix(stored, encSecretPrefix) {
		return "", errors.New("2FA secret is not in the encrypted format")
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

	// Try recovery codes.
	//
	// Matching is a bcrypt compare per stored hash, so the candidate must be
	// identified first; consumption is then delegated to an atomic conditional
	// UPDATE. Removing the entry in memory and writing the whole row back (the
	// previous approach) let concurrent submissions of the SAME code all see it
	// as present and all succeed -- ten parallel requests authenticated ten
	// times off one single-use code.
	code = strings.TrimSpace(strings.ToLower(code))
	for _, hashedCode := range user.RecoveryCodes {
		if hash.ComparePassword(hashedCode, code) != nil {
			continue
		}
		consumed, err := s.userRepo.ConsumeRecoveryCode(ctx, user.ID, hashedCode)
		if err != nil {
			return false, err
		}
		if !consumed {
			// Another request consumed this code first. Treat it as invalid so
			// a single-use code authenticates exactly once.
			break
		}
		s.resetFailedAttempts(ctx, user)
		return true, nil
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
