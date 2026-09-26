package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/weglide"
	"github.com/fjaeckel/ninerlog-api/pkg/cryptoutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type handlerWeGlideRepo struct {
	links map[uuid.UUID]*models.WeGlideLink
	files *mockFlightFileRepo
}

func (m *handlerWeGlideRepo) Get(_ context.Context, userID uuid.UUID) (*models.WeGlideLink, error) {
	l, ok := m.links[userID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	c := *l
	return &c, nil
}

func (m *handlerWeGlideRepo) Upsert(_ context.Context, userID uuid.UUID, enc []byte, wgUser string, day time.Time) error {
	if l, ok := m.links[userID]; ok {
		l.APIKeyEncrypted, l.WeGlideUserID = enc, &wgUser
		return nil
	}
	d := day
	m.links[userID] = &models.WeGlideLink{UserID: userID, APIKeyEncrypted: enc, WeGlideUserID: &wgUser, RequestsToday: 1, RequestsDay: &d}
	return nil
}

func (m *handlerWeGlideRepo) Delete(_ context.Context, userID uuid.UUID) error {
	delete(m.links, userID)
	return nil
}

func (m *handlerWeGlideRepo) TakeRequest(_ context.Context, userID uuid.UUID, day time.Time, limit int) (int, bool, error) {
	l, ok := m.links[userID]
	if !ok {
		return 0, false, repository.ErrNotFound
	}
	if l.RequestsUsedOn(day) == 0 {
		d := day
		l.RequestsDay, l.RequestsToday = &d, 0
	}
	if l.RequestsToday >= limit {
		return l.RequestsToday, false, nil
	}
	l.RequestsToday++
	return l.RequestsToday, true, nil
}

func (m *handlerWeGlideRepo) ExhaustRequests(_ context.Context, userID uuid.UUID, day time.Time, limit int) error {
	if l, ok := m.links[userID]; ok {
		d := day
		l.RequestsDay, l.RequestsToday = &d, limit
	}
	return nil
}

func (m *handlerWeGlideRepo) RecordSync(_ context.Context, userID uuid.UUID, status string, syncErr *string, completedAt *time.Time) error {
	if l, ok := m.links[userID]; ok {
		l.LastSyncStatus, l.LastSyncError = &status, syncErr
		if completedAt != nil {
			l.LastSyncAt = completedAt
		}
	}
	return nil
}

func (m *handlerWeGlideRepo) ListDueForSync(context.Context, time.Time, time.Time, int) ([]uuid.UUID, error) {
	return nil, nil
}

func (m *handlerWeGlideRepo) ImportedFlightFilenames(_ context.Context, userID uuid.UUID, prefix string) ([]string, error) {
	var out []string
	for _, f := range m.files.files {
		if f.UserID == userID && strings.HasPrefix(f.Filename, prefix) {
			out = append(out, f.Filename)
		}
	}
	return out, nil
}

const handlerTestKey = "wg_handler_key"

// fakeWeGlideServer serves /v1/user/me, two flights and their IGC files.
func fakeWeGlideServer(t *testing.T, igc map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-API-Key") != handlerTestKey {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/v1/user/me", auth(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": 4711, "name": "Petra"}`))
	}))
	mux.HandleFunc("/v1/flight", auth(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id": 11, "user": {"id": 4711}, "scoring_date": "2026-06-15"},
			{"id": 12, "user": {"id": 4711}, "scoring_date": "2026-06-16"}]`))
	}))
	mux.HandleFunc("/v1/flightdetail/", auth(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/flightdetail/")
		_, _ = w.Write([]byte(`{"igc_file": {"id": 1, "file": "igc/` + id + `.igc"}}`))
	}))
	mux.HandleFunc("/files/igc/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(igc[strings.TrimPrefix(r.URL.Path, "/files/igc/")])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func setupWeGlideHandler(t *testing.T, baseURL string) (*APIHandler, *handlerWeGlideRepo) {
	t.Helper()
	h, flights, _ := setupIgcHandler(t)
	fileRepo := &mockFlightFileRepo{files: map[uuid.UUID]*models.FlightFile{}}
	ffs := service.NewFlightFileService(fileRepo, flights)
	ffs.SetImportDependencies(h.flightService, h.aircraftService)
	h.SetFlightFileService(ffs)
	key, _ := cryptoutil.GenerateKey()
	aead, _ := cryptoutil.New(key)
	repo := &handlerWeGlideRepo{links: map[uuid.UUID]*models.WeGlideLink{}, files: fileRepo}
	client := weglide.New(weglide.Config{BaseURL: baseURL, FilesURL: baseURL + "/files", Timeout: 2 * time.Second})
	h.SetWeGlideService(service.NewWeGlideService(repo, h.flightFileService, client, aead, nil), 0)
	return h, repo
}

func weglideRequest(h *APIHandler, userID uuid.UUID, method, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var c *gin.Context
	if userID == uuid.Nil {
		c, _ = gin.CreateTestContext(w)
	} else {
		c = authenticatedContext(w, userID)
	}
	path := "/integrations/weglide"
	if method == "SYNC" {
		method, path = http.MethodPost, path+"/sync"
	}
	c.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	switch method {
	case http.MethodGet:
		h.GetWeGlideLink(c)
	case http.MethodPut:
		h.LinkWeGlide(c)
	case http.MethodDelete:
		h.UnlinkWeGlide(c)
	default:
		h.SyncWeGlide(c)
	}
	c.Writer.WriteHeaderNow()
	return w
}

func TestWeGlideHandlers_NotConfigured(t *testing.T) {
	h, _ := setupTestHandler()
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, "SYNC"} {
		if w := weglideRequest(h, uuid.New(), m, `{"apiKey":"x"}`); w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d", m, w.Code)
		}
		if w := weglideRequest(h, uuid.Nil, m, `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s unauthenticated: status = %d", m, w.Code)
		}
	}
}

