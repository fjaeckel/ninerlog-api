package cloudbackup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/google/uuid"
)

// ---- fake repositories ---------------------------------------------------

type memDestRepo struct {
	mu    sync.Mutex
	items map[uuid.UUID]*models.BackupDestination
}

func newMemDestRepo() *memDestRepo {
	return &memDestRepo{items: map[uuid.UUID]*models.BackupDestination{}}
}

func (r *memDestRepo) Create(_ context.Context, d *models.BackupDestination) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	d.UpdatedAt = time.Now()
	r.items[d.ID] = cloneDest(d)
	return nil
}

func (r *memDestRepo) GetByID(_ context.Context, id uuid.UUID) (*models.BackupDestination, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.items[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return cloneDest(d), nil
}

func (r *memDestRepo) GetByUserID(_ context.Context, userID uuid.UUID) ([]*models.BackupDestination, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*models.BackupDestination{}
	for _, d := range r.items {
		if d.UserID == userID {
			out = append(out, cloneDest(d))
		}
	}
	return out, nil
}

func (r *memDestRepo) Update(_ context.Context, d *models.BackupDestination) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[d.ID]; !ok {
		return repository.ErrNotFound
	}
	d.UpdatedAt = time.Now()
	r.items[d.ID] = cloneDest(d)
	return nil
}

func (r *memDestRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *memDestRepo) ListDueForRun(_ context.Context, _ time.Time) ([]*models.BackupDestination, error) {
	// not exercised in service unit tests
	return nil, nil
}

func cloneDest(d *models.BackupDestination) *models.BackupDestination {
	cp := *d
	if d.Config != nil {
		cp.Config = make(map[string]any, len(d.Config))
		for k, v := range d.Config {
			cp.Config[k] = v
		}
	}
	if d.CredentialsEnc != nil {
		cp.CredentialsEnc = append([]byte(nil), d.CredentialsEnc...)
	}
	if d.CredentialsNonce != nil {
		cp.CredentialsNonce = append([]byte(nil), d.CredentialsNonce...)
	}
	return &cp
}

type memRunRepo struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*models.BackupRun
}

func newMemRunRepo() *memRunRepo {
	return &memRunRepo{rows: map[uuid.UUID]*models.BackupRun{}}
}

func (r *memRunRepo) Create(_ context.Context, run *models.BackupRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	r.rows[run.ID] = cloneRun(run)
	return nil
}

func (r *memRunRepo) Update(_ context.Context, run *models.BackupRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[run.ID]; !ok {
		return repository.ErrNotFound
	}
	r.rows[run.ID] = cloneRun(run)
	return nil
}

func (r *memRunRepo) GetByID(_ context.Context, id uuid.UUID) (*models.BackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return cloneRun(v), nil
}

func (r *memRunRepo) GetByDestinationID(_ context.Context, destinationID uuid.UUID, limit, offset int) ([]*models.BackupRun, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	matches := []*models.BackupRun{}
	for _, v := range r.rows {
		if v.DestinationID == destinationID {
			matches = append(matches, cloneRun(v))
		}
	}
	total := len(matches)
	if offset > len(matches) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	return matches[offset:end], total, nil
}

func (r *memRunRepo) DeleteByDestinationID(_ context.Context, destinationID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, v := range r.rows {
		if v.DestinationID == destinationID {
			delete(r.rows, id)
		}
	}
	return nil
}

func cloneRun(r *models.BackupRun) *models.BackupRun {
	cp := *r
	return &cp
}

// ---- fake provider -------------------------------------------------------

type fakeProvider struct {
	mu          sync.Mutex
	validateErr error
	uploadErr   error
	listErr     error
	deleteErr   error

	objects        []provider.RemoteBackup
	uploads        []provider.UploadInput
	deletes        []string
	validateCalls  int
}

