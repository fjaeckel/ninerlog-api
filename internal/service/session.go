package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// Session policy defaults, applied when SessionPolicy carries a non-positive
// value.
const (
	DefaultMaxSessionsPerUser = 5
	DefaultRefreshReuseGrace  = 30 * time.Second
)

// SessionPolicy bounds how many sessions a user may hold and how long a
// just-rotated refresh token keeps working.
type SessionPolicy struct {
	// MaxPerUser is the number of concurrent sessions kept per user. A login
	// beyond it evicts the least recently used session rather than failing.
	MaxPerUser int
	// ReuseGrace is the window after rotation in which the superseded refresh
	// token is still accepted. Past it, presenting one revokes the session.
	ReuseGrace time.Duration
}

// normalized returns the policy with non-positive fields replaced by defaults.
func (p SessionPolicy) normalized() SessionPolicy {
	if p.MaxPerUser <= 0 {
		p.MaxPerUser = DefaultMaxSessionsPerUser
	}
	if p.ReuseGrace <= 0 {
		p.ReuseGrace = DefaultRefreshReuseGrace
	}
	return p
}

// ListSessions returns the user's live sessions, most recently used first.
// The session matching current is flagged; pass uuid.Nil to flag none.
func (s *AuthService) ListSessions(ctx context.Context, userID, current uuid.UUID) ([]*models.Session, error) {
	sessions, err := s.refreshTokenRepo.ListSessions(ctx, userID)
	if err != nil {
		return nil, err
	}

	for _, sess := range sessions {
		if current != uuid.Nil && sess.ID == current {
			sess.Current = true
		}
	}

	return sessions, nil
}

// RevokeSession ends one of the user's sessions. Returns ErrSessionNotFound
// when the session is already gone or belongs to somebody else.
func (s *AuthService) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	if err := s.refreshTokenRepo.RevokeSession(ctx, userID, sessionID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSessionNotFound
		}
		return err
	}
	return nil
}

// RevokeOtherSessions ends every session the user holds except keep, and
// returns how many were ended.
func (s *AuthService) RevokeOtherSessions(ctx context.Context, userID, keep uuid.UUID) (int64, error) {
	return s.refreshTokenRepo.RevokeSessionsExcept(ctx, userID, keep)
}

// startSession mints a token pair on a fresh session, evicting the user's
// least recently used sessions to stay within the policy.
func (s *AuthService) startSession(ctx context.Context, userID uuid.UUID) (*TokenPair, error) {
	policy := s.sessionPolicy.normalized()

	evicted, err := s.refreshTokenRepo.EvictOldestSessions(ctx, userID, policy.MaxPerUser-1)
	if err != nil {
		slog.Warn("failed to evict oldest sessions", "user_id", userID, "error", err)
	} else if evicted > 0 {
		SessionsEvictedTotal.Add(float64(evicted))
	}

	return s.generateTokenPair(ctx, userID, uuid.New(), DeviceFromContext(ctx))
}

// deviceForRotation prefers the calling client's details and falls back to the
// ones recorded on the token being rotated.
func deviceForRotation(ctx context.Context, previous *models.RefreshToken) DeviceInfo {
	info := DeviceFromContext(ctx)
	if info.UserAgent == "" {
		info.UserAgent = previous.UserAgent
	}
	if info.IPAddress == "" {
		info.IPAddress = previous.IPAddress
	}
	return info
}
