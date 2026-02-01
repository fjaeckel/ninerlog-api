package testutil

import (
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/google/uuid"
)

func CreateTestUser(email, name, passwordHash string) *models.User {
	now := time.Now()
	return &models.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func CreateTestRefreshToken(userID uuid.UUID, tokenHash string) *models.RefreshToken {
	now := time.Now()
	return &models.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedAt: now,
		Revoked:   false,
	}
}

func CreateTestPasswordResetToken(userID uuid.UUID, tokenHash string) *models.PasswordResetToken {
	now := time.Now()
	return &models.PasswordResetToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(1 * time.Hour),
		CreatedAt: now,
		Used:      false,
	}
}
