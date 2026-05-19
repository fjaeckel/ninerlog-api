package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
)

// fakeS3 is a tiny HTTP handler emulating just the S3 surface we exercise.
type fakeS3 struct {
	t              *testing.T
	bucketExists   bool
	listError      bool
	signatureError bool
	notFound       bool
	internalError  bool

	objects map[string][]byte // key -> body
	deletes []string
	uploads []string
}

func newFakeS3(t *testing.T) *fakeS3 {
	return &fakeS3{t: t, bucketExists: true, objects: map[string][]byte{}}
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.signatureError {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>SignatureDoesNotMatch</Code><Message>bad signature</Message></Error>`)
		return
	}
	if f.internalError {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodHead:
		if !f.bucketExists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		if f.notFound {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>NoSuchBucket</Code><Message>no bucket</Message></Error>`)
			return
		}
		if f.listError {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>AccessDenied</Code><Message>denied</Message></Error>`)
			return
		}
		// ListObjectsV2 response
		var contents strings.Builder
		for k, b := range f.objects {
			contents.WriteString(`<Contents><Key>`)
			contents.WriteString(k)
			contents.WriteString(`</Key><Size>`)
			contents.WriteString(itoa(len(b)))
			contents.WriteString(`</Size><LastModified>2024-01-01T00:00:00.000Z</LastModified></Contents>`)
		}
		body := `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><IsTruncated>false</IsTruncated>` + contents.String() + `</ListBucketResult>`
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, body)
	case http.MethodPut:
		key := strings.TrimPrefix(r.URL.Path, "/")
		// Strip bucket prefix "/bucket/key"
		if idx := strings.Index(key, "/"); idx >= 0 {
			key = key[idx+1:]
		}
		body, _ := io.ReadAll(r.Body)
		f.objects[key] = body
		f.uploads = append(f.uploads, key)
		w.Header().Set("ETag", `"abc"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		key := strings.TrimPrefix(r.URL.Path, "/")
		if idx := strings.Index(key, "/"); idx >= 0 {
			key = key[idx+1:]
		}
		delete(f.objects, key)
		f.deletes = append(f.deletes, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func startFakeS3(t *testing.T) (*fakeS3, string) {
	t.Helper()
	fake := newFakeS3(t)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	return fake, host
}

func newCfg(host string) provider.Config {
	return provider.Config{
		"bucket":   "test-bucket",
		"region":   "us-east-1",
		"prefix":   "p/",
		"endpoint": "http://" + host,
		"use_ssl":  false,
	}
}

func newCreds() provider.Credentials {
	return provider.Credentials{
		"access_key_id":     "AKIATESTING",
		"secret_access_key": "secret",
	}
}

func TestProviderMetadata(t *testing.T) {
	p := New()
	if p.Name() != "s3" {
		t.Errorf("name: %s", p.Name())
	}
	if len(p.ConfigSchema()) == 0 || len(p.CredentialSchema()) == 0 {
		t.Errorf("missing schemas")
	}
}

func TestValidateSuccess(t *testing.T) {
	fake, host := startFakeS3(t)
	fake.objects["p/existing.gz"] = []byte("data")
	p := NewWithHTTPClient(http.DefaultClient)
	if err := p.Validate(context.Background(), newCfg(host), newCreds()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateMissingBucket(t *testing.T) {
	fake, host := startFakeS3(t)
	fake.bucketExists = false
	p := NewWithHTTPClient(http.DefaultClient)
	err := p.Validate(context.Background(), newCfg(host), newCreds())
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestValidateAccessDenied(t *testing.T) {
	fake, host := startFakeS3(t)
	fake.listError = true
	p := NewWithHTTPClient(http.DefaultClient)
	err := p.Validate(context.Background(), newCfg(host), newCreds())
	if !errors.Is(err, provider.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestListInvalidCredentials(t *testing.T) {
	// Use List() because BucketExists (HEAD) discards the body and the SDK
	// can only see the HTTP status, which the BucketExists call short-circuits
	// before classifyError sees an InvalidCredentials code. List() issues a
	// GET that carries the XML <Error><Code> payload through.
	fake, host := startFakeS3(t)
	fake.signatureError = true
	p := NewWithHTTPClient(http.DefaultClient)
	_, err := p.List(context.Background(), newCfg(host), newCreds())
	if !errors.Is(err, provider.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestUploadAndList(t *testing.T) {
	fake, host := startFakeS3(t)
	p := NewWithHTTPClient(http.DefaultClient)

	body := []byte("payload data")
	res, err := p.Upload(context.Background(), newCfg(host), newCreds(), provider.UploadInput{
		Filename:    "backup-1.json.gz",
		ContentType: "application/gzip",
		Size:        int64(len(body)),
		Reader:      bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.RemotePath != "p/backup-1.json.gz" {
		t.Errorf("remote path: %s", res.RemotePath)
	}
	if len(fake.uploads) != 1 || fake.uploads[0] != "p/backup-1.json.gz" {
		t.Errorf("uploads: %v", fake.uploads)
	}

	objs, err := p.List(context.Background(), newCfg(host), newCreds())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objs) != 1 || objs[0].Path != "p/backup-1.json.gz" {
		t.Errorf("list: %+v", objs)
	}
}

func TestDelete(t *testing.T) {
	fake, host := startFakeS3(t)
	fake.objects["p/old.gz"] = []byte("x")
	p := NewWithHTTPClient(http.DefaultClient)
	if err := p.Delete(context.Background(), newCfg(host), newCreds(), "p/old.gz"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := fake.objects["p/old.gz"]; ok {
		t.Errorf("object not deleted")
	}
	if len(fake.deletes) != 1 {
		t.Errorf("deletes: %v", fake.deletes)
	}
}

func TestUploadTransientErrorMapsToTransient(t *testing.T) {
	fake, host := startFakeS3(t)
	fake.internalError = true
	p := NewWithHTTPClient(http.DefaultClient)
	_, err := p.Upload(context.Background(), newCfg(host), newCreds(), provider.UploadInput{
		Filename:    "x.gz",
		ContentType: "application/gzip",
		Size:        int64(len("body")),
		Reader:      bytes.NewReader([]byte("body")),
	})
	if !errors.Is(err, provider.ErrTransient) {
		t.Fatalf("expected ErrTransient, got %v", err)
	}
}

func TestParseConfigRequiresBucket(t *testing.T) {
	p := New()
	err := p.Validate(context.Background(), provider.Config{}, newCreds())
	if !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestParseConfigRequiresCredentials(t *testing.T) {
	p := New()
	err := p.Validate(context.Background(), provider.Config{"bucket": "b", "region": "r"}, provider.Credentials{})
	if !errors.Is(err, provider.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct {
		in     string
		host   string
		secure bool
		err    bool
	}{
		{"https://s3.example.com", "s3.example.com", true, false},
		{"http://localhost:9000", "localhost:9000", false, false},
		{"localhost:9000", "localhost:9000", true, false},
		{"", "", false, true},
	}
	for _, c := range cases {
		h, s, err := normalizeEndpoint(c.in)
		if (err != nil) != c.err {
			t.Errorf("%q: err=%v want=%v", c.in, err, c.err)
		}
		if err == nil && (h != c.host || s != c.secure) {
			t.Errorf("%q: got (%q,%v), want (%q,%v)", c.in, h, s, c.host, c.secure)
		}
	}
}
