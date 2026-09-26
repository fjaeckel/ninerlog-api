package weglide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "wg_test_key_123"

type countingBudget struct {
	limit int
	used  int
}

func (b *countingBudget) Take(context.Context) error {
	if b.used >= b.limit {
		return ErrBudgetExhausted
	}
	b.used++
	return nil
}

type fakeWeGlide struct {
	t             *testing.T
	flights       int
	apiCalls      atomic.Int32
	fileCalls     atomic.Int32
	status        int
	jsonPad       int
	igcBytes      int
	igcPath       string
	redirectTo    string
	sawKeyOnFiles atomic.Bool
}

func (f *fakeWeGlide) handler() http.Handler {
	mux := http.NewServeMux()
	api := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.apiCalls.Add(1)
			if r.Header.Get("X-API-Key") != testKey {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"detail":"leaked-upstream-text"}`))
				return
			}
			if f.redirectTo != "" {
				http.Redirect(w, r, f.redirectTo, http.StatusFound)
				return
			}
			if f.status != 0 {
				w.WriteHeader(f.status)
				_, _ = w.Write([]byte(`{"detail":"leaked-upstream-text"}`))
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/v1/user/me", api(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"id": 4711, "name": "Petra", "pad": %q}`, strings.Repeat("x", f.jsonPad))
	}))
	mux.HandleFunc("/v1/flight", api(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("user_id_in") != "4711" || q.Get("scoring_date_start") != "2026-01-01" || q.Get("limit") != "100" {
			f.t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		skip, _ := strconv.Atoi(q.Get("skip"))
		rows := []map[string]any{}
		for i := skip; i < f.flights && i < skip+PageSize; i++ {
			rows = append(rows, map[string]any{
				"id":           1000 + (f.flights - i),
				"user":         map[string]any{"id": 4711},
				"scoring_date": fmt.Sprintf("2026-06-%02d", 1+(f.flights-i)%28),
				"takeoff_time": "2026-06-01T10:00:00Z",
			})
		}
		if skip == 0 {
			rows = append(rows, map[string]any{"id": 1, "user": map[string]any{"id": 99}, "scoring_date": "2026-06-01"})
		}
		_ = json.NewEncoder(w).Encode(rows)
	}))
	mux.HandleFunc("/v1/flightdetail/", api(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/flightdetail/")
		path := f.igcPath
		if path == "" {
			path = "igc/" + id + ".igc"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "igc_file": map[string]any{"id": 1, "file": path}})
	}))
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		f.fileCalls.Add(1)
		if r.Header.Get("X-API-Key") != "" {
			f.sawKeyOnFiles.Store(true)
		}
		n := f.igcBytes
		if n == 0 {
			n = 100
		}
		_, _ = w.Write([]byte(strings.Repeat("B", n)))
	})
	return mux
}

func newTestClient(t *testing.T, f *fakeWeGlide) *Client {
	t.Helper()
	f.t = t
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL, FilesURL: srv.URL + "/files", MaxIGCBytes: 1000, Timeout: 2 * time.Second})
}

func TestClient_Me(t *testing.T) {
	f := &fakeWeGlide{}
	c := newTestClient(t, f)
	b := &countingBudget{limit: 60}
	u, err := c.Me(context.Background(), testKey, b)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "4711" || u.Name != "Petra" {
		t.Errorf("user = %+v", u)
	}
	if b.used != 1 || f.apiCalls.Load() != 1 {
		t.Errorf("budget used %d, calls %d", b.used, f.apiCalls.Load())
	}
}

func TestClient_ErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		status int
		want   error
	}{
		{"wrong key is unauthorized", "wg_wrong", 0, ErrUnauthorized},
		{"403 is unauthorized", testKey, http.StatusForbidden, ErrUnauthorized},
		{"429 is rate limited", testKey, http.StatusTooManyRequests, ErrRateLimited},
		{"404 is not found", testKey, http.StatusNotFound, ErrNotFound},
		{"500 is unavailable", testKey, http.StatusInternalServerError, ErrUnavailable},
		{"418 is a bad response", testKey, http.StatusTeapot, ErrBadResponse},
		{"malformed key never leaves", "has space", 0, ErrInvalidKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeWeGlide{status: tt.status}
			c := newTestClient(t, f)
			_, err := c.Me(context.Background(), tt.key, nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if strings.Contains(err.Error(), "leaked-upstream-text") {
				t.Errorf("upstream text in error: %v", err)
			}
		})
	}
}

