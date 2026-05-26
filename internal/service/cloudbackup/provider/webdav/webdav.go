// Package webdav implements the cloud backup provider contract against any
// RFC 4918 compliant WebDAV server (Nextcloud, ownCloud, mailbox.org, Box,
// Apache mod_dav, …) using the studio-b12/gowebdav client.
//
// Configuration model (non-secret, persisted in plain text):
//
//	base_url - required, fully qualified URL pointing at the WebDAV root for
//	           the account. Examples:
//	             https://cloud.example.com/remote.php/dav/files/alice/
//	             https://dav.example.com/
//	           Must use https:// unless allow_insecure is set (test only).
//	path     - optional sub-directory under base_url where backups are written.
//	           A trailing "/" is appended if missing. Defaults to
//	           "ninerlog-backups/". The directory is created automatically on
//	           first upload (MKCOL).
//	allow_insecure - optional boolean. When true, http:// URLs are accepted
//	                 and TLS verification is *not* relaxed (the URL scheme
//	                 simply isn't pre-validated). Intended for e2e tests
//	                 against containerised servers.
//
// Credential model (secret, encrypted at rest):
//
//	username - required
//	password - required (may also be an app-specific token, e.g. Nextcloud
//	           "app passwords")
//
// The provider performs an authenticated PROPFIND during Validate and creates
// the target directory if it does not yet exist; this confirms both
// authentication and write permission without leaving probe data behind.
package webdav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
	gowebdav "github.com/studio-b12/gowebdav"
)

// Provider implements provider.Provider for WebDAV servers.
type Provider struct {
	transport http.RoundTripper
	timeout   time.Duration
}

// New returns a WebDAV provider using a hardened default HTTP transport.
func New() *Provider {
	return &Provider{
		transport: http.DefaultTransport,
		timeout:   60 * time.Second,
	}
}

// NewWithTransport returns a WebDAV provider using the supplied transport and
// timeout. Intended for tests.
func NewWithTransport(rt http.RoundTripper, timeout time.Duration) *Provider {
	if rt == nil {
		rt = http.DefaultTransport
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Provider{transport: rt, timeout: timeout}
}

// Provider metadata --------------------------------------------------------

func (*Provider) Name() string        { return "webdav" }
func (*Provider) DisplayName() string { return "WebDAV" }
func (*Provider) Description() string {
	return "Upload encrypted JSON backups to any WebDAV server, including Nextcloud, ownCloud, mailbox.org, Box, and Apache mod_dav."
}

func (*Provider) ConfigSchema() []provider.Field {
	return []provider.Field{
		{
			Name: "base_url", Label: "Server URL", Type: provider.FieldTypeURL, Required: true,
			Placeholder: "https://cloud.example.com/remote.php/dav/files/alice/",
			Help:        "Full WebDAV URL for your account. For Nextcloud this looks like https://HOST/remote.php/dav/files/USERNAME/.",
		},
		{
			Name: "path", Label: "Sub-folder", Type: provider.FieldTypeString, Required: false,
			Placeholder: "ninerlog-backups/",
			Help:        "Optional sub-folder under the server URL. A trailing '/' is added automatically. Defaults to 'ninerlog-backups/'.",
		},
	}
}

func (*Provider) CredentialSchema() []provider.Field {
	return []provider.Field{
		{Name: "username", Label: "Username", Type: provider.FieldTypeString, Required: true, Sensitive: true},
		{Name: "password", Label: "Password or app token", Type: provider.FieldTypePassword, Required: true, Sensitive: true},
	}
}

// Operations ---------------------------------------------------------------

// Validate authenticates and confirms the target directory is writable. It:
//  1. Issues an authenticated PROPFIND against the server root.
//  2. Ensures the configured sub-folder exists, creating it via MKCOL if not.
func (p *Provider) Validate(ctx context.Context, cfg provider.Config, creds provider.Credentials) error {
	client, parsed, err := p.newClient(cfg, creds)
	if err != nil {
		return err
	}
	if err := ctxDone(ctx); err != nil {
		return err
	}

	if err := client.Connect(); err != nil {
		return classifyError(err)
	}

	if err := ctxDone(ctx); err != nil {
		return err
	}

	// Ensure the target directory exists. MkdirAll is idempotent and is the
	// cheapest write-permission probe available on WebDAV.
	if err := client.MkdirAll(parsed.Path, 0o755); err != nil {
		return classifyError(err)
	}
	return nil
}

// Upload streams the backup payload to <base_url>/<path>/<filename>.
func (p *Provider) Upload(ctx context.Context, cfg provider.Config, creds provider.Credentials, in provider.UploadInput) (*provider.UploadResult, error) {
	client, parsed, err := p.newClient(cfg, creds)
	if err != nil {
		return nil, err
	}
	if err := ctxDone(ctx); err != nil {
		return nil, err
	}

	// Make sure the target directory exists; on first run it may not.
	if err := client.MkdirAll(parsed.Path, 0o755); err != nil {
		return nil, classifyError(err)
	}

	remotePath := parsed.Path + in.Filename
	reader := in.Reader
	if reader == nil {
		reader = strings.NewReader("")
	}

	// Prefer WriteStreamWithLength so the server gets a Content-Length header
	// and can reject early on quota; fall back to WriteStream if size is
	// unknown.
	var writeErr error
	if in.Size > 0 {
		writeErr = client.WriteStreamWithLength(remotePath, reader, in.Size, 0o644)
	} else {
		writeErr = client.WriteStream(remotePath, reader, 0o644)
	}
	if writeErr != nil {
		return nil, classifyError(writeErr)
	}
	return &provider.UploadResult{RemotePath: remotePath}, nil
}

// List enumerates backup objects under the configured path. Directory
// entries and dotfiles are skipped.
func (p *Provider) List(ctx context.Context, cfg provider.Config, creds provider.Credentials) ([]provider.RemoteBackup, error) {
	client, parsed, err := p.newClient(cfg, creds)
	if err != nil {
		return nil, err
	}
	if err := ctxDone(ctx); err != nil {
		return nil, err
	}

	entries, err := client.ReadDir(parsed.Path)
	if err != nil {
		// A missing directory is treated as "no backups yet" rather than an error.
		if gowebdav.IsErrNotFound(err) {
			return nil, nil
		}
		return nil, classifyError(err)
	}

	out := make([]provider.RemoteBackup, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, provider.RemoteBackup{
			Path:         parsed.Path + name,
			SizeBytes:    e.Size(),
			LastModified: e.ModTime(),
		})
	}
	return out, nil
}

