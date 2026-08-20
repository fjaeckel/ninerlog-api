package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// fakeIdempotencyRepo is a single-record in-memory stand-in that reproduces
// the claim semantics of the SQL: a live record blocks the claim, an expired
// or abandoned one is taken over.
type fakeIdempotencyRepo struct {
	rec *models.IdempotencyRecord

	claimErr    error
	completeErr error
	releaseErr  error

	deletedBefore time.Time
	deleted       int64
}

func (f *fakeIdempotencyRepo) Claim(_ context.Context, rec *models.IdempotencyRecord, staleBefore time.Time) (*models.IdempotencyRecord, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	live := f.rec != nil &&
		f.rec.UserID == rec.UserID &&
		f.rec.Key == rec.Key &&
		f.rec.ExpiresAt.After(rec.CreatedAt) &&
		!(f.rec.State == models.IdempotencyStateInProgress && !f.rec.CreatedAt.After(staleBefore))
	if live {
		return f.rec, nil
	}
	stored := *rec
	stored.State = models.IdempotencyStateInProgress
	f.rec = &stored
	return nil, nil
}

func (f *fakeIdempotencyRepo) Complete(_ context.Context, rec *models.IdempotencyRecord) error {
	if f.completeErr != nil {
		return f.completeErr
	}
	if f.rec == nil || !f.rec.CreatedAt.Equal(rec.CreatedAt) {
		return nil // claim was taken over; a straggler must not clobber it
	}
	// Mirrors the UPDATE: only the response columns and state change; the
	// request fingerprint stays as it was claimed.
	f.rec.State = rec.State
	f.rec.ResponseStatus = rec.ResponseStatus
	f.rec.ResponseBody = rec.ResponseBody
	f.rec.ResponseContentType = rec.ResponseContentType
	f.rec.CompletedAt = rec.CompletedAt
	return nil
}

func (f *fakeIdempotencyRepo) Release(_ context.Context, userID uuid.UUID, key string, claimedAt time.Time) error {
	if f.releaseErr != nil {
		return f.releaseErr
	}
	if f.rec != nil && f.rec.UserID == userID && f.rec.Key == key && f.rec.CreatedAt.Equal(claimedAt) {
		f.rec = nil
	}
	return nil
}

func (f *fakeIdempotencyRepo) DeleteExpired(_ context.Context, before time.Time) (int64, error) {
	f.deletedBefore = before
	return f.deleted, nil
}

func newTestIdempotencyService(repo *fakeIdempotencyRepo) *IdempotencyService {
	return NewIdempotencyService(repo, time.Hour, time.Minute, 1024)
}

func TestIdempotencyService_Defaults(t *testing.T) {
	s := NewIdempotencyService(&fakeIdempotencyRepo{}, 0, 0, 0)
	if s.ttl != DefaultIdempotencyTTL {
		t.Errorf("ttl: want %v, got %v", DefaultIdempotencyTTL, s.ttl)
	}
	if s.lease != DefaultIdempotencyLease {
		t.Errorf("lease: want %v, got %v", DefaultIdempotencyLease, s.lease)
	}
	if s.MaxResponseBytes() != DefaultIdempotencyMaxResponseBytes {
		t.Errorf("maxResponseBytes: want %d, got %d", DefaultIdempotencyMaxResponseBytes, s.MaxResponseBytes())
	}
}