func (f *fakeProvider) Name() string        { return "fake" }
func (f *fakeProvider) DisplayName() string { return "Fake" }
func (f *fakeProvider) Description() string { return "" }
func (f *fakeProvider) ConfigSchema() []provider.Field {
	return []provider.Field{
		{Name: "bucket", Label: "Bucket", Type: provider.FieldTypeString, Required: true},
	}
}
func (f *fakeProvider) CredentialSchema() []provider.Field {
	return []provider.Field{
		{Name: "key", Label: "Key", Type: provider.FieldTypePassword, Required: true, Sensitive: true},
	}
}
func (f *fakeProvider) Validate(_ context.Context, _ provider.Config, _ provider.Credentials) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.validateCalls++
	return f.validateErr
}
func (f *fakeProvider) Upload(_ context.Context, _ provider.Config, _ provider.Credentials, in provider.UploadInput) (*provider.UploadResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	// drain reader to simulate real provider behaviour and capture size
	body, _ := io.ReadAll(in.Reader)
	in.Reader = bytes.NewReader(body)
	f.uploads = append(f.uploads, in)
	obj := provider.RemoteBackup{Path: "fake/" + in.Filename, SizeBytes: int64(len(body)), LastModified: time.Now()}
	f.objects = append(f.objects, obj)
	return &provider.UploadResult{RemotePath: obj.Path}, nil
}
func (f *fakeProvider) List(_ context.Context, _ provider.Config, _ provider.Credentials) ([]provider.RemoteBackup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]provider.RemoteBackup, len(f.objects))
	copy(out, f.objects)
	return out, nil
}
func (f *fakeProvider) Delete(_ context.Context, _ provider.Config, _ provider.Credentials, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for i, o := range f.objects {
		if o.Path == path {
			f.objects = append(f.objects[:i], f.objects[i+1:]...)
			break
		}
	}
	f.deletes = append(f.deletes, path)
	return nil
}

// ---- fake JSON builder ---------------------------------------------------

type fakeBuilder struct {
	body  []byte
	meta  BuildMetadata
	count int
	err   error
}

func (f *fakeBuilder) BuildJSON(_ context.Context, _ uuid.UUID) (io.ReadCloser, BuildMetadata, error) {
	if f.err != nil {
		return nil, BuildMetadata{}, f.err
	}
	f.count++
	meta := f.meta
	meta.SizeBytes = int64(len(f.body))
	return io.NopCloser(bytes.NewReader(f.body)), meta, nil
}

// ---- helpers -------------------------------------------------------------

func newTestService(t *testing.T, p provider.Provider, builder JSONBuilder) (*Service, *memDestRepo, *memRunRepo) {
	t.Helper()
	key, err := cryptoutil.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	aead, err := cryptoutil.New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reg := provider.NewRegistry()
	reg.Register(p)
	dRepo := newMemDestRepo()
	rRepo := newMemRunRepo()
	svc, err := New(Options{
		DestinationRepo: dRepo,
		RunRepo:         rRepo,
		Registry:        reg,
		Crypto:          aead,
		Builder:         builder,
	})
	if err != nil {
		t.Fatalf("New service: %v", err)
	}
	return svc, dRepo, rRepo
}

// ---- tests ---------------------------------------------------------------

func TestServiceCreateAndGetDestination(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("{}"), meta: BuildMetadata{SHA256: "abc", FlightCount: 1}}
	svc, _, _ := newTestService(t, fp, fb)

	userID := uuid.New()
	d, err := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID:          userID,
		Provider:        "fake",
		DisplayName:     "Test",
		Config:          provider.Config{"bucket": "b"},
		Credentials:     provider.Credentials{"key": "secret-12345"},
		Schedule:        models.BackupScheduleDaily,
		ScheduleHourUTC: 3,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	if d.Status != models.BackupStatusActive {
		t.Errorf("expected active, got %s", d.Status)
	}
	if d.CredentialHint == "" {
		t.Errorf("credential hint should not be empty")
	}
	if fp.validateCalls != 1 {
		t.Errorf("expected Validate to be called once, got %d", fp.validateCalls)
	}
	if string(d.CredentialsEnc) == `{"key":"secret-12345"}` {
		t.Errorf("credentials stored in plaintext!")
	}

	got, err := svc.GetDestination(context.Background(), d.ID, userID)
	if err != nil {
		t.Fatalf("GetDestination: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("mismatched ID")
	}
}

func TestServiceCreateRejectsUnknownProvider(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{}
	svc, _, _ := newTestService(t, fp, fb)

	_, err := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID:          uuid.New(),
		Provider:        "no-such-provider",
		DisplayName:     "X",
		Schedule:        models.BackupScheduleDaily,
		ScheduleHourUTC: 3,
	})
	if !errors.Is(err, ErrProviderUnknown) {
		t.Fatalf("expected ErrProviderUnknown, got %v", err)
	}
}

