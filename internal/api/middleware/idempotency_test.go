package middleware

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// stubStore records what the middleware asked of it and returns canned claims.
type stubStore struct {
	claim    service.IdempotencyClaim
	claimErr error

	beginCalls    int
	beginHashes   [][]byte
	finished      []service.IdempotentResponse
	abandonCalls  int
	maxRespBytes  int
	lastBeginKey  string
	lastBeginUser uuid.UUID
}

func (s *stubStore) Begin(_ context.Context, userID uuid.UUID, key string, hash []byte) (service.IdempotencyClaim, error) {
	s.beginCalls++
	s.beginHashes = append(s.beginHashes, hash)
	s.lastBeginKey = key
	s.lastBeginUser = userID
	if s.claimErr != nil {
		return service.IdempotencyClaim{}, s.claimErr
	}
	return s.claim, nil
}

func (s *stubStore) Finish(_ context.Context, _ uuid.UUID, _ string, _ time.Time, resp service.IdempotentResponse) error {
	s.finished = append(s.finished, resp)
	return nil
}

func (s *stubStore) Abandon(_ context.Context, _ uuid.UUID, _ string, _ time.Time) error {
	s.abandonCalls++
	return nil
}

func (s *stubStore) MaxResponseBytes() int {
	if s.maxRespBytes == 0 {
		return 1 << 20
	}
	return s.maxRespBytes
}

// newIdempotencyRouter wires the middleware behind a stand-in for
// AuthMiddleware, which is where the user ID comes from in production.
func newIdempotencyRouter(store IdempotencyStore, userID *uuid.UUID, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID != nil {
			c.Set("userID", *userID)
		}
		c.Next()
	})
	r.Use(IdempotencyMiddleware(store))
	r.POST("/flights", handler)
	r.DELETE("/flights/:id", handler)
	r.GET("/flights", handler)
	return r
}

func doRequest(r *gin.Engine, method, path, key, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(HeaderIdempotencyKey, key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func okHandler(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{"id": "flight-1"})
}

func TestIdempotency_NoHeaderIsUntouched(t *testing.T) {
	// The whole compatibility story: today's clients send no header and must
	// keep the exact behaviour they had before this middleware existed.
	store := &stubStore{}
	user := uuid.New()
	calls := 0
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
		calls++
		okHandler(c)
	})

	for i := 0; i < 2; i++ {
		w := doRequest(r, http.MethodPost, "/flights", "", `{"a":1}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("status: want 201, got %d", w.Code)
		}
	}
	if calls != 2 {
		t.Errorf("handler calls: want 2, got %d", calls)
	}
	if store.beginCalls != 0 {
		t.Errorf("store consulted %d times for keyless requests", store.beginCalls)
	}
}

func TestIdempotency_SafeMethodsAreUntouched(t *testing.T) {
	store := &stubStore{}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })

	w := doRequest(r, http.MethodGet, "/flights", "key-1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", w.Code)
	}
	if store.beginCalls != 0 {
		t.Errorf("GET must not claim a key, got %d claims", store.beginCalls)
	}
}

func TestIdempotency_UnauthenticatedPassesThrough(t *testing.T) {
	// Auth endpoints have no user to key a record by; the header is ignored
	// rather than made an error, so a client can set it unconditionally.
	store := &stubStore{}
	called := false
	r := newIdempotencyRouter(store, nil, func(c *gin.Context) {
		called = true
		okHandler(c)
	})

	w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
	if w.Code != http.StatusCreated || !called {
		t.Fatalf("status %d, handler called %v", w.Code, called)
	}
	if store.beginCalls != 0 {
		t.Errorf("store consulted without an authenticated user")
	}
}

func TestIdempotency_InvalidKeyRejected(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"too long", strings.Repeat("k", 256)},
		{"control character", "key\x01"},
		{"non ascii", "key-ü"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubStore{}
			user := uuid.New()
			called := false
			r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
				called = true
				okHandler(c)
			})

			w := doRequest(r, http.MethodPost, "/flights", tc.key, `{"a":1}`)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status: want 400, got %d", w.Code)
			}
			if called {
				t.Error("handler ran despite an invalid key")
			}
		})
	}
}

func TestIdempotency_WhitespaceOnlyKeyIsIgnored(t *testing.T) {
	store := &stubStore{}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, okHandler)

	w := doRequest(r, http.MethodPost, "/flights", "   ", `{"a":1}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d", w.Code)
	}
	if store.beginCalls != 0 {
		t.Errorf("blank key should be treated as absent")
	}
}

func TestIdempotency_ClaimedRequestStoresResponse(t *testing.T) {
	store := &stubStore{claim: service.IdempotencyClaim{
		Outcome: service.IdempotencyClaimed, ClaimedAt: time.Now().UTC(),
	}}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, okHandler)

	w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d", w.Code)
	}
	if len(store.finished) != 1 {
		t.Fatalf("Finish calls: want 1, got %d", len(store.finished))
	}
	got := store.finished[0]
	if got.Status != http.StatusCreated {
		t.Errorf("stored status: want 201, got %d", got.Status)
	}
	if !strings.Contains(string(got.Body), "flight-1") {
		t.Errorf("stored body: got %q", got.Body)
	}
	if !strings.HasPrefix(got.ContentType, "application/json") {
		t.Errorf("stored content type: got %q", got.ContentType)
	}
	// The handler must still see a readable body after fingerprinting.
	if w.Body.Len() == 0 {
		t.Error("response body was consumed")
	}
}

