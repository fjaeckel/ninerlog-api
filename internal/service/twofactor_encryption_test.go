package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/pquerna/otp/totp"
)

func setup2FAServiceEncrypted(t *testing.T) (*service.TwoFactorService, *mock2FAUserRepo) {
	t.Helper()
	repo := newMock2FAUserRepo()
	jwtMgr := jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	key, err := cryptoutil.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	aead, err := cryptoutil.New(key)
	if err != nil {
		t.Fatalf("new aead: %v", err)
	}
	return service.NewTwoFactorService(repo, jwtMgr, aead), repo
}

// The stored TOTP secret must be ciphertext, not the plaintext base32 secret.
func TestSetupTOTP_EncryptsSecretAtRest(t *testing.T) {
	svc, repo := setup2FAServiceEncrypted(t)
	ctx := context.Background()
	user := createTestUserFor2FA(repo)

	plaintextSecret, _, err := svc.SetupTOTP(ctx, user.ID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}

	stored := repo.users[user.ID].TwoFactorSecret
	if stored == nil {
		t.Fatal("no secret stored")
	}
	if !strings.HasPrefix(*stored, "enc:v1:") {
		t.Errorf("stored secret is not encrypted: %q", *stored)
	}
	if strings.Contains(*stored, plaintextSecret) {
		t.Error("plaintext TOTP secret is present in the stored value")
	}
}

// End-to-end: an encrypted secret is transparently decrypted for both the
// enable step and later validation.
func TestEncrypted2FA_VerifyAndValidateRoundTrip(t *testing.T) {
	svc, repo := setup2FAServiceEncrypted(t)
	ctx := context.Background()
	user := createTestUserFor2FA(repo)

	secret, _, err := svc.SetupTOTP(ctx, user.ID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}

	code, _ := totp.GenerateCode(secret, time.Now())
	recovery, err := svc.VerifyAndEnable(ctx, user.ID, code)
	if err != nil {
		t.Fatalf("VerifyAndEnable with encrypted secret: %v", err)
	}
	if len(recovery) == 0 {
		t.Error("expected recovery codes")
	}

	fresh, _ := totp.GenerateCode(secret, time.Now())
	valid, err := svc.ValidateTOTP(ctx, user.ID, fresh)
	if err != nil {
		t.Fatalf("ValidateTOTP with encrypted secret: %v", err)
	}
	if !valid {
		t.Error("valid TOTP code rejected for encrypted secret")
	}
}

// Backward compatibility: an encryption-enabled service must still validate a
// legacy plaintext secret written before encryption was introduced.
func TestEncrypted2FA_ReadsLegacyPlaintextSecret(t *testing.T) {
	svc, repo := setup2FAServiceEncrypted(t)
	ctx := context.Background()
	user := createTestUserFor2FA(repo)

	key, err := totp.Generate(totp.GenerateOpts{Issuer: "NinerLog", AccountName: user.Email})
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	plain := key.Secret()
	user.TwoFactorEnabled = true
	user.TwoFactorSecret = &plain // stored WITHOUT the enc: prefix (legacy)
	repo.users[user.ID] = user

	code, _ := totp.GenerateCode(plain, time.Now())
	valid, err := svc.ValidateTOTP(ctx, user.ID, code)
	if err != nil {
		t.Fatalf("ValidateTOTP legacy plaintext: %v", err)
	}
	if !valid {
		t.Error("legacy plaintext secret should still validate")
	}
}
