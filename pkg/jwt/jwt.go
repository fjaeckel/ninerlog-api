package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
)

// Token types, embedded as a claim and verified on validation.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
	TokenType2FA     = "2fa"

	// subject2FAChallenge is the subject set on 2FA challenge tokens.
	subject2FAChallenge = "2fa-challenge"
)

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	// SessionID identifies the login the token belongs to. Zero on tokens
	// minted before sessions existed.
	SessionID uuid.UUID `json:"session_id,omitempty"`
	TokenType string    `json:"token_type,omitempty"`
	jwt.RegisteredClaims
}

type Manager struct {
	accessSecret       string
	refreshSecret      string
	accessTokenExpiry  time.Duration
	refreshTokenExpiry time.Duration
}

func NewManager(accessSecret, refreshSecret string, accessExpiry, refreshExpiry time.Duration) *Manager {
	return &Manager{
		accessSecret:       accessSecret,
		refreshSecret:      refreshSecret,
		accessTokenExpiry:  accessExpiry,
		refreshTokenExpiry: refreshExpiry,
	}
}

// GenerateAccessToken creates a new JWT access token bound to sessionID.
func (m *Manager) GenerateAccessToken(userID, sessionID uuid.UUID) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		SessionID: sessionID,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.accessSecret))
}

// GenerateRefreshToken creates a new JWT refresh token bound to sessionID.
func (m *Manager) GenerateRefreshToken(userID, sessionID uuid.UUID) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		SessionID: sessionID,
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.refreshSecret))
}

// ValidateAccessToken validates and parses an access token. 2FA challenge
// tokens are rejected.
func (m *Manager) ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := m.validateToken(tokenString, m.accessSecret)
	if err != nil {
		return nil, err
	}
	if claims.TokenType == TokenType2FA || claims.Subject == subject2FAChallenge {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ValidateRefreshToken validates and parses a refresh token
func (m *Manager) ValidateRefreshToken(tokenString string) (*Claims, error) {
	return m.validateToken(tokenString, m.refreshSecret)
}

func (m *Manager) validateToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (m *Manager) GetRefreshTokenExpiry() time.Duration {
	return m.refreshTokenExpiry
}

// Generate2FAToken creates a short-lived token for 2FA challenge (5 minutes)
func (m *Manager) Generate2FAToken(userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		TokenType: TokenType2FA,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "2fa-" + uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   subject2FAChallenge,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.accessSecret))
}

// Validate2FAToken validates a 2FA challenge token
func (m *Manager) Validate2FAToken(tokenString string) (*Claims, error) {
	claims, err := m.validateToken(tokenString, m.accessSecret)
	if err != nil {
		return nil, err
	}
	if claims.Subject != subject2FAChallenge {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
