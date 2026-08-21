// Package updatecheck reports whether a newer NinerLog build has been
// published. A component carrying a semantic version is compared against the
// newest GitHub release of its repository; one carrying only a build commit —
// what the :latest images have — is compared against the head of the tracked
// branch. A component with neither reports StateUnknown.
//
// The API knows its own version and commit from link-time stamps; the
// frontend's are supplied by the browser with the request.
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

// How a component's state was determined.
const (
	// ChannelRelease compares a semantic version against the newest release.
	ChannelRelease = "release"
	// ChannelCommit compares a build commit against the tracked branch, which
	// is what a deployment running the :latest tag gets.
	ChannelCommit = "commit"
)

// Defaults applied when the corresponding environment variable is unset.
const (
	DefaultInterval     = 24 * time.Hour
	DefaultAPIRepo      = "fjaeckel/ninerlog-api"
	DefaultFrontendRepo = "fjaeckel/ninerlog-frontend"
	// DefaultBranch is the branch a :latest image is built from.
	DefaultBranch = "main"

	githubAPIBase = "https://api.github.com"
	// requestTimeout covers one lookup, body included.
	requestTimeout = 15 * time.Second
	// maxBodyBytes caps how much of a response is read.
	maxBodyBytes = 1 << 20
	// maxReportedVersionLen bounds a version string echoed back to the caller.
	maxReportedVersionLen = 64
	// maxComparisons bounds the commit comparison cache.
	maxComparisons = 16
	// comparisonRetryAfter is the minimum gap between lookups of the same
	// commit.
	comparisonRetryAfter = 15 * time.Minute
)

// Commit comparison outcomes, as reported by GitHub for base...head.
const (
	compareIdentical = "identical"
	compareAhead     = "ahead"
	compareBehind    = "behind"
	compareDiverged  = "diverged"
)

// repoPattern matches an "owner/name" GitHub repository path.
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}/[A-Za-z0-9._-]{1,100}$`)

// branchPattern matches a git branch name safe to place in a URL path.
var branchPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,255}$`)

// commitPattern matches an abbreviated or full commit SHA.
var commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// normalizeCommit lower-cases a commit SHA and reports whether it is one.
func normalizeCommit(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "sha-")
	if !commitPattern.MatchString(s) {
		return "", false
	}
	return s, true
}

// shortCommit is the 7-character form used for display.
func shortCommit(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

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
	// Branch is the branch the :latest images are built from; commit
	// comparisons are made against its head.
	Branch string
	// APIVersion is the running version of this binary.
	APIVersion string
	// APICommit is the commit this binary was built from, empty when unstamped.
	APICommit string
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

// Comparison is one build commit measured against the tracked branch head.
type Comparison struct {
	// Status is identical, ahead (the branch has moved on), behind (the build
	// is ahead of the branch) or diverged.
	Status string
	// BehindBy is how many commits the branch is ahead of this build.
	BehindBy int
	URL      string
	// CheckedAt is when the comparison was made.
	CheckedAt time.Time
}

// ComponentStatus is one component's running build measured against the newest
// published release, or against the tracked branch when it carries no
// semantic version.
type ComponentStatus struct {
	Name           string
	CurrentVersion string
	// CurrentCommit is the short commit the component was built from, empty
	// when it reported none.
	CurrentCommit string
	LatestVersion string
	State         string
	// Channel is how State was reached: ChannelRelease or ChannelCommit.
	// Empty when neither comparison could be made.
	Channel     string
	ReleaseURL  string
	PublishedAt *time.Time
	// BehindBy is how many commits behind the tracked branch this build is,
	// set only on ChannelCommit.
	BehindBy int
	// CompareURL is the GitHub comparison between this build and the branch
	// head, set only on ChannelCommit.
	CompareURL string
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
	LastError string
	// Branch is the branch commit comparisons are made against.
	Branch     string
	Components []ComponentStatus
}

// Checker holds the last known releases and commit comparisons and refreshes
// them on a timer.
type Checker struct {
	cfg Config

	mu          sync.RWMutex
	releases    map[string]Release
	comparisons map[string]Comparison
	// inflight guards against piling up lookups for the same commit.
	inflight map[string]bool
	// frontendCommit is the commit last reported by a browser, kept warm by
	// the periodic refresh.
	frontendCommit string
	lastChecked    time.Time
	lastError      string

	// bgCtx bounds the on-demand comparison lookups; set by Start.
	bgCtx context.Context
}

// comparisonKey identifies a cached comparison.
func comparisonKey(component, sha string) string { return component + "@" + sha }

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
	if cfg.Branch == "" {
		cfg.Branch = DefaultBranch
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = githubAPIBase
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: requestTimeout}
	}
	if sha, ok := normalizeCommit(cfg.APICommit); ok {
		cfg.APICommit = sha
	} else {
		cfg.APICommit = ""
	}
	return &Checker{
		cfg:         cfg,
		releases:    map[string]Release{},
		comparisons: map[string]Comparison{},
		inflight:    map[string]bool{},
		bgCtx:       context.Background(),
	}
}

