package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/weglide"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/google/uuid"
)

type mockWeGlideRepo struct {
	links map[uuid.UUID]*models.WeGlideLink
	files *mockFlightFileRepo
}

func newMockWeGlideRepo(files *mockFlightFileRepo) *mockWeGlideRepo {
	return &mockWeGlideRepo{links: map[uuid.UUID]*models.WeGlideLink{}, files: files}
}

func sameDay(a *time.Time, b time.Time) bool {
	return a != nil && a.UTC().Format("2006-01-02") == b.UTC().Format("2006-01-02")
}

func (m *mockWeGlideRepo) Get(_ context.Context, userID uuid.UUID) (*models.WeGlideLink, error) {
	l, ok := m.links[userID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *l
	return &c, nil
}

func (m *mockWeGlideRepo) Upsert(_ context.Context, userID uuid.UUID, enc []byte, wgUser string, day time.Time) error {
	l, ok := m.links[userID]
	if !ok {
		d := day
		m.links[userID] = &models.WeGlideLink{UserID: userID, APIKeyEncrypted: enc, WeGlideUserID: &wgUser, RequestsToday: 1, RequestsDay: &d}
		return nil
	}
	if l.WeGlideUserID == nil || *l.WeGlideUserID != wgUser {
		l.LastSyncAt, l.LastSyncStatus, l.LastSyncError = nil, nil, nil
	}
	l.APIKeyEncrypted = enc
	l.WeGlideUserID = &wgUser
	return nil
}

func (m *mockWeGlideRepo) Delete(_ context.Context, userID uuid.UUID) error {
	delete(m.links, userID)
	return nil
}

func (m *mockWeGlideRepo) TakeRequest(_ context.Context, userID uuid.UUID, day time.Time, limit int) (int, bool, error) {
	l, ok := m.links[userID]
	if !ok {
		return 0, false, repository.ErrNotFound
	}
	if !sameDay(l.RequestsDay, day) {
		d := day
		l.RequestsDay, l.RequestsToday = &d, 0
	}
	if l.RequestsToday >= limit {
		return l.RequestsToday, false, nil
	}
	l.RequestsToday++
	return l.RequestsToday, true, nil
}

func (m *mockWeGlideRepo) ExhaustRequests(_ context.Context, userID uuid.UUID, day time.Time, limit int) error {
	if l, ok := m.links[userID]; ok {
		d := day
		l.RequestsDay, l.RequestsToday = &d, limit
	}
	return nil
}

func (m *mockWeGlideRepo) RecordSync(_ context.Context, userID uuid.UUID, status string, syncErr *string, completedAt *time.Time) error {
	l, ok := m.links[userID]
	if !ok {
		return nil
	}
	l.LastSyncStatus, l.LastSyncError = &status, syncErr
	if completedAt != nil {
		t := *completedAt
		l.LastSyncAt = &t
	}
	return nil
}

func (m *mockWeGlideRepo) ListDueForSync(_ context.Context, before, day time.Time, limit int) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for id, l := range m.links {
		if (l.LastSyncAt == nil || l.LastSyncAt.Before(before)) && !sameDay(l.RequestsDay, day) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (m *mockWeGlideRepo) ImportedFlightFilenames(_ context.Context, userID uuid.UUID, prefix string) ([]string, error) {
	var out []string
	for _, f := range m.files.files {
		if f.UserID == userID && strings.HasPrefix(f.Filename, prefix) {
			out = append(out, f.Filename)
		}
	}
	return out, nil
}

type fakeWeGlideAPI struct {
	userID      string
	meErr       error
	flights     []weglide.Flight
	igc         map[int64][]byte
	listErr     error
	downloadErr map[int64]error
	sinces      []time.Time
	downloads   []int64
}

func (f *fakeWeGlideAPI) Me(ctx context.Context, key string, b weglide.Budget) (*weglide.User, error) {
	if b != nil {
		if err := b.Take(ctx); err != nil {
			return nil, err
		}
	}
	if f.meErr != nil {
		return nil, f.meErr
	}
	return &weglide.User{ID: f.userID, Name: "Petra"}, nil
}

func (f *fakeWeGlideAPI) ListFlights(ctx context.Context, key string, b weglide.Budget, userID string, since time.Time) ([]weglide.Flight, error) {
	f.sinces = append(f.sinces, since)
	if err := b.Take(ctx); err != nil {
		return nil, err
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []weglide.Flight
	for _, fl := range f.flights {
		if fl.ScoringDate >= since.Format("2006-01-02") {
			out = append(out, fl)
		}
	}
	return out, nil
}

func (f *fakeWeGlideAPI) DownloadIGC(ctx context.Context, key string, b weglide.Budget, id int64) ([]byte, error) {
	if err := b.Take(ctx); err != nil {
		return nil, err
	}
	f.downloads = append(f.downloads, id)
	if err := f.downloadErr[id]; err != nil {
		return nil, err
	}
	return f.igc[id], nil
}

type weglideFixture struct {
	svc     *WeGlideService
	repo    *mockWeGlideRepo
	api     *fakeWeGlideAPI
	files   *FlightFileService
	flights *mockFlightRepo
	now     time.Time
}

func newWeGlideFixture(t *testing.T) *weglideFixture {
	t.Helper()
	withAirports(t)
	fileRepo := newMockFlightFileRepo()
	flightRepo := newMockFlightRepo()
	files := NewFlightFileService(fileRepo, flightRepo)
	files.SetImportDependencies(NewFlightService(flightRepo, nil), NewAircraftService(&sessionAircraftRepo{}))
	key, err := cryptoutil.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cryptoutil.New(key)
	if err != nil {
		t.Fatal(err)
	}
	repo := newMockWeGlideRepo(fileRepo)
	api := &fakeWeGlideAPI{userID: "4711", igc: map[int64][]byte{}, downloadErr: map[int64]error{}}
	fx := &weglideFixture{
		repo: repo, api: api, files: files, flights: flightRepo,
		now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC),
	}
	fx.svc = NewWeGlideService(repo, files, api, aead, func(context.Context, uuid.UUID) string { return "Petra Example" })
	fx.svc.SetClock(func() time.Time { return fx.now })
	return fx
}

func (fx *weglideFixture) addFlight(t *testing.T, id int64, date, fixture string) {
	t.Helper()
	fx.api.flights = append(fx.api.flights, weglide.Flight{ID: id, UserID: "4711", ScoringDate: date})
	fx.api.igc[id] = igcFixture(t, fixture)
}

func (fx *weglideFixture) link(t *testing.T, userID uuid.UUID) {
	t.Helper()
	if _, err := fx.svc.Link(context.Background(), userID, "wg_petra_key"); err != nil {
		t.Fatal(err)
	}
}

func TestWeGlideLink(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		meErr   error
		wantErr error
	}{
		{"malformed key", "has space", nil, ErrWeGlideInvalidKey},
		{"empty key", "   ", nil, ErrWeGlideInvalidKey},
		{"key rejected by WeGlide", "wg_revoked", weglide.ErrUnauthorized, ErrWeGlideInvalidKey},
		{"WeGlide unreachable", "wg_ok", weglide.ErrUnavailable, ErrWeGlideUnavailable},
		{"WeGlide answers garbage", "wg_ok", weglide.ErrBadResponse, ErrWeGlideUnavailable},
		{"WeGlide rate limit", "wg_ok", weglide.ErrRateLimited, ErrWeGlideBudgetUsed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newWeGlideFixture(t)
			fx.api.meErr = tt.meErr
			userID := uuid.New()
			_, err := fx.svc.Link(context.Background(), userID, tt.key)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if _, ok := fx.repo.links[userID]; ok {
				t.Error("link stored after a failed validation")
			}
		})
	}

	t.Run("P4 Petra links her key; stored encrypted, never returned", func(t *testing.T) {
		fx := newWeGlideFixture(t)
		userID := uuid.New()
		st, err := fx.svc.Link(context.Background(), userID, "  wg_petra_key  ")
		if err != nil {
			t.Fatal(err)
		}
		if !st.Linked || st.WeGlideUserID == nil || *st.WeGlideUserID != "4711" || st.RequestsUsedToday != 1 || st.RequestsPerDay != 60 {
			t.Fatalf("status = %+v", st)
		}
		stored := fx.repo.links[userID].APIKeyEncrypted
		if bytes.Contains(stored, []byte("wg_petra_key")) {
			t.Fatal("key stored in plaintext")
		}
		if got, err := fx.svc.decrypt(stored); err != nil || got != "wg_petra_key" {
			t.Fatalf("decrypt = %q, %v", got, err)
		}
	})

	t.Run("re-link counts against the same day's budget", func(t *testing.T) {
		fx := newWeGlideFixture(t)
		userID := uuid.New()
		fx.link(t, userID)
		fx.repo.links[userID].RequestsToday = 60
		if _, err := fx.svc.Link(context.Background(), userID, "wg_other"); !errors.Is(err, ErrWeGlideBudgetUsed) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unlink removes the link, status reports unlinked", func(t *testing.T) {
		fx := newWeGlideFixture(t)
		userID := uuid.New()
		fx.link(t, userID)
		if err := fx.svc.Unlink(context.Background(), userID); err != nil {
			t.Fatal(err)
		}
		st, err := fx.svc.Status(context.Background(), userID)
		if err != nil || st.Linked || st.RequestsPerDay != 60 {
			t.Fatalf("status = %+v, %v", st, err)
		}
	})
}

