package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// Defaults for idempotent-write bookkeeping. All three are overridable at
// startup (IDEMPOTENCY_TTL, IDEMPOTENCY_LEASE, IDEMPOTENCY_MAX_RESPONSE_BYTES).
const (
	// DefaultIdempotencyTTL is how long a key stays replayable.
	DefaultIdempotencyTTL = 24 * time.Hour

	// DefaultIdempotencyLease is how long an in-progress claim is honoured
	// before another request may take it over. Must exceed the request
	// timeout (15s).
	DefaultIdempotencyLease = 60 * time.Second

	// DefaultIdempotencyMaxResponseBytes caps what is stored per record.
	DefaultIdempotencyMaxResponseBytes = 256 << 10
)

// ErrIdempotencyUnavailable is returned when the replay store cannot be
// reached. Callers must fail the request rather than execute it.
var ErrIdempotencyUnavailable = errors.New("idempotency store unavailable")

// IdempotencyOutcome is the verdict on a claim attempt.
type IdempotencyOutcome int

const (
	// IdempotencyClaimed means the key is ours: run the request, then call
	// Finish or Abandon.
	IdempotencyClaimed IdempotencyOutcome = iota

	// IdempotencyReplay means the request already ran to completion; return
	// the stored response instead of executing anything.
	IdempotencyReplay

	// IdempotencyInProgress means an earlier request with this key is still
	// running. Retrying after it settles yields the replay.
	IdempotencyInProgress

	// IdempotencyMismatch means the key was used before with a different
	// request.
	IdempotencyMismatch

	// IdempotencyNotReplayable means the original request completed but its
	// response was not stored, so it can be neither replayed nor safely
	// re-executed.
	IdempotencyNotReplayable
)

// IdempotentResponse is a captured response, replayed verbatim on retry.
type IdempotentResponse struct {
	Status      int
	ContentType string
	Body        []byte
}

// IdempotencyClaim is the result of Begin.
type IdempotencyClaim struct {
	Outcome IdempotencyOutcome

	// ClaimedAt identifies this claim for Finish/Abandon. Set when the
	// outcome is IdempotencyClaimed.
	ClaimedAt time.Time

	// Response is set when the outcome is IdempotencyReplay.
	Response *IdempotentResponse
}

// IdempotencyService turns the raw replay records into the claim/finish
// lifecycle the HTTP layer needs, and owns their retention.
type IdempotencyService struct {
	repo             repository.IdempotencyRepository
	ttl              time.Duration
	lease            time.Duration
	maxResponseBytes int
	now              func() time.Time
}

// NewIdempotencyService creates the service. Non-positive values fall back to
// the defaults above.
func NewIdempotencyService(repo repository.IdempotencyRepository, ttl, lease time.Duration, maxResponseBytes int) *IdempotencyService {
	if ttl <= 0 {
		ttl = DefaultIdempotencyTTL
	}
	if lease <= 0 {
		lease = DefaultIdempotencyLease
	}
	if maxResponseBytes <= 0 {
		maxResponseBytes = DefaultIdempotencyMaxResponseBytes
	}
	return &IdempotencyService{
		repo:             repo,
		ttl:              ttl,
		lease:            lease,
		maxResponseBytes: maxResponseBytes,
		now:              func() time.Time { return time.Now().UTC() },
	}
}

// MaxResponseBytes is the largest response body the service will store.
func (s *IdempotencyService) MaxResponseBytes() int { return s.maxResponseBytes }

// Begin claims the key for this request, or reports why the request must not
// run. requestHash fingerprints the request; a key reused for a different
// payload is refused.
func (s *IdempotencyService) Begin(ctx context.Context, userID uuid.UUID, key string, requestHash []byte) (IdempotencyClaim, error) {
	// Truncate to Postgres's TIMESTAMPTZ microsecond precision; the value is
	// compared as a fencing token.
	now := s.now().Truncate(time.Microsecond)

	rec := &models.IdempotencyRecord{
		UserID:      userID,
		Key:         key,
		RequestHash: requestHash,
		CreatedAt:   now,
		ExpiresAt:   now.Add(s.ttl),
	}

	existing, err := s.repo.Claim(ctx, rec, now.Add(-s.lease))
	if err != nil {
		return IdempotencyClaim{}, fmt.Errorf("%w: %v", ErrIdempotencyUnavailable, err)
	}
	if existing == nil {
		return IdempotencyClaim{Outcome: IdempotencyClaimed, ClaimedAt: now}, nil
	}

	// The mismatch check comes before any replay.
	if !bytes.Equal(existing.RequestHash, requestHash) {
		return IdempotencyClaim{Outcome: IdempotencyMismatch}, nil
	}
	if existing.State == models.IdempotencyStateInProgress {
		return IdempotencyClaim{Outcome: IdempotencyInProgress}, nil
	}
	if !existing.Replayable() {
		return IdempotencyClaim{Outcome: IdempotencyNotReplayable}, nil
	}
	return IdempotencyClaim{
		Outcome: IdempotencyReplay,
		Response: &IdempotentResponse{
			Status:      *existing.ResponseStatus,
			ContentType: existing.ResponseContentType,
			Body:        existing.ResponseBody,
		},
	}, nil
}

// Finish stores the response against a claim so retries replay it.
//
// A response the caller could not capture (signalled with Status 0), or one
// larger than the cap, is recorded as completed with nothing to replay: the
// key stays consumed and a retry is told the response is gone.
func (s *IdempotencyService) Finish(ctx context.Context, userID uuid.UUID, key string, claimedAt time.Time, resp IdempotentResponse) error {
	completedAt := s.now().Truncate(time.Microsecond)
	rec := &models.IdempotencyRecord{
		UserID:      userID,
		Key:         key,
		CreatedAt:   claimedAt,
		State:       models.IdempotencyStateCompleted,
		CompletedAt: &completedAt,
	}
	if resp.Status > 0 && len(resp.Body) <= s.maxResponseBytes {
		status := resp.Status
		rec.ResponseStatus = &status
		rec.ResponseBody = resp.Body
		rec.ResponseContentType = resp.ContentType
	}
	return s.repo.Complete(ctx, rec)
}

// Abandon drops the claim, leaving the key free for a genuine retry. Used on
// a server error or a panic.
func (s *IdempotencyService) Abandon(ctx context.Context, userID uuid.UUID, key string, claimedAt time.Time) error {
	return s.repo.Release(ctx, userID, key, claimedAt)
}

// StartReaper deletes expired records on a timer until ctx is cancelled.
// Hygiene only: Begin takes over any record past its expires_at.
func (s *IdempotencyService) StartReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	go func() {
		slog.Info("Idempotency key reaper started", "interval", interval.String(), "ttl", s.ttl.String())
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("Idempotency key reaper stopped")
				return
			case <-ticker.C:
				n, err := s.repo.DeleteExpired(ctx, s.now())
				if err != nil {
					slog.Warn("Idempotency key cleanup failed", "error", err)
					continue
				}
				if n > 0 {
					slog.Debug("Expired idempotency keys removed", "count", n)
				}
			}
		}
	}()
}

// IdempotencyStatus maps an outcome to the HTTP status a client should see.
func IdempotencyStatus(o IdempotencyOutcome) int {
	switch o {
	case IdempotencyInProgress:
		return http.StatusConflict
	case IdempotencyMismatch:
		return http.StatusUnprocessableEntity
	case IdempotencyNotReplayable:
		return http.StatusConflict
	default:
		return http.StatusOK
	}
}
