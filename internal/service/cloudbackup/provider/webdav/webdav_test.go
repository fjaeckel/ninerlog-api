package webdav

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
)

// fakeWebDAV is a minimal WebDAV server emulating just the verbs the provider
// exercises: OPTIONS (for Connect), PROPFIND, MKCOL, PUT, DELETE.
//
// It is intentionally permissive about XML — the gowebdav client only inspects
// a small subset of the multistatus response.
type fakeWebDAV struct {
	mu       sync.Mutex
	files    map[string][]byte // key: "/" + path, value: body
	dirs     map[string]bool   // key: "/" + path (always trailing slash)
	username string
	password string

	requireAuth bool
	forbid      bool
	transient   bool
	notFound    bool
}

func newFakeWebDAV() *fakeWebDAV {
	f := &fakeWebDAV{
		files:       map[string][]byte{},
		dirs:        map[string]bool{"/": true},
		username:    "alice",
		password:    "secret",
		requireAuth: true,
	}
	return f
}

func (f *fakeWebDAV) checkAuth(r *http.Request) bool {
	if !f.requireAuth {
		return true
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	return user == f.username && pass == f.password
}

func (f *fakeWebDAV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.transient {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if f.forbid {
		w.Header().Set("WWW-Authenticate", `Basic realm="webdav"`)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !f.checkAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="webdav"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	path := r.URL.Path

	switch r.Method {
	case "OPTIONS":
		w.Header().Set("DAV", "1, 2")
		w.Header().Set("Allow", "OPTIONS, GET, HEAD, PUT, DELETE, PROPFIND, MKCOL")
		w.WriteHeader(http.StatusOK)
	case "PROPFIND":
		if f.notFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Look up the resource: file, directory, or 404.
		isDir := f.dirs[path] || (!strings.HasSuffix(path, "/") && f.dirs[path+"/"])
		_, isFile := f.files[path]
		if !isDir && !isFile {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
		b.WriteString(`<D:multistatus xmlns:D="DAV:">`)
		// The directory itself.
		if isDir {
			selfPath := path
			if !strings.HasSuffix(selfPath, "/") {
				selfPath += "/"
			}
			writePropfindEntry(&b, selfPath, 0, true)
			// Children (1 level deep — Depth header is ignored, this fake
			// server is fine to over-report).
			for k, v := range f.files {
				if strings.HasPrefix(k, selfPath) && !strings.Contains(strings.TrimPrefix(k, selfPath), "/") {
					writePropfindEntry(&b, k, int64(len(v)), false)
				}
			}
			for k := range f.dirs {
				if k == selfPath {
					continue
				}
				if strings.HasPrefix(k, selfPath) {
					rest := strings.TrimPrefix(k, selfPath)
					if rest != "" && !strings.Contains(strings.TrimSuffix(rest, "/"), "/") {
						writePropfindEntry(&b, k, 0, true)
					}
				}
			}
		} else {
			writePropfindEntry(&b, path, int64(len(f.files[path])), false)
		}
		b.WriteString(`</D:multistatus>`)
		w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, b.String())
	case "MKCOL":
		if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		// Parent must exist.
		parent := parentDir(path)
		if !f.dirs[parent] {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if f.dirs[path] {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		f.dirs[path] = true
		w.WriteHeader(http.StatusCreated)
	case http.MethodPut:
		// Auto-create parent if it's there.
		parent := parentDir(path)
		if !f.dirs[parent] {
			w.WriteHeader(http.StatusConflict)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		f.files[path] = body
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		if _, ok := f.files[path]; ok {
			delete(f.files, path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writePropfindEntry(b *strings.Builder, href string, size int64, isDir bool) {
	b.WriteString(`<D:response><D:href>`)
	b.WriteString(href)
	b.WriteString(`</D:href><D:propstat><D:prop>`)
	if isDir {
		b.WriteString(`<D:resourcetype><D:collection/></D:resourcetype>`)
	} else {
		b.WriteString(`<D:resourcetype/>`)
		b.WriteString(fmt.Sprintf(`<D:getcontentlength>%d</D:getcontentlength>`, size))
	}
	b.WriteString(`<D:getlastmodified>Mon, 01 Jan 2024 00:00:00 GMT</D:getlastmodified>`)
	b.WriteString(`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`)
}

func parentDir(p string) string {
	p = strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i+1]
	}
	return "/"
}

func startFakeWebDAV(t *testing.T) (*fakeWebDAV, string) {
	t.Helper()
	fake := newFakeWebDAV()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv.URL
}

func newCfg(baseURL string) provider.Config {
	return provider.Config{
		"base_url":       baseURL + "/",
		"path":           "tests/",
		"allow_insecure": true,
	}
}

func newCreds() provider.Credentials {
	return provider.Credentials{
		"username": "alice",
		"password": "secret",
	}
}

func TestProviderMetadata(t *testing.T) {
	p := New()
	if p.Name() != "webdav" {
		t.Errorf("name: %s", p.Name())
	}
	if p.DisplayName() == "" || p.Description() == "" {
		t.Errorf("missing display name/description")
	}
	if len(p.ConfigSchema()) == 0 || len(p.CredentialSchema()) == 0 {
		t.Errorf("missing schemas")
	}
	// Credential fields must all be marked sensitive.
	for _, f := range p.CredentialSchema() {
		if !f.Sensitive {
			t.Errorf("credential field %q not marked sensitive", f.Name)
		}
	}
}

func TestValidateSuccessCreatesDir(t *testing.T) {
	fake, baseURL := startFakeWebDAV(t)
	p := New()
	if err := p.Validate(context.Background(), newCfg(baseURL), newCreds()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !fake.dirs["/tests/"] {
		t.Errorf("expected /tests/ to be created, dirs=%v", fake.dirs)
	}
}

func TestValidateBadCredentials(t *testing.T) {
	_, baseURL := startFakeWebDAV(t)
	p := New()
	creds := provider.Credentials{"username": "alice", "password": "wrong"}
	err := p.Validate(context.Background(), newCfg(baseURL), creds)
	if !errors.Is(err, provider.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestValidateForbidden(t *testing.T) {
	fake, baseURL := startFakeWebDAV(t)
	fake.forbid = true
	p := New()
	err := p.Validate(context.Background(), newCfg(baseURL), newCreds())
	if !errors.Is(err, provider.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestValidateTransient(t *testing.T) {
	fake, baseURL := startFakeWebDAV(t)
	fake.transient = true
	p := New()
	err := p.Validate(context.Background(), newCfg(baseURL), newCreds())
	if !errors.Is(err, provider.ErrTransient) {
		t.Fatalf("expected ErrTransient, got %v", err)
	}
}

func TestUploadListDelete(t *testing.T) {
	fake, baseURL := startFakeWebDAV(t)
	p := New()
	ctx := context.Background()
	cfg := newCfg(baseURL)
	creds := newCreds()

	body := []byte("payload-data")
	res, err := p.Upload(ctx, cfg, creds, provider.UploadInput{
		Filename:    "backup-1.json.gz",
		ContentType: "application/gzip",
		Size:        int64(len(body)),
		Reader:      bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.RemotePath != "/tests/backup-1.json.gz" {
		t.Errorf("remote path: %s", res.RemotePath)
	}
	if got := fake.files["/tests/backup-1.json.gz"]; !bytes.Equal(got, body) {
		t.Errorf("body mismatch: got %q", string(got))
	}

	// A second file to make sure List enumerates correctly.
	_, err = p.Upload(ctx, cfg, creds, provider.UploadInput{
		Filename:    "backup-2.json.gz",
		ContentType: "application/gzip",
		Size:        4,
		Reader:      bytes.NewReader([]byte("body")),
	})
	if err != nil {
		t.Fatalf("Upload 2: %v", err)
	}

	objs, err := p.List(ctx, cfg, creds)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	names := map[string]bool{}
	for _, o := range objs {
		names[o.Path] = true
	}
	if !names["/tests/backup-1.json.gz"] || !names["/tests/backup-2.json.gz"] {
		t.Errorf("missing entries; got %+v", objs)
	}

	// Delete the first file.
	if err := p.Delete(ctx, cfg, creds, "/tests/backup-1.json.gz"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := fake.files["/tests/backup-1.json.gz"]; ok {
		t.Errorf("file not deleted")
	}

	// Deleting again is idempotent (provider swallows not-found).
	if err := p.Delete(ctx, cfg, creds, "/tests/backup-1.json.gz"); err != nil {
		t.Errorf("expected nil on repeat delete, got %v", err)
	}
}

func TestListMissingDirReturnsEmpty(t *testing.T) {
	fake, baseURL := startFakeWebDAV(t)
	fake.notFound = true
	p := New()
	objs, err := p.List(context.Background(), newCfg(baseURL), newCreds())
	if err != nil {
		t.Fatalf("expected nil error on missing dir, got %v", err)
	}
	if len(objs) != 0 {
		t.Errorf("expected empty result, got %+v", objs)
	}
}

func TestParseConfigRequiresHTTPS(t *testing.T) {
	p := New()
	cfg := provider.Config{"base_url": "http://example.com/dav/"}
	err := p.Validate(context.Background(), cfg, newCreds())
	if !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestParseConfigAllowsInsecureWhenFlagSet(t *testing.T) {
	cfg := provider.Config{
		"base_url":       "http://example.com/dav/",
		"path":           "/x/",
		"allow_insecure": true,
	}
	_, _, _, err := parseConfig(cfg, newCreds())
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
}

func TestParseConfigRequiresBaseURL(t *testing.T) {
	_, _, _, err := parseConfig(provider.Config{}, newCreds())
	if !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestParseConfigRequiresCredentials(t *testing.T) {
	_, _, _, err := parseConfig(provider.Config{
		"base_url":       "http://example.com/dav/",
		"allow_insecure": true,
	}, provider.Credentials{})
	if !errors.Is(err, provider.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestParseConfigDefaultsPath(t *testing.T) {
	pc, _, _, err := parseConfig(provider.Config{
		"base_url":       "http://example.com/dav/",
		"allow_insecure": true,
	}, newCreds())
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if pc.Path != "/ninerlog-backups/" {
		t.Errorf("default path: %s", pc.Path)
	}
}

func TestParseConfigAppendsTrailingSlash(t *testing.T) {
	pc, _, _, err := parseConfig(provider.Config{
		"base_url":       "http://example.com/dav/",
		"path":           "Backups/ninerlog",
		"allow_insecure": true,
	}, newCreds())
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if pc.Path != "/Backups/ninerlog/" {
		t.Errorf("path: %s", pc.Path)
	}
}

func TestParseConfigRejectsInvalidURL(t *testing.T) {
	_, _, _, err := parseConfig(provider.Config{
		"base_url": "://not a url",
	}, newCreds())
	if !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

// TestClientTimeoutIsApplied ensures our configured timeout flows through to
// the underlying client by pointing it at a server that never responds and
// asserting the request fails quickly.
func TestClientTimeoutIsApplied(t *testing.T) {
	// Server that blocks forever (until test teardown closes it).
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-hang
	}))
	t.Cleanup(func() {
		close(hang)
		srv.Close()
	})

	p := NewWithTransport(nil, 200*time.Millisecond)
	start := time.Now()
	err := p.Validate(context.Background(), newCfg(srv.URL), newCreds())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected error from short timeout, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout not honored, elapsed=%s", elapsed)
	}
}

// TestParseURLNormalisation makes sure trailing slashes are normalised.
func TestParseURLNormalisation(t *testing.T) {
	pc, _, _, err := parseConfig(provider.Config{
		"base_url":       "http://example.com/dav////",
		"allow_insecure": true,
	}, newCreds())
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if strings.HasSuffix(pc.BaseURL, "/") {
		t.Errorf("BaseURL should not end with /, got %q", pc.BaseURL)
	}
	if _, err := url.Parse(pc.BaseURL); err != nil {
		t.Errorf("BaseURL not parseable: %v", err)
	}
}