func TestWeGlideSync_ImportsAndDedups(t *testing.T) {
	fx := newWeGlideFixture(t)
	userID := uuid.New()
	fx.link(t, userID)
	fx.addFlight(t, 501, "2026-06-15", "winch.igc")
	fx.addFlight(t, 502, "2026-06-15", "selflaunch.igc")

	res, err := fx.svc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 2 || res.Skipped != 0 || res.Remaining != 0 || !res.Complete {
		t.Fatalf("P4 first sync = %+v", res)
	}
	if res.RequestsUsedToday != 4 {
		t.Errorf("requests used = %d, want link 1 + list 1 + 2 downloads", res.RequestsUsedToday)
	}
	if want := fx.now.Add(-WeGlideFirstSyncWindow); !fx.api.sinces[0].Equal(want) {
		t.Errorf("first since = %v, want %v", fx.api.sinces[0], want)
	}
	flights, _ := fx.flights.GetByUserID(context.Background(), userID, nil)
	regs := map[string]bool{}
	for _, f := range flights {
		regs[f.AircraftReg] = true
	}
	if len(flights) != 2 || !regs["D-1234"] || !regs["D-KXYZ"] {
		t.Fatalf("flights = %d, regs %v", len(flights), regs)
	}
	link := fx.repo.links[userID]
	if link.LastSyncAt == nil || *link.LastSyncStatus != models.WeGlideSyncOK || link.LastSyncError != nil {
		t.Fatalf("link = %+v", link)
	}

	firstSyncAt := *link.LastSyncAt
	fx.now = fx.now.Add(time.Hour)
	again, err := fx.svc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Imported != 0 || again.Skipped != 0 || !again.Complete || len(fx.api.downloads) != 2 {
		t.Fatalf("second sync = %+v, downloads %v", again, fx.api.downloads)
	}
	if want := firstSyncAt.Add(-WeGlideSyncOverlap); !fx.api.sinces[1].Equal(want) {
		t.Errorf("second since = %v, want %v", fx.api.sinces[1], want)
	}
}

