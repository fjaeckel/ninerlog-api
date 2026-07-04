package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/pquerna/otp/totp"
)

// After enough wrong codes the account locks, and subsequent attempts are
// rejected with ErrAccountLocked even when the code is correct.
func TestValidateTOTP_LocksAfterRepeatedFailures(t *testing.T) {
	svc, repo := setup2FAService()
	ctx := context.Background()
	user := createTestUserFor2FA(repo)

	secret, _, err := svc.SetupTOTP(ctx, user.ID)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	if _, err := svc.VerifyAndEnable(ctx, user.ID, code); err != nil {
		t.Fatalf("VerifyAndEnable: %v", err)
	}

	realCode, _ := totp.GenerateCode(secret, time.Now())
	wrong := "000000"
	if wrong == realCode {
		wrong = "111111"
	}

	// 5 failed attempts (maxFailedLoginAttempts) trip the lockout.
	for i := 0; i < 5; i++ {
		valid, err := svc.ValidateTOTP(ctx, user.ID, wrong)
		if valid {
			t.Fatalf("attempt %d: wrong code unexpectedly accepted", i+1)
		}
		if err != nil && !errors.Is(err, service.ErrAccountLocked) {
			t.Fatalf("attempt %d: unexpected error %v", i+1, err)
		}
	}

	// A correct code must now be refused because the account is locked.
	fresh, _ := totp.GenerateCode(secret, time.Now())
	valid, err := svc.ValidateTOTP(ctx, user.ID, fresh)
	if valid {
		t.Error("valid code accepted on a locked account")
	}
	if !errors.Is(err, service.ErrAccountLocked) {
		t.Errorf("expected ErrAccountLocked, got %v", err)
	}
}

// A successful validation before the threshold clears the failure counter, so
// intermittent typos don't accumulate into a lockout.
func TestValidateTOTP_SuccessResetsFailureCounter(t *testing.T) {
	svc, repo := setup2FAService()
	ctx := context.Background()
	user := createTestUserFor2FA(repo)

	secret, _, _ := svc.SetupTOTP(ctx, user.ID)
	code, _ := totp.GenerateCode(secret, time.Now())
	if _, err := svc.VerifyAndEnable(ctx, user.ID, code); err != nil {
		t.Fatalf("VerifyAndEnable: %v", err)
	}

	// A few failures, then a success.
	for i := 0; i < 3; i++ {
		_, _ = svc.ValidateTOTP(ctx, user.ID, "000000")
	}
	if repo.users[user.ID].FailedLoginAttempts == 0 {
		t.Fatal("precondition: expected some recorded failures")
	}

	good, _ := totp.GenerateCode(secret, time.Now())
	valid, err := svc.ValidateTOTP(ctx, user.ID, good)
	if err != nil || !valid {
		t.Fatalf("valid code should succeed: valid=%v err=%v", valid, err)
	}
	if repo.users[user.ID].FailedLoginAttempts != 0 {
		t.Errorf("failure counter not reset after success: %d", repo.users[user.ID].FailedLoginAttempts)
	}
}
