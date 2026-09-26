// Package weglide is the HTTP client for the WeGlide public API, used with a
// pilot's personal API key (header X-API-Key). Every WeGlide endpoint, field
// name and host lives in this package; callers see only the API interface.
//
// Endpoints (see docs/SAILPLANES.md, "WeGlide link"):
//
//	GET /v1/user/me                        the key's own user
//	GET /v1/flight?user_id_in=…            the user's flights, 100 per page
//	GET /v1/flightdetail/{id}              igc_file.file, the IGC path
//	GET {files host}/{igc_file.file}       the IGC bytes, without the key
package weglide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Hosts, limits and defaults.
const (
	DefaultBaseURL  = "https://api.weglide.org"
	DefaultFilesURL = "https://weglidefiles.b-cdn.net"
	// RequestsPerDay is WeGlide's limit per API key and UTC day.
	RequestsPerDay = 60
	// DefaultTimeout bounds one HTTP request.
	DefaultTimeout = 10 * time.Second
	// MaxJSONBytes caps a JSON response body.
	MaxJSONBytes = 2 << 20
	// DefaultMaxIGCBytes caps a downloaded IGC file.
	DefaultMaxIGCBytes = 5 << 20
	// PageSize is the flight list page size.
	PageSize = 100
	// MaxPages caps the pages ListFlights reads in one call.
	MaxPages = 10
	// MaxKeyLen is the longest API key accepted.
	MaxKeyLen = 256
)

// Errors returned by the client. Messages are fixed text and never contain
// upstream response content.
var (
	ErrUnauthorized    = errors.New("WeGlide rejected the API key")
	ErrRateLimited     = errors.New("WeGlide's daily request limit for this key is reached")
	ErrNotFound        = errors.New("WeGlide has no such resource")
	ErrUnavailable     = errors.New("WeGlide could not be reached")
	ErrTooLarge        = errors.New("the WeGlide response exceeds the size limit")
	ErrBadResponse     = errors.New("WeGlide returned an unexpected response")
	ErrBudgetExhausted = errors.New("the daily WeGlide request budget is used up")
	ErrInvalidKey      = errors.New("the API key is malformed")
)

// Budget hands out WeGlide API requests. Take returns ErrBudgetExhausted when
// none is left; it is called once before every request to the API host.
type Budget interface {
	Take(ctx context.Context) error
}

// User is the account an API key belongs to.
type User struct {
	ID   string
	Name string
}

// Flight is one entry of the user's flight list.
type Flight struct {
	ID          int64
	UserID      string
	ScoringDate string
	TakeoffTime time.Time
}

// API is the WeGlide surface NinerLog uses.
type API interface {
	// Me returns the key's user. Costs one request.
	Me(ctx context.Context, key string, b Budget) (*User, error)
	// ListFlights returns userID's flights with a scoring date on or after
	// since, oldest take-off first. Costs one request per page of PageSize.
	// On a budget or request error after the first page it returns the
	// flights read so far together with the error.
	ListFlights(ctx context.Context, key string, b Budget, userID string, since time.Time) ([]Flight, error)
	// DownloadIGC returns a flight's IGC file. Costs one request; the file
	// download from the files host is not counted.
	DownloadIGC(ctx context.Context, key string, b Budget, flightID int64) ([]byte, error)
}

// Config configures a Client. Zero values take the defaults.
type Config struct {
	BaseURL     string
	FilesURL    string
	Timeout     time.Duration
	MaxIGCBytes int
	Transport   http.RoundTripper
}

// ConfigFromEnv returns the production configuration. WEGLIDE_API_URL and
// WEGLIDE_FILES_URL override the hosts only when WEGLIDE_ALLOW_TEST_URL=true;
// both are test-only.
func ConfigFromEnv() Config {
	cfg := Config{}
	if os.Getenv("WEGLIDE_ALLOW_TEST_URL") == "true" {
		cfg.BaseURL = strings.TrimRight(os.Getenv("WEGLIDE_API_URL"), "/")
		cfg.FilesURL = strings.TrimRight(os.Getenv("WEGLIDE_FILES_URL"), "/")
	}
	return cfg
}

