package s3

import (
	"context"
	"strings"
	"testing"
)

// TestNew_BlocksSSRFToMetadata verifies the default (guarded) S3 provider
// refuses to connect to the cloud metadata endpoint via a user-supplied
// endpoint. A literal link-local IP is used so the guard rejects at the dial
// Control hook with no real connection.
func TestNew_BlocksSSRFToMetadata(t *testing.T) {
	p := New() // default, guarded

	err := p.Validate(context.Background(), newCfg("169.254.169.254"), newCreds())
	if err == nil {
		t.Fatal("Validate against link-local metadata endpoint should fail (SSRF)")
	}
	if !strings.Contains(err.Error(), "netguard") {
		t.Errorf("expected a netguard SSRF-block error, got: %v", err)
	}
}

func TestNew_BlocksSSRFToLoopback(t *testing.T) {
	p := New()
	err := p.Validate(context.Background(), newCfg("127.0.0.1:9000"), newCreds())
	if err == nil {
		t.Fatal("Validate against loopback should fail (SSRF)")
	}
	if !strings.Contains(err.Error(), "netguard") {
		t.Errorf("expected a netguard SSRF-block error, got: %v", err)
	}
}