func TestServiceCreateRequiresValidSchedule(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{}
	svc, _, _ := newTestService(t, fp, fb)
	_, err := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID:          uuid.New(),
		Provider:        "fake",
		DisplayName:     "X",
		Config:          provider.Config{"bucket": "b"},
		Credentials:     provider.Credentials{"key": "k"},
		Schedule:        models.BackupScheduleWeekly,
		ScheduleHourUTC: 3,
	})
	if !errors.Is(err, ErrInvalidSchedule) {
		t.Fatalf("expected ErrInvalidSchedule (weekly missing dow), got %v", err)
	}
}

func TestServiceCreateRequiresCredentials(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{}
	svc, _, _ := newTestService(t, fp, fb)
	_, err := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID:          uuid.New(),
		Provider:        "fake",
		DisplayName:     "X",
		Config:          provider.Config{"bucket": "b"},
		Credentials:     provider.Credentials{},
		Schedule:        models.BackupScheduleManual,
		ScheduleHourUTC: 3,
	})
	if err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("expected missing-field error mentioning 'key', got %v", err)
	}
}

func TestServiceCreateSurfacesValidateError(t *testing.T) {
	fp := &fakeProvider{validateErr: errors.New("nope")}
	fb := &fakeBuilder{}
	svc, _, _ := newTestService(t, fp, fb)
	_, err := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID:          uuid.New(),
		Provider:        "fake",
		DisplayName:     "X",
		Config:          provider.Config{"bucket": "b"},
		Credentials:     provider.Credentials{"key": "k"},
		Schedule:        models.BackupScheduleManual,
		ScheduleHourUTC: 3,
	})
	if err == nil || err.Error() != "nope" {
		t.Fatalf("expected 'nope', got %v", err)
	}
}

func TestServiceTestDestinationFlipsStatusOnFailure(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{}
	svc, dRepo, _ := newTestService(t, fp, fb)

	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "X",
		Config:      provider.Config{"bucket": "b"},
		Credentials: provider.Credentials{"key": "k"},
		Schedule:    models.BackupScheduleManual, ScheduleHourUTC: 3,
	})
	fp.validateErr = errors.New("auth failed")

	ok, msg, err := svc.TestDestination(context.Background(), d.ID, userID)
	if err != nil {
		t.Fatalf("TestDestination: %v", err)
	}
	if ok {
		t.Fatalf("expected failure")
	}
	if msg == "" {
		t.Fatalf("expected error message")
	}
	got, _ := dRepo.GetByID(context.Background(), d.ID)
	if got.Status != models.BackupStatusError {
		t.Fatalf("expected status=error, got %s", got.Status)
	}
}

func TestServiceRunOnceSuccess(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("payload"), meta: BuildMetadata{SHA256: "h1", FlightCount: 1, AircraftCount: 2, LicenseCount: 3, CredentialCount: 4, ContentType: "application/gzip", Filename: "x.gz"}}
	svc, dRepo, _ := newTestService(t, fp, fb)

	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "X",
		Config:      provider.Config{"bucket": "b"},
		Credentials: provider.Credentials{"key": "k"},
		Schedule:    models.BackupScheduleManual, ScheduleHourUTC: 3, Enabled: true,
	})

	run, err := svc.RunOnce(context.Background(), RunRequest{DestinationID: d.ID, UserID: userID, Trigger: models.BackupRunTriggerManual})
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if run.Status != models.BackupRunStatusSuccess {
		t.Fatalf("expected success, got %s (err=%s)", run.Status, run.ErrorMessage)
	}
	if len(fp.uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(fp.uploads))
	}
	got, _ := dRepo.GetByID(context.Background(), d.ID)
	if got.LastSuccessSHA256 != "h1" {
		t.Errorf("destination did not record sha256")
	}
	if got.LastSuccessAt == nil {
		t.Errorf("destination did not record last_success_at")
	}
}