func TestWeGlideSync_SkipsFileAlreadyStored(t *testing.T) {
	fx := newWeGlideFixture(t)
	userID := uuid.New()
	fx.link(t, userID)
	if _, err := fx.files.ImportAsNewFlight(context.Background(), userID, "Petra", "manual.igc", igcFixture(t, "winch.igc")); err != nil {
		t.Fatal(err)
	}
	fx.addFlight(t, 501, "2026-06-15", "winch.igc")
	fx.addFlight(t, 502, "2026-06-16", "selflaunch.igc")
	fx.api.downloadErr[503] = weglide.ErrNotFound
	fx.api.flights = append(fx.api.flights, weglide.Flight{ID: 503, ScoringDate: "2026-06-17"})

	res, err := fx.svc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 || res.Skipped != 2 || !res.Complete {
		t.Fatalf("res = %+v", res)
	}
}

func TestWeGlideSync_BudgetStopAndResume(t *testing.T) {
	fx := newWeGlideFixture(t)
	userID := uuid.New()
	fx.link(t, userID)
	fx.repo.links[userID].RequestsToday = 58
	fx.addFlight(t, 501, "2026-06-15", "winch.igc")
	fx.addFlight(t, 502, "2026-06-16", "selflaunch.igc")
	fx.addFlight(t, 503, "2026-06-17", "aerotow.igc")

	res, err := fx.svc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 || res.Remaining != 2 || res.Complete || res.RequestsUsedToday != 60 {
		t.Fatalf("partial = %+v", res)
	}
	link := fx.repo.links[userID]
	if link.LastSyncAt != nil || *link.LastSyncStatus != models.WeGlideSyncPartial || link.LastSyncError == nil {
		t.Fatalf("link after partial = %+v", link)
	}
	if fx.api.downloads[0] != 501 {
		t.Errorf("oldest flight not imported first: %v", fx.api.downloads)
	}

	if _, err := fx.svc.Sync(context.Background(), userID); !errors.Is(err, ErrWeGlideBudgetUsed) {
		t.Fatalf("same-day sync err = %v", err)
	}

	fx.now = fx.now.Add(24 * time.Hour)
	next, err := fx.svc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if next.Imported != 2 || next.Remaining != 0 || !next.Complete || next.RequestsUsedToday != 3 {
		t.Fatalf("next day = %+v", next)
	}
	if want := fx.now.Add(-WeGlideFirstSyncWindow); !fx.api.sinces[len(fx.api.sinces)-1].Equal(want) {
		t.Errorf("resume since = %v, want the first-sync window", fx.api.sinces[len(fx.api.sinces)-1])
	}
}