func TestIdempotencyService_ClaimAndReplay(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)
	ctx := context.Background()
	user := uuid.New()
	hash := []byte("request-fingerprint")

	claim, err := s.Begin(ctx, user, "key-1", hash)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if claim.Outcome != IdempotencyClaimed {
		t.Fatalf("first Begin: want IdempotencyClaimed, got %v", claim.Outcome)
	}

	// A duplicate arriving while the first is still running must not execute.
	inProgress, err := s.Begin(ctx, user, "key-1", hash)
	if err != nil {
		t.Fatalf("Begin (in progress): %v", err)
	}
	if inProgress.Outcome != IdempotencyInProgress {
		t.Fatalf("concurrent Begin: want IdempotencyInProgress, got %v", inProgress.Outcome)
	}

	body := []byte(`{"id":"abc"}`)
	if err := s.Finish(ctx, user, "key-1", claim.ClaimedAt, IdempotentResponse{
		Status:      http.StatusCreated,
		ContentType: "application/json; charset=utf-8",
		Body:        body,
	}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	replay, err := s.Begin(ctx, user, "key-1", hash)
	if err != nil {
		t.Fatalf("Begin (replay): %v", err)
	}
	if replay.Outcome != IdempotencyReplay {
		t.Fatalf("replay Begin: want IdempotencyReplay, got %v", replay.Outcome)
	}
	if replay.Response.Status != http.StatusCreated {
		t.Errorf("replay status: want 201, got %d", replay.Response.Status)
	}
	if string(replay.Response.Body) != string(body) {
		t.Errorf("replay body: want %q, got %q", body, replay.Response.Body)
	}
	if replay.Response.ContentType != "application/json; charset=utf-8" {
		t.Errorf("replay content type: got %q", replay.Response.ContentType)
	}
}

func TestIdempotencyService_ScopedPerUserAndKey(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)
	ctx := context.Background()
	hash := []byte("fingerprint")

	first, err := s.Begin(ctx, uuid.New(), "shared-key", hash)
	if err != nil || first.Outcome != IdempotencyClaimed {
		t.Fatalf("first claim: %v / %v", first.Outcome, err)
	}
	// Same key, different user: must not collide with the first user's record.
	second, err := s.Begin(ctx, uuid.New(), "shared-key", hash)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if second.Outcome != IdempotencyClaimed {
		t.Errorf("other user's identical key: want IdempotencyClaimed, got %v", second.Outcome)
	}
}

