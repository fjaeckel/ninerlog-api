package sftp

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/service/cloudbackup/provider"
)

// TestNew_BlocksSSRFToMetadata verifies the default (guarded) SFTP provider
// refuses to connect to the cloud metadata endpoint. A literal link-local IP is
// used so no DNS lookup or connection actually happens — the guard rejects at
// the dial Control hook.
func TestNew_BlocksSSRFToMetadata(t *testing.T) {
	p := New() // default, guarded

	cfg := provider.Config{
		"host":                "169.254.169.254",
		"port":                strconv.Itoa(22),
		"path":                "ninerlog-test/",
		"accept_any_host_key": true,
	}
	err := p.Validate(context.Background(), cfg, providerCreds())
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
		"host":                "127.0.0.1",
		"port":                strconv.Itoa(22),
		"path":                "ninerlog-test/",
		"accept_any_host_key": true,
	}
	err := p.Validate(context.Background(), cfg, providerCreds())
	if err == nil {
		t.Fatal("Validate against loopback should fail (SSRF)")
	}
	if !strings.Contains(err.Error(), "netguard") {
		t.Errorf("expected a netguard SSRF-block error, got: %v", err)
	}
}
