package netguard

import (
	"net"
	"strings"
	"testing"
)

func TestGuard_AlwaysBlocked(t *testing.T) {
	// These must be blocked regardless of the allowPrivate setting.
	blocked := []string{
		"127.0.0.1",       // loopback
		"127.0.0.53",      // loopback
		"::1",             // loopback v6
		"0.0.0.0",         // unspecified
		"::",              // unspecified v6
		"169.254.169.254", // link-local — cloud metadata endpoint
		"169.254.1.1",     // link-local
		"fe80::1",         // link-local v6
		"224.0.0.1",       // multicast
		"ff02::1",         // multicast v6
	}
	for _, allowPrivate := range []bool{false, true} {
		g := New(allowPrivate)
		for _, s := range blocked {
			ip := net.ParseIP(s)
			if ip == nil {
				t.Fatalf("test bug: bad IP %q", s)
			}
			if g.Allowed(ip) {
				t.Errorf("allowPrivate=%v: %s should be blocked", allowPrivate, s)
			}
		}
	}
}

func TestGuard_PrivateBlockedByDefault(t *testing.T) {
	private := []string{
		"10.0.0.5",        // RFC-1918
		"172.16.9.9",      // RFC-1918
		"192.168.1.10",    // RFC-1918
		"100.64.0.1",      // CGNAT (RFC 6598)
		"fd00::1",         // unique-local v6
		"::ffff:10.0.0.1", // IPv4-mapped private
	}
	g := New(false)
	for _, s := range private {
		ip := net.ParseIP(s)
		if g.Allowed(ip) {
			t.Errorf("private address %s should be blocked by default", s)
		}
	}
}

func TestGuard_PrivateAllowedWhenOptedIn(t *testing.T) {
	// Self-hosters that back up to a LAN NAS opt in via BACKUP_ALLOW_PRIVATE_NETWORKS.
	g := New(true)
	for _, s := range []string{"10.0.0.5", "192.168.1.10", "172.16.9.9", "100.64.0.1", "fd00::1"} {
		ip := net.ParseIP(s)
		if !g.Allowed(ip) {
			t.Errorf("private address %s should be allowed when opted in", s)
		}
	}
	// ...but loopback/link-local stay blocked even when opted in.
	if g.Allowed(net.ParseIP("169.254.169.254")) {
		t.Error("metadata endpoint must stay blocked even with allowPrivate=true")
	}
}

func TestGuard_PublicAllowed(t *testing.T) {
	for _, allowPrivate := range []bool{false, true} {
		g := New(allowPrivate)
		for _, s := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"} {
			ip := net.ParseIP(s)
			if !g.Allowed(ip) {
				t.Errorf("allowPrivate=%v: public address %s should be allowed", allowPrivate, s)
			}
		}
	}
}

func TestGuard_Control_BlocksMetadata(t *testing.T) {
	g := New(true) // even the most permissive config
	err := g.Control("tcp", "169.254.169.254:80", nil)
	if err == nil {
		t.Fatal("Control should reject the cloud metadata endpoint")
	}
	if !strings.Contains(err.Error(), "netguard") {
		t.Errorf("error should be a netguard error, got: %v", err)
	}
}

func TestGuard_Control_BlocksLoopbackWithPort(t *testing.T) {
	g := New(false)
	if err := g.Control("tcp", "127.0.0.1:22", nil); err == nil {
		t.Error("Control should reject loopback:22")
	}
}

func TestGuard_Control_AllowsPublicWithPort(t *testing.T) {
	g := New(false)
	if err := g.Control("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("Control should allow a public address, got: %v", err)
	}
}

func TestGuard_Control_RejectsNonIP(t *testing.T) {
	// Control receives an already-resolved address; a hostname here means
	// something went wrong, so fail closed.
	g := New(false)
	if err := g.Control("tcp", "example.com:443", nil); err == nil {
		t.Error("Control should reject a non-IP address")
	}
}

func TestFromEnv_DefaultBlocksPrivate(t *testing.T) {
	t.Setenv(AllowPrivateNetworksEnv, "")
	g := FromEnv()
	if g.Allowed(net.ParseIP("192.168.1.1")) {
		t.Error("FromEnv default should block private addresses")
	}
}

func TestFromEnv_OptIn(t *testing.T) {
	t.Setenv(AllowPrivateNetworksEnv, "true")
	g := FromEnv()
	if !g.Allowed(net.ParseIP("192.168.1.1")) {
		t.Error("FromEnv with opt-in should allow private addresses")
	}
}
