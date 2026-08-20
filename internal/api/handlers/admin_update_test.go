package handlers

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/updatecheck"
)

func TestToUpdateStatus(t *testing.T) {
	checked := time.Date(2026, 8, 20, 6, 0, 0, 0, time.UTC)
	published := time.Date(2026, 8, 18, 20, 32, 52, 0, time.UTC)

	out := toUpdateStatus(updatecheck.Status{
		Enabled:         true,
		UpdateAvailable: true,
		LastCheckedAt:   &checked,
		Components: []updatecheck.ComponentStatus{
			{
				Name:           updatecheck.ComponentAPI,
				CurrentVersion: "v1.3.4",
				LatestVersion:  "v1.3.5",
				State:          updatecheck.StateUpdateAvailable,
				ReleaseURL:     "https://example.test/releases/v1.3.5",
				PublishedAt:    &published,
			},
			{
				Name:           updatecheck.ComponentFrontend,
				CurrentVersion: "dev",
				State:          updatecheck.StateUnknown,
			},
		},
	})

	if !out.CheckEnabled || !out.UpdateAvailable {
		t.Fatalf("flags = enabled %v, available %v; want both true", out.CheckEnabled, out.UpdateAvailable)
	}
	if out.LastCheckedAt == nil || !out.LastCheckedAt.Equal(checked) {
		t.Errorf("LastCheckedAt = %v, want %v", out.LastCheckedAt, checked)
	}
	if out.LastError != nil {
		t.Errorf("LastError = %v, want nil", *out.LastError)
	}
	if len(out.Components) != 2 {
		t.Fatalf("got %d components, want 2", len(out.Components))
	}

	api := out.Components[0]
	if api.Name != generated.Api || api.State != generated.UpdateComponentStateUpdateAvailable {
		t.Errorf("api component = %+v", api)
	}
	if api.LatestVersion == nil || *api.LatestVersion != "v1.3.5" {
		t.Errorf("api latest version = %v, want v1.3.5", api.LatestVersion)
	}
	if api.ReleaseUrl == nil || *api.ReleaseUrl != "https://example.test/releases/v1.3.5" {
		t.Errorf("api release URL = %v", api.ReleaseUrl)
	}
	if api.PublishedAt == nil || !api.PublishedAt.Equal(published) {
		t.Errorf("api publishedAt = %v, want %v", api.PublishedAt, published)
	}

	frontend := out.Components[1]
	if frontend.State != generated.UpdateComponentStateUnknown {
		t.Errorf("frontend state = %q, want unknown", frontend.State)
	}
	if frontend.LatestVersion != nil || frontend.ReleaseUrl != nil || frontend.PublishedAt != nil {
		t.Errorf("frontend carries release detail it never had: %+v", frontend)
	}
}

func TestToUpdateStatusErrorReasons(t *testing.T) {
	tests := []struct {
		reason string
		want   generated.UpdateStatusLastError
	}{
		{reason: "request", want: generated.UpdateStatusLastErrorRequest},
		{reason: "status", want: generated.UpdateStatusLastErrorStatus},
		{reason: "decode", want: generated.UpdateStatusLastErrorDecode},
		{reason: "empty", want: generated.UpdateStatusLastErrorEmpty},
		{reason: "something else entirely", want: generated.UpdateStatusLastErrorError},
	}

	for _, tt := range tests {
		out := toUpdateStatus(updatecheck.Status{Enabled: true, LastError: tt.reason})
		if out.LastError == nil {
			t.Errorf("LastError nil for reason %q", tt.reason)
			continue
		}
		if *out.LastError != tt.want {
			t.Errorf("LastError for %q = %q, want %q", tt.reason, *out.LastError, tt.want)
		}
	}

	if out := toUpdateStatus(updatecheck.Status{Enabled: true}); out.LastError != nil {
		t.Errorf("LastError = %q for a successful check, want nil", *out.LastError)
	}
}

func TestToUpdateStatusEmptyComponents(t *testing.T) {
	out := toUpdateStatus(updatecheck.Status{})
	if out.Components == nil {
		t.Error("Components is nil; the field must marshal as [] rather than null")
	}
}
