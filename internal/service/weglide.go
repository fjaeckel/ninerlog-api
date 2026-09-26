package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/weglide"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/google/uuid"
)

// WeGlide link errors.
var (
	ErrWeGlideNotLinked      = errors.New("no WeGlide account is linked")
	ErrWeGlideInvalidKey     = errors.New("WeGlide did not accept the API key")
	ErrWeGlideUnavailable    = errors.New("WeGlide could not be reached")
	ErrWeGlideBudgetUsed     = errors.New("the daily WeGlide request limit is used up; try again tomorrow (UTC)")
	ErrWeGlideSyncInProgress = errors.New("a WeGlide sync is already running for this account")
	errWeGlideStoreFailed    = errors.New("a WeGlide flight could not be stored")
	errWeGlideOutOfTime      = errors.New("stopped within the request time limit; sync again to continue")
)

// WeGlide sync constants.
const (
	// WeGlideFirstSyncWindow is how far back the first sync lists flights.
	WeGlideFirstSyncWindow = 365 * 24 * time.Hour
	// WeGlideSyncOverlap is subtracted from the last complete sync when
	// listing flights.
	WeGlideSyncOverlap = 30 * 24 * time.Hour
	// WeGlideFilePrefix starts the filename of every IGC file a sync stores;
	// the WeGlide flight ID follows.
	WeGlideFilePrefix = "weglide-"
	// DefaultWeGlideSyncInterval is the scheduler's default interval.
	DefaultWeGlideSyncInterval = 24 * time.Hour
	// weglideSweepBatch caps the users one scheduler sweep syncs.
	weglideSweepBatch = 200
	// weglideTimeReserve is the time left on a context deadline below which
	// no further flight is started.
	weglideTimeReserve = 5 * time.Second
	// weglideRecordTimeout bounds the status write after a run.
	weglideRecordTimeout = 5 * time.Second
)

// UserNameFunc returns a user's display name, or "".
type UserNameFunc func(ctx context.Context, userID uuid.UUID) string

// WeGlideStatus is a user's link state. The API key is never part of it.
type WeGlideStatus struct {
	Linked            bool
	WeGlideUserID     *string
	LastSyncAt        *time.Time
	LastSyncStatus    *string
	LastSyncError     *string
	RequestsUsedToday int
	RequestsPerDay    int
}

// WeGlideSyncResult reports one sync. Remaining counts listed flights left
// unprocessed; Complete is false when more flights may be waiting.
type WeGlideSyncResult struct {
	Imported          int
	Skipped           int
	Remaining         int
	RequestsUsedToday int
	Complete          bool
}

// WeGlideService links a pilot's WeGlide account through their personal API
// key and imports their WeGlide flights as IGC flights.
type WeGlideService struct {
	repo     repository.WeGlideLinkRepository
	files    *FlightFileService
	client   weglide.API
	aead     *cryptoutil.AEAD
	userName UserNameFunc
	now      func() time.Time

	mu      sync.Mutex
	running map[uuid.UUID]bool
}

// NewWeGlideService returns a WeGlideService. aead encrypts stored keys.
func NewWeGlideService(repo repository.WeGlideLinkRepository, files *FlightFileService, client weglide.API, aead *cryptoutil.AEAD, userName UserNameFunc) *WeGlideService {
	if userName == nil {
		userName = func(context.Context, uuid.UUID) string { return "" }
	}
	return &WeGlideService{
		repo:     repo,
		files:    files,
		client:   client,
		aead:     aead,
		userName: userName,
		now:      time.Now,
		running:  map[uuid.UUID]bool{},
	}
}

// SetClock replaces the clock; for tests.
func (s *WeGlideService) SetClock(now func() time.Time) { s.now = now }