func TestIdempotency_HandlerStillReadsBody(t *testing.T) {
	store := &stubStore{claim: service.IdempotencyClaim{Outcome: service.IdempotencyClaimed}}
	user := uuid.New()
	var seen string
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		seen, _ = payload["aircraftReg"].(string)
		c.JSON(http.StatusCreated, payload)
	})

	w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"aircraftReg":"D-EFLY"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d (%s)", w.Code, w.Body.String())
	}
	if seen != "D-EFLY" {
		t.Errorf("handler saw %q, want D-EFLY", seen)
	}
}

func TestIdempotency_ReplayReturnsStoredResponse(t *testing.T) {
	store := &stubStore{claim: service.IdempotencyClaim{
		Outcome: service.IdempotencyReplay,
		Response: &service.IdempotentResponse{
			Status:      http.StatusCreated,
			ContentType: "application/json; charset=utf-8",
			Body:        []byte(`{"id":"flight-1"}`),
		},
	}}
	user := uuid.New()
	called := false
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
		called = true
		okHandler(c)
	})

	w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
	if called {
		t.Error("handler re-executed on replay")
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d", w.Code)
	}
	if got := w.Body.String(); got != `{"id":"flight-1"}` {
		t.Errorf("body: got %q", got)
	}
	if w.Header().Get(HeaderIdempotencyReplayed) != "true" {
		t.Error("replay must be flagged with Idempotency-Replayed")
	}
	if len(store.finished) != 0 {
		t.Error("a replay must not rewrite the stored record")
	}
}

func TestIdempotency_ReplayOfEmptyBody(t *testing.T) {
	// A successful DELETE is 204 with no body — the shape that matters most
	// for a client reconciling deletions after being offline.
	store := &stubStore{claim: service.IdempotencyClaim{
		Outcome:  service.IdempotencyReplay,
		Response: &service.IdempotentResponse{Status: http.StatusNoContent},
	}}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := doRequest(r, http.MethodDelete, "/flights/abc", "key-1", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body: want empty, got %q", w.Body.String())
	}
	if w.Header().Get(HeaderIdempotencyReplayed) != "true" {
		t.Error("replay must be flagged")
	}
}

func TestIdempotency_EmptySuccessIsStored(t *testing.T) {
	store := &stubStore{claim: service.IdempotencyClaim{Outcome: service.IdempotencyClaimed}}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := doRequest(r, http.MethodDelete, "/flights/abc", "key-1", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", w.Code)
	}
	if len(store.finished) != 1 {
		t.Fatalf("Finish calls: want 1, got %d", len(store.finished))
	}
	if store.finished[0].Status != http.StatusNoContent {
		t.Errorf("stored status: want 204, got %d", store.finished[0].Status)
	}
	if store.abandonCalls != 0 {
		t.Errorf("a bodyless 204 must not be treated as a failed request")
	}
}

func TestIdempotency_ConflictOutcomes(t *testing.T) {
	cases := []struct {
		name       string
		outcome    service.IdempotencyOutcome
		wantStatus int
		wantHeader string
	}{
		{"in progress", service.IdempotencyInProgress, http.StatusConflict, "Retry-After"},
		{"mismatch", service.IdempotencyMismatch, http.StatusUnprocessableEntity, ""},
		{"not replayable", service.IdempotencyNotReplayable, http.StatusConflict, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubStore{claim: service.IdempotencyClaim{Outcome: tc.outcome}}
			user := uuid.New()
			called := false
			r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
				called = true
				okHandler(c)
			})

			w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
			if w.Code != tc.wantStatus {
				t.Fatalf("status: want %d, got %d", tc.wantStatus, w.Code)
			}
			if called {
				t.Error("handler ran despite a blocked claim")
			}
			if tc.wantHeader != "" && w.Header().Get(tc.wantHeader) == "" {
				t.Errorf("missing %s header", tc.wantHeader)
			}
			if !strings.Contains(w.Body.String(), "error") {
				t.Errorf("body should carry an error message, got %q", w.Body.String())
			}
		})
	}
}

func TestIdempotency_StoreUnavailableFailsClosed(t *testing.T) {
	// Silently downgrading to at-least-once is exactly how a duplicate
	// logbook entry gets created, so an unreachable store must fail the
	// request the client asked to be exactly-once.
	store := &stubStore{claimErr: errors.New("connection refused")}
	user := uuid.New()
	called := false
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
		called = true
		okHandler(c)
	})

	w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: want 503, got %d", w.Code)
	}
	if called {
		t.Error("handler ran with no idempotency guarantee")
	}
}

