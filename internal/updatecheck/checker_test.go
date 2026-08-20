package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// releaseServer answers the GitHub latest-release endpoint for both repos.
func releaseServer(t *testing.T, byRepo map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/releases/latest")
		body, ok := byRepo[repo]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func release(tag string) string {
	return `{"tag_name":"` + tag + `","html_url":"https://example.test/releases/` + tag +
		`","published_at":"2026-08-18T20:32:52Z"}`
}

func newTestChecker(t *testing.T, baseURL, apiVersion string) *Checker {
	t.Helper()
	return New(Config{
		Enabled:      true,
		APIRepo:      "owner/api",
		FrontendRepo: "owner/frontend",
		APIVersion:   apiVersion,
		BaseURL:      baseURL,
	})
}

func componentByName(t *testing.T, status Status, name string) ComponentStatus {
	t.Helper()
	for _, c := range status.Components {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("component %q missing from status %+v", name, status)
	return ComponentStatus{}
}

func TestRefreshReportsUpdateAvailable(t *testing.T) {
	srv, _ := releaseServer(t, map[string]string{
		"owner/api":      release("v1.3.5"),
		"owner/frontend": release("v1.3.2"),
	})
	checker := newTestChecker(t, srv.URL, "v1.3.4")

	if err := checker.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	status := checker.Status("v1.3.2")
	if !status.UpdateAvailable {
		t.Error("UpdateAvailable = false, want true")
	}
	if status.LastCheckedAt == nil {
		t.Error("LastCheckedAt is nil after a successful check")
	}
	if status.LastError != "" {
		t.Errorf("LastError = %q, want empty", status.LastError)
	}

	api := componentByName(t, status, ComponentAPI)
	if api.State != StateUpdateAvailable {
		t.Errorf("api state = %q, want %q", api.State, StateUpdateAvailable)
	}
	if api.LatestVersion != "v1.3.5" || api.CurrentVersion != "v1.3.4" {
		t.Errorf("api versions = %q → %q, want v1.3.4 → v1.3.5", api.CurrentVersion, api.LatestVersion)
	}
	if api.ReleaseURL != "https://example.test/releases/v1.3.5" {
		t.Errorf("api release URL = %q", api.ReleaseURL)
	}
	if api.PublishedAt == nil || api.PublishedAt.Year() != 2026 {
		t.Errorf("api publishedAt = %v", api.PublishedAt)
	}

	frontend := componentByName(t, status, ComponentFrontend)
	if frontend.State != StateUpToDate {
		t.Errorf("frontend state = %q, want %q", frontend.State, StateUpToDate)
	}
}

func TestStatusUpToDateWhenBothCurrent(t *testing.T) {
	srv, _ := releaseServer(t, map[string]string{
		"owner/api":      release("v1.3.4"),
		"owner/frontend": release("v1.3.2"),
	})
	checker := newTestChecker(t, srv.URL, "v1.3.4")
	if err := checker.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	status := checker.Status("v1.3.2")
	if status.UpdateAvailable {
		t.Error("UpdateAvailable = true, want false")
	}
}

func TestStatusUnknownForUnstampedVersions(t *testing.T) {
	srv, _ := releaseServer(t, map[string]string{
		"owner/api":      release("v1.3.5"),
		"owner/frontend": release("v1.3.2"),
	})
	checker := newTestChecker(t, srv.URL, DevVersion)
	if err := checker.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	status := checker.Status("")
	if status.UpdateAvailable {
		t.Error("UpdateAvailable = true for unstamped builds, want false")
	}
	for _, name := range []string{ComponentAPI, ComponentFrontend} {
		component := componentByName(t, status, name)
		if component.State != StateUnknown {
			t.Errorf("%s state = %q, want %q", name, component.State, StateUnknown)
		}
		if component.LatestVersion == "" {
			t.Errorf("%s latest version is empty; the release is known even when the running version is not", name)
		}
	}
}

func TestStatusBeforeFirstCheck(t *testing.T) {
	checker := newTestChecker(t, "https://example.invalid", "v1.3.4")

	status := checker.Status("v1.3.2")
	if !status.Enabled {
		t.Error("Enabled = false, want true")
	}
	if status.UpdateAvailable {
		t.Error("UpdateAvailable = true before any check ran")
	}
	if status.LastCheckedAt != nil {
		t.Error("LastCheckedAt set before any check ran")
	}
	for _, component := range status.Components {
		if component.State != StateUnknown {
			t.Errorf("%s state = %q, want %q", component.Name, component.State, StateUnknown)
		}
	}
}

func TestDisabledCheckerPerformsNoRequest(t *testing.T) {
	srv, calls := releaseServer(t, map[string]string{"owner/api": release("v1.3.5")})
	checker := New(Config{
		Enabled: false, APIRepo: "owner/api", FrontendRepo: "owner/frontend",
		APIVersion: "v1.3.4", BaseURL: srv.URL,
	})

	if err := checker.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh on a disabled checker: %v", err)
	}
	checker.Start(context.Background())
	time.Sleep(50 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Errorf("disabled checker made %d requests, want 0", got)
	}
	status := checker.Status("v1.3.2")
	if status.Enabled || status.UpdateAvailable {
		t.Errorf("disabled status = %+v", status)
	}
}

func TestRefreshRecordsFailureReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	checker := newTestChecker(t, srv.URL, "v1.3.4")
	if err := checker.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh returned nil on a 403 answer")
	}

	status := checker.Status("v1.3.2")
	if status.LastError != "status" {
		t.Errorf("LastError = %q, want %q", status.LastError, "status")
	}
	if status.LastCheckedAt == nil {
		t.Error("LastCheckedAt is nil after a failed check")
	}
	if status.UpdateAvailable {
		t.Error("UpdateAvailable = true after a failed check")
	}
}