// Status returns the user's link state; Linked is false without a link.
func (s *WeGlideService) Status(ctx context.Context, userID uuid.UUID) (*WeGlideStatus, error) {
	st := &WeGlideStatus{RequestsPerDay: weglide.RequestsPerDay}
	link, err := s.repo.Get(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.Linked = true
	st.WeGlideUserID = link.WeGlideUserID
	st.LastSyncAt = link.LastSyncAt
	st.LastSyncStatus = link.LastSyncStatus
	st.LastSyncError = link.LastSyncError
	st.RequestsUsedToday = link.RequestsUsedOn(s.now())
	return st, nil
}

// Link validates apiKey with one WeGlide request and stores it encrypted,
// replacing any key the user had linked.
//
// Errors: ErrWeGlideInvalidKey (malformed or rejected), ErrWeGlideBudgetUsed,
// ErrWeGlideUnavailable.
func (s *WeGlideService) Link(ctx context.Context, userID uuid.UUID, apiKey string) (*WeGlideStatus, error) {
	key := strings.TrimSpace(apiKey)
	if !weglide.ValidKey(key) {
		return nil, ErrWeGlideInvalidKey
	}
	day := s.now()
	var budget weglide.Budget
	if _, err := s.repo.Get(ctx, userID); err == nil {
		budget = &linkBudget{repo: s.repo, userID: userID, day: day}
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	user, err := s.client.Me(ctx, key, budget)
	if err != nil {
		return nil, s.linkError(ctx, userID, day, err)
	}
	enc, err := s.encrypt(key)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, userID, enc, user.ID, day); err != nil {
		return nil, fmt.Errorf("store weglide link: %w", err)
	}
	return s.Status(ctx, userID)
}

func (s *WeGlideService) linkError(ctx context.Context, userID uuid.UUID, day time.Time, err error) error {
	switch {
	case errors.Is(err, weglide.ErrUnauthorized), errors.Is(err, weglide.ErrInvalidKey), errors.Is(err, weglide.ErrNotFound):
		return ErrWeGlideInvalidKey
	case errors.Is(err, weglide.ErrBudgetExhausted):
		return ErrWeGlideBudgetUsed
	case errors.Is(err, weglide.ErrRateLimited):
		_ = s.repo.ExhaustRequests(ctx, userID, day, weglide.RequestsPerDay)
		return ErrWeGlideBudgetUsed
	case errors.Is(err, repository.ErrNotFound):
		return ErrWeGlideNotLinked
	case errors.Is(err, weglide.ErrUnavailable), errors.Is(err, weglide.ErrBadResponse), errors.Is(err, weglide.ErrTooLarge),
		errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return ErrWeGlideUnavailable
	}
	return err
}

// Unlink removes the user's link. Flights already imported stay.
func (s *WeGlideService) Unlink(ctx context.Context, userID uuid.UUID) error {
	return s.repo.Delete(ctx, userID)
}

func (s *WeGlideService) encrypt(key string) ([]byte, error) {
	ct, nonce, err := s.aead.Encrypt([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("encrypt weglide key: %w", err)
	}
	return append(append([]byte{}, nonce...), ct...), nil
}

func (s *WeGlideService) decrypt(blob []byte) (string, error) {
	if len(blob) < cryptoutil.NonceSize {
		return "", cryptoutil.ErrInvalidCiphertext
	}
	pt, err := s.aead.Decrypt(blob[cryptoutil.NonceSize:], blob[:cryptoutil.NonceSize])
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// linkBudget takes WeGlide requests from the link's daily count.
type linkBudget struct {
	repo   repository.WeGlideLinkRepository
	userID uuid.UUID
	day    time.Time
}

func (b *linkBudget) Take(ctx context.Context) error {
	_, ok, err := b.repo.TakeRequest(ctx, b.userID, b.day, weglide.RequestsPerDay)
	if err != nil {
		return err
	}
	if !ok {
		return weglide.ErrBudgetExhausted
	}
	return nil
}

func (s *WeGlideService) acquire(userID uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[userID] {
		return false
	}
	s.running[userID] = true
	return true
}

func (s *WeGlideService) release(userID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, userID)
}

// Sync imports the user's WeGlide flights not yet imported, oldest first,
// until the list is done or the daily request budget is used up. Flights
// are listed from 30 days before the last complete sync, or the last 12
// months on the first sync. A flight whose IGC file the user already stores
// is skipped.
//
// Errors: ErrWeGlideNotLinked, ErrWeGlideSyncInProgress; and, when the
// flight list could not be read or no flight was processed,
// ErrWeGlideInvalidKey, ErrWeGlideBudgetUsed (list only) and
// ErrWeGlideUnavailable. Otherwise the result is returned with a nil error;
// Complete is false when the run stopped early.
func (s *WeGlideService) Sync(ctx context.Context, userID uuid.UUID) (*WeGlideSyncResult, error) {
	if !s.acquire(userID) {
		return nil, ErrWeGlideSyncInProgress
	}
	defer s.release(userID)

	start := time.Now()
	res, err := s.sync(ctx, userID)
	WeGlideSyncDurationSeconds.Observe(time.Since(start).Seconds())
	return res, err
}

func (s *WeGlideService) sync(ctx context.Context, userID uuid.UUID) (*WeGlideSyncResult, error) {
	link, err := s.repo.Get(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrWeGlideNotLinked
	}
	if err != nil {
		WeGlideSyncRunsTotal.WithLabelValues(models.WeGlideSyncFailed).Inc()
		return nil, err
	}
	now := s.now()
	day := now
	key, err := s.decrypt(link.APIKeyEncrypted)
	if err != nil || link.WeGlideUserID == nil {
		s.record(ctx, userID, models.WeGlideSyncFailed, "The stored WeGlide key cannot be used; link WeGlide again.", key, nil)
		return nil, ErrWeGlideInvalidKey
	}
	budget := &linkBudget{repo: s.repo, userID: userID, day: day}

	since := now.Add(-WeGlideFirstSyncWindow)
	if link.LastSyncAt != nil {
		since = link.LastSyncAt.Add(-WeGlideSyncOverlap)
	}
	done, err := s.importedIDs(ctx, userID)
	if err != nil {
		WeGlideSyncRunsTotal.WithLabelValues(models.WeGlideSyncFailed).Inc()
		return nil, err
	}

	flights, listErr := s.client.ListFlights(ctx, key, budget, *link.WeGlideUserID, since)
	if listErr != nil && len(flights) == 0 {
		mapped := s.linkError(ctx, userID, day, listErr)
		s.recordError(ctx, userID, mapped, key)
		return nil, mapped
	}

	var todo []weglide.Flight
	for _, f := range flights {
		if !done[f.ID] {
			todo = append(todo, f)
		}
	}
	res := &WeGlideSyncResult{}
	name := s.userName(ctx, userID)
	var stopErr error
	processed := 0
	for _, f := range todo {
		if dl, ok := ctx.Deadline(); ctx.Err() != nil || (ok && time.Until(dl) < weglideTimeReserve) {
			stopErr = errWeGlideOutOfTime
			break
		}
		data, err := s.client.DownloadIGC(ctx, key, budget, f.ID)
		if err != nil {
			if errors.Is(err, weglide.ErrNotFound) || errors.Is(err, weglide.ErrTooLarge) || errors.Is(err, weglide.ErrBadResponse) {
				res.Skipped++
				processed++
				continue
			}
			stopErr = s.linkError(ctx, userID, day, err)
			break
		}
		filename := WeGlideFilePrefix + strconv.FormatInt(f.ID, 10) + ".igc"
		_, err = s.files.ImportAsNewFlight(ctx, userID, name, filename, data)
		processed++
		switch {
		case err == nil:
			res.Imported++
			WeGlideSyncFlightsImportedTotal.Inc()
		case isSkippableIGCError(err):
			res.Skipped++
		default:
			processed--
			slog.Error("weglide sync: import failed", "userId", userID, "weglideFlightId", f.ID, "error", err)
			stopErr = errWeGlideStoreFailed
		}
		if stopErr != nil {
			break
		}
	}
	res.Remaining = len(todo) - processed
	if listErr != nil && stopErr == nil {
		stopErr = s.linkError(ctx, userID, day, listErr)
		if res.Remaining == 0 {
			res.Remaining = 1
		}
	}
	res.Complete = stopErr == nil && res.Remaining == 0
	if stopErr != nil && res.Imported+res.Skipped == 0 &&
		!errors.Is(stopErr, ErrWeGlideBudgetUsed) && !errors.Is(stopErr, errWeGlideOutOfTime) {
		s.recordError(ctx, userID, stopErr, key)
		return nil, stopErr
	}

	switch {
	case res.Complete:
		t := now
		s.record(ctx, userID, models.WeGlideSyncOK, "", key, &t)
		WeGlideSyncLastSuccessTimestampSeconds.SetToCurrentTime()
	case stopErr == nil, errors.Is(stopErr, ErrWeGlideBudgetUsed), errors.Is(stopErr, errWeGlideOutOfTime):
		msg := ""
		if stopErr != nil {
			msg = stopErr.Error()
		}
		s.record(ctx, userID, models.WeGlideSyncPartial, msg, key, nil)
	default:
		s.recordError(ctx, userID, stopErr, key)
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), weglideRecordTimeout)
	defer cancel()
	if l, err := s.repo.Get(rctx, userID); err == nil {
		res.RequestsUsedToday = l.RequestsUsedOn(day)
	}
	return res, nil
}

func isSkippableIGCError(err error) bool {
	return errors.Is(err, ErrFlightFileDuplicate) || errors.Is(err, ErrInvalidIGC) ||
		errors.Is(err, ErrFlightFileEmpty) || errors.Is(err, ErrFlightFileTooLarge) ||
		errors.Is(err, ErrIGCNoFlight) || errors.Is(err, ErrIGCNoRegistration) ||
		errors.Is(err, ErrIGCInvalidFlight)
}

func (s *WeGlideService) importedIDs(ctx context.Context, userID uuid.UUID) (map[int64]bool, error) {
	names, err := s.repo.ImportedFlightFilenames(ctx, userID, WeGlideFilePrefix)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(names))
	for _, n := range names {
		idText := strings.TrimSuffix(strings.TrimPrefix(n, WeGlideFilePrefix), ".igc")
		if id, err := strconv.ParseInt(idText, 10, 64); err == nil {
			out[id] = true
		}
	}
	return out, nil
}

