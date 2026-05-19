// Package cloudbackup is the user-facing service for cloud backup
// destinations. It owns:
//   - building the JSON backup payload (a stable, ordered representation of
//     the pilot's data);
//   - CRUD over backup destinations (with AES-256-GCM encryption of
//     credentials at rest);
//   - executing one-off backup runs (BuildJSON → upload → retention prune →
//     audit log);
//   - a process-local scheduler that ticks every minute and queues due
//     destinations.
//
// The package depends only on the repository layer and the provider plugin
// registry. HTTP handlers, JWT, and Gin live elsewhere.
package cloudbackup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/google/uuid"
)

// Errors returned by the service layer. These map cleanly onto HTTP status
// codes in the handler layer.
var (
	ErrDestinationNotFound = errors.New("backup destination not found")
	ErrProviderUnknown     = errors.New("unknown backup provider")
	ErrInvalidSchedule     = errors.New("invalid schedule")
	ErrUnauthorized        = errors.New("not authorized for this destination")
	ErrConcurrentRun       = errors.New("a backup is already running for this destination")
)

// Service is the cloud backup application service. Construct via New.
type Service struct {
	destRepo  repository.BackupDestinationRepository
	runRepo   repository.BackupRunRepository
	registry  *provider.Registry
	crypto    *cryptoutil.AEAD
	jsonBldr  JSONBuilder
	clock     func() time.Time
	scheduler *Scheduler
	runLocks  *runLocks
}

// JSONBuilder is implemented by anything that can produce the data half of a
// backup. The default implementation queries the existing flight / aircraft /
// license / credential services.
type JSONBuilder interface {
	BuildJSON(ctx context.Context, userID uuid.UUID) (io.ReadCloser, BuildMetadata, error)
}

// BuildMetadata accompanies a generated backup payload. The counts feed the
// BackupRun audit record and the SHA-256 powers the "skip if unchanged"
// optimization.
type BuildMetadata struct {
	SHA256          string
	SizeBytes       int64
	FlightCount     int
	AircraftCount   int
	LicenseCount    int
	CredentialCount int
	// ContentType describes the bytes returned by BuildJSON. Currently always
	// "application/gzip" — JSON wrapped in gzip — but we expose it explicitly
	// so the provider can set the correct Content-Type header.
	ContentType string
	// Filename is a recommended object name, e.g. "ninerlog-backup-2025-...".
	Filename string
}

// Options bundles the dependencies for New.
type Options struct {
	DestinationRepo repository.BackupDestinationRepository
	RunRepo         repository.BackupRunRepository
	Registry        *provider.Registry
	Crypto          *cryptoutil.AEAD
	Builder         JSONBuilder
	// Clock is the function used everywhere "now" is needed. Defaults to
	// time.Now.UTC.
	Clock func() time.Time
}

// New constructs a Service. Returns an error if any required dependency is
// missing.
func New(opts Options) (*Service, error) {
	if opts.DestinationRepo == nil {
		return nil, errors.New("cloudbackup: DestinationRepo required")
	}
	if opts.RunRepo == nil {
		return nil, errors.New("cloudbackup: RunRepo required")
	}
	if opts.Registry == nil {
		return nil, errors.New("cloudbackup: Registry required")
	}
	if opts.Crypto == nil {
		return nil, errors.New("cloudbackup: Crypto required")
	}
	if opts.Builder == nil {
		return nil, errors.New("cloudbackup: Builder required")
	}
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	s := &Service{
		destRepo: opts.DestinationRepo,
		runRepo:  opts.RunRepo,
		registry: opts.Registry,
		crypto:   opts.Crypto,
		jsonBldr: opts.Builder,
		clock:    clock,
	}
	return s, nil
}

// Provider returns the registered provider by name, or ErrProviderUnknown.
func (s *Service) Provider(name string) (provider.Provider, error) {
	p, ok := s.registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProviderUnknown, name)
	}
	return p, nil
}

// ListProviders returns all registered providers, sorted by name.
func (s *Service) ListProviders() []provider.Provider {
	return s.registry.All()
}

// requireOwned fetches a destination and verifies it belongs to userID.
func (s *Service) requireOwned(ctx context.Context, destinationID, userID uuid.UUID) (*models.BackupDestination, error) {
	d, err := s.destRepo.GetByID(ctx, destinationID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDestinationNotFound
		}
		return nil, err
	}
	if d.UserID != userID {
		return nil, ErrUnauthorized
	}
	return d, nil
}

// ListDestinations returns all destinations owned by the user.
func (s *Service) ListDestinations(ctx context.Context, userID uuid.UUID) ([]*models.BackupDestination, error) {
	return s.destRepo.GetByUserID(ctx, userID)
}

// GetDestination returns one destination by ID, scoped to the user.
func (s *Service) GetDestination(ctx context.Context, destinationID, userID uuid.UUID) (*models.BackupDestination, error) {
	return s.requireOwned(ctx, destinationID, userID)
}

// validateSchedule applies sanity checks on schedule shape. Returns
// ErrInvalidSchedule on bad input.
func validateSchedule(sched models.BackupSchedule, hour int, dow, dom *int) error {
	if !sched.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidSchedule, sched)
	}
	if hour < 0 || hour > 23 {
		return fmt.Errorf("%w: hour out of range", ErrInvalidSchedule)
	}
	switch sched {
	case models.BackupScheduleWeekly:
		if dow == nil || *dow < 0 || *dow > 6 {
			return fmt.Errorf("%w: scheduleDayOfWeek required for weekly schedule (0-6)", ErrInvalidSchedule)
		}
	case models.BackupScheduleMonthly:
		if dom == nil || *dom < 1 || *dom > 28 {
			return fmt.Errorf("%w: scheduleDayOfMonth required for monthly schedule (1-28)", ErrInvalidSchedule)
		}
	}
	return nil
}

// sanitizeMessage trims provider error text to a single line within
// the supplied limit and removes obvious credential leakage.
func sanitizeMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Single-line, max ~500 chars.
	if i := indexOfNewline(msg); i >= 0 {
		msg = msg[:i]
	}
	const max = 500
	if len(msg) > max {
		msg = msg[:max] + "…"
	}
	return msg
}

func indexOfNewline(s string) int {
	for i, r := range s {
		if r == '\n' || r == '\r' {
			return i
		}
	}
	return -1
}

// SetScheduler attaches a scheduler to this service so that handler-side
// "destination updated" hints can propagate. Returning the scheduler also
// lets callers Start/Stop it during process lifecycle.
func (s *Service) SetScheduler(sc *Scheduler) {
	s.scheduler = sc
}

// stringSlice returns a copy of vals with empty strings removed.
func stringSlice(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// withinClockSkew returns true if t1 and t2 are within d of each other.
// Used by tests; exposed at package scope to avoid lint warnings.
func withinClockSkew(t1, t2 time.Time, d time.Duration) bool {
	diff := t1.Sub(t2)
	if diff < 0 {
		diff = -diff
	}
	return diff <= d
}

// timeOrNil returns &t if t is non-zero, else nil.
func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// Service composition note: the service code is split across files to keep
// each below ~300 lines. See:
//   - destinations.go  (CRUD + Validate)
//   - runner.go        (RunOnce, retention pruning)
//   - jsonbuilder.go   (default JSONBuilder implementation)
//   - scheduler.go     (in-process tick loop)