// Delete removes one file by its remote path.
func (p *Provider) Delete(ctx context.Context, cfg provider.Config, creds provider.Credentials, path string) error {
	client, _, err := p.newClient(cfg, creds)
	if err != nil {
		return err
	}
	if err := ctxDone(ctx); err != nil {
		return err
	}
	if err := client.Remove(path); err != nil {
		// Treat "already gone" as success — retention pruning is idempotent.
		if gowebdav.IsErrNotFound(err) {
			return nil
		}
		return classifyError(err)
	}
	return nil
}

// Internal helpers ---------------------------------------------------------

type parsedConfig struct {
	BaseURL string
	Path    string
}

func parseConfig(cfg provider.Config, creds provider.Credentials) (*parsedConfig, string, string, error) {
	baseURL, _ := stringField(cfg, "base_url")
	subPath, _ := stringField(cfg, "path")
	username, _ := stringField(creds, "username")
	password, _ := stringField(creds, "password")
	allowInsecure := boolField(cfg, "allow_insecure")

	if baseURL == "" {
		return nil, "", "", fmt.Errorf("%w: base_url is required", provider.ErrInvalidConfig)
	}
	if username == "" {
		return nil, "", "", fmt.Errorf("%w: username is required", provider.ErrInvalidCredentials)
	}
	if password == "" {
		return nil, "", "", fmt.Errorf("%w: password is required", provider.ErrInvalidCredentials)
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: base_url is not a valid URL: %v", provider.ErrInvalidConfig, err)
	}
	if u.Scheme != "https" && !(allowInsecure && u.Scheme == "http") {
		return nil, "", "", fmt.Errorf("%w: base_url must use https://", provider.ErrInvalidConfig)
	}
	if u.Host == "" {
		return nil, "", "", fmt.Errorf("%w: base_url missing host", provider.ErrInvalidConfig)
	}

	subPath = strings.TrimLeft(subPath, "/")
	if subPath == "" {
		subPath = "ninerlog-backups/"
	} else if !strings.HasSuffix(subPath, "/") {
		subPath += "/"
	}

	return &parsedConfig{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Path:    "/" + subPath,
	}, username, password, nil
}

// newClient builds a gowebdav client configured with the provider's transport
// and timeout.
func (p *Provider) newClient(cfg provider.Config, creds provider.Credentials) (*gowebdav.Client, *parsedConfig, error) {
	pc, username, password, err := parseConfig(cfg, creds)
	if err != nil {
		return nil, nil, err
	}
	client := gowebdav.NewClient(pc.BaseURL, username, password)
	if p.transport != nil {
		client.SetTransport(p.transport)
	}
	client.SetTimeout(p.timeout)
	return client, pc, nil
}

func stringField(m map[string]any, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

func ctxDone(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// classifyError maps WebDAV / HTTP errors onto the provider.Err* sentinels so
// the service can render appropriate HTTP status codes.
func classifyError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case gowebdav.IsErrCode(err, http.StatusUnauthorized):
		return fmt.Errorf("%w: %v", provider.ErrInvalidCredentials, err)
	case gowebdav.IsErrCode(err, http.StatusForbidden):
		return fmt.Errorf("%w: %v", provider.ErrPermissionDenied, err)
	case gowebdav.IsErrCode(err, http.StatusNotFound):
		return fmt.Errorf("%w: %v", provider.ErrNotFound, err)
	case gowebdav.IsErrCode(err, http.StatusBadGateway),
		gowebdav.IsErrCode(err, http.StatusServiceUnavailable),
		gowebdav.IsErrCode(err, http.StatusGatewayTimeout),
		gowebdav.IsErrCode(err, http.StatusInternalServerError):
		return fmt.Errorf("%w: %v", provider.ErrTransient, err)
	}
	// Treat os.ErrNotExist as not-found for ReadDir/Remove paths.
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %v", provider.ErrNotFound, err)
	}
	// Fall back to ErrTransient for opaque transport/IO failures so the
	// service's retry policy kicks in.
	var statusErr *gowebdav.StatusError
	if errors.As(err, &statusErr) {
		return err
	}
	if _, ok := err.(interface{ Timeout() bool }); ok {
		return fmt.Errorf("%w: %v", provider.ErrTransient, err)
	}
	// As a last resort, surface the raw error verbatim.
	return err
}

// Compile-time assertion that we satisfy the provider interface.
var _ provider.Provider = (*Provider)(nil)

// Ensure io.Reader is referenced even when callers pass nil readers; this
// keeps go vet happy in case future refactors trim the import.
var _ io.Reader = (*strings.Reader)(nil)
