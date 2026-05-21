package cloudbackup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/google/uuid"
)

// RunRequest describes a single backup execution.
type RunRequest struct {
	DestinationID uuid.UUID
	// UserID must match the destination's owner; the runner re-checks this
	// to guarantee scheduler bugs cannot leak data across tenants.
	UserID  uuid.UUID
	Trigger models.BackupRunTrigger
}

// runLocks serializes concurrent runs against the same destination. The
// runner uses tryLock semantics so that a manual "Run now" while another
// run is in progress fails fast with ErrConcurrentRun rather than queueing.
type runLocks struct {
	mu       sync.Mutex
	inflight map[uuid.UUID]struct{}
}

func newRunLocks() *runLocks {
	return &runLocks{inflight: make(map[uuid.UUID]struct{})}
}

func (l *runLocks) tryAcquire(id uuid.UUID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, in := l.inflight[id]; in {
		return false
	}
	l.inflight[id] = struct{}{}
	return true
}

func (l *runLocks) release(id uuid.UUID) {
	l.mu.Lock()
	delete(l.inflight, id)
	l.mu.Unlock()
}

// ensureRunLocks lazily initialises the per-destination lock map. This is a
// method on Service so we can keep state across RunOnce calls.
func (s *Service) ensureRunLocks() {
	if s.runLocks == nil {
		s.runLocks = newRunLocks()
	}
}

// RunOnce executes a single backup and records the result. It is safe to call
// concurrently; the second concurrent call against the same destination
// returns ErrConcurrentRun.
//
// The high-level flow is:
//  1. Look up & authorise the destination.
//  2. Build the JSON payload.
//  3. Optionally short-circuit ("skipped") if the payload hash matches the
//     last successful run.
//  4. Upload via the provider.
//  5. Prune old objects to honour RetentionCount.
//  6. Persist a BackupRun row and update the destination's status.
//
// Errors at any step are recorded as a "failed" BackupRun with a sanitised
// message; the function returns the original error so callers can surface it.
func (s *Service) RunOnce(ctx context.Context, req RunRequest) (*models.BackupRun, error) {
	s.ensureRunLocks()

	d, err := s.requireOwned(ctx, req.DestinationID, req.UserID)
	if err != nil {
		return nil, err
	}
	p, err := s.Provider(d.Provider)
	if err != nil {
		return nil, err
	}

	if !s.runLocks.tryAcquire(d.ID) {
		return nil, ErrConcurrentRun
	}
	defer s.runLocks.release(d.ID)

	startedAt := s.clock()
	run := &models.BackupRun{
		DestinationID: d.ID,
		UserID:        d.UserID,
		Status:        models.BackupRunStatusFailed, // pessimistic; flip on success
		Trigger:       req.Trigger,
		StartedAt:     startedAt,
	}
	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("create run record: %w", err)
	}

	// helper that completes the run with the supplied status/message and
	// updates the destination accordingly.
	finalize := func(status models.BackupRunStatus, runErr error) (*models.BackupRun, error) {
		now := s.clock()
		duration := int(now.Sub(startedAt) / time.Millisecond)
		run.Status = status
		run.CompletedAt = &now
		run.DurationMs = &duration
		if runErr != nil {
			run.ErrorMessage = sanitizeMessage(runErr)
		}
		if err := s.runRepo.Update(ctx, run); err != nil {
			return run, fmt.Errorf("update run record: %w", err)
		}

		switch status {
		case models.BackupRunStatusSuccess, models.BackupRunStatusSkipped:
			s.recordSuccess(ctx, d, run.SHA256)
		case models.BackupRunStatusFailed:
			s.recordError(ctx, d, runErr)
		}
		return run, runErr
	}

	creds, err := s.decryptCredentials(d)
	if err != nil {
		return finalize(models.BackupRunStatusFailed, err)
	}

	reader, meta, err := s.jsonBldr.BuildJSON(ctx, d.UserID)
	if err != nil {
		return finalize(models.BackupRunStatusFailed, err)
	}
	defer reader.Close()

	// Snapshot counts immediately so we record them even on upload failure.
	run.SHA256 = meta.SHA256
	run.SizeBytes = &meta.SizeBytes
	run.FlightCount = ptrInt(meta.FlightCount)
	run.AircraftCount = ptrInt(meta.AircraftCount)
	run.LicenseCount = ptrInt(meta.LicenseCount)
	run.CredentialCount = ptrInt(meta.CredentialCount)

	// Skip-if-unchanged: if the previous *successful* run produced the same
	// hash, we record a "skipped" run without touching the remote store.
	if d.LastSuccessSHA256 != "" && d.LastSuccessSHA256 == meta.SHA256 {
		return finalize(models.BackupRunStatusSkipped, nil)
	}

	// Read the body into memory once so the provider can use Content-Length;
	// the gzipped JSON is small (kBs) so this is cheap and simpler than
	// computing a teeing reader twice.
	body, err := io.ReadAll(reader)
	if err != nil {
		return finalize(models.BackupRunStatusFailed, err)
	}

	uploadIn := provider.UploadInput{
		Reader:      bytes.NewReader(body),
		Size:        int64(len(body)),
		Filename:    meta.Filename,
		ContentType: meta.ContentType,
	}
	cfg := provider.Config(d.Config)
	res, err := p.Upload(ctx, cfg, creds, uploadIn)
	if err != nil {
		return finalize(models.BackupRunStatusFailed, err)
	}
	run.RemotePath = res.RemotePath

	// Retention pruning runs best-effort: a failure here does NOT fail the
	// run (the backup itself succeeded), but it does get logged in the
	// destination's last_error field.
	if d.RetentionCount > 0 {
		if err := s.prune(ctx, d, p, cfg, creds); err != nil {
			d.LastError = sanitizeMessage(err)
		}
	}

	return finalize(models.BackupRunStatusSuccess, nil)
}

// prune deletes the oldest backups so that at most d.RetentionCount remain.
func (s *Service) prune(ctx context.Context, d *models.BackupDestination, p provider.Provider, cfg provider.Config, creds provider.Credentials) error {
	objects, err := p.List(ctx, cfg, creds)
	if err != nil {
		return fmt.Errorf("list for retention: %w", err)
	}
	if len(objects) <= d.RetentionCount {
		return nil
	}
	// Newest first.
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].LastModified.After(objects[j].LastModified)
	})
	toDelete := objects[d.RetentionCount:]
	var firstErr error
	for _, obj := range toDelete {
		if err := p.Delete(ctx, cfg, creds, obj.Path); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func ptrInt(v int) *int { return &v }

// ListRuns returns audit-log rows for one destination scoped to the user.
func (s *Service) ListRuns(ctx context.Context, destinationID, userID uuid.UUID, page, pageSize int) ([]*models.BackupRun, int, error) {
	if _, err := s.requireOwned(ctx, destinationID, userID); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize
	return s.runRepo.GetByDestinationID(ctx, destinationID, pageSize, offset)
}

// GetRun returns a single run record, scoped to the user.
func (s *Service) GetRun(ctx context.Context, runID, userID uuid.UUID) (*models.BackupRun, error) {
	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDestinationNotFound
		}
		return nil, err
	}
	if run.UserID != userID {
		return nil, ErrUnauthorized
	}
	return run, nil
}
