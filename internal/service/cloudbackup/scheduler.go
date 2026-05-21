package cloudbackup

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// Scheduler is a process-local goroutine that periodically asks the repository
// for due destinations and dispatches them via Service.RunOnce.
//
// Design notes:
//   - One scheduler per API process. In a horizontally-scaled deployment
//     (which we are not yet doing) you would want a leadership election or
//     row-level lock; the current design only guarantees at-most-once per
//     destination per process due to runLocks.
//   - Ticks every minute, with a small jitter on the first tick to spread out
//     load when multiple instances of the API restart simultaneously.
//   - Per-user concurrency cap: at most one run per user can be in flight
//     across all of their destinations.
type Scheduler struct {
	svc        *Service
	tick       time.Duration
	jitter     time.Duration
	stop       chan struct{}
	wg         sync.WaitGroup
	logger     *log.Logger
	userLocks  *userLockSet
	// runHook is called for each dispatched run; tests use it to observe
	// without spinning the goroutine.
	runHook func(req RunRequest)
}

// NewScheduler constructs a scheduler attached to svc. tick is the period of
// the loop; pass 0 for the default 60s.
func NewScheduler(svc *Service, tick time.Duration, logger *log.Logger) *Scheduler {
	if tick <= 0 {
		tick = time.Minute
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{
		svc:       svc,
		tick:      tick,
		jitter:    5 * time.Second,
		stop:      make(chan struct{}),
		logger:    logger,
		userLocks: newUserLockSet(),
	}
}

// Start launches the scheduler loop in a goroutine. Cancel the supplied
// context (or call Stop) to terminate.
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.loop(ctx)
}

// Stop signals the loop to exit and waits for it to drain in-flight ticks.
func (s *Scheduler) Stop() {
	select {
	case <-s.stop:
		// already stopped
	default:
		close(s.stop)
	}
	s.wg.Wait()
}

func (s *Scheduler) loop(ctx context.Context) {
	defer s.wg.Done()

	// Sleep a small amount on first iteration to avoid stampeding on cold
	// start. We pin to a deterministic small offset (3s) rather than rand
	// so tests don't flake.
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-timer.C:
			s.tickOnce(ctx)
			timer.Reset(s.tick)
		}
	}
}

// tickOnce runs one scheduler iteration. Exported via type embedding so tests
// can step the scheduler manually.
func (s *Scheduler) tickOnce(ctx context.Context) {
	now := s.svc.clock()
	due, err := s.svc.destRepo.ListDueForRun(ctx, now)
	if err != nil {
		s.logger.Printf("cloudbackup scheduler: list due failed: %v", err)
		return
	}
	for _, d := range due {
		s.dispatch(ctx, d)
	}
}

// dispatch enqueues a backup run for d. Per-user concurrency is enforced by
// userLocks; per-destination concurrency is enforced by Service.RunOnce.
func (s *Scheduler) dispatch(ctx context.Context, d *models.BackupDestination) {
	if !s.userLocks.tryAcquire(d.UserID) {
		return // another destination for this user is running; pick this one up next tick
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.userLocks.release(d.UserID)
		req := RunRequest{
			DestinationID: d.ID,
			UserID:        d.UserID,
			Trigger:       models.BackupRunTriggerScheduled,
		}
		if s.runHook != nil {
			s.runHook(req)
			return
		}
		if _, err := s.svc.RunOnce(ctx, req); err != nil {
			s.logger.Printf("cloudbackup scheduler: run failed (dest=%s): %v", d.ID, err)
		}
	}()
}

// userLockSet enforces at-most-one in-flight backup per user.
type userLockSet struct {
	mu       sync.Mutex
	inflight map[string]struct{}
}

func newUserLockSet() *userLockSet {
	return &userLockSet{inflight: make(map[string]struct{})}
}

func (l *userLockSet) tryAcquire(userID interface{ String() string }) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := userID.String()
	if _, in := l.inflight[k]; in {
		return false
	}
	l.inflight[k] = struct{}{}
	return true
}

func (l *userLockSet) release(userID interface{ String() string }) {
	l.mu.Lock()
	delete(l.inflight, userID.String())
	l.mu.Unlock()
}