func TestRefreshKeepsLastKnownReleaseOnFailure(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(release("v1.3.5")))
	}))
	defer srv.Close()

	checker := newTestChecker(t, srv.URL, "v1.3.4")
	if err := checker.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	fail.Store(true)
	if err := checker.Refresh(context.Background()); err == nil {
		t.Fatal("second Refresh returned nil while the upstream was failing")
	}

	api := componentByName(t, checker.Status(""), ComponentAPI)
	if api.LatestVersion != "v1.3.5" || api.State != StateUpdateAvailable {
		t.Errorf("api component after a failed refresh = %+v, want the previously known v1.3.5", api)
	}
}

func TestRefreshRejectsReleaseWithoutTag(t *testing.T) {
	srv, _ := releaseServer(t, map[string]string{
		"owner/api":      `{"tag_name":"","html_url":"https://example.test"}`,
		"owner/frontend": release("v1.3.2"),
	})
	checker := newTestChecker(t, srv.URL, "v1.3.4")

	if err := checker.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh returned nil for a release with no tag")
	}
	if got := checker.Status("").LastError; got != "empty" {
		t.Errorf("LastError = %q, want %q", got, "empty")
	}
}

func TestStatusSanitizesReportedFrontendVersion(t *testing.T) {
	checker := newTestChecker(t, "https://example.invalid", "v1.3.4")

	frontend := componentByName(t, checker.Status("  v1.3.2\n\x00"+strings.Repeat("x", 200)), ComponentFrontend)
	if len(frontend.CurrentVersion) > maxReportedVersionLen {
		t.Errorf("CurrentVersion length = %d, want <= %d", len(frontend.CurrentVersion), maxReportedVersionLen)
	}
	if strings.ContainsAny(frontend.CurrentVersion, "\n\x00") {
		t.Errorf("CurrentVersion = %q, want control characters removed", frontend.CurrentVersion)
	}
}

func TestStartRunsOneCheckWhenRefreshDisabled(t *testing.T) {
	srv, calls := releaseServer(t, map[string]string{
		"owner/api":      release("v1.3.5"),
		"owner/frontend": release("v1.3.2"),
	})
	checker := New(Config{
		Enabled: true, Interval: 0, APIRepo: "owner/api", FrontendRepo: "owner/frontend",
		APIVersion: "v1.3.4", BaseURL: srv.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if checker.Status("").LastCheckedAt != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("startup check made %d requests, want 2 (one per component)", got)
	}
}

func TestRepoFromEnv(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "", want: DefaultAPIRepo},
		{value: "acme/ninerlog-api", want: "acme/ninerlog-api"},
		{value: "  acme/fork  ", want: "acme/fork"},
		{value: "acme/api/releases", want: DefaultAPIRepo},
		{value: "../../etc", want: DefaultAPIRepo},
		{value: "acme", want: DefaultAPIRepo},
		{value: "acme/api?x=1", want: DefaultAPIRepo},
	}

	for _, tt := range tests {
		t.Setenv("UPDATE_CHECK_API_REPO", tt.value)
		if got := repoFromEnv("UPDATE_CHECK_API_REPO", DefaultAPIRepo); got != tt.want {
			t.Errorf("repoFromEnv(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestIntervalFromEnv(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{value: "", want: DefaultInterval},
		{value: "6h", want: 6 * time.Hour},
		{value: "off", want: 0},
		{value: "OFF", want: 0},
		{value: "nonsense", want: DefaultInterval},
	}

	for _, tt := range tests {
		t.Setenv("UPDATE_CHECK_INTERVAL", tt.value)
		if got := intervalFromEnv(); got != tt.want {
			t.Errorf("intervalFromEnv(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestFromEnvDefaultsToEnabled(t *testing.T) {
	t.Setenv("UPDATE_CHECK_ENABLED", "")
	if !FromEnv().Enabled {
		t.Error("Enabled = false with UPDATE_CHECK_ENABLED unset, want true")
	}
	t.Setenv("UPDATE_CHECK_ENABLED", "false")
	if FromEnv().Enabled {
		t.Error("Enabled = true with UPDATE_CHECK_ENABLED=false")
	}
}
