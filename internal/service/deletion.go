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

const (
	// DefaultTombstoneRetention bounds how long a deletion stays reportable.
	// Ninety days is long enough that a client offline for a season still
	// resyncs incrementally, and short enough that the table stays small after
	// a bulk delete.
	DefaultTombstoneRetention = 90 * 24 * time.Hour

	// DefaultDeletionPageSize / MaxDeletionPageSize bound the feed. A
	// delete-all writes one tombstone per flight, so this endpoint must page.
	DefaultDeletionPageSize = 100
	MaxDeletionPageSize     = 500
)

// ErrInvalidDeletionEntity is returned for an entity filter outside the enum.
// Answering with an empty feed instead would read to a sync client as "nothing
// was deleted", which is the one answer that must never be wrong.
var ErrInvalidDeletionEntity = errors.New("unknown deletion entity type")

// DeletionService serves the tombstone feed and sweeps it.
type DeletionService struct {
	repo      repository.DeletionRepository
	retention time.Duration

	// now is overridable so tests can pin the retention horizon.
	now func() time.Time
}

func NewDeletionService(repo repository.DeletionRepository, retention time.Duration) *DeletionService {
	if retention <= 0 {
		retention = DefaultTombstoneRetention
	}
	return &DeletionService{repo: repo, retention: retention, now: time.Now}
}

// Retention is how long a tombstone is kept, reported to clients so they can
// schedule their fallback reconciliation.
func (s *DeletionService) Retention() time.Duration { return s.retention }

// DeletionPage is one page of the feed plus the two things a client needs in
// order to trust it: how many deletions match, and whether its watermark is old
// enough that the answer may be incomplete.
type DeletionPage struct {
	Deletions []*models.Deletion
	Total     int

	// WatermarkExpired is true when `since` predates the retention horizon.
	// Tombstones older than the horizon have been swept, so the feed is no
	// longer a complete account of what was deleted and the client must fall
	// back to a full ID-set reconciliation.
	WatermarkExpired bool
}

// ListDeletions returns the deletions recorded for the user after `since`.
//
// The result is always user-scoped: a tombstone carries only an id and a type,
// but leaking another pilot's deletion timings would still be a disclosure, so
// the filter is applied in SQL rather than after the fact.
func (s *DeletionService) ListDeletions(
	ctx context.Context, userID uuid.UUID, since time.Time,
	entity *models.DeletionEntityType, page, pageSize int,
) (*DeletionPage, error) {
	if entity != nil && !entity.IsValid() {
		return nil, ErrInvalidDeletionEntity
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultDeletionPageSize
	}
	if pageSize > MaxDeletionPageSize {
		pageSize = MaxDeletionPageSize
	}

	total, err := s.repo.CountSince(ctx, userID, since, entity)
	if err != nil {
		return nil, err
	}

	deletions, err := s.repo.ListSince(ctx, userID, since, entity, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	if deletions == nil {
		deletions = []*models.Deletion{}
	}

	return &DeletionPage{
		Deletions:        deletions,
		Total:            total,
		WatermarkExpired: since.Before(s.now().Add(-s.retention)),
	}, nil
}

// StartReaper sweeps tombstones past the retention horizon. Unlike the
// idempotency reaper this is not purely hygiene: retention is part of the
// contract, and a client is told its watermark expired based on the same
// horizon the sweeper enforces.
func (s *DeletionService) StartReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	go func() {
		slog.Info("Deletion tombstone reaper started",
			"interval", interval.String(), "retention", s.retention.String())
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("Deletion tombstone reaper stopped")
				return
			case <-ticker.C:
				n, err := s.repo.DeleteExpired(ctx, s.now().Add(-s.retention))
				if err != nil {
					slog.Warn("Deletion tombstone cleanup failed", "error", err)
					continue
				}
				if n > 0 {
					slog.Debug("Expired deletion tombstones removed", "count", n)
				}
			}
		}
	}()
}