// FromEnv builds a Config from UPDATE_CHECK_ENABLED, UPDATE_CHECK_INTERVAL,
// UPDATE_CHECK_API_REPO, UPDATE_CHECK_FRONTEND_REPO and UPDATE_CHECK_BRANCH.
func FromEnv() Config {
	return Config{
		Enabled:      os.Getenv("UPDATE_CHECK_ENABLED") != "false",
		Interval:     intervalFromEnv(),
		APIRepo:      repoFromEnv("UPDATE_CHECK_API_REPO", DefaultAPIRepo),
		FrontendRepo: repoFromEnv("UPDATE_CHECK_FRONTEND_REPO", DefaultFrontendRepo),
		Branch:       branchFromEnv(),
		APIVersion:   RunningVersion(),
		APICommit:    RunningCommit(),
	}
}

// branchFromEnv reads UPDATE_CHECK_BRANCH, defaulting to DefaultBranch.
func branchFromEnv() string {
	val := strings.TrimSpace(os.Getenv("UPDATE_CHECK_BRANCH"))
	if val == "" {
		return DefaultBranch
	}
	if !branchPattern.MatchString(val) {
		slog.Warn("Invalid UPDATE_CHECK_BRANCH, using default", "value", val, "default", DefaultBranch)
		return DefaultBranch
	}
	return val
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

	c.mu.Lock()
	c.bgCtx = ctx
	c.mu.Unlock()

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

// Refresh looks up the newest release of every component, and the branch
// position of every build commit it knows about, and stores them. A lookup
// that fails leaves the previously known answer in place.
func (c *Checker) Refresh(ctx context.Context) error {
	if !c.cfg.Enabled {
		return nil
	}

	start := time.Now()

	var firstErr error
	record := func(component string, err error) {
		ErrorsTotal.WithLabelValues(reasonOf(err)).Inc()
		if firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", component, err)
		}
	}

	for _, component := range []string{ComponentAPI, ComponentFrontend} {
		release, err := c.latestRelease(ctx, c.repoOf(component))
		if err != nil {
			record(component, err)
			continue
		}
		c.mu.Lock()
		c.releases[component] = release
		c.mu.Unlock()
		LatestVersionInfo.DeletePartialMatch(prometheus.Labels{"component": component})
		LatestVersionInfo.WithLabelValues(component, release.Version).Set(1)
	}

	for component, sha := range c.trackedCommits() {
		if err := c.refreshComparison(ctx, component, sha); err != nil {
			record(component, err)
		}
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

// repoOf is the repository a component's releases and commits are read from.
func (c *Checker) repoOf(component string) string {
	if component == ComponentFrontend {
		return c.cfg.FrontendRepo
	}
	return c.cfg.APIRepo
}

// trackedCommits is every build commit worth keeping a comparison for: this
// binary's own, and the one a browser reported last.
func (c *Checker) trackedCommits() map[string]string {
	tracked := map[string]string{}
	if c.cfg.APICommit != "" {
		tracked[ComponentAPI] = c.cfg.APICommit
	}
	c.mu.RLock()
	frontendCommit := c.frontendCommit
	c.mu.RUnlock()
	if frontendCommit != "" {
		tracked[ComponentFrontend] = frontendCommit
	}
	return tracked
}

// refreshComparison looks a commit's branch position up and caches it.
func (c *Checker) refreshComparison(ctx context.Context, component, sha string) error {
	comparison, err := c.compareCommit(ctx, c.repoOf(component), sha)
	if err != nil {
		return err
	}
	c.storeComparison(component, sha, comparison)
	return nil
}

// storeComparison caches a comparison, evicting the oldest entry once the
// cache is full.
func (c *Checker) storeComparison(component, sha string, comparison Comparison) {
	key := comparisonKey(component, sha)

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.comparisons[key]; !exists && len(c.comparisons) >= maxComparisons {
		oldestKey, oldest := "", time.Time{}
		for k, v := range c.comparisons {
			if oldest.IsZero() || v.CheckedAt.Before(oldest) {
				oldestKey, oldest = k, v.CheckedAt
			}
		}
		delete(c.comparisons, oldestKey)
	}
	c.comparisons[key] = comparison
}

// publishAPIGauge sets app_update_available for the API, whose running version
// is known server-side.
func (c *Checker) publishAPIGauge() {
	status := c.componentStatus(ComponentAPI, c.cfg.APIVersion, c.cfg.APICommit)
	switch status.State {
	case StateUpdateAvailable:
		UpdateAvailable.WithLabelValues(ComponentAPI).Set(1)
	case StateUpToDate:
		UpdateAvailable.WithLabelValues(ComponentAPI).Set(0)
	default:
		UpdateAvailable.DeleteLabelValues(ComponentAPI)
	}

	if status.Channel == ChannelCommit {
		CommitsBehind.WithLabelValues(ComponentAPI).Set(float64(status.BehindBy))
	} else {
		CommitsBehind.DeleteLabelValues(ComponentAPI)
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

// compareCommit reads how far the tracked branch has moved past a build
// commit. Errors carry the same reasons as a release lookup.
func (c *Checker) compareCommit(ctx context.Context, repo, sha string) (Comparison, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/repos/%s/compare/%s...%s",
		strings.TrimSuffix(c.cfg.BaseURL, "/"), repo, sha, c.cfg.Branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Comparison{}, failure("request", "build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ninerlog-api/"+c.cfg.APIVersion)

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return Comparison{}, failure("request", "compare %s: %w", repo, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Comparison{}, failure("status", "compare %s: status %d", repo, resp.StatusCode)
	}

	var payload struct {
		Status  string `json:"status"`
		AheadBy int    `json:"ahead_by"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&payload); err != nil {
		return Comparison{}, failure("decode", "decode comparison for %s: %w", repo, err)
	}
	if payload.Status == "" {
		return Comparison{}, failure("empty", "compare %s: no status reported", repo)
	}

	return Comparison{
		Status:    payload.Status,
		BehindBy:  payload.AheadBy,
		URL:       payload.HTMLURL,
		CheckedAt: time.Now(),
	}, nil
}

// ensureComparison starts a background lookup for a commit that has no usable
// cached comparison, at most one at a time per commit.
func (c *Checker) ensureComparison(component, sha string) {
	if !c.cfg.Enabled {
		return
	}
	key := comparisonKey(component, sha)

	c.mu.Lock()
	comparison, cached := c.comparisons[key]
	fresh := cached && time.Since(comparison.CheckedAt) < comparisonRetryAfter
	if fresh || c.inflight[key] {
		c.mu.Unlock()
		return
	}
	c.inflight[key] = true
	ctx := c.bgCtx
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			delete(c.inflight, key)
			c.mu.Unlock()
		}()
		if err := c.refreshComparison(ctx, component, sha); err != nil {
			ErrorsTotal.WithLabelValues(reasonOf(err)).Inc()
			slog.Warn("Commit comparison failed", "component", component, "error", err)
		}
	}()
}

// Status reports every component against the last known releases, falling back
// to the branch position of its build commit. frontendVersion and
// frontendCommit describe the calling browser's build; a component that
// reports neither a semantic version nor a commit yields StateUnknown.
func (c *Checker) Status(frontendVersion, frontendCommit string) Status {
	frontendSHA, _ := normalizeCommit(frontendCommit)
	if frontendSHA != "" {
		c.rememberFrontendCommit(frontendSHA)
	}

	c.mu.RLock()
	lastChecked, lastError := c.lastChecked, c.lastError
	c.mu.RUnlock()

	status := Status{
		Enabled:   c.cfg.Enabled,
		LastError: lastError,
		Branch:    c.cfg.Branch,
		Components: []ComponentStatus{
			c.componentStatus(ComponentAPI, c.cfg.APIVersion, c.cfg.APICommit),
			c.componentStatus(ComponentFrontend, frontendVersion, frontendSHA),
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

// rememberFrontendCommit keeps the browser-reported commit for the periodic
// refresh.
func (c *Checker) rememberFrontendCommit(sha string) {
	c.mu.Lock()
	c.frontendCommit = sha
	c.mu.Unlock()
}

// componentStatus compares one running build against the component's last
// known release, falling back to the branch position of its commit when the
// running version is not a semantic version.
func (c *Checker) componentStatus(name, current, commit string) ComponentStatus {
	current = sanitizeVersion(current)
	status := ComponentStatus{
		Name:           name,
		CurrentVersion: current,
		CurrentCommit:  shortCommit(commit),
		State:          StateUnknown,
	}

	c.mu.RLock()
	release, releaseKnown := c.releases[name]
	comparison, comparisonKnown := c.comparisons[comparisonKey(name, commit)]
	c.mu.RUnlock()

	if releaseKnown {
		status.LatestVersion = release.Version
		status.ReleaseURL = release.URL
		if !release.PublishedAt.IsZero() {
			published := release.PublishedAt
			status.PublishedAt = &published
		}

		if behind, comparable := newer(current, release.Version); comparable {
			status.Channel = ChannelRelease
			if behind {
				status.State = StateUpdateAvailable
			} else {
				status.State = StateUpToDate
			}
			return status
		}
	}

	if commit == "" {
		return status
	}

	if !comparisonKnown {
		c.ensureComparison(name, commit)
		return status
	}

	status.CompareURL = comparison.URL
	switch comparison.Status {
	case compareAhead:
		status.Channel = ChannelCommit
		status.State = StateUpdateAvailable
		status.BehindBy = comparison.BehindBy
	case compareIdentical, compareBehind:
		status.Channel = ChannelCommit
		status.State = StateUpToDate
	default:
		status.CompareURL = ""
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