func (s *WeGlideService) recordError(ctx context.Context, userID uuid.UUID, err error, key string) {
	status := models.WeGlideSyncFailed
	if errors.Is(err, ErrWeGlideBudgetUsed) {
		status = models.WeGlideSyncPartial
	}
	msg := "The sync failed."
	switch {
	case errors.Is(err, ErrWeGlideInvalidKey), errors.Is(err, ErrWeGlideBudgetUsed),
		errors.Is(err, ErrWeGlideUnavailable), errors.Is(err, errWeGlideStoreFailed):
		msg = err.Error()
	}
	s.record(ctx, userID, status, msg, key, nil)
}

func (s *WeGlideService) record(ctx context.Context, userID uuid.UUID, status, msg, key string, completedAt *time.Time) {
	var syncErr *string
	if msg != "" {
		clean := SanitizeWeGlideError(msg, key)
		syncErr = &clean
	}
	WeGlideSyncRunsTotal.WithLabelValues(status).Inc()
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), weglideRecordTimeout)
	defer cancel()
	if err := s.repo.RecordSync(rctx, userID, status, syncErr, completedAt); err != nil {
		slog.Error("weglide sync: failed to record status", "userId", userID, "error", err)
	}
}

// SanitizeWeGlideError removes key and control characters from msg and caps
// it at models.MaxWeGlideSyncErrorLen runes.
func SanitizeWeGlideError(msg, key string) string {
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "[redacted]")
	}
	msg = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, msg)
	msg = strings.TrimSpace(msg)
	if r := []rune(msg); len(r) > models.MaxWeGlideSyncErrorLen {
		msg = string(r[:models.MaxWeGlideSyncErrorLen])
	}
	return msg
}

// SyncDue syncs every link whose last complete sync is older than 90% of
// interval and that made no WeGlide request today (UTC). Returns the number of users synced.
func (s *WeGlideService) SyncDue(ctx context.Context, interval time.Duration) int {
	now := s.now()
	ids, err := s.repo.ListDueForSync(ctx, now.Add(-(interval - interval/10)), now, weglideSweepBatch)
	if err != nil {
		slog.Error("weglide sync: listing due links failed", "error", err)
		return 0
	}
	n := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		if _, err := s.Sync(ctx, id); err != nil && !errors.Is(err, ErrWeGlideSyncInProgress) {
			slog.Info("weglide scheduled sync did not complete", "userId", id, "reason", err.Error())
		}
		n++
	}
	return n
}

// Start runs SyncDue once a minute after start and then every interval
// until ctx is done.
func (s *WeGlideService) Start(ctx context.Context, interval time.Duration) {
	go func() {
		slog.Info("WeGlide sync scheduler started", "interval", interval.String())
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("WeGlide sync scheduler stopped")
				return
			case <-timer.C:
				s.SyncDue(ctx, interval)
				timer.Reset(interval)
			}
		}
	}()
}