func TestWeGlideHandlers_Statuses(t *testing.T) {
	igc := map[string][]byte{
		"11.igc": igcFixtureBytes(t, "winch.igc"),
		"12.igc": igcFixtureBytes(t, "selflaunch.igc"),
	}
	srv := fakeWeGlideServer(t, igc)

	t.Run("unlinked status", func(t *testing.T) {
		h, _ := setupWeGlideHandler(t, srv.URL)
		w := weglideRequest(h, uuid.New(), http.MethodGet, "")
		var st generated.WeGlideLinkStatus
		_ = json.Unmarshal(w.Body.Bytes(), &st)
		if w.Code != http.StatusOK || st.Linked || st.RequestsPerDay != 60 {
			t.Fatalf("status %d, body %s", w.Code, w.Body.String())
		}
	})

	t.Run("link errors", func(t *testing.T) {
		h, _ := setupWeGlideHandler(t, srv.URL)
		tests := []struct {
			name string
			body string
			want int
		}{
			{"invalid body", `{`, http.StatusBadRequest},
			{"malformed key", `{"apiKey":"has space"}`, http.StatusBadRequest},
			{"rejected key", `{"apiKey":"wg_wrong"}`, http.StatusBadRequest},
		}
		for _, tt := range tests {
			if w := weglideRequest(h, uuid.New(), http.MethodPut, tt.body); w.Code != tt.want {
				t.Errorf("%s: status = %d, want %d", tt.name, w.Code, tt.want)
			}
		}
	})

	t.Run("WeGlide unreachable is 502", func(t *testing.T) {
		down := httptest.NewServer(http.NotFoundHandler())
		url := down.URL
		down.Close()
		h, _ := setupWeGlideHandler(t, url)
		if w := weglideRequest(h, uuid.New(), http.MethodPut, `{"apiKey":"`+handlerTestKey+`"}`); w.Code != http.StatusBadGateway {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("sync without a link is 404", func(t *testing.T) {
		h, _ := setupWeGlideHandler(t, srv.URL)
		if w := weglideRequest(h, uuid.New(), "SYNC", ""); w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("P4 Petra links, syncs two flights, re-syncs, unlinks", func(t *testing.T) {
		h, repo := setupWeGlideHandler(t, srv.URL)
		userID := uuid.New()
		w := weglideRequest(h, userID, http.MethodPut, `{"apiKey":"`+handlerTestKey+`"}`)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), handlerTestKey) {
			t.Fatalf("link: %d %s", w.Code, w.Body.String())
		}
		w = weglideRequest(h, userID, "SYNC", "")
		var res generated.WeGlideSyncResult
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if w.Code != http.StatusOK || res.Imported != 2 || res.Remaining != 0 || res.RequestsUsedToday != 4 {
			t.Fatalf("sync: %d %s", w.Code, w.Body.String())
		}
		w = weglideRequest(h, userID, "SYNC", "")
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if w.Code != http.StatusOK || res.Imported != 0 || res.Skipped != 0 {
			t.Fatalf("re-sync: %d %s", w.Code, w.Body.String())
		}
		w = weglideRequest(h, userID, http.MethodGet, "")
		var st generated.WeGlideLinkStatus
		_ = json.Unmarshal(w.Body.Bytes(), &st)
		if !st.Linked || st.LastSyncAt == nil || st.LastSyncStatus == nil || *st.LastSyncStatus != "ok" || strings.Contains(w.Body.String(), handlerTestKey) {
			t.Fatalf("status: %s", w.Body.String())
		}
		if w := weglideRequest(h, userID, http.MethodDelete, ""); w.Code != http.StatusNoContent {
			t.Fatalf("unlink = %d", w.Code)
		}
		if _, ok := repo.links[userID]; ok {
			t.Fatal("link kept after unlink")
		}
		if w := weglideRequest(h, userID, http.MethodDelete, ""); w.Code != http.StatusNoContent {
			t.Fatalf("second unlink = %d", w.Code)
		}
	})

	t.Run("partial sync is 202, exhausted budget is 429", func(t *testing.T) {
		h, repo := setupWeGlideHandler(t, srv.URL)
		userID := uuid.New()
		if w := weglideRequest(h, userID, http.MethodPut, `{"apiKey":"`+handlerTestKey+`"}`); w.Code != http.StatusOK {
			t.Fatalf("link = %d", w.Code)
		}
		repo.links[userID].RequestsToday = 58
		w := weglideRequest(h, userID, "SYNC", "")
		var res generated.WeGlideSyncResult
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if w.Code != http.StatusAccepted || res.Imported != 1 || res.Remaining != 1 || res.RequestsUsedToday != 60 {
			t.Fatalf("partial: %d %s", w.Code, w.Body.String())
		}
		if w := weglideRequest(h, userID, "SYNC", ""); w.Code != http.StatusTooManyRequests {
			t.Fatalf("exhausted = %d", w.Code)
		}
	})
}