func TestIdempotencyService_MismatchedFingerprint(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)
	ctx := context.Background()
	user := uuid.New()

	claim, err := s.Begin(ctx, user, "key-1", []byte("first-body"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := s.Finish(ctx, user, "key-1", claim.ClaimedAt, IdempotentResponse{
		Status: http.StatusCreated, Body: []byte("{}"),
	}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Reusing the key for a different payload must be refused, not answered
	// with the first request's response.
	got, err := s.Begin(ctx, user, "key-1", []byte("second-body"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if got.Outcome != IdempotencyMismatch {
		t.Fatalf("want IdempotencyMismatch, got %v", got.Outcome)
	}
	if got.Response != nil {
		t.Error("mismatch must not leak the stored response")
	}
	if s := IdempotencyStatus(got.Outcome); s != http.StatusUnprocessableEntity {
		t.Errorf("mismatch status: want 422, got %d", s)
	}
}

func TestIdempotencyService_MismatchWinsOverInProgress(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)
	ctx := context.Background()
	user := uuid.New()

	if _, err := s.Begin(ctx, user, "key-1", []byte("first-body")); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	got, err := s.Begin(ctx, user, "key-1", []byte("other-body"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if got.Outcome != IdempotencyMismatch {
		t.Fatalf("want IdempotencyMismatch, got %v", got.Outcome)
	}
}

func TestIdempotencyService_AbandonFreesTheKey(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)
	ctx := context.Background()
	user := uuid.New()

	claim, err := s.Begin(ctx, user, "key-1", []byte("body"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := s.Abandon(ctx, user, "key-1", claim.ClaimedAt); err != nil {
		t.Fatalf("Abandon: %v", err)
	}

	again, err := s.Begin(ctx, user, "key-1", []byte("body"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if again.Outcome != IdempotencyClaimed {
		t.Fatalf("after Abandon: want IdempotencyClaimed, got %v", again.Outcome)
	}
}

func TestIdempotencyService_OversizedResponseIsNotReplayable(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := NewIdempotencyService(repo, time.Hour, time.Minute, 8)
	ctx := context.Background()
	user := uuid.New()
	hash := []byte("body")

	claim, err := s.Begin(ctx, user, "key-1", hash)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := s.Finish(ctx, user, "key-1", claim.ClaimedAt, IdempotentResponse{
		Status: http.StatusCreated,
		Body:   []byte("a response well over the eight byte cap"),
	}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got, err := s.Begin(ctx, user, "key-1", hash)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// A completed claim without a stored response must be refused, not
	// re-executed.
	if got.Outcome != IdempotencyNotReplayable {
		t.Fatalf("want IdempotencyNotReplayable, got %v", got.Outcome)
	}
	if s := IdempotencyStatus(got.Outcome); s != http.StatusConflict {
		t.Errorf("status: want 409, got %d", s)
	}
}

func TestIdempotencyService_UncapturedResponseIsNotReplayable(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)
	ctx := context.Background()
	user := uuid.New()

	claim, err := s.Begin(ctx, user, "key-1", []byte("body"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// Status 0 is the caller's signal for "response could not be captured".
	if err := s.Finish(ctx, user, "key-1", claim.ClaimedAt, IdempotentResponse{}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got, err := s.Begin(ctx, user, "key-1", []byte("body"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if got.Outcome != IdempotencyNotReplayable {
		t.Fatalf("want IdempotencyNotReplayable, got %v", got.Outcome)
	}
}

func TestIdempotencyService_AbandonedClaimIsTakenOver(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)
	ctx := context.Background()
	user := uuid.New()
	hash := []byte("body")

	if _, err := s.Begin(ctx, user, "key-1", hash); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// A process that dies mid-request must not wedge the key for the whole
	// retention window — only for the lease.
	s.now = func() time.Time { return time.Now().UTC().Add(2 * time.Minute) }

	got, err := s.Begin(ctx, user, "key-1", hash)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if got.Outcome != IdempotencyClaimed {
		t.Fatalf("after lease expiry: want IdempotencyClaimed, got %v", got.Outcome)
	}
}

func TestIdempotencyService_ExpiredRecordIsTakenOver(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)
	ctx := context.Background()
	user := uuid.New()
	hash := []byte("body")

	claim, err := s.Begin(ctx, user, "key-1", hash)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := s.Finish(ctx, user, "key-1", claim.ClaimedAt, IdempotentResponse{
		Status: http.StatusCreated, Body: []byte("{}"),
	}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	s.now = func() time.Time { return time.Now().UTC().Add(2 * time.Hour) }

	got, err := s.Begin(ctx, user, "key-1", hash)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if got.Outcome != IdempotencyClaimed {
		t.Fatalf("after TTL: want IdempotencyClaimed, got %v", got.Outcome)
	}
}

func TestIdempotencyService_StoreFailureIsReported(t *testing.T) {
	repo := &fakeIdempotencyRepo{claimErr: errors.New("connection refused")}
	s := newTestIdempotencyService(repo)

	_, err := s.Begin(context.Background(), uuid.New(), "key-1", []byte("body"))
	if !errors.Is(err, ErrIdempotencyUnavailable) {
		t.Fatalf("want ErrIdempotencyUnavailable, got %v", err)
	}
}

func TestIdempotencyService_ClaimedAtIsMicrosecondPrecision(t *testing.T) {
	repo := &fakeIdempotencyRepo{}
	s := newTestIdempotencyService(repo)

	claim, err := s.Begin(context.Background(), uuid.New(), "key-1", []byte("body"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// The claim timestamp is a fencing token compared against a TIMESTAMPTZ,
	// which Postgres stores at microsecond precision.
	if claim.ClaimedAt.Nanosecond()%1000 != 0 {
		t.Errorf("ClaimedAt %v carries sub-microsecond precision", claim.ClaimedAt)
	}
}
