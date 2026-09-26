//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const e2eWeGlideKey = "wg_e2e_petra_key"

// fakeWeGlide stands in for api.weglide.org and its IGC files host. The API
// under test reaches it through WEGLIDE_API_URL / WEGLIDE_FILES_URL, set
// together with WEGLIDE_ALLOW_TEST_URL=true, and the test binds it to
// E2E_WEGLIDE_FAKE_ADDR.
type fakeWeGlide struct {
	srv          *httptest.Server
	detailCalls  atomic.Int32
	keyOnFiles   atomic.Bool
	flightsByIGC map[string][]byte
}

func startFakeWeGlide(t *testing.T, addr string, flights map[string][]byte) *fakeWeGlide {
	t.Helper()
	f := &fakeWeGlide{flightsByIGC: flights}
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-API-Key") != e2eWeGlideKey {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"detail":"Invalid API key"}`))
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/v1/user/me", auth(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": 4711, "name": "Petra Example"}`))
	}))
	mux.HandleFunc("/v1/flight", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user_id_in") != "4711" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		day := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")
		rows := []map[string]any{}
		for i, id := range []int{9001, 9002} {
			rows = append(rows, map[string]any{
				"id": id, "user": map[string]any{"id": 4711}, "scoring_date": day,
				"takeoff_time": fmt.Sprintf("%sT1%d:00:00Z", day, i),
			})
		}
		_ = json.NewEncoder(w).Encode(rows)
	}))
	mux.HandleFunc("/v1/flightdetail/", auth(func(w http.ResponseWriter, r *http.Request) {
		f.detailCalls.Add(1)
		id := strings.TrimPrefix(r.URL.Path, "/v1/flightdetail/")
		_, _ = fmt.Fprintf(w, `{"id": %s, "igc_file": {"id": 1, "file": "igc/2026/%s.igc"}}`, id, id)
	}))
	mux.HandleFunc("/files/igc/2026/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "" {
			f.keyOnFiles.Store(true)
		}
		data, ok := f.flightsByIGC[strings.TrimPrefix(r.URL.Path, "/files/igc/2026/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	})
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen on E2E_WEGLIDE_FAKE_ADDR %s: %v", addr, err)
	}
	f.srv = httptest.NewUnstartedServer(mux)
	_ = f.srv.Listener.Close()
	f.srv.Listener = ln
	f.srv.Start()
	t.Cleanup(f.srv.Close)
	return f
}

type weglideStatus struct {
	Linked            bool    `json:"linked"`
	WeglideUserID     *string `json:"weglideUserId"`
	LastSyncAt        *string `json:"lastSyncAt"`
	LastSyncStatus    *string `json:"lastSyncStatus"`
	LastSyncError     *string `json:"lastSyncError"`
	RequestsUsedToday int     `json:"requestsUsedToday"`
	RequestsPerDay    int     `json:"requestsPerDay"`
}

type weglideSync struct {
	Imported          int `json:"imported"`
	Skipped           int `json:"skipped"`
	Remaining         int `json:"remaining"`
	RequestsUsedToday int `json:"requestsUsedToday"`
}

func TestWeGlideLink(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("weglide"), "SecurePass123!", "Petra Example")

	probe := c.GET("/integrations/weglide")
	if probe.StatusCode == http.StatusServiceUnavailable {
		t.Skip("WeGlide link not configured on this server (BACKUP_CREDENTIALS_KEY unset)")
	}

	t.Run("unauthenticated is 401", func(t *testing.T) {
		anon := NewE2EClient(t)
		assertStatus(t, anon.GET("/integrations/weglide"), http.StatusUnauthorized)
		assertStatus(t, anon.POST("/integrations/weglide/sync", nil), http.StatusUnauthorized)
	})

	t.Run("unlinked status never carries a key", func(t *testing.T) {
		requireStatus(t, probe, http.StatusOK)
		var st weglideStatus
		probe.JSON(&st)
		if st.Linked || st.RequestsPerDay != 60 || st.RequestsUsedToday != 0 {
			t.Fatalf("status = %+v", st)
		}
	})

	t.Run("malformed key is 400 without calling WeGlide", func(t *testing.T) {
		assertStatus(t, c.PUT("/integrations/weglide", map[string]string{"apiKey": "has a space"}), http.StatusBadRequest)
		assertStatus(t, c.PUT("/integrations/weglide", map[string]string{}), http.StatusBadRequest)
	})

	t.Run("sync without a link is 404", func(t *testing.T) {
		assertStatus(t, c.POST("/integrations/weglide/sync", nil), http.StatusNotFound)
	})

	t.Run("unlink without a link is 204", func(t *testing.T) {
		assertStatus(t, c.DELETE("/integrations/weglide"), http.StatusNoContent)
	})

	addr := os.Getenv("E2E_WEGLIDE_FAKE_ADDR")
	if addr == "" {
		t.Log("E2E_WEGLIDE_FAKE_ADDR unset: skipping link and sync against the fake WeGlide")
		return
	}
	fake := startFakeWeGlide(t, addr, map[string][]byte{
		"9001.igc": igcFixture(t, "winch.igc"),
		"9002.igc": igcFixture(t, "selflaunch.igc"),
	})

	t.Run("key WeGlide rejects is 400", func(t *testing.T) {
		assertStatus(t, c.PUT("/integrations/weglide", map[string]string{"apiKey": "wg_wrong"}), http.StatusBadRequest)
		var st weglideStatus
		c.GET("/integrations/weglide").JSON(&st)
		if st.Linked {
			t.Fatal("rejected key was linked")
		}
	})

	t.Run("P4 Petra links her WeGlide key", func(t *testing.T) {
		resp := c.PUT("/integrations/weglide", map[string]string{"apiKey": e2eWeGlideKey})
		requireStatus(t, resp, http.StatusOK)
		if strings.Contains(string(resp.Body), e2eWeGlideKey) {
			t.Fatal("key echoed in the response")
		}
		var st weglideStatus
		resp.JSON(&st)
		if !st.Linked || st.WeglideUserID == nil || *st.WeglideUserID != "4711" || st.RequestsUsedToday != 1 {
			t.Fatalf("status = %+v", st)
		}
	})

	t.Run("P4 first sync imports her two WeGlide flights with their IGC files", func(t *testing.T) {
		resp := c.POST("/integrations/weglide/sync", nil)
		requireStatus(t, resp, http.StatusOK)
		var res weglideSync
		resp.JSON(&res)
		if res.Imported != 2 || res.Skipped != 0 || res.Remaining != 0 || res.RequestsUsedToday != 4 {
			t.Fatalf("sync = %+v", res)
		}
		if fake.keyOnFiles.Load() {
			t.Error("API key sent to the IGC files host")
		}

		var list struct {
			Data []map[string]interface{} `json:"data"`
		}
		flightsResp := c.GET("/flights?pageSize=50")
		requireStatus(t, flightsResp, http.StatusOK)
		flightsResp.JSON(&list)
		regs := map[string]string{}
		for _, f := range list.Data {
			regs[f["aircraftReg"].(string)] = f["id"].(string)
		}
		if len(list.Data) != 2 || regs["D-1234"] == "" || regs["D-KXYZ"] == "" {
			t.Fatalf("flights = %v", regs)
		}
		var files []map[string]interface{}
		c.GET("/flights/" + regs["D-1234"] + "/files").JSON(&files)
		if len(files) != 1 || files[0]["filename"] != "weglide-9001.igc" {
			t.Fatalf("files = %v", files)
		}
	})

	t.Run("second sync imports nothing and spends no download", func(t *testing.T) {
		before := fake.detailCalls.Load()
		resp := c.POST("/integrations/weglide/sync", nil)
		requireStatus(t, resp, http.StatusOK)
		var res weglideSync
		resp.JSON(&res)
		if res.Imported != 0 || res.Skipped != 0 || res.Remaining != 0 || res.RequestsUsedToday != 5 {
			t.Fatalf("re-sync = %+v", res)
		}
		if fake.detailCalls.Load() != before {
			t.Error("already imported flights were downloaded again")
		}
		var st weglideStatus
		c.GET("/integrations/weglide").JSON(&st)
		if st.LastSyncAt == nil || st.LastSyncStatus == nil || *st.LastSyncStatus != "ok" || st.LastSyncError != nil {
			t.Fatalf("status = %+v", st)
		}
	})

	t.Run("an IGC file uploaded by hand is skipped, not duplicated", func(t *testing.T) {
		other := NewE2EClient(t)
		registerAndLogin(t, other, uniqueEmail("weglide-manual"), "SecurePass123!", "Petra Example")
		requireStatus(t, postIGC(t, other, "/flights/igc", "manual.igc", igcFixture(t, "winch.igc"), nil), http.StatusCreated)
		requireStatus(t, other.PUT("/integrations/weglide", map[string]string{"apiKey": e2eWeGlideKey}), http.StatusOK)
		resp := other.POST("/integrations/weglide/sync", nil)
		requireStatus(t, resp, http.StatusOK)
		var res weglideSync
		resp.JSON(&res)
		if res.Imported != 1 || res.Skipped != 1 {
			t.Fatalf("sync = %+v", res)
		}
	})

	t.Run("unlink keeps the imported flights", func(t *testing.T) {
		assertStatus(t, c.DELETE("/integrations/weglide"), http.StatusNoContent)
		var st weglideStatus
		c.GET("/integrations/weglide").JSON(&st)
		if st.Linked {
			t.Fatal("still linked")
		}
		var list struct {
			Data []map[string]interface{} `json:"data"`
		}
		c.GET("/flights?pageSize=50").JSON(&list)
		if len(list.Data) != 2 {
			t.Fatalf("flights after unlink = %d", len(list.Data))
		}
	})

	t.Run("the key is not in the JSON export", func(t *testing.T) {
		requireStatus(t, c.PUT("/integrations/weglide", map[string]string{"apiKey": e2eWeGlideKey}), http.StatusOK)
		resp := c.GET("/exports/json")
		requireStatus(t, resp, http.StatusOK)
		if strings.Contains(string(resp.Body), e2eWeGlideKey) || strings.Contains(strings.ToLower(string(resp.Body)), "weglidelink") {
			t.Fatal("WeGlide link material in the export")
		}
	})
}
