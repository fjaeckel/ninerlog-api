// Package netguard restricts the network destinations the cloud-backup
// subsystem is allowed to connect to (SSRF mitigation). The guard is applied
// as a net.Dialer Control hook, which runs against the concrete IP that will
// actually be dialed, after DNS resolution.
package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"
)

// AllowPrivateNetworksEnv, when set to "true", permits connections to RFC-1918 /
// unique-local / CGNAT ranges. Loopback, link-local, unspecified, and multicast
// addresses are always blocked regardless of this setting.
const AllowPrivateNetworksEnv = "BACKUP_ALLOW_PRIVATE_NETWORKS"

// Guard decides whether a connection to a resolved IP is permitted.
type Guard struct {
	allowPrivate bool
}

// New returns a Guard. When allowPrivate is true, private/ULA/CGNAT ranges are
// permitted (loopback, link-local, unspecified, and multicast are always
// blocked).
func New(allowPrivate bool) *Guard {
	return &Guard{allowPrivate: allowPrivate}
}

// FromEnv builds a Guard from AllowPrivateNetworksEnv. Private networks are
// blocked unless the variable is explicitly set to "true".
func FromEnv() *Guard {
	return New(os.Getenv(AllowPrivateNetworksEnv) == "true")
}

// blockedReason returns a human-readable reason if ip must not be dialed, or an
// empty string if it is allowed.
func (g *Guard) blockedReason(ip net.IP) string {
	if ip == nil {
		return "unparseable address"
	}
	// Normalize IPv4-in-IPv6.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	// Re-evaluate NAT64 (64:ff9b::/96) and IPv4-compatible IPv6 (::a.b.c.d)
	// against the embedded IPv4 address.
	if v4 := embeddedIPv4(ip); v4 != nil {
		if reason := g.blockedReason(v4); reason != "" {
			return "embedded IPv4 " + reason
		}
	}

	switch {
	case ip.IsLoopback():
		return "loopback address"
	case ip.IsUnspecified():
		return "unspecified address"
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		// 169.254.0.0/16 and fe80::/10.
		return "link-local address"
	case ip.IsMulticast(), ip.IsInterfaceLocalMulticast():
		return "multicast address"
	}
	if reason := reservedReason(ip); reason != "" {
		return reason
	}
	if !g.allowPrivate && isPrivate(ip) {
		return "private network address"
	}
	return ""
}

// nat64Prefix is the well-known NAT64 prefix (RFC 6052). Addresses inside it
// carry an IPv4 address in their low 32 bits.
var nat64Prefix = net.IPNet{
	IP:   net.ParseIP("64:ff9b::"),
	Mask: net.CIDRMask(96, 128),
}

// embeddedIPv4 returns the IPv4 address wrapped inside a NAT64 (64:ff9b::/96)
// or IPv4-compatible (::a.b.c.d) IPv6 address, or nil when there is none.
func embeddedIPv4(ip net.IP) net.IP {
	if len(ip) != net.IPv6len || ip.To4() != nil {
		return nil
	}
	if nat64Prefix.Contains(ip) {
		return net.IPv4(ip[12], ip[13], ip[14], ip[15])
	}
	// ::a.b.c.d — all-zero high 96 bits, non-zero low 32; excludes :: and ::1.
	if isZeros(ip[0:12]) && !(ip[12] == 0 && ip[13] == 0 && ip[14] == 0 && ip[15] <= 1) {
		return net.IPv4(ip[12], ip[13], ip[14], ip[15])
	}
	return nil
}

func isZeros(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// reservedRanges are non-RFC-1918 IPv4 blocks that are always blocked.
var reservedRanges = []struct {
	cidr string
	name string
}{
	{"192.0.0.0/24", "IETF protocol assignments"},
	{"192.0.2.0/24", "TEST-NET-1 documentation range"},
	{"198.18.0.0/15", "benchmarking range"},
	{"198.51.100.0/24", "TEST-NET-2 documentation range"},
	{"203.0.113.0/24", "TEST-NET-3 documentation range"},
	{"240.0.0.0/4", "reserved range"},
}

var parsedReserved = func() []*net.IPNet {
	out := make([]*net.IPNet, 0, len(reservedRanges))
	for _, r := range reservedRanges {
		if _, n, err := net.ParseCIDR(r.cidr); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// reservedReason blocks IPv4 ranges reserved for special purposes.
func reservedReason(ip net.IP) string {
	v4 := ip.To4()
	if v4 == nil {
		return ""
	}
	for i, n := range parsedReserved {
		if n.Contains(v4) {
			return reservedRanges[i].name
		}
	}
	return ""
}

// isPrivate reports whether ip is in a private/internal range that should be
// blocked by default: RFC-1918 + unique-local (via net.IP.IsPrivate) plus the
// CGNAT range 100.64.0.0/10.
func isPrivate(ip net.IP) bool {
	if ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 (RFC 6598 carrier-grade NAT).
		if v4[0] == 100 && v4[1]&0xc0 == 64 {
			return true
		}
	}
	return false
}

// Allowed reports whether a connection to ip is permitted.
func (g *Guard) Allowed(ip net.IP) bool {
	return g.blockedReason(ip) == ""
}

// Control is a net.Dialer Control hook. It rejects the connection if the
// resolved address is a blocked destination. address is "ip:port".
func (g *Guard) Control(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("netguard: refusing to connect to non-IP address %q", address)
	}
	if reason := g.blockedReason(ip); reason != "" {
		return fmt.Errorf("netguard: refusing to connect to %s: %s not permitted for backup destinations", ip, reason)
	}
	return nil
}

// Dialer returns a net.Dialer whose Control hook enforces the guard.
func (g *Guard) Dialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{Timeout: timeout, Control: g.Control}
}

// DialContext is a guarded dial function suitable for SFTP and any other caller
// that dials directly.
func (g *Guard) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return g.Dialer(15*time.Second).DialContext(ctx, network, addr)
}

// HTTPTransport returns an *http.Transport whose connections are guarded. It is
// used by the S3 and WebDAV providers.
func (g *Guard) HTTPTransport() *http.Transport {
	return &http.Transport{
		// Proxy stays nil; environment proxies are never consulted.
		Proxy:                 nil,
		DialContext:           g.Dialer(10 * time.Second).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// HTTPClient returns an *http.Client using a guarded transport and the given
// overall timeout.
func (g *Guard) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Transport: g.HTTPTransport(), Timeout: timeout}
}
