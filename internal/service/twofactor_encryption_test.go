package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

func setup2FAServiceEncrypted(t *testing.T) (*service.TwoFactorService, *mock2FAUserRepo) {
	t.Helper()
	repo := newMock2FAUserRepo()
	jwtMgr := jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	return service.NewTwoFactorService(repo, jwtMgr, testTOTPAEAD(t)), repo
}

// testTOTPAEAD builds the 2FA cipher the way main does — a subkey derived from
// a master key — so the tests exercise the real key path rather than a raw key
// the production code never constructs.
func testTOTPAEAD(t *testing.T) *cryptoutil.AEAD {
	t.Helper()
	master, err := cryptoutil.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	aead, err := cryptoutil.DeriveAEAD(master, cryptoutil.PurposeTOTPSecrets)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	return aead
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

// An unprefixed secret is refused, not read as a legacy plaintext seed.
//
// Migration 61 cleared every enrolment that predates mandatory encryption, so
// nothing legitimate writes one. Accepting it would mean anyone who can write to
// the column — a restored dump, a stray admin query, SQL injection — could
// choose a victim's TOTP seed simply by storing it unencrypted, which is a far
// worse outcome than a failed login.
func TestEncrypted2FA_RefusesAnUnprefixedSecret(t *testing.T) {
	svc, repo := setup2FAServiceEncrypted(t)
	ctx := context.Background()
	user := createTestUserFor2FA(repo)

	key, err := totp.Generate(totp.GenerateOpts{Issuer: "NinerLog", AccountName: user.Email})
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	plain := key.Secret()
	user.TwoFactorEnabled = true
	user.TwoFactorSecret = &plain // stored WITHOUT the enc: prefix
	repo.users[user.ID] = user

	code, _ := totp.GenerateCode(plain, time.Now())
	valid, err := svc.ValidateTOTP(ctx, user.ID, code)
	if err == nil {
		t.Fatal("an unencrypted secret was accepted")
	}
	if valid {
		t.Error("ValidateTOTP reported success for an unencrypted secret")
	}
}

// Without a key, 2FA is unavailable rather than unencrypted. A running server
// cannot reach this — ENCRYPTION_KEY is required at startup — but the service
// must not have a mode that writes seeds in the clear.
func TestTwoFactor_WithoutAKeyRefusesRatherThanStoringPlaintext(t *testing.T) {
	repo := newMock2FAUserRepo()
	jwtMgr := jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	svc := service.NewTwoFactorService(repo, jwtMgr, nil)
	user := createTestUserFor2FA(repo)

	if _, _, err := svc.SetupTOTP(context.Background(), user.ID); !errors.Is(err, service.ErrTwoFactorKeyMissing) {
		t.Fatalf("SetupTOTP: err = %v, want ErrTwoFactorKeyMissing", err)
	}
	if repo.users[user.ID].TwoFactorSecret != nil {
		t.Error("a secret was stored despite the failure")
	}
}

func (m *mock2FAUserRepo) ConsumeRecoveryCode(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return true, nil
}