func TestClient_UnreachableHost(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := New(Config{BaseURL: url, Timeout: time.Second})
	if _, err := c.Me(context.Background(), testKey, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_RedirectNotFollowed(t *testing.T) {
	f := &fakeWeGlide{redirectTo: "https://elsewhere.example/steal"}
	c := newTestClient(t, f)
	if _, err := c.Me(context.Background(), testKey, nil); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_BudgetExhausted(t *testing.T) {
	f := &fakeWeGlide{}
	c := newTestClient(t, f)
	b := &countingBudget{limit: 0}
	if _, err := c.Me(context.Background(), testKey, b); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("err = %v", err)
	}
	if f.apiCalls.Load() != 0 {
		t.Errorf("a request was sent without budget")
	}
}

func TestClient_ListFlights(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t.Run("paginates, drops other users, oldest first", func(t *testing.T) {
		f := &fakeWeGlide{flights: 150}
		c := newTestClient(t, f)
		b := &countingBudget{limit: 60}
		got, err := c.ListFlights(context.Background(), testKey, b, "4711", since)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 150 {
			t.Fatalf("flights = %d", len(got))
		}
		if b.used != 2 {
			t.Errorf("budget used = %d, want 2 pages", b.used)
		}
		for i := 1; i < len(got); i++ {
			if got[i].ScoringDate < got[i-1].ScoringDate {
				t.Fatalf("not sorted at %d", i)
			}
		}
	})
	t.Run("budget runs out on page 2", func(t *testing.T) {
		f := &fakeWeGlide{flights: 150}
		c := newTestClient(t, f)
		b := &countingBudget{limit: 1}
		got, err := c.ListFlights(context.Background(), testKey, b, "4711", since)
		if !errors.Is(err, ErrBudgetExhausted) || len(got) != 100 {
			t.Fatalf("got %d flights, err %v", len(got), err)
		}
	})
}

func TestClient_DownloadIGC(t *testing.T) {
	t.Run("key goes to the API only; files host is not budgeted", func(t *testing.T) {
		f := &fakeWeGlide{}
		c := newTestClient(t, f)
		b := &countingBudget{limit: 60}
		data, err := c.DownloadIGC(context.Background(), testKey, b, 1234)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 100 || b.used != 1 || f.fileCalls.Load() != 1 {
			t.Errorf("len %d, budget %d, file calls %d", len(data), b.used, f.fileCalls.Load())
		}
		if f.sawKeyOnFiles.Load() {
			t.Error("API key sent to the files host")
		}
	})
	t.Run("IGC over the cap", func(t *testing.T) {
		f := &fakeWeGlide{igcBytes: 1001}
		c := newTestClient(t, f)
		if _, err := c.DownloadIGC(context.Background(), testKey, nil, 1); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("err = %v", err)
		}
	})
	for _, p := range []string{"../../etc/passwd", "a/../b.igc", "https://evil.example/x.igc", "//evil.example/x", "a b.igc"} {
		t.Run("rejects IGC path "+p, func(t *testing.T) {
			f := &fakeWeGlide{igcPath: p}
			c := newTestClient(t, f)
			if _, err := c.DownloadIGC(context.Background(), testKey, nil, 1); !errors.Is(err, ErrBadResponse) {
				t.Fatalf("err = %v", err)
			}
			if f.fileCalls.Load() != 0 {
				t.Error("files host was called")
			}
		})
	}
}

func TestClient_JSONSizeCap(t *testing.T) {
	f := &fakeWeGlide{jsonPad: MaxJSONBytes}
	c := newTestClient(t, f)
	if _, err := c.Me(context.Background(), testKey, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigFromEnv_TestURLGated(t *testing.T) {
	t.Setenv("WEGLIDE_API_URL", "http://127.0.0.1:1")
	t.Setenv("WEGLIDE_FILES_URL", "http://127.0.0.1:2")
	t.Setenv("WEGLIDE_ALLOW_TEST_URL", "")
	if cfg := ConfigFromEnv(); cfg.BaseURL != "" || cfg.FilesURL != "" {
		t.Fatalf("override applied without the flag: %+v", cfg)
	}
	if New(ConfigFromEnv()).BaseURL() != DefaultBaseURL {
		t.Fatal("default base URL not used")
	}
	t.Setenv("WEGLIDE_ALLOW_TEST_URL", "true")
	if cfg := ConfigFromEnv(); cfg.BaseURL != "http://127.0.0.1:1" || cfg.FilesURL != "http://127.0.0.1:2" {
		t.Fatalf("override not applied: %+v", cfg)
	}
}

func TestValidKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"wg_abc", true},
		{"", false},
		{"a b", false},
		{"line\nbreak", false},
		{strings.Repeat("k", MaxKeyLen), true},
		{strings.Repeat("k", MaxKeyLen+1), false},
	}
	for _, tt := range tests {
		if got := ValidKey(tt.key); got != tt.want {
			t.Errorf("ValidKey(%q) = %v", tt.key, got)
		}
	}
}
