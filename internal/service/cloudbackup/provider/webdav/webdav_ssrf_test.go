package webdav

import (
	"context"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
)

// TestNew_BlocksSSRFToMetadata verifies the SSRF guard is wired into the
// default provider constructor: a user-supplied base_url pointing at the cloud
// metadata endpoint must be refused before any connection is made. Uses a
// literal IP so no DNS lookup or network traffic occurs.
func TestNew_BlocksSSRFToMetadata(t *testing.T) {
	p := New() // default, guarded

	cfg := newCfg("https://169.254.169.254")
	err := p.Validate(context.Background(), cfg, newCreds())
	if err == nil {
		t.Fatal("Validate against link-local metadata endpoint should fail (SSRF)")
	}
	if !strings.Contains(err.Error(), "netguard") {
		t.Errorf("expected a netguard SSRF-block error, got: %v", err)
	}
}

func TestNew_BlocksSSRFToLoopback(t *testing.T) {
	p := New()
	cfg := provider.Config{
		"base_url":       "http://127.0.0.1/",
		"path":           "backups/",
		"allow_insecure": true,
	}
	err := p.Validate(context.Background(), cfg, newCreds())
	if err == nil {
		t.Fatal("Validate against loopback should fail (SSRF)")
	}
	if !strings.Contains(err.Error(), "netguard") {
		t.Errorf("expected a netguard SSRF-block error, got: %v", err)
	}
}
