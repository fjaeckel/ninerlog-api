package netguard

import (
	"net"
	"testing"
)

// NAT64 (64:ff9b::/96) and IPv4-compatible (::a.b.c.d) addresses embedding an
// internal IPv4 address are blocked.
func TestBlocked_EmbeddedIPv4Forms(t *testing.T) {
	g := New(false) // private networks blocked, the production default

	cases := []struct{ name, addr string }{
		{"NAT64 loopback", "64:ff9b::7f00:1"},          // 127.0.0.1
		{"NAT64 cloud metadata", "64:ff9b::a9fe:a9fe"}, // 169.254.169.254
		{"NAT64 RFC1918", "64:ff9b::c0a8:1"},           // 192.168.0.1
		{"IPv4-compatible loopback", "::127.0.0.1"},
		{"IPv4-compatible metadata", "::169.254.169.254"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.addr)
			if ip == nil {
				t.Fatalf("could not parse %s", tc.addr)
			}
			if g.Allowed(ip) {
				t.Errorf("%s (%s) was allowed; it reaches an internal address", tc.name, tc.addr)
			}
		})
	}
}

// Ranges that are not RFC-1918 but are not valid backup destinations either.
func TestBlocked_ReservedRanges(t *testing.T) {
	g := New(false)
	for _, addr := range []string{
		"192.0.0.1",    // IETF protocol assignments
		"192.0.2.5",    // TEST-NET-1
		"198.18.0.1",   // benchmarking
		"198.51.100.5", // TEST-NET-2
		"203.0.113.5",  // TEST-NET-3
		"240.0.0.1",    // reserved
	} {
		if g.Allowed(net.ParseIP(addr)) {
			t.Errorf("reserved address %s was allowed", addr)
		}
	}
}

// Ordinary public destinations are permitted.
func TestAllowed_PublicDestinations(t *testing.T) {
	g := New(false)
	for _, addr := range []string{
		"1.1.1.1",
		"52.216.0.1",           // AWS S3 range
		"2606:4700:4700::1111", // public IPv6
		"::ffff:1.1.1.1",       // IPv4-mapped public
	} {
		if !g.Allowed(net.ParseIP(addr)) {
			t.Errorf("public address %s was blocked", addr)
		}
	}
}

// The opt-in switch permits private ranges without re-opening loopback or
// link-local.
func TestAllowPrivate_StillBlocksLoopbackAndMetadata(t *testing.T) {
	g := New(true)
	if !g.Allowed(net.ParseIP("192.168.1.10")) {
		t.Error("private address blocked despite allowPrivate")
	}
	for _, addr := range []string{"127.0.0.1", "169.254.169.254", "64:ff9b::7f00:1", "::127.0.0.1"} {
		if g.Allowed(net.ParseIP(addr)) {
			t.Errorf("%s allowed under allowPrivate; loopback/link-local must always be blocked", addr)
		}
	}
}

// HTTPTransport never consults environment proxies.
func TestHTTPTransport_DoesNotUseEnvironmentProxy(t *testing.T) {
	if tr := New(false).HTTPTransport(); tr.Proxy != nil {
		t.Error("HTTPTransport must not use a proxy: the SSRF guard validates the dialled address, " +
			"so proxying would validate the proxy rather than the backup target")
	}
}