// Client implements API over HTTP.
type Client struct {
	base        *url.URL
	files       *url.URL
	http        *http.Client
	maxIGCBytes int64
}

var _ API = (*Client)(nil)

// New returns a Client. It panics on an unparsable URL in cfg.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.FilesURL == "" {
		cfg.FilesURL = DefaultFilesURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.MaxIGCBytes <= 0 {
		cfg.MaxIGCBytes = DefaultMaxIGCBytes
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		panic(fmt.Sprintf("weglide: invalid base URL: %v", err))
	}
	files, err := url.Parse(cfg.FilesURL)
	if err != nil {
		panic(fmt.Sprintf("weglide: invalid files URL: %v", err))
	}
	return &Client{
		base:  base,
		files: files,
		http: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: cfg.Transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		maxIGCBytes: int64(cfg.MaxIGCBytes),
	}
}

// BaseURL returns the API host in use.
func (c *Client) BaseURL() string { return c.base.String() }

var keyPattern = regexp.MustCompile(`^[\x21-\x7e]+$`)

// ValidKey reports whether key is non-empty printable ASCII without spaces,
// at most MaxKeyLen long.
func ValidKey(key string) bool {
	return len(key) > 0 && len(key) <= MaxKeyLen && keyPattern.MatchString(key)
}

// Me implements API.
func (c *Client) Me(ctx context.Context, key string, b Budget) (*User, error) {
	var body struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	}
	if err := c.getJSON(ctx, key, b, c.apiURL("/v1/user/me", nil), &body); err != nil {
		return nil, err
	}
	id := body.ID.String()
	if id == "" || len(id) > 64 {
		RequestsTotal.WithLabelValues(statusBadResponse).Inc()
		return nil, ErrBadResponse
	}
	return &User{ID: id, Name: body.Name}, nil
}

type flightJSON struct {
	ID   json.Number `json:"id"`
	User struct {
		ID json.Number `json:"id"`
	} `json:"user"`
	ScoringDate string `json:"scoring_date"`
	TakeoffTime string `json:"takeoff_time"`
}

// ListFlights implements API.
func (c *Client) ListFlights(ctx context.Context, key string, b Budget, userID string, since time.Time) ([]Flight, error) {
	var out []Flight
	for page := 0; page < MaxPages; page++ {
		q := url.Values{}
		q.Set("user_id_in", userID)
		q.Set("scoring_date_start", since.UTC().Format("2006-01-02"))
		q.Set("order_by", "scoring_date")
		q.Set("skip", strconv.Itoa(page*PageSize))
		q.Set("limit", strconv.Itoa(PageSize))
		var rows []flightJSON
		if err := c.getJSON(ctx, key, b, c.apiURL("/v1/flight", q), &rows); err != nil {
			sortFlights(out)
			return out, err
		}
		for _, r := range rows {
			id, err := r.ID.Int64()
			if err != nil || id <= 0 {
				continue
			}
			if uid := r.User.ID.String(); uid != "" && uid != userID {
				continue
			}
			f := Flight{ID: id, UserID: userID, ScoringDate: r.ScoringDate}
			if t, err := time.Parse(time.RFC3339, r.TakeoffTime); err == nil {
				f.TakeoffTime = t.UTC()
			} else if t, err := time.Parse("2006-01-02T15:04:05", r.TakeoffTime); err == nil {
				f.TakeoffTime = t
			}
			out = append(out, f)
		}
		if len(rows) < PageSize {
			break
		}
	}
	sortFlights(out)
	return out, nil
}