func TestServiceRunOnceSkipsIfUnchanged(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("payload"), meta: BuildMetadata{SHA256: "stable", FlightCount: 1, Filename: "x.gz"}}
	svc, _, runRepo := newTestService(t, fp, fb)

	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "X",
		Config:      provider.Config{"bucket": "b"},
		Credentials: provider.Credentials{"key": "k"},
		Schedule:    models.BackupScheduleManual, ScheduleHourUTC: 3,
	})

	if _, err := svc.RunOnce(context.Background(), RunRequest{DestinationID: d.ID, UserID: userID, Trigger: models.BackupRunTriggerManual}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	uploadsAfterFirst := len(fp.uploads)
	run, err := svc.RunOnce(context.Background(), RunRequest{DestinationID: d.ID, UserID: userID, Trigger: models.BackupRunTriggerManual})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run.Status != models.BackupRunStatusSkipped {
		t.Fatalf("expected skipped on second run, got %s", run.Status)
	}
	if len(fp.uploads) != uploadsAfterFirst {
		t.Fatalf("upload should not have been called on skipped run")
	}
	// Both runs persisted.
	rows, total, _ := runRepo.GetByDestinationID(context.Background(), d.ID, 10, 0)
	if total != 2 || len(rows) != 2 {
		t.Fatalf("expected 2 run rows, got %d", total)
	}
}

func TestServiceRunOnceRecordsFailureOnUploadError(t *testing.T) {
	fp := &fakeProvider{uploadErr: errors.New("network down")}
	fb := &fakeBuilder{body: []byte("payload"), meta: BuildMetadata{SHA256: "h", Filename: "x.gz"}}
	svc, dRepo, _ := newTestService(t, fp, fb)

	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "X",
		Config:      provider.Config{"bucket": "b"},
		Credentials: provider.Credentials{"key": "k"},
		Schedule:    models.BackupScheduleManual, ScheduleHourUTC: 3,
	})

	run, err := svc.RunOnce(context.Background(), RunRequest{DestinationID: d.ID, UserID: userID, Trigger: models.BackupRunTriggerManual})
	if err == nil || err.Error() != "network down" {
		t.Fatalf("expected error 'network down', got %v", err)
	}
	if run.Status != models.BackupRunStatusFailed {
		t.Fatalf("expected failed run, got %s", run.Status)
	}
	got, _ := dRepo.GetByID(context.Background(), d.ID)
	if got.ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", got.ConsecutiveFailures)
	}
}

func TestServiceRunOnceRejectsCrossUser(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("p"), meta: BuildMetadata{SHA256: "h", Filename: "x.gz"}}
	svc, _, _ := newTestService(t, fp, fb)

	ownerID := uuid.New()
	otherID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: ownerID, Provider: "fake", DisplayName: "X",
		Config:      provider.Config{"bucket": "b"},
		Credentials: provider.Credentials{"key": "k"},
		Schedule:    models.BackupScheduleManual, ScheduleHourUTC: 3,
	})

	_, err := svc.RunOnce(context.Background(), RunRequest{DestinationID: d.ID, UserID: otherID, Trigger: models.BackupRunTriggerManual})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestServiceRunOnceConcurrent(t *testing.T) {
	// Force second concurrent call to bounce off the destination lock.
	block := make(chan struct{})
	fp := &fakeProvider{}
	fb := &fakeBuilder{body: []byte("p"), meta: BuildMetadata{SHA256: "h", Filename: "x.gz"}}
	svc, _, _ := newTestService(t, fp, fb)

	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "X",
		Config:      provider.Config{"bucket": "b"},
		Credentials: provider.Credentials{"key": "k"},
		Schedule:    models.BackupScheduleManual, ScheduleHourUTC: 3,
	})

	// Slow down upload so the second goroutine can hit the lock.
	svc.ensureRunLocks()
	if !svc.runLocks.tryAcquire(d.ID) {
		t.Fatalf("could not acquire test lock")
	}
	defer svc.runLocks.release(d.ID)
	defer close(block)

	_, err := svc.RunOnce(context.Background(), RunRequest{DestinationID: d.ID, UserID: userID, Trigger: models.BackupRunTriggerManual})
	if !errors.Is(err, ErrConcurrentRun) {
		t.Fatalf("expected ErrConcurrentRun, got %v", err)
	}
}

