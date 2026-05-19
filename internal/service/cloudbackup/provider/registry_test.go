package provider

import (
	"context"
	"testing"
)

type stubProvider struct {
	name        string
	display     string
	configSch   []Field
	credSch     []Field
	validateErr error
}

func (s stubProvider) Name() string              { return s.name }
func (s stubProvider) DisplayName() string       { return s.display }
func (s stubProvider) Description() string       { return "stub" }
func (s stubProvider) ConfigSchema() []Field     { return s.configSch }
func (s stubProvider) CredentialSchema() []Field { return s.credSch }
func (s stubProvider) Validate(ctx context.Context, cfg Config, creds Credentials) error {
	return s.validateErr
}
func (s stubProvider) Upload(ctx context.Context, cfg Config, creds Credentials, in UploadInput) (*UploadResult, error) {
	return &UploadResult{RemotePath: in.Filename}, nil
}
func (s stubProvider) List(ctx context.Context, cfg Config, creds Credentials) ([]RemoteBackup, error) {
	return nil, nil
}
func (s stubProvider) Delete(ctx context.Context, cfg Config, creds Credentials, path string) error {
	return nil
}

func TestRegistryEmpty(t *testing.T) {
	r := NewRegistry()
	if names := r.Names(); len(names) != 0 {
		t.Fatalf("expected empty registry, got %v", names)
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatalf("Get on empty registry returned ok")
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(stubProvider{name: "s3", display: "S3"})
	r.Register(stubProvider{name: "gcs", display: "GCS"})

	got, ok := r.Get("s3")
	if !ok || got.DisplayName() != "S3" {
		t.Fatalf("Get(s3) = %v, %v", got, ok)
	}
	if names := r.Names(); len(names) != 2 || names[0] != "gcs" || names[1] != "s3" {
		t.Fatalf("Names() = %v, want sorted [gcs s3]", names)
	}
	all := r.All()
	if len(all) != 2 || all[0].Name() != "gcs" || all[1].Name() != "s3" {
		t.Fatalf("All() returned %v", all)
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(stubProvider{name: "s3"})
	defer func() {
		if recover() == nil {
			t.Fatalf("duplicate registration did not panic")
		}
	}()
	r.Register(stubProvider{name: "s3"})
}

func TestRegistryRejectsNil(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatalf("nil registration did not panic")
		}
	}()
	r.Register(nil)
}

func TestRegistryRejectsEmptyName(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatalf("empty-name registration did not panic")
		}
	}()
	r.Register(stubProvider{name: ""})
}

func TestRegistryMustGetPanicsOnMissing(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatalf("MustGet on missing did not panic")
		}
	}()
	_ = r.MustGet("missing")
}
