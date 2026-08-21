package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAndValidateAccessToken(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	token, err := manager.GenerateAccessToken(userID, uuid.New())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	if token == "" {
		t.Error("GenerateAccessToken() returned empty token")
	}

	claims, err := manager.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error = %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("UserID = %v, want %v", claims.UserID, userID)
	}
}

func TestGenerateAndValidateRefreshToken(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	token, err := manager.GenerateRefreshToken(userID, uuid.New())
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}

	claims, err := manager.ValidateRefreshToken(token)
	if err != nil {
		t.Fatalf("ValidateRefreshToken() error = %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("UserID = %v, want %v", claims.UserID, userID)
	}
}

func TestValidateExpiredToken(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 1*time.Millisecond, 7*24*time.Hour)
	userID := uuid.New()

	token, _ := manager.GenerateAccessToken(userID, uuid.New())
	time.Sleep(10 * time.Millisecond)

	_, err := manager.ValidateAccessToken(token)
	if err != ErrExpiredToken {
		t.Errorf("Expected ErrExpiredToken, got %v", err)
	}
}

func TestValidateWithWrongSecret(t *testing.T) {
	manager1 := NewManager("secret1", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	manager2 := NewManager("secret2", "refresh-secret", 15*time.Minute, 7*24*time.Hour)

	token, _ := manager1.GenerateAccessToken(uuid.New(), uuid.New())

	_, err := manager2.ValidateAccessToken(token)
	if err == nil {
		t.Error("ValidateAccessToken() should fail with wrong secret")
	}
}

func TestGetRefreshTokenExpiry(t *testing.T) {
	expiry := 7 * 24 * time.Hour
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, expiry)
	if got := manager.GetRefreshTokenExpiry(); got != expiry {
		t.Errorf("GetRefreshTokenExpiry() = %v, want %v", got, expiry)
	}
}

func TestGenerate2FAToken(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	token, err := manager.Generate2FAToken(userID)
	if err != nil {
		t.Fatalf("Generate2FAToken() error = %v", err)
	}
	if token == "" {
		t.Error("Generate2FAToken() returned empty token")
	}

	claims, err := manager.Validate2FAToken(token)
	if err != nil {
		t.Fatalf("Validate2FAToken() error = %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("UserID = %v, want %v", claims.UserID, userID)
	}
	if claims.Subject != "2fa-challenge" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "2fa-challenge")
	}
}

func TestValidate2FAToken_RejectsRegularAccessToken(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	accessToken, _ := manager.GenerateAccessToken(userID, uuid.New())

	_, err := manager.Validate2FAToken(accessToken)
	if err != ErrInvalidToken {
		t.Errorf("Validate2FAToken(accessToken) error = %v, want ErrInvalidToken", err)
	}
}

func TestValidate2FAToken_Expired(t *testing.T) {
	// Asserts the 2FA token is signed with the access secret.
	manager1 := NewManager("secret1", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	manager2 := NewManager("secret2", "refresh-secret", 15*time.Minute, 7*24*time.Hour)

	token, _ := manager1.Generate2FAToken(uuid.New())

	_, err := manager2.Validate2FAToken(token)
	if err == nil {
		t.Error("Validate2FAToken() should fail with wrong secret")
	}
}

// TestValidateAccessToken_Rejects2FAToken asserts a 2FA challenge token is
// rejected on the access path.
func TestValidateAccessToken_Rejects2FAToken(t *testing.T) {
	manager := NewManager("access-secret-value", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	twoFAToken, err := manager.Generate2FAToken(userID)
	if err != nil {
		t.Fatalf("Generate2FAToken() error = %v", err)
	}

	if _, err := manager.ValidateAccessToken(twoFAToken); err != ErrInvalidToken {
		t.Errorf("ValidateAccessToken(2faToken) error = %v, want ErrInvalidToken (2FA bypass!)", err)
	}
}

func TestAccessToken_HasAccessTokenType(t *testing.T) {
	manager := NewManager("access-secret-value", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	token, _ := manager.GenerateAccessToken(uuid.New(), uuid.New())
	claims, err := manager.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error = %v", err)
	}
	if claims.TokenType != TokenTypeAccess {
		t.Errorf("access token TokenType = %q, want %q", claims.TokenType, TokenTypeAccess)
	}
}

func Test2FAToken_HasTwoFactorTokenType(t *testing.T) {
	manager := NewManager("access-secret-value", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	token, _ := manager.Generate2FAToken(uuid.New())
	claims, err := manager.Validate2FAToken(token)
	if err != nil {
		t.Fatalf("Validate2FAToken() error = %v", err)
	}
	if claims.TokenType != TokenType2FA {
		t.Errorf("2FA token TokenType = %q, want %q", claims.TokenType, TokenType2FA)
	}
}

func TestValidateAccessToken_MalformedToken(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)

	_, err := manager.ValidateAccessToken("not-a-jwt")
	if err != ErrInvalidToken {
		t.Errorf("ValidateAccessToken(malformed) error = %v, want ErrInvalidToken", err)
	}
}

func TestValidateRefreshToken_MalformedToken(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)

	_, err := manager.ValidateRefreshToken("not-a-jwt")
	if err != ErrInvalidToken {
		t.Errorf("ValidateRefreshToken(malformed) error = %v, want ErrInvalidToken", err)
	}
}

func TestAccessTokenCannotValidateAsRefresh(t *testing.T) {
	manager := NewManager("access-secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	accessToken, _ := manager.GenerateAccessToken(userID, uuid.New())

	_, err := manager.ValidateRefreshToken(accessToken)
	if err == nil {
		t.Error("Access token should not validate as refresh token")
	}
}

func TestRefreshTokenCannotValidateAsAccess(t *testing.T) {
	manager := NewManager("access-secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	refreshToken, _ := manager.GenerateRefreshToken(userID, uuid.New())

	_, err := manager.ValidateAccessToken(refreshToken)
	if err == nil {
		t.Error("Refresh token should not validate as access token")
	}
}

func TestTokensHaveUniqueJTI(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	token1, _ := manager.GenerateAccessToken(userID, uuid.New())
	token2, _ := manager.GenerateAccessToken(userID, uuid.New())

	claims1, _ := manager.ValidateAccessToken(token1)
	claims2, _ := manager.ValidateAccessToken(token2)

	if claims1.ID == claims2.ID {
		t.Error("Two tokens should have different JTI values")
	}
}

func Test2FATokenHas2FAPrefix(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	token, _ := manager.Generate2FAToken(userID)
	claims, _ := manager.Validate2FAToken(token)

	if !strings.HasPrefix(claims.ID, "2fa-") {
		t.Errorf("2FA token JTI = %q, want prefix '2fa-'", claims.ID)
	}
}
