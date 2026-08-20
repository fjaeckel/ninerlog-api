// Package updatecheck reports whether a newer NinerLog release has been
// published, by comparing the running component versions against the latest
// GitHub release of each component's repository.
//
// The API knows its own version from the link-time stamp; the frontend's
// version is supplied by the browser with the request. A component whose
// running version is not a semantic version reports StateUnknown.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Component names used in the API response and in metric labels.
const (
	ComponentAPI      = "api"
	ComponentFrontend = "frontend"
)

// Component states.
const (
	StateUpToDate        = "up_to_date"
	StateUpdateAvailable = "update_available"
	StateUnknown         = "unknown"
)

// Defaults applied when the corresponding environment variable is unset.
const (
	DefaultInterval     = 24 * time.Hour
	DefaultAPIRepo      = "fjaeckel/ninerlog-api"
	DefaultFrontendRepo = "fjaeckel/ninerlog-frontend"

	githubAPIBase = "https://api.github.com"
	// requestTimeout covers one release lookup, body included.
	requestTimeout = 15 * time.Second
	// maxBodyBytes caps how much of a release response is read.
	maxBodyBytes = 1 << 20
	// maxReportedVersionLen bounds a version string echoed back to the caller.
	maxReportedVersionLen = 64
)

// repoPattern matches an "owner/name" GitHub repository path.
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}/[A-Za-z0-9._-]{1,100}$`)

// Config configures a Checker.
type Config struct {
	// Enabled is false when the deployment opted out of the check; a disabled
	// Checker performs no outbound request and reports StateUnknown.
	Enabled bool
	// Interval between checks. Zero or less disables the periodic refresh,
	// leaving only the check performed at startup.
	Interval time.Duration
	// APIRepo and FrontendRepo are "owner/name" GitHub repository paths.
	APIRepo      string
	FrontendRepo string
	// APIVersion is the running version of this binary.
	APIVersion string
	// BaseURL is the GitHub API root.
	BaseURL string
	// HTTPClient performs the release lookups.
	HTTPClient *http.Client
}

// Release is the newest published release of one component's repository.
type Release struct {
	Version     string
	URL         string
	PublishedAt time.Time
}

// ComponentStatus is one component's running version measured against the
// newest published release.
type ComponentStatus struct {
	Name           string
	CurrentVersion string
	LatestVersion  string
	State          string
	ReleaseURL     string
	PublishedAt    *time.Time
}

// Status is the result of the most recent check, evaluated against the
// versions currently running.
type Status struct {
	Enabled bool
	// UpdateAvailable is true when any component is behind its newest
	// published release.
	UpdateAvailable bool
	LastCheckedAt   *time.Time
	// LastError is a coarse reason for the last failed check, empty when the
	// last check succeeded or none has run.
	LastError  string
	Components []ComponentStatus
}

// Checker holds the last known releases and refreshes them on a timer.
type Checker struct {
	cfg Config

	mu          sync.RWMutex
	releases    map[string]Release
	lastChecked time.Time
	lastError   string
}

// fetchError carries the failure reason used as a metric label.
type fetchError struct {
	reason string
	err    error
}

func (e *fetchError) Error() string { return fmt.Sprintf("%s: %v", e.reason, e.err) }
func (e *fetchError) Unwrap() error { return e.err }

func failure(reason, format string, args ...any) *fetchError {
	return &fetchError{reason: reason, err: fmt.Errorf(format, args...)}
}

// reasonOf returns the metric label for a check failure.
func reasonOf(err error) string {
	var fe *fetchError
	if errors.As(err, &fe) {
		return fe.reason
	}
	return "error"
}

// New returns a Checker, filling unset Config fields with their defaults.
func New(cfg Config) *Checker {
	if cfg.APIRepo == "" {
		cfg.APIRepo = DefaultAPIRepo
	}
	if cfg.FrontendRepo == "" {
		cfg.FrontendRepo = DefaultFrontendRepo
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = DevVersion
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = githubAPIBase
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: requestTimeout}
	}
	return &Checker{cfg: cfg, releases: map[string]Release{}}
}

// FromEnv builds a Config from UPDATE_CHECK_ENABLED, UPDATE_CHECK_INTERVAL,
// UPDATE_CHECK_API_REPO and UPDATE_CHECK_FRONTEND_REPO.
func FromEnv() Config {
	return Config{
		Enabled:      os.Getenv("UPDATE_CHECK_ENABLED") != "false",
		Interval:     intervalFromEnv(),
		APIRepo:      repoFromEnv("UPDATE_CHECK_API_REPO", DefaultAPIRepo),
		FrontendRepo: repoFromEnv("UPDATE_CHECK_FRONTEND_REPO", DefaultFrontendRepo),
		APIVersion:   RunningVersion(),
	}
}

// intervalFromEnv reads UPDATE_CHECK_INTERVAL, defaulting to DefaultInterval.
// "off" or a non-positive duration disables the periodic refresh.
func intervalFromEnv() time.Duration {
	val := strings.TrimSpace(os.Getenv("UPDATE_CHECK_INTERVAL"))
	if val == "" {
		return DefaultInterval
	}
	if strings.EqualFold(val, "off") {
		return 0
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		slog.Warn("Invalid UPDATE_CHECK_INTERVAL, using default",
			"value", val, "default", DefaultInterval.String())
		return DefaultInterval
	}
	return d
}

// repoFromEnv reads an "owner/name" repository path, falling back to def when
// unset or malformed.
func repoFromEnv(key, def string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	if !repoPattern.MatchString(val) {
		slog.Warn("Invalid repository path, using default", "var", key, "value", val, "default", def)
		return def
	}
	return val
}

// Enabled reports whether the check runs.
func (c *Checker) Enabled() bool { return c.cfg.Enabled }

// Interval is how often the check repeats. Zero or less means it runs only at
// startup.
func (c *Checker) Interval() time.Duration { return c.cfg.Interval }

// Start runs a check immediately and then every configured interval until ctx
// is done. It returns at once; a disabled Checker starts nothing.
func (c *Checker) Start(ctx context.Context) {
	if !c.cfg.Enabled {
		slog.Info("Update check disabled")
		return
	}

	go func() {
		if err := c.Refresh(ctx); err != nil {
			slog.Warn("Update check failed", "error", err)
		}
		if c.cfg.Interval <= 0 {
			slog.Info("Update check refresher disabled; checked once at startup")
			return
		}
		slog.Info("Update check refresher started", "interval", c.cfg.Interval.String())
		ticker := time.NewTicker(c.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("Update check refresher stopped")
				return
			case <-ticker.C:
				if err := c.Refresh(ctx); err != nil {
					slog.Warn("Update check failed", "error", err)
				}
			}
		}
	}()
}

// Refresh looks up the newest release of every component and stores it. A
// component that fails leaves its previously known release in place.
func (c *Checker) Refresh(ctx context.Context) error {
	if !c.cfg.Enabled {
		return nil
	}

	start := time.Now()
	repos := map[string]string{
		ComponentAPI:      c.cfg.APIRepo,
		ComponentFrontend: c.cfg.FrontendRepo,
	}

	var firstErr error
	for _, component := range []string{ComponentAPI, ComponentFrontend} {
		release, err := c.latestRelease(ctx, repos[component])
		if err != nil {
			ErrorsTotal.WithLabelValues(reasonOf(err)).Inc()
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", component, err)
			}
			continue
		}
		c.mu.Lock()
		c.releases[component] = release
		c.mu.Unlock()
		LatestVersionInfo.DeletePartialMatch(prometheus.Labels{"component": component})
		LatestVersionInfo.WithLabelValues(component, release.Version).Set(1)
	}

	DurationSeconds.Observe(time.Since(start).Seconds())

	c.mu.Lock()
	c.lastChecked = time.Now()
	if firstErr != nil {
		c.lastError = reasonOf(firstErr)
	} else {
		c.lastError = ""
	}
	c.mu.Unlock()

	if firstErr != nil {
		RunsTotal.WithLabelValues("error").Inc()
		return firstErr
	}

	RunsTotal.WithLabelValues("success").Inc()
	LastSuccessTimestampSeconds.SetToCurrentTime()
	c.publishAPIGauge()
	return nil
}

// publishAPIGauge sets app_update_available for the API, whose running version
// is known server-side.
func (c *Checker) publishAPIGauge() {
	status := c.componentStatus(ComponentAPI, c.cfg.APIVersion)
	switch status.State {
	case StateUpdateAvailable:
		UpdateAvailable.WithLabelValues(ComponentAPI).Set(1)
	case StateUpToDate:
		UpdateAvailable.WithLabelValues(ComponentAPI).Set(0)
	default:
		UpdateAvailable.DeleteLabelValues(ComponentAPI)
	}
}

// latestRelease reads the newest published release of an "owner/name"
// repository. Drafts and prereleases are excluded by the endpoint itself.
func (c *Checker) latestRelease(ctx context.Context, repo string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimSuffix(c.cfg.BaseURL, "/"), repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, failure("request", "build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ninerlog-api/"+c.cfg.APIVersion)

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return Release{}, failure("request", "get %s: %w", repo, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Release{}, failure("status", "get %s: status %d", repo, resp.StatusCode)
	}

	var payload struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&payload); err != nil {
		return Release{}, failure("decode", "decode %s: %w", repo, err)
	}

	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return Release{}, failure("empty", "get %s: release has no tag", repo)
	}

	return Release{Version: tag, URL: payload.HTMLURL, PublishedAt: payload.PublishedAt}, nil
}

// Status reports every component against the last known releases.
// frontendVersion is the version the calling browser was built from; an empty
// or unparseable value yields StateUnknown for the frontend.
func (c *Checker) Status(frontendVersion string) Status {
	c.mu.RLock()
	lastChecked, lastError := c.lastChecked, c.lastError
	c.mu.RUnlock()

	status := Status{
		Enabled:   c.cfg.Enabled,
		LastError: lastError,
		Components: []ComponentStatus{
			c.componentStatus(ComponentAPI, c.cfg.APIVersion),
			c.componentStatus(ComponentFrontend, frontendVersion),
		},
	}
	if !lastChecked.IsZero() {
		status.LastCheckedAt = &lastChecked
	}
	for _, component := range status.Components {
		if component.State == StateUpdateAvailable {
			status.UpdateAvailable = true
		}
	}
	return status
}

// componentStatus compares one running version against the component's last
// known release.
func (c *Checker) componentStatus(name, current string) ComponentStatus {
	current = sanitizeVersion(current)
	status := ComponentStatus{Name: name, CurrentVersion: current, State: StateUnknown}

	c.mu.RLock()
	release, known := c.releases[name]
	c.mu.RUnlock()

	if !known {
		return status
	}

	status.LatestVersion = release.Version
	status.ReleaseURL = release.URL
	if !release.PublishedAt.IsZero() {
		published := release.PublishedAt
		status.PublishedAt = &published
	}

	behind, comparable := newer(current, release.Version)
	switch {
	case !comparable:
		status.State = StateUnknown
	case behind:
		status.State = StateUpdateAvailable
	default:
		status.State = StateUpToDate
	}
	return status
}

// sanitizeVersion trims a caller-supplied version to a bounded, printable
// string.
func sanitizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > maxReportedVersionLen {
		v = v[:maxReportedVersionLen]
	}
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, v)
}