func TestWeGlideSync_UpstreamRateLimitExhaustsBudget(t *testing.T) {
	fx := newWeGlideFixture(t)
	userID := uuid.New()
	fx.link(t, userID)
	fx.addFlight(t, 501, "2026-06-15", "winch.igc")
	fx.addFlight(t, 502, "2026-06-16", "selflaunch.igc")
	fx.api.downloadErr[502] = weglide.ErrRateLimited

	res, err := fx.svc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 || res.Remaining != 1 || res.RequestsUsedToday != 60 {
		t.Fatalf("res = %+v", res)
	}
}

func TestWeGlideSync_Errors(t *testing.T) {
	tests := []struct {
		name       string
		linked     bool
		listErr    error
		download   error
		wantErr    error
		wantStatus string
	}{
		{"not linked", false, nil, nil, ErrWeGlideNotLinked, ""},
		{"key revoked", true, weglide.ErrUnauthorized, nil, ErrWeGlideInvalidKey, models.WeGlideSyncFailed},
		{"WeGlide down on list", true, weglide.ErrUnavailable, nil, ErrWeGlideUnavailable, models.WeGlideSyncFailed},
		{"WeGlide down on first download", true, nil, weglide.ErrUnavailable, ErrWeGlideUnavailable, models.WeGlideSyncFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newWeGlideFixture(t)
			userID := uuid.New()
			if tt.linked {
				fx.link(t, userID)
			}
			fx.api.listErr = tt.listErr
			fx.addFlight(t, 501, "2026-06-15", "winch.igc")
			if tt.download != nil {
				fx.api.downloadErr[501] = tt.download
			}
			_, err := fx.svc.Sync(context.Background(), userID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantStatus == "" {
				return
			}
			link := fx.repo.links[userID]
			if link.LastSyncStatus == nil || *link.LastSyncStatus != tt.wantStatus || link.LastSyncError == nil {
				t.Fatalf("link = %+v", link)
			}
			if strings.Contains(*link.LastSyncError, "wg_petra_key") {
				t.Error("key in sync error")
			}
		})
	}
}

func TestWeGlideSync_InProgress(t *testing.T) {
	fx := newWeGlideFixture(t)
	userID := uuid.New()
	fx.link(t, userID)
	if !fx.svc.acquire(userID) {
		t.Fatal("acquire")
	}
	defer fx.svc.release(userID)
	if _, err := fx.svc.Sync(context.Background(), userID); !errors.Is(err, ErrWeGlideSyncInProgress) {
		t.Fatalf("err = %v", err)
	}
}

func TestWeGlideSyncDue(t *testing.T) {
	fx := newWeGlideFixture(t)
	due, fresh, usedToday := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{due, fresh, usedToday} {
		fx.link(t, id)
	}
	yesterday := fx.now.Add(-24 * time.Hour)
	for _, id := range []uuid.UUID{due, fresh} {
		fx.repo.links[id].RequestsDay = &yesterday
	}
	recent := fx.now.Add(-time.Hour)
	fx.repo.links[fresh].LastSyncAt = &recent
	fx.addFlight(t, 501, "2026-06-15", "winch.igc")

	if n := fx.svc.SyncDue(context.Background(), 24*time.Hour); n != 1 {
		t.Fatalf("synced %d users, want 1", n)
	}
	if fx.repo.links[due].LastSyncAt == nil || !fx.repo.links[fresh].LastSyncAt.Equal(recent) {
		t.Fatalf("due %+v, fresh %+v", fx.repo.links[due], fx.repo.links[fresh])
	}
	if fx.repo.links[usedToday].LastSyncStatus != nil {
		t.Error("a link that already used WeGlide today was synced")
	}
}

func TestSanitizeWeGlideError(t *testing.T) {
	tests := []struct {
		name, msg, key, want string
	}{
		{"redacts key", "bad key wg_secret here", "wg_secret", "bad key [redacted] here"},
		{"strips control characters", "line\nbreak\x00", "", "line break"},
		{"caps length", strings.Repeat("é", 600), "", strings.Repeat("é", models.MaxWeGlideSyncErrorLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeWeGlideError(tt.msg, tt.key); got != tt.want {
				t.Errorf("got %q", got)
			}
		})
	}
}

func TestWeGlideSync_StopsBeforeRequestDeadline(t *testing.T) {
	fx := newWeGlideFixture(t)
	userID := uuid.New()
	fx.link(t, userID)
	fx.addFlight(t, 501, "2026-06-15", "winch.igc")
	fx.addFlight(t, 502, "2026-06-16", "selflaunch.igc")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res, err := fx.svc.Sync(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 0 || res.Remaining != 2 || res.Complete || len(fx.api.downloads) != 0 {
		t.Fatalf("res = %+v, downloads %v", res, fx.api.downloads)
	}
	link := fx.repo.links[userID]
	if *link.LastSyncStatus != models.WeGlideSyncPartial || link.LastSyncAt != nil {
		t.Fatalf("link = %+v", link)
	}
}