func TestServiceRunOncePrunesPastRetention(t *testing.T) {
	fp := &fakeProvider{}
	// Pre-populate 5 historical objects, sorted oldest→newest by LastModified.
	now := time.Now()
	for i := 0; i < 5; i++ {
		fp.objects = append(fp.objects, provider.RemoteBackup{
			Path:         "fake/old-" + string(rune('a'+i)),
			LastModified: now.Add(time.Duration(i) * time.Hour),
		})
	}
	fb := &fakeBuilder{body: []byte("p"), meta: BuildMetadata{SHA256: "h", Filename: "new.gz"}}
	svc, _, _ := newTestService(t, fp, fb)

	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "X",
		Config:      provider.Config{"bucket": "b"},
		Credentials: provider.Credentials{"key": "k"},
		Schedule:    models.BackupScheduleManual, ScheduleHourUTC: 3,
		RetentionCount: 3,
	})

	if _, err := svc.RunOnce(context.Background(), RunRequest{DestinationID: d.ID, UserID: userID, Trigger: models.BackupRunTriggerManual}); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	// 5 pre-existing + 1 new = 6. Retention=3, so 3 oldest deletions.
	if len(fp.deletes) != 3 {
		t.Errorf("expected 3 deletions, got %d (%v)", len(fp.deletes), fp.deletes)
	}
}

func TestServiceUpdateDestination(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{}
	svc, _, _ := newTestService(t, fp, fb)

	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "Old",
		Config: provider.Config{"bucket": "b"}, Credentials: provider.Credentials{"key": "k"},
		Schedule: models.BackupScheduleManual, ScheduleHourUTC: 3, RetentionCount: 30, Enabled: true,
	})

	newName := "New"
	updated, err := svc.UpdateDestination(context.Background(), d.ID, userID, UpdateDestinationInput{DisplayName: &newName})
	if err != nil {
		t.Fatalf("UpdateDestination: %v", err)
	}
	if updated.DisplayName != "New" {
		t.Errorf("expected New, got %s", updated.DisplayName)
	}

	// Schedule transition: switch from manual → daily without dow/dom should be valid.
	daily := models.BackupScheduleDaily
	hour := 5
	updated, err = svc.UpdateDestination(context.Background(), d.ID, userID, UpdateDestinationInput{Schedule: &daily, ScheduleHourUTC: &hour})
	if err != nil {
		t.Fatalf("UpdateDestination schedule: %v", err)
	}
	if updated.Schedule != models.BackupScheduleDaily || updated.ScheduleHourUTC != 5 {
		t.Errorf("schedule not applied: %+v", updated)
	}
}

func TestServiceUpdateDestinationRejectsBadSchedule(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{}
	svc, _, _ := newTestService(t, fp, fb)
	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "X",
		Config: provider.Config{"bucket": "b"}, Credentials: provider.Credentials{"key": "k"},
		Schedule: models.BackupScheduleManual, ScheduleHourUTC: 3,
	})
	sched := models.BackupScheduleWeekly
	_, err := svc.UpdateDestination(context.Background(), d.ID, userID, UpdateDestinationInput{Schedule: &sched})
	if !errors.Is(err, ErrInvalidSchedule) {
		t.Fatalf("expected ErrInvalidSchedule (weekly without dow), got %v", err)
	}
}

func TestServiceDeleteDestination(t *testing.T) {
	fp := &fakeProvider{}
	fb := &fakeBuilder{}
	svc, _, _ := newTestService(t, fp, fb)
	userID := uuid.New()
	d, _ := svc.CreateDestination(context.Background(), CreateDestinationInput{
		UserID: userID, Provider: "fake", DisplayName: "X",
		Config: provider.Config{"bucket": "b"}, Credentials: provider.Credentials{"key": "k"},
		Schedule: models.BackupScheduleManual, ScheduleHourUTC: 3,
	})

	if err := svc.DeleteDestination(context.Background(), d.ID, userID); err != nil {
		t.Fatalf("DeleteDestination: %v", err)
	}
	if _, err := svc.GetDestination(context.Background(), d.ID, userID); !errors.Is(err, ErrDestinationNotFound) {
		t.Fatalf("expected ErrDestinationNotFound, got %v", err)
	}
}

func TestSanitizeMessage(t *testing.T) {
	if got := sanitizeMessage(nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	long := strings.Repeat("a", 1000) + "\nstacktrace\n…"
	got := sanitizeMessage(errors.New(long))
	if len(got) > 510 {
		t.Errorf("did not truncate: %d chars", len(got))
	}
	if strings.Contains(got, "\n") {
		t.Errorf("multi-line message leaked: %q", got)
	}
}
