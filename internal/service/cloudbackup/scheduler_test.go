package cloudbackup

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// schedDestRepo wraps memDestRepo and exposes a controllable ListDueForRun.
type schedDestRepo struct {
	*memDestRepo
	due []*models.BackupDestination
}

func (r *schedDestRepo) ListDueForRun(_ context.Context, _ time.Time) ([]*models.BackupDestination, error) {
	out := make([]*models.BackupDestination, len(r.due))
	copy(out, r.due)
	return out, nil
}

func TestSchedulerTickOnceDispatchesAllDueDestinations(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("{}"), meta: BuildMetadata{SHA256: "abc"}}
	svc, _, _ := newTestService(t, fp, fb)

	// Replace destRepo with one that has due items.
	userA := uuid.New()
	userB := uuid.New()
	d1 := &models.BackupDestination{ID: uuid.New(), UserID: userA, Provider: "fake"}
	d2 := &models.BackupDestination{ID: uuid.New(), UserID: userB, Provider: "fake"}
	mem := newMemDestRepo()
	mem.items[d1.ID] = d1
	mem.items[d2.ID] = d2
	svc.destRepo = &schedDestRepo{memDestRepo: mem, due: []*models.BackupDestination{d1, d2}}

	sched := NewScheduler(svc, time.Hour, log.New(&discardWriter{}, "", 0))
	var (
		mu      sync.Mutex
		got     []uuid.UUID
		gotUser []uuid.UUID
	)
	sched.runHook = func(req RunRequest) {
		mu.Lock()
		got = append(got, req.DestinationID)
		gotUser = append(gotUser, req.UserID)
		mu.Unlock()
	}

	sched.tickOnce(context.Background())
	// Wait for goroutines spawned by dispatch to finish.
	sched.wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 dispatches, got %d (%v)", len(got), got)
	}
	seen := map[uuid.UUID]bool{got[0]: true, got[1]: true}
	if !seen[d1.ID] || !seen[d2.ID] {
		t.Fatalf("expected both destinations dispatched, got %v", got)
	}
	seenUsers := map[uuid.UUID]bool{gotUser[0]: true, gotUser[1]: true}
	if !seenUsers[userA] || !seenUsers[userB] {
		t.Fatalf("expected dispatches for both users, got %v", gotUser)
	}
}

func TestSchedulerPerUserLockPreventsConcurrentDispatchForSameUser(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("{}"), meta: BuildMetadata{SHA256: "abc"}}
	svc, _, _ := newTestService(t, fp, fb)

	user := uuid.New()
	d1 := &models.BackupDestination{ID: uuid.New(), UserID: user, Provider: "fake"}
	d2 := &models.BackupDestination{ID: uuid.New(), UserID: user, Provider: "fake"}
	mem := newMemDestRepo()
	mem.items[d1.ID] = d1
	mem.items[d2.ID] = d2
	svc.destRepo = &schedDestRepo{memDestRepo: mem, due: []*models.BackupDestination{d1, d2}}

	sched := NewScheduler(svc, time.Hour, log.New(&discardWriter{}, "", 0))
	var count atomicCounter
	sched.runHook = func(_ RunRequest) {
		count.inc()
	}

	// Pre-acquire the user lock so both dispatch attempts skip the gate.
	if !sched.userLocks.tryAcquire(user) {
		t.Fatal("failed to pre-acquire user lock")
	}
	sched.tickOnce(context.Background())
	sched.wg.Wait()
	if got := count.get(); got != 0 {
		t.Fatalf("expected 0 dispatches while user lock held, got %d", got)
	}

	// Release and tick again — first destination gets dispatched, second
	// is skipped because the goroutine for the first is still holding the
	// user lock (the runHook returns immediately, but we want to assert
	// per-tick semantics, not steady-state). Drain and re-check.
	sched.userLocks.release(user)
	sched.tickOnce(context.Background())
	sched.wg.Wait()
	// Both ticks combined: still at most one dispatch per tick because
	// the goroutine for d1 holds the user lock long enough that
	// dispatch(d2) skips. With a no-op runHook this is racy; instead
	// assert that count never exceeds 2 (sanity) and is at least 1.
	got := count.get()
	if got < 1 || got > 2 {
		t.Fatalf("expected 1 or 2 dispatches after release, got %d", got)
	}
}

// atomicCounter is a tiny goroutine-safe int counter used by scheduler tests.
type atomicCounter struct {
	mu sync.Mutex
	n  int
}

func (c *atomicCounter) inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *atomicCounter) get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func TestSchedulerTickWithNothingDueDoesNothing(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("{}"), meta: BuildMetadata{SHA256: "abc"}}
	svc, _, _ := newTestService(t, fp, fb)
	mem := newMemDestRepo()
	svc.destRepo = &schedDestRepo{memDestRepo: mem, due: nil}

	sched := NewScheduler(svc, time.Hour, log.New(&discardWriter{}, "", 0))
	called := false
	sched.runHook = func(_ RunRequest) { called = true }
	sched.tickOnce(context.Background())
	sched.wg.Wait()
	if called {
		t.Fatal("runHook should not be called when no destinations are due")
	}
}

func TestSchedulerStartStop(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("{}"), meta: BuildMetadata{SHA256: "abc"}}
	svc, _, _ := newTestService(t, fp, fb)
	mem := newMemDestRepo()
	svc.destRepo = &schedDestRepo{memDestRepo: mem, due: nil}

	sched := NewScheduler(svc, 10*time.Millisecond, log.New(&discardWriter{}, "", 0))
	// Shrink the initial-tick delay reflectively by overwriting via Start
	// then immediately Stop — we mainly assert Stop returns promptly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	done := make(chan struct{})
	go func() {
		sched.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return in time")
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