func TestIdempotency_ServerErrorReleasesTheKey(t *testing.T) {
	store := &stubStore{claim: service.IdempotencyClaim{Outcome: service.IdempotencyClaimed}}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
	})

	w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: want 500, got %d", w.Code)
	}
	if store.abandonCalls != 1 {
		t.Errorf("Abandon calls: want 1, got %d", store.abandonCalls)
	}
	if len(store.finished) != 0 {
		t.Error("a 5xx must not be stored for replay")
	}
}

func TestIdempotency_ClientErrorIsStored(t *testing.T) {
	// A 4xx is a deterministic verdict on the request: replaying it is
	// correct, and re-running validation on every retry is wasted work.
	store := &stubStore{claim: service.IdempotencyClaim{Outcome: service.IdempotencyClaimed}}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"})
	})

	doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
	if len(store.finished) != 1 || store.finished[0].Status != http.StatusBadRequest {
		t.Fatalf("want a stored 400, got %+v", store.finished)
	}
}

func TestIdempotency_PanicReleasesTheKey(t *testing.T) {
	store := &stubStore{claim: service.IdempotencyClaim{Outcome: service.IdempotencyClaimed}}
	user := uuid.New()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) { c.Set("userID", user); c.Next() })
	r.Use(IdempotencyMiddleware(store))
	r.POST("/flights", func(c *gin.Context) { panic("boom") })

	w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: want 500, got %d", w.Code)
	}
	if store.abandonCalls != 1 {
		t.Errorf("a panicking request must leave the key claimable, Abandon calls = %d", store.abandonCalls)
	}
}

func TestIdempotency_OversizedResponseIsNotStored(t *testing.T) {
	store := &stubStore{
		claim:        service.IdempotencyClaim{Outcome: service.IdempotencyClaimed},
		maxRespBytes: 16,
	}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", bytes.Repeat([]byte("x"), 64))
	})

	w := doRequest(r, http.MethodPost, "/flights", "key-1", `{"a":1}`)
	if w.Code != http.StatusOK || w.Body.Len() != 64 {
		t.Fatalf("client must still get the full response: status %d, %d bytes", w.Code, w.Body.Len())
	}
	if len(store.finished) != 1 {
		t.Fatalf("Finish calls: want 1, got %d", len(store.finished))
	}
	// Status 0 marks the record consumed-but-unreplayable, so a retry is
	// refused instead of duplicating the write.
	if store.finished[0].Status != 0 || len(store.finished[0].Body) != 0 {
		t.Errorf("oversized response should be recorded empty, got %+v", store.finished[0])
	}
}

func TestIdempotency_FingerprintCoversMethodPathAndBody(t *testing.T) {
	newStore := func() *stubStore {
		return &stubStore{claim: service.IdempotencyClaim{Outcome: service.IdempotencyClaimed}}
	}
	user := uuid.New()

	hashFor := func(method, path, body string) []byte {
		store := newStore()
		r := newIdempotencyRouter(store, &user, okHandler)
		doRequest(r, method, path, "key-1", body)
		if len(store.beginHashes) != 1 {
			t.Fatalf("expected one claim, got %d", len(store.beginHashes))
		}
		return store.beginHashes[0]
	}

	base := hashFor(http.MethodPost, "/flights", `{"a":1}`)
	same := hashFor(http.MethodPost, "/flights", `{"a":1}`)
	otherBody := hashFor(http.MethodPost, "/flights", `{"a":2}`)
	otherQuery := hashFor(http.MethodPost, "/flights?dryRun=true", `{"a":1}`)

	if !bytes.Equal(base, same) {
		t.Error("identical requests must fingerprint identically")
	}
	if bytes.Equal(base, otherBody) {
		t.Error("a different body must change the fingerprint")
	}
	if bytes.Equal(base, otherQuery) {
		t.Error("a different query string must change the fingerprint")
	}
}

func TestIdempotency_KeyAndUserReachTheStore(t *testing.T) {
	store := &stubStore{claim: service.IdempotencyClaim{Outcome: service.IdempotencyClaimed}}
	user := uuid.New()
	r := newIdempotencyRouter(store, &user, okHandler)

	doRequest(r, http.MethodPost, "/flights", "  key-1  ", `{"a":1}`)
	if store.lastBeginKey != "key-1" {
		t.Errorf("key: want trimmed %q, got %q", "key-1", store.lastBeginKey)
	}
	if store.lastBeginUser != user {
		t.Errorf("user: want %v, got %v", user, store.lastBeginUser)
	}
}

func TestValidIdempotencyKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"", false},
		{uuid.New().String(), true},
		{"01ARZ3NDEKTSV4RRFFQ69G5FAV", true},
		{"queue:flight:42", true},
		{strings.Repeat("k", maxIdempotencyKeyLength), true},
		{strings.Repeat("k", maxIdempotencyKeyLength+1), false},
		{"has space", false},
		{"tab\there", false},
		{"emoji✈", false},
	}
	for _, tc := range cases {
		if got := validIdempotencyKey(tc.key); got != tc.want {
			t.Errorf("validIdempotencyKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}
