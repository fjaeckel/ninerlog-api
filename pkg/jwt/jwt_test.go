package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAndValidateAccessToken(t *testing.T) {
	manager := NewManager("secret", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	userID := uuid.New()

	token, err := manager.GenerateAccessToken(userID)
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

	token, err := manager.GenerateRefreshToken(userID)
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

	token, _ := manager.GenerateAccessToken(userID)
	time.Sleep(10 * time.Millisecond)

	_, err := manager.ValidateAccessToken(token)
	if err != ErrExpiredToken {
		t.Errorf("Expected ErrExpiredToken, got %v", err)
	}
}

func TestValidateWithWrongSecret(t *testing.T) {
	manager1 := NewManager("secret1", "refresh-secret", 15*time.Minute, 7*24*time.Hour)
	manager2 := NewManager("secret2", "refresh-secret", 15*time.Minute, 7*24*time.Hour)

	token, _ := manager1.GenerateAccessToken(uuid.New())

	_, err := manager2.ValidateAccessToken(token)
	if err == nil {
		t.Error("ValidateAccessToken() should fail with wrong secret")
	}
}