func sortFlights(fs []Flight) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].ScoringDate != fs[j].ScoringDate {
			return fs[i].ScoringDate < fs[j].ScoringDate
		}
		if !fs[i].TakeoffTime.Equal(fs[j].TakeoffTime) {
			return fs[i].TakeoffTime.Before(fs[j].TakeoffTime)
		}
		return fs[i].ID < fs[j].ID
	})
}

var igcPathPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,511}$`)

// DownloadIGC implements API.
func (c *Client) DownloadIGC(ctx context.Context, key string, b Budget, flightID int64) ([]byte, error) {
	var detail struct {
		IGCFile *struct {
			File string `json:"file"`
		} `json:"igc_file"`
	}
	path := "/v1/flightdetail/" + strconv.FormatInt(flightID, 10)
	if err := c.getJSON(ctx, key, b, c.apiURL(path, nil), &detail); err != nil {
		return nil, err
	}
	if detail.IGCFile == nil || detail.IGCFile.File == "" {
		return nil, ErrNotFound
	}
	file := strings.TrimPrefix(detail.IGCFile.File, "/")
	if !igcPathPattern.MatchString(file) || strings.Contains(file, "..") || strings.Contains(file, "//") {
		RequestsTotal.WithLabelValues(statusBadResponse).Inc()
		return nil, ErrBadResponse
	}
	u := *c.files
	u.Path = strings.TrimRight(u.Path, "/") + "/" + file
	u.RawQuery = ""
	return c.get(ctx, "", u.String(), c.maxIGCBytes)
}

func (c *Client) apiURL(path string, q url.Values) string {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = ""
	if q != nil {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (c *Client) getJSON(ctx context.Context, key string, b Budget, rawURL string, v any) error {
	if !ValidKey(key) {
		return ErrInvalidKey
	}
	if b != nil {
		if err := b.Take(ctx); err != nil {
			return err
		}
	}
	body, err := c.get(ctx, key, rawURL, MaxJSONBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		RequestsTotal.WithLabelValues(statusBadResponse).Inc()
		return ErrBadResponse
	}
	return nil
}

// get performs one GET, sending key as X-API-Key when non-empty, and reads at
// most limit bytes of the body.
func (c *Client) get(ctx context.Context, key, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, ErrBadResponse
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NinerLog (+https://ninerlog.com)")
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	start := time.Now()
	resp, err := c.http.Do(req)
	RequestDurationSeconds.Observe(time.Since(start).Seconds())
	if err != nil {
		RequestsTotal.WithLabelValues(statusNetworkError).Inc()
		return nil, ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()

	if err := statusError(resp.StatusCode); err != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, err
	}
	if resp.ContentLength > limit {
		RequestsTotal.WithLabelValues(statusTooLarge).Inc()
		return nil, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		RequestsTotal.WithLabelValues(statusNetworkError).Inc()
		return nil, ErrUnavailable
	}
	if int64(len(data)) > limit {
		RequestsTotal.WithLabelValues(statusTooLarge).Inc()
		return nil, ErrTooLarge
	}
	RequestsTotal.WithLabelValues(statusOK).Inc()
	return data, nil
}

// statusError maps a non-2xx status onto a sentinel and counts it.
func statusError(code int) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		RequestsTotal.WithLabelValues(statusUnauthorized).Inc()
		return ErrUnauthorized
	case code == http.StatusTooManyRequests:
		RequestsTotal.WithLabelValues(statusRateLimited).Inc()
		return ErrRateLimited
	case code == http.StatusNotFound:
		RequestsTotal.WithLabelValues(statusNotFound).Inc()
		return ErrNotFound
	case code >= 500:
		RequestsTotal.WithLabelValues(statusUpstreamError).Inc()
		return fmt.Errorf("%w (HTTP %d)", ErrUnavailable, code)
	default:
		RequestsTotal.WithLabelValues(statusBadResponse).Inc()
		return fmt.Errorf("%w (HTTP %d)", ErrBadResponse, code)
	}
}
